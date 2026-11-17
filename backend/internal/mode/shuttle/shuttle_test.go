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

func TestPassagesWrapAroundTheLoop(t *testing.T) {
	stops := []int{10, 2, 5, 67, 96, 53, 41, 106, 97, 118, 42, 98, 38, 9, 72, 1, 121, 12, 35, 43}
	got := passages(stops, 35, 2)
	if len(got) != 1 || got[0] != (passage{board: 18, alight: 1, stopCount: 3}) {
		t.Fatalf("passages(35, 2) = %+v", got)
	}
	got = passages(stops, 2, 35)
	if len(got) != 1 || got[0].stopCount != 17 {
		t.Fatalf("passages(2, 35) = %+v", got)
	}
	if got := passages(stops, 2, 999); got != nil {
		t.Fatalf("passages to unserved stop = %+v", got)
	}
}

func TestPassagesWithStopsListedTwice(t *testing.T) {
	stops := []int{10, 9, 1, 122, 127, 26, 25, 24, 23, 22, 23, 24, 25, 26, 72}
	got := passages(stops, 25, 26)
	want := []passage{{board: 6, alight: 13, stopCount: 7}, {board: 12, alight: 13, stopCount: 1}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("passages(25, 26) = %+v, want %+v", got, want)
	}
	got = passages(stops, 26, 25)
	want = []passage{{board: 5, alight: 6, stopCount: 1}, {board: 13, alight: 6, stopCount: 8}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("passages(26, 25) = %+v, want %+v", got, want)
	}
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
	for _, p := range []passage{{board: 0, alight: 6, stopCount: 6}, {board: 18, alight: 1, stopCount: 3}} {
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

func TestPlanUsesDowntownerETAs(t *testing.T) {
	lines, _ := fixtureLines(t)
	etas := fixtureLookup(t)
	calculatedAt := time.Unix(1794957921, 0)
	p, ok := lines[14].plan(fixtureBus(t, 69274), 10, 41, etas, &rideObservations{}, fixtureNow, fixtureNow)
	if !ok {
		t.Fatal("no pass")
	}
	if !p.boardAt.Equal(calculatedAt.Add(25*time.Minute)) || !p.alightAt.Equal(fixtureNow.Add(38*time.Minute)) {
		t.Fatalf("board %v alight %v", p.boardAt, p.alightAt)
	}
	if p.passage.stopCount != 4 {
		t.Fatalf("stop count %d", p.passage.stopCount)
	}
	if want := lateDelay(p.alightAt.Sub(fixtureNow)); p.lateBy != want || want <= minimumLateDelay {
		t.Fatalf("late by %v, want %v", p.lateBy, want)
	}
}

func TestPlanTakesTheNextPassWhenTheWalkIsLonger(t *testing.T) {
	lines, _ := fixtureLines(t)
	etas := fixtureLookup(t)
	l := lines[14]
	b := fixtureBus(t, 69274)
	early, _ := l.plan(b, 10, 41, etas, &rideObservations{}, fixtureNow, fixtureNow)
	late, ok := l.plan(b, 10, 41, etas, &rideObservations{}, early.boardAt.Add(time.Minute), fixtureNow)
	if !ok {
		t.Fatal("no later pass")
	}
	loop := late.boardAt.Sub(early.boardAt)
	if loop != l.estimatedLoop() {
		t.Fatalf("loop %v, want %v", loop, l.estimatedLoop())
	}
	if late.alightAt.Sub(late.boardAt) != early.alightAt.Sub(early.boardAt) {
		t.Fatal("ride time changed across passes")
	}
}

func TestPlanFallsBackWhenTheBusReachesTheAlightStopFirst(t *testing.T) {
	lines, _ := fixtureLines(t)
	etas := fixtureLookup(t)
	l := lines[14]
	p, ok := l.plan(fixtureBus(t, 69273), 10, 41, etas, &rideObservations{}, fixtureNow, fixtureNow)
	if !ok {
		t.Fatal("no pass")
	}
	if got, want := p.alightAt.Sub(p.boardAt), l.estimatedRide(p.passage); got != want {
		t.Fatalf("ride %v, want estimate %v", got, want)
	}
	observations := &rideObservations{}
	observations.record(rideKey{route: 14, board: 10, alight: 41}, 9*time.Minute)
	p, _ = l.plan(fixtureBus(t, 69273), 10, 41, etas, observations, fixtureNow, fixtureNow)
	if got := p.alightAt.Sub(p.boardAt); got != 9*time.Minute {
		t.Fatalf("ride %v, want observed median", got)
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

func TestOptionsBuildsConsistentWalkShuttleWalk(t *testing.T) {
	server := fixtureServer(t)
	defer server.Close()
	m := newMode(streets.Estimate{}, server.URL)
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
	for _, id := range m.current.Load().stopsWithFreshBuses(fixtureNow) {
		if err := m.pollStopETAs(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	trip := plan.Trip{
		Origin:      plan.Point{At: geo.LatLon{Lat: 41.3030, Lon: -72.9350}},
		Destination: plan.Point{At: geo.LatLon{Lat: 41.3108, Lon: -72.9258}},
		DepartAt:    fixtureNow,
	}
	options, err := m.Options(ctx, trip)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) == 0 || len(options) > optionsKept {
		t.Fatalf("%d options", len(options))
	}
	for _, option := range options {
		access, ride, egress := option.Legs[0], option.Legs[1], option.Legs[2]
		if access.Mode != plan.Walk || ride.Mode != plan.Shuttle || egress.Mode != plan.Walk {
			t.Fatalf("modes %s %s %s", access.Mode, ride.Mode, egress.Mode)
		}
		if access.To != ride.From || ride.To != egress.From || ride.From.ID == "" {
			t.Fatal("legs do not connect at shuttle stops")
		}
		if access.ArriveAt.After(ride.DepartAt) || !egress.DepartAt.Equal(ride.ArriveAt) {
			t.Fatal("leg times are inconsistent")
		}
		if lateBy := ride.ArriveAtLate.Sub(ride.ArriveAt); egress.ArriveAtLate.Sub(egress.ArriveAt) < lateBy {
			t.Fatal("final walk dropped the shuttle's late delay")
		}
		if ride.Line == nil || ride.Vehicle == nil || len(ride.Line.Path) == 0 || len(ride.Path) < 2 || ride.StopCount < 1 {
			t.Fatalf("shuttle leg incomplete: %+v", ride)
		}
	}
	m.now = func() time.Time { return fixtureNow.Add(2 * time.Minute) }
	if options, _ := m.Options(ctx, trip); options != nil {
		t.Fatal("stale feeds still produced options")
	}
}
