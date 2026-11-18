package veospots

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/mode/gbfs"
)

const DefaultPath = "./data/veo_spots.json"

type Tuning struct {
	TripMinMeters       float64
	SettleMeters        float64
	SettlePolls         int
	MaxTripGap          time.Duration
	ChargeGainMeters    float64
	RestAfter           time.Duration
	SpotRadiusMeters    float64
	MinTripEnds         int
	MinRests            int
	MinLearnedSpots     int
	MaxSpotVehicles     int
	CentroidWeightLimit int
	SpotExpiry          time.Duration
	CandidateExpiry     time.Duration
	TrackExpiry         time.Duration
	MaxTracks           int
	MaxClusters         int
	MaxRecentTripEnds   int
	PollEvery           time.Duration
	SaveEvery           time.Duration
}

var DefaultTuning = Tuning{
	TripMinMeters:       150,
	SettleMeters:        30,
	SettlePolls:         2,
	MaxTripGap:          3 * time.Hour,
	ChargeGainMeters:    1000,
	RestAfter:           10 * time.Minute,
	SpotRadiusMeters:    30,
	MinTripEnds:         3,
	MinRests:            3,
	MinLearnedSpots:     10,
	MaxSpotVehicles:     30,
	CentroidWeightLimit: 50,
	SpotExpiry:          30 * 24 * time.Hour,
	CandidateExpiry:     7 * 24 * time.Hour,
	TrackExpiry:         7 * 24 * time.Hour,
	MaxTracks:           5000,
	MaxClusters:         3000,
	MaxRecentTripEnds:   500,
	PollEvery:           30 * time.Second,
	SaveEvery:           5 * time.Minute,
}

type Track struct {
	Rest           geo.LatLon
	RestRange      *float64
	RestSince      time.Time
	RestRecorded   bool
	LastAtRest     time.Time
	Candidate      geo.LatLon
	CandidateSince time.Time
	CandidatePolls int
	CandidateRange *float64
	LastSeen       time.Time
}

type Cluster struct {
	At        geo.LatLon
	Count     int
	Peak      int
	FirstSeen time.Time
	LastSeen  time.Time
}

type TripEnd struct {
	Vehicle     string
	From        geo.LatLon
	To          geo.LatLon
	MovedMeters float64
	StartedAt   time.Time
	EndedAt     time.Time
}

type Collector struct {
	path    string
	tuning  Tuning
	mutex   sync.Mutex
	state   state
	spots   []gbfs.Spot
	latest  []gbfs.FreeVehicle
	lastAt  time.Time
	pending bool
}

type state struct {
	Tracks   map[string]*Track
	TripEnds []*Cluster
	Rests    []*Cluster
	Recent   []TripEnd
}

func New(path string) *Collector {
	if path == "" {
		path = DefaultPath
	}
	collector := NewWithTuning(path, DefaultTuning)
	if err := collector.Load(); err != nil {
		log.Printf("veo spots: %v", err)
	}
	return collector
}

func NewWithTuning(path string, tuning Tuning) *Collector {
	return &Collector{path: path, tuning: tuning, state: state{Tracks: map[string]*Track{}}}
}

func (c *Collector) Run(ctx context.Context, reader *gbfs.Reader) {
	poll := time.NewTicker(c.tuning.PollEvery)
	defer poll.Stop()
	save := time.NewTicker(c.tuning.SaveEvery)
	defer save.Stop()
	for {
		select {
		case <-ctx.Done():
			c.saveLogged()
			return
		case <-poll.C:
			if snapshot := reader.Current(); snapshot != nil {
				c.Observe(snapshot)
			}
		case <-save.C:
			c.saveLogged()
		}
	}
}

func (c *Collector) saveLogged() {
	if err := c.Save(); err != nil {
		log.Printf("veo spots: %v", err)
	}
}

func (c *Collector) Spots() []gbfs.Spot {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.spots
}

