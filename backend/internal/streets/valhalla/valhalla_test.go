package valhalla

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/streets"
)

var (
	crossCampus = geo.LatLon{Lat: 41.3111, Lon: -72.9260}
	scienceHill = geo.LatLon{Lat: 41.3163, Lon: -72.9223}
)

type recorded struct {
	path string
	body map[string]any
}

func fixtureServer(t *testing.T, status int, fixture string, seen *recorded) *httptest.Server {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		if seen != nil {
			seen.path = r.URL.Path
			if err := json.Unmarshal(raw, &seen.body); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(payload)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRouteFoot(t *testing.T) {
	var seen recorded
	server := fixtureServer(t, http.StatusOK, "route_foot.json", &seen)
	trace, err := New(server.URL).Route(context.Background(), crossCampus, scienceHill, streets.Walking)
	if err != nil {
		t.Fatal(err)
	}
	if seen.path != "/route" {
		t.Errorf("path = %s, want /route", seen.path)
	}
	if seen.body["costing"] != "pedestrian" {
		t.Errorf("costing = %v, want pedestrian", seen.body["costing"])
	}
	assertPlausibleTrace(t, trace)
}

func TestRouteBicycle(t *testing.T) {
	var seen recorded
	server := fixtureServer(t, http.StatusOK, "route_bicycle.json", &seen)
	profile := streets.Profile{Kind: streets.Bicycle, SpeedKmh: 16}
	trace, err := New(server.URL).Route(context.Background(), crossCampus, scienceHill, profile)
	if err != nil {
		t.Fatal(err)
	}
	if seen.body["costing"] != "bicycle" {
		t.Errorf("costing = %v, want bicycle", seen.body["costing"])
	}
	options, _ := seen.body["costing_options"].(map[string]any)
	bicycle, _ := options["bicycle"].(map[string]any)
	if bicycle["cycling_speed"] != 16.0 {
		t.Errorf("cycling_speed = %v, want 16", bicycle["cycling_speed"])
	}
	if bicycle["bicycle_type"] != "Hybrid" {
		t.Errorf("bicycle_type = %v, want Hybrid", bicycle["bicycle_type"])
	}
	assertPlausibleTrace(t, trace)
}

func assertPlausibleTrace(t *testing.T, trace streets.Trace) {
	t.Helper()
	if trace.Duration <= 0 || trace.Duration > 30*time.Minute {
		t.Errorf("duration = %v", trace.Duration)
	}
	if trace.Meters < 500 || trace.Meters > 2000 {
		t.Errorf("meters = %v", trace.Meters)
	}
	if len(trace.Path) < 2 {
		t.Fatalf("path has %d points", len(trace.Path))
	}
	if geo.Distance(trace.Path[0], crossCampus) > 100 {
		t.Errorf("path starts at %v, far from origin", trace.Path[0])
	}
	if geo.Distance(trace.Path[len(trace.Path)-1], scienceHill) > 100 {
		t.Errorf("path ends at %v, far from destination", trace.Path[len(trace.Path)-1])
	}
	ratio := geo.PathLength(trace.Path) / trace.Meters
	if math.Abs(ratio-1) > 0.1 {
		t.Errorf("decoded path length ratio = %v, want near 1", ratio)
	}
}

func TestMatrix(t *testing.T) {
	var seen recorded
	server := fixtureServer(t, http.StatusOK, "matrix_foot.json", &seen)
	from := []geo.LatLon{crossCampus, scienceHill}
	to := []geo.LatLon{scienceHill, crossCampus, {Lat: 41.3083, Lon: -72.9279}}
	durations, err := New(server.URL).Matrix(context.Background(), from, to, streets.Walking)
	if err != nil {
		t.Fatal(err)
	}
	if seen.path != "/sources_to_targets" {
		t.Errorf("path = %s, want /sources_to_targets", seen.path)
	}
	if len(durations) != 2 || len(durations[0]) != 3 {
		t.Fatalf("shape = %d rows", len(durations))
	}
	if durations[0][1] != 0 || durations[1][0] != 0 {
		t.Errorf("self durations = %v, %v, want 0", durations[0][1], durations[1][0])
	}
	if durations[0][0] <= 0 || durations[0][0] > 30*time.Minute {
		t.Errorf("cross campus to science hill = %v", durations[0][0])
	}
}

func TestMatrixShapeMismatch(t *testing.T) {
	server := fixtureServer(t, http.StatusOK, "matrix_foot.json", nil)
	_, err := New(server.URL).Matrix(context.Background(), []geo.LatLon{crossCampus}, []geo.LatLon{scienceHill}, streets.Walking)
	if err == nil {
		t.Fatal("want error on shape mismatch")
	}
}

func TestErrorStatusFallsBack(t *testing.T) {
	server := fixtureServer(t, http.StatusBadRequest, "route_error.json", nil)
	client := New(server.URL)
	if _, err := client.Route(context.Background(), crossCampus, scienceHill, streets.Walking); err == nil {
		t.Fatal("want error on non-200")
	}
	fallback := streets.Fallback{Primary: client, Backup: streets.Estimate{}}
	trace, err := fallback.Route(context.Background(), crossCampus, scienceHill, streets.Walking)
	if err != nil || trace.Duration <= 0 {
		t.Fatalf("fallback trace = %v, %v", trace, err)
	}
}

func TestDecodeShapePrecisionSix(t *testing.T) {
	path := decodeShape("_izlhA~rlgdF_{geC~ywl@_kwzCn`{nI")
	want := []geo.LatLon{{Lat: 38.5, Lon: -120.2}, {Lat: 40.7, Lon: -120.95}, {Lat: 43.252, Lon: -126.453}}
	if len(path) != len(want) {
		t.Fatalf("decoded %d points", len(path))
	}
	for i := range want {
		if math.Abs(path[i].Lat-want[i].Lat) > 1e-6 || math.Abs(path[i].Lon-want[i].Lon) > 1e-6 {
			t.Errorf("point %d = %v, want %v", i, path[i], want[i])
		}
	}
}
