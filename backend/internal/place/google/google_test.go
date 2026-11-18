package google

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/place"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testResolver(t *testing.T, handler http.HandlerFunc) *Resolver {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	resolver := New("test-key")
	resolver.base = server.URL
	return resolver
}

func TestResolveSendsRequestAndScores(t *testing.T) {
	near := geo.LatLon{Lat: 41.3110, Lon: -72.9270}
	var got searchRequest
	resolver := testResolver(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/places:searchText" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Goog-Api-Key") != "test-key" {
			t.Error("missing api key header")
		}
		if r.Header.Get("X-Goog-FieldMask") != searchFieldMask {
			t.Errorf("field mask %q", r.Header.Get("X-Goog-FieldMask"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Write(fixture(t, "search.json"))
	})
	found, err := resolver.Resolve(context.Background(), "atticus bookstore", near)
	if err != nil {
		t.Fatal(err)
	}
	if got.TextQuery != "atticus bookstore" || got.LocationBias == nil || got.LocationBias.Circle.Radius != biasRadiusMeters || got.LocationBias.Circle.Center.Latitude != near.Lat {
		t.Errorf("request %+v", got)
	}
	if len(found) != 3 {
		t.Fatalf("got %d candidates", len(found))
	}
	first := found[0]
	if first.Place.ID != "google:ChIJmamoun" || first.Place.Source != place.Google || first.Place.Address == "" || first.Place.At.Lat != 41.30829 {
		t.Errorf("first %+v", first.Place)
	}
	if first.Confidence != matchedConfidence {
		t.Errorf("first confidence %v", first.Confidence)
	}
	if !(found[1].Confidence < first.Confidence && found[2].Confidence < found[1].Confidence) {
		t.Errorf("confidences not decreasing: %v %v %v", first.Confidence, found[1].Confidence, found[2].Confidence)
	}
}

func TestResolveUnmatchedFirstIsLower(t *testing.T) {
	resolver := testResolver(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture(t, "search.json"))
	})
	found, err := resolver.Resolve(context.Background(), "coffee near chapel", geo.LatLon{})
	if err != nil {
		t.Fatal(err)
	}
	if found[0].Confidence != firstConfidence {
		t.Errorf("got %v", found[0].Confidence)
	}
}

func TestResolveSingleResult(t *testing.T) {
	resolver := testResolver(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"places":[{"id":"x","displayName":{"text":"Somewhere"},"location":{"latitude":1,"longitude":2}}]}`))
	})
	found, err := resolver.Resolve(context.Background(), "unrelated words", geo.LatLon{})
	if err != nil || len(found) != 1 || found[0].Confidence != matchedConfidence {
		t.Fatalf("got %+v, %v", found, err)
	}
}

func TestResolveEmptyAndError(t *testing.T) {
	resolver := testResolver(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"status":"PERMISSION_DENIED"}}`))
	})
	if _, err := resolver.Resolve(context.Background(), "anything", geo.LatLon{}); err == nil {
		t.Fatal("expected error")
	}
	empty := testResolver(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) })
	found, err := empty.Resolve(context.Background(), "nothing", geo.LatLon{})
	if err != nil || len(found) != 0 {
		t.Fatalf("got %+v, %v", found, err)
	}
}

func TestNoKeyDoesNothing(t *testing.T) {
	resolver := New("")
	resolver.base = "http://127.0.0.1:1"
	found, err := resolver.Resolve(context.Background(), "atticus", geo.LatLon{})
	if found != nil || err != nil {
		t.Fatalf("got %+v, %v", found, err)
	}
	if _, err := resolver.Lookup(context.Background(), "google:x"); !errors.Is(err, place.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestLookup(t *testing.T) {
	resolver := testResolver(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-FieldMask") != lookupFieldMask {
			t.Errorf("field mask %q", r.Header.Get("X-Goog-FieldMask"))
		}
		switch r.URL.Path {
		case "/places/ChIJmamoun":
			w.Write(fixture(t, "details.json"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	p, err := resolver.Lookup(context.Background(), "google:ChIJmamoun")
	if err != nil || p.ID != "google:ChIJmamoun" || p.Name != "Atticus Bookstore Cafe" {
		t.Fatalf("got %+v, %v", p, err)
	}
	if _, err := resolver.Lookup(context.Background(), "google:missing"); !errors.Is(err, place.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := resolver.Lookup(context.Background(), "yale:1"); !errors.Is(err, place.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestMergeKeepsTheSpecificName(t *testing.T) {
	results := []result{
		{ID: "a", PrimaryType: "train_station", FormattedAddress: "50 Union Ave, New Haven, CT 06519, USA", Location: latLng{Latitude: 41.29755, Longitude: -72.92662}},
		{ID: "b", PrimaryType: "transportation_service", FormattedAddress: "50 Union Ave, New Haven, CT 06519, USA", Location: latLng{Latitude: 41.29768, Longitude: -72.92668}},
		{ID: "c", PrimaryType: "train_station", FormattedAddress: "20 Railroad Ave, West Haven, CT 06516, USA", Location: latLng{Latitude: 41.27115, Longitude: -72.96330}},
	}
	results[0].DisplayName.Text = "New Haven"
	results[1].DisplayName.Text = "New Haven Union Station"
	results[2].DisplayName.Text = "West Haven"
	merged := merge(results)
	if len(merged) != 2 || merged[0].ID != "google:a" || merged[0].Name != "New Haven Union Station" || merged[1].Name != "West Haven Station" {
		t.Fatalf("merged %+v", merged)
	}
}
