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

func checkChained(t *testing.T, option plan.Option, mode plan.ModeName) plan.Leg {
	t.Helper()
	if len(option.Legs) != 3 {
		t.Fatalf("legs = %d, want 3", len(option.Legs))
	}
	walkIn, ride, walkOut := option.Legs[0], option.Legs[1], option.Legs[2]
	if walkIn.Mode != plan.Walk || ride.Mode != mode || walkOut.Mode != plan.Walk {
		t.Fatalf("modes = %s %s %s", walkIn.Mode, ride.Mode, walkOut.Mode)
	}
	if !ride.DepartAt.Equal(walkIn.ArriveAt) || !walkOut.DepartAt.Equal(ride.ArriveAt) {
		t.Fatalf("legs are not chained")
	}
	if walkIn.To != ride.From || ride.To != walkOut.From {
		t.Fatalf("leg points do not join")
	}
	if !option.ArriveAtLate().After(option.ArriveAt()) {
		t.Fatalf("late arrival %v is not after %v", option.ArriveAtLate(), option.ArriveAt())
	}
	if len(ride.Path) < 2 {
		t.Fatalf("ride path is empty")
	}
	return ride
}

func TestBikeOptions(t *testing.T) {
	server := feedServer(t, nil)
	mode := newBike(streets.Estimate{}, testKey, server.URL+"/ridenewhaven/gbfs.json")
	mode.reader.Refresh(context.Background(), true)
	options, err := mode.Options(context.Background(), trip())
	if err != nil {
		t.Fatal(err)
	}
	if len(options) == 0 || len(options) > optionsReturned {
		t.Fatalf("options = %d", len(options))
	}
	for _, option := range options {
		ride := checkChained(t, option, plan.Bike)
		if !strings.HasPrefix(ride.From.ID, "bike:") || !strings.HasPrefix(ride.To.ID, "bike:") {
			t.Fatalf("point ids %q %q", ride.From.ID, ride.To.ID)
		}
		if ride.From.Name != strings.TrimSpace(ride.From.Name) || ride.To.Name != strings.TrimSpace(ride.To.Name) {
			t.Fatalf("names not trimmed: %q %q", ride.From.Name, ride.To.Name)
		}
		if ride.BikesAvailable == nil || *ride.BikesAvailable < 1 {
			t.Fatalf("bikes available = %v", ride.BikesAvailable)
		}
		want := 100 + 25*int(math.Ceil(ride.ArriveAt.Sub(ride.DepartAt).Minutes()))
		if ride.CostCents != want {
			t.Fatalf("cost = %d, want %d", ride.CostCents, want)
		}
	}
}

func TestBikeSkipsEmptyStations(t *testing.T) {
	server := feedServer(t, nil)
	mode := newBike(streets.Estimate{}, testKey, server.URL+"/ridenewhaven/gbfs.json")
	mode.reader.Refresh(context.Background(), true)
	snapshot := mode.reader.Current()
	if snapshot == nil || len(snapshot.Stations) != 8 {
		t.Fatalf("snapshot stations = %v", snapshot)
	}
	for _, pickup := range nearestPickups(snapshot, oldCampus.At) {
		if pickup.bikes < 1 {
			t.Fatalf("pickup %s has no bikes", pickup.point.ID)
		}
	}
}

func TestBikeWithoutKeyOffersNothing(t *testing.T) {
	var requests atomic.Int32
	server := feedServer(t, &requests)
	mode := newBike(streets.Estimate{}, "", server.URL+"/ridenewhaven/gbfs.json")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	mode.Run(ctx)
	options, err := mode.Options(context.Background(), trip())
	if err != nil || options != nil || requests.Load() != 0 {
		t.Fatalf("options = %v, err = %v, requests = %d", options, err, requests.Load())
	}
}

func TestBikeWrongKeyHasNoSnapshot(t *testing.T) {
	server := feedServer(t, nil)
	mode := newBike(streets.Estimate{}, "wrong", server.URL+"/ridenewhaven/gbfs.json")
	mode.reader.Refresh(context.Background(), true)
	if mode.reader.Current() != nil {
		t.Fatal("snapshot stored after failed poll")
	}
}

func TestStaleSnapshotTurnsModeOff(t *testing.T) {
	server := feedServer(t, nil)
	mode := newVeo(streets.Estimate{}, server.URL+"/veo/gbfs")
	mode.reader.Refresh(context.Background(), true)
	mode.now = func() time.Time { return time.Now().Add(staleAfter + time.Second) }
	options, err := mode.Options(context.Background(), trip())
	if err != nil || options != nil {
		t.Fatalf("options = %v, err = %v", options, err)
	}
}

func TestVeoOptions(t *testing.T) {
	server := feedServer(t, nil)
	mode := newVeo(streets.Estimate{}, server.URL+"/veo/gbfs")
	mode.reader.Refresh(context.Background(), true)
	snapshot := mode.reader.Current()
	if snapshot == nil || len(snapshot.Spots) == 0 {
		t.Fatalf("no spots derived")
	}
	options, err := mode.Options(context.Background(), trip())
	if err != nil {
		t.Fatal(err)
	}
	if len(options) == 0 || len(options) > optionsReturned {
		t.Fatalf("options = %d", len(options))
	}
	for _, option := range options {
		ride := checkChained(t, option, plan.Veo)
		if ride.Vehicle == nil || ride.Vehicle.ID == "" || ride.From.ID != "veo:"+ride.Vehicle.ID {
			t.Fatalf("vehicle = %+v, from = %q", ride.Vehicle, ride.From.ID)
		}
		if ride.Form != "scooter" && ride.Form != "bike" {
			t.Fatalf("form = %q", ride.Form)
		}
		if !strings.HasPrefix(ride.RentalURI, "https://") {
			t.Fatalf("rental uri = %q", ride.RentalURI)
		}
		if !strings.HasPrefix(ride.To.ID, "veo:spot:") {
			t.Fatalf("drop-off id = %q", ride.To.ID)
		}
		minutes := math.Ceil(ride.ArriveAt.Sub(ride.DepartAt).Minutes())
		want := int(math.Round((1 + 0.4*minutes) * 1.0635 * 100))
		if ride.CostCents != want {
			t.Fatalf("cost = %d, want %d", ride.CostCents, want)
		}
	}
}

func TestVeoSkipsShortRange(t *testing.T) {
	server := feedServer(t, nil)
	mode := newVeo(streets.Estimate{}, server.URL+"/veo/gbfs")
	mode.reader.Refresh(context.Background(), true)
	snapshot := mode.reader.Current()
	for _, vehicle := range nearestVeoVehicles(snapshot, oldCampus.At, 5000) {
		if vehicle.vehicle.RangeMeters != nil && *vehicle.vehicle.RangeMeters < 1.3*5000+500 {
			t.Fatalf("vehicle %s with range %v kept", vehicle.vehicle.ID, *vehicle.vehicle.RangeMeters)
		}
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
