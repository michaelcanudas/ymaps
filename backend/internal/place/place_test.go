package place

import (
	"testing"

	"ymaps/backend/internal/geo"
)

func TestSamePlaceFromTwoSourcesIsDecided(t *testing.T) {
	yale := Candidate{Place: Place{ID: "yale:1", Name: "Yale Peabody Museum", At: geo.LatLon{Lat: 41.3159, Lon: -72.9208}}, Confidence: 0.7}
	google := Candidate{Place: Place{ID: "google:x", Name: "Yale Peabody Museum", At: geo.LatLon{Lat: 41.3160, Lon: -72.9210}}, Confidence: 0.7}
	parking := Candidate{Place: Place{ID: "yale:2", Name: "Yale Peabody Museum Visitor Parking", At: geo.LatLon{Lat: 41.3170, Lon: -72.9190}}, Confidence: 0.5}
	best, ok := decided(rank([]Candidate{yale, google, parking}, geo.LatLon{}))
	if !ok || best.Place.ID != "yale:1" {
		t.Fatalf("best %+v ok %v", best, ok)
	}
}
