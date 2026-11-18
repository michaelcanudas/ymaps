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

func TestFarGoogleResultsDropOut(t *testing.T) {
	near := geo.LatLon{Lat: 41.3083, Lon: -72.9257}
	hall := Candidate{Place: Place{ID: "yale:1", Name: "Watson Hall", Source: Yale, At: geo.LatLon{Lat: 41.3117, Lon: -72.9244}}, Confidence: 0.6}
	clinic := Candidate{Place: Place{ID: "google:b", Name: "Brekus-Watson", Source: Google, At: geo.LatLon{Lat: 41.2830, Lon: -72.8150}}, Confidence: 0.5}
	ranked := rank([]Candidate{hall, clinic}, near)
	if len(ranked) != 1 || ranked[0].Place.ID != "yale:1" {
		t.Fatalf("ranked %+v", ranked)
	}
}
