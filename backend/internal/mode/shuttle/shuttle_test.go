package shuttle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/streets"
)

var fixtureNow = time.Unix(1794957922, 0)

func readFixture(t *testing.T, name string, into any) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatal(err)
	}
}

func fixtureLines(t *testing.T) (map[int]*line, map[int]stop) {
	t.Helper()
	var routes []route
	var stops []stop
	readFixture(t, "routes.json", &routes)
	readFixture(t, "stops.json", &stops)
	byID := map[int]stop{}
	for _, s := range stops {
		byID[s.ID] = s
	}
	return buildLines(routes, byID), byID
}

func fixtureLookup(t *testing.T) etaLookup {
	t.Helper()
	byStop := map[int]map[int]time.Time{}
	for _, id := range []int{10, 41, 42, 98} {
		var response etaResponse
		readFixture(t, "eta_"+strconv.Itoa(id)+".json", &response)
		byStop[id] = arrivals(response, id)
	}
	return func(stopID, busID int) (time.Time, bool) {
		at, ok := byStop[stopID][busID]
		return at, ok
	}
}

func fixtureBus(t *testing.T, id int) bus {
	t.Helper()
	var buses []bus
	readFixture(t, "buses.json", &buses)
	for _, b := range buses {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("bus %d not in fixture", id)
	return bus{}
}

func TestSnappingMakesExactlyOneLoop(t *testing.T) {
	lines, _ := fixtureLines(t)
	for _, id := range []int{10, 13, 14} {
		l := lines[id]
		if !l.trusted {
			t.Fatalf("route %d untrusted", id)
		}
		total := 0.0
		for k, g := range l.gaps {
			total += g
			if meters := geo.Distance(l.pointAt(l.snapped[k]), l.locations[k]); meters > snapRadiusMeters {
				t.Fatalf("route %d stop %d snapped %.0f m away", id, l.stops[k], meters)
			}
		}
		if total < l.length*0.99 || total > l.length*1.01 {
			t.Fatalf("route %d gaps %.0f m, loop %.0f m", id, total, l.length)
		}
	}
	if lines[9].trusted {
		t.Fatal("route 9 runs against its path and must be untrusted")
	}
}

func TestLegPathFollowsTheRouteAndWraps(t *testing.T) {
	lines, stops := fixtureLines(t)
	l := lines[13]
	for _, p := range []struct{ board, alight int }{{board: 0, alight: 6}, {board: 18, alight: 1}} {
		path := l.legPath(p.board, p.alight)
		if len(path) < 3 {
			t.Fatalf("passage %+v gave %d points", p, len(path))
		}
		want := l.rideMeters(p.board, p.alight)
		if got := geo.PathLength(path); got < want-1 || got > want+1 {
			t.Fatalf("passage %+v path %.0f m, want %.0f m", p, got, want)
		}
		if meters := geo.Distance(path[0], stops[l.stops[p.board]].at()); meters > snapRadiusMeters {
			t.Fatalf("passage %+v starts %.0f m from board stop", p, meters)
		}
		if meters := geo.Distance(path[len(path)-1], stops[l.stops[p.alight]].at()); meters > snapRadiusMeters {
			t.Fatalf("passage %+v ends %.0f m from alight stop", p, meters)
		}
	}
	if l.rideMeters(18, 1) >= l.length/2 {
		t.Fatal("wrapping ride should be short")
	}
	untrusted := lines[9]
	if path := untrusted.legPath(0, 3); len(path) != 2 || path[0] != untrusted.locations[0] || path[1] != untrusted.locations[3] {
		t.Fatalf("untrusted leg path = %v", path)
	}
}

func TestRideFollowsDowntownerETAs(t *testing.T) {
	lines, stops := fixtureLines(t)
	etas := fixtureLookup(t)
	calculatedAt := time.Unix(1794957921, 0)
	ride, ok := lines[14].ride(fixtureBus(t, 69274), stops, etas, fixtureNow, fixtureNow.Add(90*time.Minute))
	if !ok {
		t.Fatal("no ride")
	}
	var at10 time.Time
	for j, s := range ride.Stops {
		if j > 0 && s.Arrive.Before(ride.Stops[j-1].Depart) {
			t.Fatalf("stop %d goes back in time", j)
		}
		if s.Stop.ID == "shuttle:10" && at10.IsZero() {
			at10 = s.Depart
		}
	}
	if !at10.Equal(calculatedAt.Add(25 * time.Minute)) {
		t.Fatalf("stop 10 at %v", at10)
	}
	if ride.Fare.Cents != 0 || ride.Line == nil || ride.Vehicle == nil || len(ride.Path(0, 3)) < 2 {
		t.Fatalf("ride incomplete: %+v", ride)
	}
}

func TestAccuracyLearnsFromArrivals(t *testing.T) {
	a := NewAccuracy([]time.Duration{-30 * time.Second, 0, 0, 0, 0})
	if got := a.Correction(14, time.Minute); got != -30*time.Second {
		t.Fatalf("prior %v", got)
	}
	start := time.Unix(1794957921, 0)
	for i := 0; i < 40; i++ {
		madeAt := start.Add(time.Duration(i) * time.Hour)
		a.Predicted(14, 1, 7, madeAt, madeAt.Add(8*time.Minute))
		a.Passed(1, 7, madeAt.Add(7*time.Minute))
		a.Passed(1, 8, madeAt.Add(9*time.Minute))
	}
	if got := a.Correction(14, 8*time.Minute); got > -50*time.Second || got < -61*time.Second {
		t.Fatalf("learned %v, want about -1m", got)
	}
	if got := a.Correction(14, time.Minute); got != -30*time.Second {
		t.Fatalf("other bucket changed to %v", got)
	}
	if got := a.Correction(13, 8*time.Minute); got != 0 {
		t.Fatalf("other route changed to %v", got)
	}
}

func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := map[string]string{
			"/routes_routes.php": "routes.json",
			"/routes_stops.php":  "stops.json",
			"/routes_buses.php":  "buses.json",
		}[r.URL.Path]
		if r.URL.Path == "/routes_eta.php" {
			name = "eta_" + r.URL.Query().Get("stop") + ".json"
		}
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			body = []byte("{}")
		}
		etag := `W/"` + name + `"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Write(body)
	}))
}

func TestNetworkFromLiveFeeds(t *testing.T) {
	server := fixtureServer(t)
	defer server.Close()
	m := newMode(server.URL)
	m.now = func() time.Time { return fixtureNow }
	ctx := context.Background()
	for _, poll := range []func(context.Context) error{m.pollStops, m.pollRoutes, m.pollBuses} {
		if err := poll(ctx); err != nil {
			t.Fatal(err)
		}
	}
	before := m.current.Load()
	if err := m.pollRoutes(ctx); err != nil || m.current.Load() != before {
		t.Fatal("unchanged routes replaced the snapshot")
	}
	query := plan.Query{
		Origin:      geo.LatLon{Lat: 41.3030, Lon: -72.9350},
		Destination: geo.LatLon{Lat: 41.3108, Lon: -72.9258},
		DepartAt:    fixtureNow,
		Until:       fixtureNow.Add(90 * time.Minute),
	}
	network, err := m.Network(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(network.Rides) == 0 {
		t.Fatal("no rides")
	}
	if _, ok := m.current.Load().etas[10]; !ok {
		t.Fatal("request did not fetch etas near the student")
	}
	planner := plan.Planner{Streets: streets.Estimate{}, Modes: []plan.Mode{m}}
	if _, err := planner.Plan(ctx, plan.Trip{Origin: plan.Point{At: query.Origin}, Destination: plan.Point{At: query.Destination}, DepartAt: fixtureNow}); err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return fixtureNow.Add(2 * time.Minute) }
	if network, _ := m.Network(ctx, query); network.Rides != nil {
		t.Fatal("stale feeds still produced rides")
	}
}