func (c *Collector) Observe(snapshot *gbfs.Snapshot) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !snapshot.FetchedAt.After(c.lastAt) {
		return
	}
	now := snapshot.FetchedAt
	c.lastAt = now
	c.latest = snapshot.Vehicles
	for _, vehicle := range snapshot.Vehicles {
		c.observeVehicle(VehicleNumber(vehicle), vehicle, now)
	}
	c.prune(now)
	c.spots = c.activeSpots()
	c.pending = true
}

func (c *Collector) observeVehicle(number string, vehicle gbfs.FreeVehicle, now time.Time) {
	t := c.tuning
	track, known := c.state.Tracks[number]
	if !known {
		c.state.Tracks[number] = &Track{
			Rest: vehicle.At, RestRange: vehicle.RangeMeters, RestSince: now, LastAtRest: now,
			Candidate: vehicle.At, CandidateSince: now, CandidatePolls: 1, CandidateRange: vehicle.RangeMeters,
			LastSeen: now,
		}
		return
	}
	track.LastSeen = now
	if geo.Distance(vehicle.At, track.Rest) <= t.SettleMeters {
		track.LastAtRest = now
		track.Candidate, track.CandidateSince, track.CandidatePolls, track.CandidateRange = vehicle.At, now, 1, vehicle.RangeMeters
		if !track.RestRecorded && now.Sub(track.RestSince) >= t.RestAfter {
			track.RestRecorded = true
			c.state.Rests = c.addPoint(c.state.Rests, track.Rest, now)
		}
		return
	}
	if geo.Distance(vehicle.At, track.Candidate) <= t.SettleMeters {
		track.CandidatePolls++
	} else {
		track.Candidate, track.CandidateSince, track.CandidatePolls, track.CandidateRange = vehicle.At, now, 1, vehicle.RangeMeters
	}
	if track.CandidatePolls < t.SettlePolls {
		return
	}
	moved := geo.Distance(track.Candidate, track.Rest)
	if moved > t.TripMinMeters && c.looksLikeRide(track) {
		c.recordTripEnd(number, track, moved)
	}
	track.Rest, track.RestRange, track.RestSince = track.Candidate, track.CandidateRange, track.CandidateSince
	track.LastAtRest, track.RestRecorded = now, false
}

func (c *Collector) looksLikeRide(track *Track) bool {
	if track.CandidateSince.Sub(track.LastAtRest) > c.tuning.MaxTripGap {
		return false
	}
	if track.RestRange != nil && track.CandidateRange != nil && *track.CandidateRange > *track.RestRange+c.tuning.ChargeGainMeters {
		return false
	}
	return true
}

func (c *Collector) recordTripEnd(number string, track *Track, moved float64) {
	c.state.TripEnds = c.addPoint(c.state.TripEnds, track.Candidate, track.CandidateSince)
	c.state.Recent = append(c.state.Recent, TripEnd{
		Vehicle: number, From: track.Rest, To: track.Candidate, MovedMeters: moved,
		StartedAt: track.LastAtRest, EndedAt: track.CandidateSince,
	})
	if overflow := len(c.state.Recent) - c.tuning.MaxRecentTripEnds; overflow > 0 {
		c.state.Recent = append([]TripEnd(nil), c.state.Recent[overflow:]...)
	}
}

func (c *Collector) addPoint(clusters []*Cluster, at geo.LatLon, now time.Time) []*Cluster {
	peak := c.vehiclesNear(at)
	nearest, nearestMeters := -1, c.tuning.SpotRadiusMeters
	for i, cluster := range clusters {
		if meters := geo.Distance(cluster.At, at); meters <= nearestMeters {
			nearest, nearestMeters = i, meters
		}
	}
	if nearest < 0 {
		return append(clusters, &Cluster{At: at, Count: 1, Peak: peak, FirstSeen: now, LastSeen: now})
	}
	cluster := clusters[nearest]
	weight := float64(min(cluster.Count, c.tuning.CentroidWeightLimit))
	cluster.At = geo.LatLon{
		Lat: (cluster.At.Lat*weight + at.Lat) / (weight + 1),
		Lon: (cluster.At.Lon*weight + at.Lon) / (weight + 1),
	}
	cluster.Count++
	cluster.Peak = max(cluster.Peak, peak)
	cluster.LastSeen = now
	return clusters
}

