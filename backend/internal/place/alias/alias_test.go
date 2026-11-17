package alias

import (
	"context"
	"errors"
	"testing"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/place"
)

func TestResolveExactAlias(t *testing.T) {
	resolver := New()
	cases := map[string]string{
		"Commons":           "alias:schwarzman",
		"  the COMMONS! ":   "alias:schwarzman",
		"SML":               "alias:sterling",
		"Silliman Buttery":  "alias:silliman",
		"JE":                "alias:jonathan-edwards",
		"jonathan edwards'": "alias:jonathan-edwards",
		"science-hill":      "alias:science-hill",
		"Cross Campus":      "alias:cross-campus",
	}
	for query, want := range cases {
		found, err := resolver.Resolve(context.Background(), query, geo.LatLon{})
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 || found[0].Place.ID != want || found[0].Confidence != exactConfidence {
			t.Errorf("%q: got %+v, want %s", query, found, want)
		}
	}
}

func TestResolveMiss(t *testing.T) {
	found, err := New().Resolve(context.Background(), "silliman bagels", geo.LatLon{})
	if err != nil || len(found) != 0 {
		t.Fatalf("got %+v, %v", found, err)
	}
}

func TestLookup(t *testing.T) {
	resolver := New()
	p, err := resolver.Lookup(context.Background(), "alias:bass")
	if err != nil || p.Name != "Bass Library" || p.Source != place.Alias {
		t.Fatalf("got %+v, %v", p, err)
	}
	if _, err := resolver.Lookup(context.Background(), "alias:nowhere"); !errors.Is(err, place.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestSeedIsOnCampus(t *testing.T) {
	campus := geo.LatLon{Lat: 41.3110, Lon: -72.9270}
	for id, p := range New().byID {
		if geo.Distance(campus, p.At) > 2000 {
			t.Errorf("%s is %.0f m from campus", id, geo.Distance(campus, p.At))
		}
	}
}

func TestParseRejectsConflictingTerms(t *testing.T) {
	data := []byte(`[{"id":"a","name":"A","terms":["x"]},{"id":"b","name":"B","terms":["x"]}]`)
	if _, err := parse(data); err == nil {
		t.Fatal("expected conflict error")
	}
}
