package gbfs

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/streets"
)

const testKey = "fixture-key"

var (
	oldCampus   = plan.Point{Name: "Old Campus", At: geo.LatLon{Lat: 41.3083, Lon: -72.9289}}
	scienceHill = plan.Point{Name: "Science Hill", At: geo.LatLon{Lat: 41.3170, Lon: -72.9220}}
)

func feedServer(t *testing.T, requests *atomic.Int32) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			requests.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/ridenewhaven/") && r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/"))))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(strings.ReplaceAll(string(body), "{{base}}", server.URL)))
	}))
	t.Cleanup(server.Close)
	return server
}

func trip() plan.Trip {
	return plan.Trip{Origin: oldCampus, Destination: scienceHill, DepartAt: time.Date(2026, 11, 17, 18, 0, 0, 0, time.UTC)}
}

func query() plan.Query {
	t := trip()
	return plan.Query{Origin: t.Origin.At, Destination: t.Destination.At, DepartAt: t.DepartAt, Until: t.DepartAt.Add(time.Hour)}
}

func checkChained(t *testing.T, mode plan.Mode, want plan.ModeName) plan.Leg {
	t.Helper()
	planner := plan.Planner{Streets: streets.Estimate{}, Modes: []plan.Mode{mode}}
	option, err := planner.Plan(context.Background(), trip())
	if err != nil {
		t.Fatal(err)
	}
	if len(option.Legs) != 3 {
		t.Fatalf("legs = %d, want 3", len(option.Legs))
	}
	walkIn, ride, walkOut := option.Legs[0], option.Legs[1], option.Legs[2]
	if walkIn.Mode != plan.Walk || ride.Mode != want || walkOut.Mode != plan.Walk {
		t.Fatalf("modes = %s %s %s", walkIn.Mode, ride.Mode, walkOut.Mode)
	}
	if !ride.DepartAt.Equal(walkIn.ArriveAt) || !walkOut.DepartAt.Equal(ride.ArriveAt) {
		t.Fatalf("legs are not chained")
	}
	if walkIn.To != ride.From || ride.To != walkOut.From {
		t.Fatalf("leg points do not join")
	}
	if len(ride.Path) < 2 {
		t.Fatalf("ride path is empty")
	}
	return ride
}

func TestBikeNetwork(t *testing.T) {
	server := feedServer(t, nil)
	mode := newBike(testKey, server.URL+"/ridenewhaven/gbfs.json")
	mode.reader.Refresh(context.Background(), true)
	network, err := mode.Network(context.Background(), query())
	if err != nil || len(network.Rentals) != 1 {
		t.Fatalf("network %+v err %v", network, err)
	}
	rental := network.Rentals[0]
	if rental.Mode != plan.Bike || len(rental.Pickups) == 0 || len(rental.Dropoffs) == 0 {
		t.Fatalf("rental %+v", rental)
	}
	for _, pickup := range rental.Pickups {
		if !strings.HasPrefix(pickup.Point.ID, "bike:") || pickup.Point.Name != strings.TrimSpace(pickup.Point.Name) {
			t.Fatalf("pickup %+v", pickup.Point)
		}
		if pickup.BikesAvailable != nil && *pickup.BikesAvailable < 1 {
			t.Fatalf("pickup %s has no bikes", pickup.Point.ID)
		}
		if got := pickup.Cost(4*time.Minute + time.Second); got != 100+25*5 {
			t.Fatalf("cost = %d", got)
		}
	}
	ride := checkChained(t, mode, plan.Bike)
	want := 100 + 25*int(math.Ceil(ride.ArriveAt.Sub(ride.DepartAt).Minutes()))
	if ride.CostCents != want {
		t.Fatalf("cost = %d, want %d", ride.CostCents, want)
	}
}

func TestBikeWithoutKeyOffersNothing(t *testing.T) {
	var requests atomic.Int32
	server := feedServer(t, &requests)
	mode := newBike("", server.URL+"/ridenewhaven/gbfs.json")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	mode.Run(ctx)
	network, err := mode.Network(context.Background(), query())
	if err != nil || network.Rentals != nil || requests.Load() != 0 {
		t.Fatalf("network = %v, err = %v, requests = %d", network, err, requests.Load())
	}
}

