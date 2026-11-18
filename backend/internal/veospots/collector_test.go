package veospots

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/mode/gbfs"
)

var (
	start    = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	chapel   = geo.LatLon{Lat: 41.30800, Lon: -72.92900}
	broadway = geo.LatLon{Lat: 41.31200, Lon: -72.93000}
	wooster  = geo.LatLon{Lat: 41.30400, Lon: -72.91800}
)

func offset(at geo.LatLon, northMeters, eastMeters float64) geo.LatLon {
	return geo.LatLon{
		Lat: at.Lat + northMeters/111195,
		Lon: at.Lon + eastMeters/(111195*math.Cos(at.Lat*math.Pi/180)),
	}
}

func vehicle(number string, at geo.LatLon, rangeMeters float64) gbfs.FreeVehicle {
	return gbfs.FreeVehicle{
		ID:          fmt.Sprintf("rotating-%s-%.6f", number, at.Lat),
		At:          at,
		RangeMeters: &rangeMeters,
		RentalURI:   "https://gmjc.adj.st/?adj_t=5vyf0nr&number=" + number,
	}
}

type script struct {
	t         *testing.T
	collector *Collector
	at        time.Time
}

func newScript(t *testing.T, tuning Tuning) *script {
	return &script{t: t, collector: NewWithTuning(filepath.Join(t.TempDir(), "spots.json"), tuning), at: start}
}

func (s *script) poll(vehicles ...gbfs.FreeVehicle) {
	s.collector.Observe(&gbfs.Snapshot{FetchedAt: s.at, Vehicles: vehicles})
	s.at = s.at.Add(30 * time.Second)
}

func (s *script) wait(d time.Duration) {
	s.at = s.at.Add(d)
}

func (s *script) tripEnds() []TripEnd {
	return s.collector.state.Recent
}

