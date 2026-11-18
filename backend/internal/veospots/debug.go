package veospots

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"ymaps/backend/internal/geo"
)

type debugSpot struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	TripEnds  int       `json:"tripEnds,omitempty"`
	Rests     int       `json:"rests,omitempty"`
	Peak      int       `json:"peakVehicles"`
	Qualified bool      `json:"qualified"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

type debugTripEnd struct {
	Vehicle     string    `json:"vehicle"`
	Lat         float64   `json:"lat"`
	Lon         float64   `json:"lon"`
	FromLat     float64   `json:"fromLat"`
	FromLon     float64   `json:"fromLon"`
	MovedMeters float64   `json:"movedMeters"`
	StartedAt   time.Time `json:"startedAt"`
	EndedAt     time.Time `json:"endedAt"`
}

type debugReport struct {
	Mode            string         `json:"mode"`
	ObservedAt      time.Time      `json:"observedAt"`
	VehiclesTracked int            `json:"vehiclesTracked"`
	ActiveSpots     int            `json:"activeSpots"`
	Spots           []debugSpot    `json:"spots"`
	RecentTripEnds  []debugTripEnd `json:"recentTripEnds"`
}

type feature struct {
	Type       string         `json:"type"`
	Geometry   pointGeometry  `json:"geometry"`
	Properties map[string]any `json:"properties"`
}

type pointGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

type featureCollection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
}

func (c *Collector) DebugHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/veo/spots", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, "application/json", c.report())
	})
	mux.HandleFunc("GET /debug/veo/spots.geojson", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, "application/geo+json", c.geoJSON())
	})
	return mux
}

func writeJSON(w http.ResponseWriter, contentType string, body any) {
	w.Header().Set("Content-Type", contentType)
	_ = json.NewEncoder(w).Encode(body)
}

func (c *Collector) report() debugReport {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	report := debugReport{
		Mode:            c.mode(),
		ObservedAt:      c.lastAt,
		VehiclesTracked: len(c.state.Tracks),
		ActiveSpots:     len(c.spots),
		Spots:           []debugSpot{},
		RecentTripEnds:  []debugTripEnd{},
	}
	for _, cluster := range c.state.TripEnds {
		report.Spots = append(report.Spots, c.debugSpot("tripEnd", cluster, c.tuning.MinTripEnds))
	}
	for _, cluster := range c.state.Rests {
		report.Spots = append(report.Spots, c.debugSpot("rest", cluster, c.tuning.MinRests))
	}
	sort.Slice(report.Spots, func(i, j int) bool {
		if report.Spots[i].Kind != report.Spots[j].Kind {
			return report.Spots[i].Kind > report.Spots[j].Kind
		}
		return report.Spots[i].TripEnds+report.Spots[i].Rests > report.Spots[j].TripEnds+report.Spots[j].Rests
	})
	for i := len(c.state.Recent) - 1; i >= 0; i-- {
		end := c.state.Recent[i]
		report.RecentTripEnds = append(report.RecentTripEnds, debugTripEnd{
			Vehicle: end.Vehicle, Lat: end.To.Lat, Lon: end.To.Lon, FromLat: end.From.Lat, FromLon: end.From.Lon,
			MovedMeters: end.MovedMeters, StartedAt: end.StartedAt, EndedAt: end.EndedAt,
		})
	}
	return report
}

func (c *Collector) debugSpot(kind string, cluster *Cluster, minimum int) debugSpot {
	spot := debugSpot{
		ID: spotID(cluster.At), Kind: kind, Lat: cluster.At.Lat, Lon: cluster.At.Lon, Peak: cluster.Peak,
		Qualified: cluster.Count >= minimum && cluster.Peak <= c.tuning.MaxSpotVehicles,
		FirstSeen: cluster.FirstSeen, LastSeen: cluster.LastSeen,
	}
	if kind == "tripEnd" {
		spot.TripEnds = cluster.Count
	} else {
		spot.Rests = cluster.Count
	}
	return spot
}

func (c *Collector) geoJSON() featureCollection {
	report := c.report()
	collection := featureCollection{Type: "FeatureCollection", Features: []feature{}}
	for _, spot := range report.Spots {
		collection.Features = append(collection.Features, pointFeature(geo.LatLon{Lat: spot.Lat, Lon: spot.Lon}, map[string]any{
			"kind": spot.Kind + "Spot", "id": spot.ID, "tripEnds": spot.TripEnds, "rests": spot.Rests,
			"peakVehicles": spot.Peak, "qualified": spot.Qualified, "lastSeen": spot.LastSeen,
			"active": spot.Qualified && (spot.Kind == "tripEnd") == (report.Mode == "tripEnds"),
		}))
	}
	for _, end := range report.RecentTripEnds {
		collection.Features = append(collection.Features, pointFeature(geo.LatLon{Lat: end.Lat, Lon: end.Lon}, map[string]any{
			"kind": "tripEnd", "vehicle": end.Vehicle, "movedMeters": end.MovedMeters, "endedAt": end.EndedAt,
		}))
	}
	return collection
}

func pointFeature(at geo.LatLon, properties map[string]any) feature {
	return feature{
		Type:       "Feature",
		Geometry:   pointGeometry{Type: "Point", Coordinates: []float64{at.Lon, at.Lat}},
		Properties: properties,
	}
}