func (c *Collector) vehiclesNear(at geo.LatLon) int {
	count := 0
	for _, vehicle := range c.latest {
		if geo.Distance(vehicle.At, at) <= c.tuning.SpotRadiusMeters {
			count++
		}
	}
	return count
}

func (c *Collector) prune(now time.Time) {
	for number, track := range c.state.Tracks {
		if now.Sub(track.LastSeen) > c.tuning.TrackExpiry {
			delete(c.state.Tracks, number)
		}
	}
	if overflow := len(c.state.Tracks) - c.tuning.MaxTracks; overflow > 0 {
		numbers := make([]string, 0, len(c.state.Tracks))
		for number := range c.state.Tracks {
			numbers = append(numbers, number)
		}
		sort.Slice(numbers, func(i, j int) bool {
			return c.state.Tracks[numbers[i]].LastSeen.Before(c.state.Tracks[numbers[j]].LastSeen)
		})
		for _, number := range numbers[:overflow] {
			delete(c.state.Tracks, number)
		}
	}
	c.state.TripEnds = c.pruneClusters(c.state.TripEnds, c.tuning.MinTripEnds, now)
	c.state.Rests = c.pruneClusters(c.state.Rests, c.tuning.MinRests, now)
}

func (c *Collector) pruneClusters(clusters []*Cluster, minimum int, now time.Time) []*Cluster {
	kept := clusters[:0]
	for _, cluster := range clusters {
		age := now.Sub(cluster.LastSeen)
		if age > c.tuning.SpotExpiry || (cluster.Count < minimum && age > c.tuning.CandidateExpiry) {
			continue
		}
		kept = append(kept, cluster)
	}
	if overflow := len(kept) - c.tuning.MaxClusters; overflow > 0 {
		sort.Slice(kept, func(i, j int) bool { return kept[i].LastSeen.After(kept[j].LastSeen) })
		kept = kept[:c.tuning.MaxClusters]
	}
	return kept
}

func (c *Collector) Mode() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.mode()
}

func (c *Collector) mode() string {
	if len(c.qualified(c.state.TripEnds, c.tuning.MinTripEnds)) >= c.tuning.MinLearnedSpots {
		return "tripEnds"
	}
	if len(c.qualified(c.state.Rests, c.tuning.MinRests)) > 0 {
		return "rests"
	}
	return "none"
}

func (c *Collector) qualified(clusters []*Cluster, minimum int) []*Cluster {
	var kept []*Cluster
	for _, cluster := range clusters {
		if cluster.Count >= minimum && cluster.Peak <= c.tuning.MaxSpotVehicles {
			kept = append(kept, cluster)
		}
	}
	return kept
}

func (c *Collector) activeSpots() []gbfs.Spot {
	var clusters []*Cluster
	switch c.mode() {
	case "tripEnds":
		clusters = c.qualified(c.state.TripEnds, c.tuning.MinTripEnds)
	case "rests":
		clusters = c.qualified(c.state.Rests, c.tuning.MinRests)
	}
	spots := make([]gbfs.Spot, 0, len(clusters))
	for _, cluster := range clusters {
		spots = append(spots, gbfs.Spot{ID: spotID(cluster.At), At: cluster.At, Vehicles: c.vehiclesNear(cluster.At)})
	}
	sort.Slice(spots, func(i, j int) bool { return spots[i].ID < spots[j].ID })
	if len(spots) == 0 {
		return nil
	}
	return spots
}

func spotID(at geo.LatLon) string {
	return fmt.Sprintf("learned:%.5f,%.5f", at.Lat, at.Lon)
}

func VehicleNumber(vehicle gbfs.FreeVehicle) string {
	if parsed, err := url.Parse(vehicle.RentalURI); err == nil {
		if number := strings.TrimSpace(parsed.Query().Get("number")); number != "" {
			return number
		}
	}
	return "id:" + vehicle.ID
}