func TestVehicleNumberComesFromRentalURI(t *testing.T) {
	if got := VehicleNumber(vehicle("2004624", chapel, 1)); got != "2004624" {
		t.Fatalf("number = %q", got)
	}
	if got := VehicleNumber(gbfs.FreeVehicle{ID: "abc"}); got != "id:abc" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestTripEndNeedsMoveThenSettle(t *testing.T) {
	s := newScript(t, DefaultTuning)
	s.poll(vehicle("1", chapel, 9000))
	s.poll(vehicle("1", chapel, 9000))
	s.wait(8 * time.Minute)
	s.poll(vehicle("1", broadway, 8200))
	if len(s.tripEnds()) != 0 {
		t.Fatal("trip end recorded after a single poll at the new position")
	}
	s.poll(vehicle("1", offset(broadway, 8, -5), 8200))
	ends := s.tripEnds()
	if len(ends) != 1 {
		t.Fatalf("trip ends = %d, want 1", len(ends))
	}
	if ends[0].Vehicle != "1" || geo.Distance(ends[0].To, broadway) > 1 || ends[0].MovedMeters < 150 {
		t.Fatalf("trip end = %+v", ends[0])
	}
	s.poll(vehicle("1", broadway, 8200))
	if len(s.tripEnds()) != 1 {
		t.Fatal("staying put recorded a second trip end")
	}
}

func TestShortMovesAndPassingThroughAreNotTripEnds(t *testing.T) {
	s := newScript(t, DefaultTuning)
	s.poll(vehicle("1", chapel, 9000))
	s.poll(vehicle("1", offset(chapel, 100, 0), 9000))
	s.poll(vehicle("1", offset(chapel, 105, 0), 9000))
	if len(s.tripEnds()) != 0 {
		t.Fatal("a 100 m reposition counted as a trip")
	}
	s.poll(vehicle("1", offset(chapel, 500, 0), 8900))
	s.poll(vehicle("1", offset(chapel, 700, 0), 8800))
	s.poll(vehicle("1", offset(chapel, 900, 0), 8700))
	if len(s.tripEnds()) != 0 {
		t.Fatal("a moving vehicle counted as a trip end")
	}
}

func TestChargedOrLongAbsentVehiclesAreNotTripEnds(t *testing.T) {
	s := newScript(t, DefaultTuning)
	s.poll(vehicle("1", chapel, 3000), vehicle("2", chapel, 9000))
	s.wait(30 * time.Minute)
	s.poll(vehicle("1", broadway, 20000))
	s.poll(vehicle("1", broadway, 20000))
	if len(s.tripEnds()) != 0 {
		t.Fatal("a recharged vehicle counted as a trip end")
	}
	s.wait(5 * time.Hour)
	s.poll(vehicle("2", wooster, 8000))
	s.poll(vehicle("2", wooster, 8000))
	if len(s.tripEnds()) != 0 {
		t.Fatal("a vehicle absent for hours counted as a trip end")
	}
}

func TestTripEndsClusterIntoSpots(t *testing.T) {
	tuning := DefaultTuning
	tuning.MinLearnedSpots = 1
	s := newScript(t, tuning)
	ride := func(number string, to geo.LatLon) {
		s.poll(vehicle(number, wooster, 9000))
		s.poll(vehicle(number, wooster, 9000))
		s.wait(10 * time.Minute)
		s.poll(vehicle(number, to, 8500))
		s.poll(vehicle(number, to, 8500))
	}
	ride("1", offset(broadway, 0, 0))
	ride("2", offset(broadway, 10, 5))
	if spots := s.collector.Spots(); len(spots) != 0 {
		t.Fatalf("spots after 2 trip ends = %d, want 0", len(spots))
	}
	ride("3", offset(broadway, -8, 12))
	ride("4", offset(chapel, 0, 0))
	spots := s.collector.Spots()
	if len(spots) != 1 {
		t.Fatalf("spots = %+v, want 1", spots)
	}
	if geo.Distance(spots[0].At, broadway) > 15 {
		t.Fatalf("spot at %v, %.0f m from Broadway", spots[0].At, geo.Distance(spots[0].At, broadway))
	}
	if s.collector.Mode() != "tripEnds" {
		t.Fatalf("mode = %s", s.collector.Mode())
	}
	if len(s.collector.state.TripEnds) != 2 {
		t.Fatalf("trip-end clusters = %d, want 2", len(s.collector.state.TripEnds))
	}
}

func TestRestingVehiclesFillInUntilTripEndsAccumulate(t *testing.T) {
	s := newScript(t, DefaultTuning)
	parked := []gbfs.FreeVehicle{
		vehicle("1", chapel, 9000),
		vehicle("2", offset(chapel, 6, 3), 9000),
		vehicle("3", offset(chapel, -4, 8), 9000),
		vehicle("4", broadway, 9000),
	}
	for range 25 {
		s.poll(parked...)
	}
	if s.collector.Mode() != "rests" {
		t.Fatalf("mode = %s, want rests", s.collector.Mode())
	}
	spots := s.collector.Spots()
	if len(spots) != 1 || geo.Distance(spots[0].At, chapel) > 10 || spots[0].Vehicles != 3 {
		t.Fatalf("spots = %+v", spots)
	}
}

func TestDepotSizedClustersAreDropped(t *testing.T) {
	s := newScript(t, DefaultTuning)
	var depot []gbfs.FreeVehicle
	for i := range 40 {
		depot = append(depot, vehicle(string(rune('A'+i)), offset(chapel, float64(i%5), float64(i/5)), 9000))
	}
	for range 25 {
		s.poll(depot...)
	}
	if spots := s.collector.Spots(); len(spots) != 0 {
		t.Fatalf("depot became a spot: %+v", spots)
	}
}

func TestStateSurvivesRestart(t *testing.T) {
	tuning := DefaultTuning
	tuning.MinLearnedSpots = 1
	path := filepath.Join(t.TempDir(), "nested", "spots.json")
	s := &script{t: t, collector: NewWithTuning(path, tuning), at: start}
	for i, number := range []string{"1", "2", "3"} {
		s.poll(vehicle(number, wooster, 9000))
		s.wait(5 * time.Minute)
		s.poll(vehicle(number, offset(broadway, float64(i*5), 0), 8000))
		s.poll(vehicle(number, offset(broadway, float64(i*5), 0), 8000))
	}
	if err := s.collector.Save(); err != nil {
		t.Fatal(err)
	}
	restarted := NewWithTuning(path, tuning)
	if err := restarted.Load(); err != nil {
		t.Fatal(err)
	}
	if len(restarted.Spots()) != 1 || len(restarted.state.Recent) != 3 || len(restarted.state.Tracks) != 3 {
		t.Fatalf("restored spots=%d recent=%d tracks=%d", len(restarted.Spots()), len(restarted.state.Recent), len(restarted.state.Tracks))
	}
}

func TestMemoryIsBounded(t *testing.T) {
	tuning := DefaultTuning
	tuning.MaxRecentTripEnds = 5
	tuning.MaxTracks = 10
	s := newScript(t, tuning)
	for i := range 30 {
		number := string(rune('a' + i))
		s.poll(vehicle(number, wooster, 9000))
		s.poll(vehicle(number, offset(chapel, float64(i*40), 0), 8000))
		s.poll(vehicle(number, offset(chapel, float64(i*40), 0), 8000))
	}
	if len(s.tripEnds()) != 5 || len(s.collector.state.Tracks) != 10 {
		t.Fatalf("recent=%d tracks=%d", len(s.tripEnds()), len(s.collector.state.Tracks))
	}
}

func TestDebugHandlerServesJSONAndGeoJSON(t *testing.T) {
	s := newScript(t, DefaultTuning)
	s.poll(vehicle("1", chapel, 9000))
	s.poll(vehicle("1", broadway, 8000))
	s.poll(vehicle("1", broadway, 8000))
	handler := s.collector.DebugHandler()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/debug/veo/spots", nil))
	var report debugReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil || len(report.RecentTripEnds) != 1 || len(report.Spots) != 1 {
		t.Fatalf("report = %s (%v)", recorder.Body.String(), err)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/debug/veo/spots.geojson", nil))
	var collection featureCollection
	if err := json.Unmarshal(recorder.Body.Bytes(), &collection); err != nil || collection.Type != "FeatureCollection" || len(collection.Features) != 2 {
		t.Fatalf("geojson = %s (%v)", recorder.Body.String(), err)
	}
	if coordinates := collection.Features[1].Geometry.Coordinates; coordinates[0] != broadway.Lon {
		t.Fatalf("geojson coordinates must be lon,lat: %v", coordinates)
	}
}
