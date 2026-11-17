package yalemap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/place"
)

var oldCampus = geo.LatLon{Lat: 41.3083, Lon: -72.9279}

type fakeMap struct {
	locationCalls atomic.Int32
	detailCalls   atomic.Int32
	release       chan struct{}
	once          sync.Once
}

func (f *fakeMap) open() {
	f.once.Do(func() { close(f.release) })
}

func serve(t *testing.T, gateDetails bool) (*Resolver, *fakeMap) {
	t.Helper()
	fake := &fakeMap{release: make(chan struct{})}
	if !gateDetails {
		fake.open()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-key" || r.URL.Query().Get("map") != mapID {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case r.URL.Path == "/categories":
			writeFixture(t, w, "categories.json")
		case r.URL.Path == "/locations":
			fake.locationCalls.Add(1)
			writeFixture(t, w, "locations.json")
		case strings.HasPrefix(r.URL.Path, "/locations/"):
			fake.detailCalls.Add(1)
			<-fake.release
			id := strings.TrimPrefix(r.URL.Path, "/locations/")
			if _, err := os.Stat(filepath.Join("testdata", "details", id+".json")); err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			writeFixture(t, w, filepath.Join("details", id+".json"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(func() {
		fake.open()
		server.Close()
	})
	resolver := New("test-key")
	resolver.base = server.URL
	return resolver, fake
}

func writeFixture(t *testing.T, w http.ResponseWriter, name string) {
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

func resolve(t *testing.T, r *Resolver, query string) []place.Candidate {
	t.Helper()
	found, err := r.Resolve(context.Background(), query, oldCampus)
	if err != nil {
		t.Fatalf("%q: %v", query, err)
	}
	return found
}

func expectTop(t *testing.T, found []place.Candidate, id string, confidence float64) place.Place {
	t.Helper()
	if len(found) == 0 {
		t.Fatalf("no candidates, want %s", id)
	}
	if found[0].Place.ID != id || found[0].Confidence != confidence {
		t.Fatalf("top %s %.2f, want %s %.2f", found[0].Place.ID, found[0].Confidence, id, confidence)
	}
	return found[0].Place
}

func TestAbbreviationsArriveInBackground(t *testing.T) {
	r, fake := serve(t, true)
	expectTop(t, resolve(t, r, "Watson Hall"), "yale:560119", nameConfidence)
	if found := resolve(t, r, "AKW"); len(found) != 0 && found[0].Confidence >= abbreviationConfidence {
		t.Fatalf("abbreviation known before details: %+v", found)
	}
	fake.open()
	r.background.Wait()
	p := expectTop(t, resolve(t, r, "akw"), "yale:560119", abbreviationConfidence)
	if p.Address != "51 Prospect Street" {
		t.Errorf("address %q", p.Address)
	}
	expectTop(t, resolve(t, r, "WLH"), "yale:560127", abbreviationConfidence)
	if got := fake.detailCalls.Load(); got != 5 {
		t.Errorf("detail calls %d, want 5 buildings", got)
	}
	resolve(t, r, "WLH")
	r.background.Wait()
	if got := fake.locationCalls.Load(); got != 1 {
		t.Errorf("location calls %d, want 1", got)
	}
}

func TestCourseLocations(t *testing.T) {
	r, _ := serve(t, false)
	resolve(t, r, "warm up")
	r.background.Wait()
	expectTop(t, resolve(t, r, "WLH 120"), "yale:560127", courseConfidence)
	expectTop(t, resolve(t, r, "LC 102"), "yale:560111", courseConfidence)
	expectTop(t, resolve(t, r, "WLH 120 - William L. Harkness Hall 120"), "yale:560127", courseConfidence)
	expectTop(t, resolve(t, r, "https://map.yale.edu/?id=1910#!m/560119"), "yale:560119", linkConfidence)
}

func TestNameMatching(t *testing.T) {
	r, _ := serve(t, false)
	resolve(t, r, "warm up")
	r.background.Wait()
	sml := expectTop(t, resolve(t, r, "Sterling Memorial Library"), "yale:563698", nameConfidence)
	if sml.Entryway == nil || sml.Entryway.Name != "Sterling Memorial Library, main entrance" {
		t.Errorf("entryway %+v", sml.Entryway)
	}
	for _, c := range resolve(t, r, "sterling memorial library")[1:] {
		if c.Confidence >= nameConfidence {
			t.Errorf("duplicate listing not merged: %+v", c)
		}
	}
	expectTop(t, resolve(t, r, "Silliman College"), "yale:1148228", nameConfidence)
	expectTop(t, resolve(t, r, "arthur k watson"), "yale:560119", keywordConfidence)
	whl := expectTop(t, resolve(t, r, "william l harkness hall"), "yale:560127", tokenBaseConfidence+tokenCoverageWeight)
	if whl.Name != "W. L. Harkness Hall" {
		t.Errorf("name %q", whl.Name)
	}
	watson := resolve(t, r, "watson")
	if len(watson) != 2 || watson[0].Confidence != prefixConfidence || watson[1].Confidence != prefixConfidence {
		t.Fatalf("watson %+v", watson)
	}
	for _, c := range resolve(t, r, "entrance") {
		if strings.HasSuffix(c.Place.Name, "entrance") {
			t.Errorf("entrance offered as a place: %s", c.Place.Name)
		}
	}
}

func TestEntrywayNearestToStudent(t *testing.T) {
	r, _ := serve(t, false)
	fromHillhouse, err := r.Resolve(context.Background(), "Watson Hall", geo.LatLon{Lat: 41.31290, Lon: -72.92560})
	if err != nil {
		t.Fatal(err)
	}
	if e := fromHillhouse[0].Place.Entryway; e == nil || e.Name != "Watson Hall, Hillhouse Avenue entrance" {
		t.Errorf("from Hillhouse got %+v", e)
	}
	fromProspect, err := r.Resolve(context.Background(), "Watson Hall", geo.LatLon{Lat: 41.31330, Lon: -72.92440})
	if err != nil {
		t.Fatal(err)
	}
	if e := fromProspect[0].Place.Entryway; e == nil || e.Name != "Watson Hall entrance" {
		t.Errorf("from Prospect got %+v", e)
	}
	center := resolve(t, r, "Watson Center")
	if e := center[0].Place.Entryway; e == nil || e.Name != "Watson Center entrance" {
		t.Errorf("Watson Center got %+v", e)
	}
}

func TestLookup(t *testing.T) {
	r, _ := serve(t, false)
	p, err := r.Lookup(context.Background(), "yale:560127")
	if err != nil || p.Name != "W. L. Harkness Hall" || p.Source != place.Yale {
		t.Fatalf("got %+v, %v", p, err)
	}
	merged, err := r.Lookup(context.Background(), "yale:860764")
	if err != nil || merged.ID != "yale:563698" {
		t.Fatalf("merged got %+v, %v", merged, err)
	}
	for _, id := range []string{"yale:1", "google:560127", "yale:"} {
		if _, err := r.Lookup(context.Background(), id); !errors.Is(err, place.ErrNotFound) {
			t.Errorf("%s: got %v", id, err)
		}
	}
}

func TestRefreshAfterADay(t *testing.T) {
	r, fake := serve(t, false)
	clock := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	r.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	resolve(t, r, "Watson Hall")
	r.background.Wait()
	mu.Lock()
	clock = clock.Add(25 * time.Hour)
	mu.Unlock()
	resolve(t, r, "Watson Hall")
	r.background.Wait()
	if got := fake.locationCalls.Load(); got != 2 {
		t.Errorf("location calls %d, want 2", got)
	}
	if got := fake.detailCalls.Load(); got != 5 {
		t.Errorf("detail calls %d, want details fetched only once", got)
	}
}

func TestNoKeyDoesNothing(t *testing.T) {
	r := New("")
	r.base = "http://127.0.0.1:1"
	found, err := r.Resolve(context.Background(), "WLH", oldCampus)
	if found != nil || err != nil {
		t.Fatalf("got %+v, %v", found, err)
	}
}

func TestErrorsHideKey(t *testing.T) {
	r := New("secret-value")
	r.base = "http://127.0.0.1:1"
	_, err := r.Resolve(context.Background(), "WLH", oldCampus)
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("got %v", err)
	}
}

func TestParseDetail(t *testing.T) {
	d := parseDetail(rawDetail{
		Description: "<p><strong>Abbreviation:</strong> HGS / HQ</p><p>Address: 320 York Street</p>",
		Keywords:    flexList{"HGS", "Hall of Graduate Studies", "Computer Science"},
	})
	if strings.Join(d.abbreviations, ",") != "HGS,HQ" || d.address != "320 York Street" || len(d.keywords) != 2 {
		t.Fatalf("got %+v", d)
	}
}
