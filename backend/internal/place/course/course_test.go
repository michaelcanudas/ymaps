package course

import (
	"context"
	"testing"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/place"
)

type fakeYale struct{}

func (fakeYale) Lookup(_ context.Context, id string) (place.Place, error) {
	return place.Place{ID: id, Name: "Building " + id, Source: place.Yale}, nil
}

func (fakeYale) Resolve(_ context.Context, query string, _ geo.LatLon) ([]place.Candidate, error) {
	return []place.Candidate{{Place: place.Place{ID: "yale:" + query, Name: query}, Confidence: 0.95}}, nil
}

func TestCodesAcceptOldAndNewNumbers(t *testing.T) {
	r := New(fakeYale{})
	for _, query := range []string{"ACCT 270", "acct270", "ACCT 2700", "acct-2700"} {
		found, err := r.Resolve(context.Background(), query, geo.LatLon{})
		if err != nil || len(found) != 1 || found[0].Place.ID != "yale:560094" || found[0].Confidence != courseConfidence {
			t.Fatalf("%q: %+v %v", query, found, err)
		}
	}
}

func TestOtherTextIsNotACourse(t *testing.T) {
	r := New(fakeYale{})
	for _, query := range []string{"Bass Library", "WLH 120 - William L. Harkness Hall", "commons"} {
		if found, _ := r.Resolve(context.Background(), query, geo.LatLon{}); found != nil {
			t.Fatalf("%q resolved to %+v", query, found)
		}
	}
}

func TestUnknownCourseFallsBackToItsSubject(t *testing.T) {
	r := New(fakeYale{})
	found, _ := r.Resolve(context.Background(), "CHEM 9999", geo.LatLon{})
	if len(found) != 1 || found[0].Confidence != subjectConfidence {
		t.Fatalf("%+v", found)
	}
}