func TestBikeWrongKeyHasNoSnapshot(t *testing.T) {
	server := feedServer(t, nil)
	mode := newBike("wrong", server.URL+"/ridenewhaven/gbfs.json")
	mode.reader.Refresh(context.Background(), true)
	if mode.reader.Current() != nil {
		t.Fatal("snapshot stored after failed poll")
	}
}

func TestStaleSnapshotTurnsModeOff(t *testing.T) {
	server := feedServer(t, nil)
	mode := newVeo(server.URL+"/veo/gbfs", nil)
	mode.reader.Refresh(context.Background(), true)
	mode.now = func() time.Time { return time.Now().Add(staleAfter + time.Second) }
	network, err := mode.Network(context.Background(), query())
	if err != nil || network.Rentals != nil {
		t.Fatalf("network = %v, err = %v", network, err)
	}
}

func TestVeoNetwork(t *testing.T) {
	server := feedServer(t, nil)
	mode := newVeo(server.URL+"/veo/gbfs", nil)
	mode.reader.Refresh(context.Background(), true)
	if snapshot := mode.reader.Current(); snapshot == nil || len(snapshot.Spots) == 0 {
		t.Fatalf("no spots derived")
	}
	ride := checkChained(t, mode, plan.Veo)
	if ride.Vehicle == nil || ride.Vehicle.ID == "" || ride.From.ID != "veo:"+ride.Vehicle.ID {
		t.Fatalf("vehicle = %+v, from = %q", ride.Vehicle, ride.From.ID)
	}
	if ride.Form != "scooter" && ride.Form != "bike" {
		t.Fatalf("form = %q", ride.Form)
	}
	if !strings.HasPrefix(ride.RentalURI, "https://") || !strings.HasPrefix(ride.To.ID, "veo:spot:") {
		t.Fatalf("rental uri = %q, drop-off = %q", ride.RentalURI, ride.To.ID)
	}
	minutes := math.Ceil(ride.ArriveAt.Sub(ride.DepartAt).Minutes())
	want := int(math.Round((1 + 0.4*minutes) * 1.0635 * 100))
	if ride.CostCents != want {
		t.Fatalf("cost = %d, want %d", ride.CostCents, want)
	}
}

func TestPickupRange(t *testing.T) {
	short := 5000.0
	pickup := plan.Pickup{RangeMeters: &short}
	if pickup.Reaches(4000) || !pickup.Reaches(3000) || !(plan.Pickup{}).Reaches(1e6) {
		t.Fatal("range check")
	}
}

func TestClusterSpots(t *testing.T) {
	at := func(lat, lon float64) FreeVehicle { return FreeVehicle{At: geo.LatLon{Lat: lat, Lon: lon}} }
	vehicles := []FreeVehicle{
		at(41.30000, -72.92000), at(41.30005, -72.92000), at(41.30010, -72.92000),
		at(41.31000, -72.92000), at(41.31005, -72.92000),
	}
	for i := 0; i < 31; i++ {
		vehicles = append(vehicles, at(41.32000+float64(i)*0.00001, -72.92000))
	}
	spots := clusterSpots(vehicles)
	if len(spots) != 1 || spots[0].Vehicles != 3 {
		t.Fatalf("spots = %+v", spots)
	}
	if math.Abs(spots[0].At.Lat-41.30005) > 1e-9 {
		t.Fatalf("centroid = %+v", spots[0].At)
	}
}

func TestPricingPlanCents(t *testing.T) {
	plan := PricingPlan{Price: 1, Taxable: true, PerMinute: []PriceSegment{{Start: 0, Rate: 0.4, Interval: 1}}}
	if cents := plan.Cents(4.2); cents != 319 {
		t.Fatalf("cents = %d, want 319", cents)
	}
}

func TestDecodeTolerance(t *testing.T) {
	information, err := decodeStationInformation([]byte(`{"stations":[{"station_id":"a","name":"Yale Post Office ","lat":41.3,"lon":-72.9,"is_virtual_station":1}]}`))
	if err != nil {
		t.Fatal(err)
	}
	stations, err := decodeStationStatus([]byte(`{"stations":[{"station_id":"a","num_bikes_available":0,"is_installed":1,"is_renting":"true","is_returning":true,"vehicles":null},{"station_id":"unknown","num_bikes_available":2}]}`), information)
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 1 || stations[0].Name != "Yale Post Office" || !stations[0].Renting || !stations[0].Returning {
		t.Fatalf("stations = %+v", stations)
	}
}
