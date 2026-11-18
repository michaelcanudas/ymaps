package bus

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
)

const (
	scheduleURL         = "https://www.cttransit.com/sites/default/files/gtfs/googlect_transit.zip"
	tripUpdatesURL      = "https://cttprdtmgtfs.ctttrpcloud.com/TMGTFSRealTimeWebService/TripUpdate/TripUpdates.pb"
	vehiclePositionsURL = "https://cttprdtmgtfs.ctttrpcloud.com/TMGTFSRealTimeWebService/Vehicle/VehiclePositions.pb"

	scheduleRecheck      = 24 * time.Hour
	scheduleRetry        = 5 * time.Minute
	realtimeInterval     = 30 * time.Second
	realtimeWaitInterval = 5 * time.Second
	realtimeFreshness    = 2 * time.Minute
	vehicleFreshness     = 5 * time.Minute
	maxScheduleBytes     = 128 << 20
	maxRealtimeBytes     = 32 << 20

	departureWindow = 90 * time.Minute
	earlySlack      = 30 * time.Minute
	fareCents       = 175
	fareGroup       = "cttransit"
)

type Mode struct {
	client              *http.Client
	scheduleURL         string
	tripUpdatesURL      string
	vehiclePositionsURL string
	now                 func() time.Time
	schedule            atomic.Pointer[schedule]
	updates             atomic.Pointer[tripUpdates]
	positions           atomic.Pointer[vehiclePositions]
}

func New() *Mode {
	return &Mode{
		client:              &http.Client{Timeout: 2 * time.Minute},
		scheduleURL:         scheduleURL,
		tripUpdatesURL:      tripUpdatesURL,
		vehiclePositionsURL: vehiclePositionsURL,
		now:                 time.Now,
	}
}

func (m *Mode) Name() plan.ModeName {
	return plan.Bus
}

func (m *Mode) Run(ctx context.Context) {
	go m.pollRealtime(ctx)
	for {
		wait := scheduleRecheck
		if err := m.refreshSchedule(ctx); err != nil {
			log.Printf("bus schedule: %v", err)
			if m.schedule.Load() == nil {
				wait = scheduleRetry
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (m *Mode) refreshSchedule(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, m.scheduleURL, nil)
	if err != nil {
		return err
	}
	current := m.schedule.Load()
	if current != nil && current.lastModified != "" {
		request.Header.Set("If-Modified-Since", current.lastModified)
	}
	started := m.now()
	response, err := m.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		return nil
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("schedule status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxScheduleBytes))
	if err != nil {
		return err
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	loaded, err := parseSchedule(archive)
	if err != nil {
		return err
	}
	loaded.lastModified = response.Header.Get("Last-Modified")
	loaded.loadedAt = m.now()
	m.schedule.Store(loaded)
	log.Printf("bus schedule: %d routes, %d trips, %d stop times, %d stops in %v", len(loaded.routes), len(loaded.trips), len(loaded.times), len(loaded.stops), m.now().Sub(started).Round(time.Millisecond))
	return nil
}

func (m *Mode) pollRealtime(ctx context.Context) {
	for {
		wait := realtimeInterval
		if s := m.schedule.Load(); s == nil {
			wait = realtimeWaitInterval
		} else {
			m.refreshRealtime(ctx, s)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (m *Mode) refreshRealtime(ctx context.Context, s *schedule) {
	if data, err := m.fetch(ctx, m.tripUpdatesURL); err != nil {
		log.Printf("bus trip updates: %v", err)
	} else if updates, err := parseTripUpdates(data, s, m.now()); err != nil {
		log.Printf("bus trip updates: %v", err)
	} else {
		m.updates.Store(updates)
	}
	if data, err := m.fetch(ctx, m.vehiclePositionsURL); err != nil {
		log.Printf("bus vehicle positions: %v", err)
	} else if positions, err := parseVehiclePositions(data, s, m.now()); err != nil {
		log.Printf("bus vehicle positions: %v", err)
	} else {
		m.positions.Store(positions)
	}
}

func (m *Mode) fetch(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := m.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, maxRealtimeBytes))
}

func (m *Mode) freshUpdates(s *schedule) *tripUpdates {
	updates := m.updates.Load()
	if updates == nil || updates.schedule != s || m.now().Sub(updates.at) > realtimeFreshness {
		return nil
	}
	return updates
}

func (m *Mode) freshPositions(s *schedule) *vehiclePositions {
	positions := m.positions.Load()
	if positions == nil || positions.schedule != s || m.now().Sub(positions.at) > realtimeFreshness {
		return nil
	}
	return positions
}

func (m *Mode) Network(_ context.Context, query plan.Query) (plan.Network, error) {
	s := m.schedule.Load()
	if s == nil {
		return plan.Network{}, nil
	}
	departAt := query.DepartAt
	if departAt.IsZero() {
		departAt = m.now()
	}
	until := query.Until
	if until.IsZero() {
		until = departAt.Add(departureWindow)
	}
	updates := m.freshUpdates(s)
	positions := m.freshPositions(s)
	covered := make([]bool, len(s.stops))
	for number, found := range s.stops {
		if found.at != (geo.LatLon{}) && s.visitStart[number] != s.visitStart[number+1] && query.Covers(found.at) {
			covered[number] = true
		}
	}
	type tripDay struct {
		trip int32
		date int32
	}
	seen := map[tripDay]bool{}
	var network plan.Network
	for _, day := range []serviceDay{serviceDayOf(departAt, -1), serviceDayOf(departAt, 0)} {
		low := int32(departAt.Add(-earlySlack).Sub(day.base) / time.Second)
		high := int32(until.Sub(day.base) / time.Second)
		for number, ok := range covered {
			if !ok {
				continue
			}
			visits := s.stopVisits(int32(number))
			start := sort.Search(len(visits), func(i int) bool { return s.times[visits[i].position].departure >= low })
			for _, found := range visits[start:] {
				if s.times[found.position].departure > high {
					break
				}
				key := tripDay{trip: found.trip, date: day.date}
				if seen[key] {
					continue
				}
				seen[key] = true
				if !s.active(s.trips[found.trip].service, day) {
					continue
				}
				if ride, ok := s.ride(found.trip, day, covered, departAt, until, updates, positions); ok {
					network.Rides = append(network.Rides, ride)
				}
			}
		}
	}
	return network, nil
}

func (s *schedule) ride(number int32, day serviceDay, covered []bool, departAt, until time.Time, updates *tripUpdates, positions *vehiclePositions) (plan.Ride, bool) {
	t := s.trips[number]
	delay := tripDelay{}
	if updates != nil {
		if known, ok := updates.trips[number]; ok && known.date == day.date {
			if known.canceled {
				return plan.Ride{}, false
			}
			delay = known
		}
	}
	var stops []plan.StopTime
	var used []int32
	for position := t.first; position < t.first+t.count; position++ {
		scheduled := s.times[position]
		if !covered[scheduled.stop] || scheduled.flags&(noPickup|noDropOff) == noPickup|noDropOff {
			continue
		}
		arrivalDelay, departureDelay, usable := delay.at(scheduled.sequence)
		if !usable {
			continue
		}
		arrive := day.at(scheduled.arrival + arrivalDelay)
		depart := day.at(scheduled.departure + departureDelay)
		if depart.Before(arrive) {
			depart = arrive
		}
		if depart.Before(departAt) {
			stops, used = stops[:0], used[:0]
			continue
		}
		if arrive.After(until) {
			break
		}
		if n := len(stops); n > 0 && arrive.Before(stops[n-1].Depart) {
			arrive = stops[n-1].Depart
			if depart.Before(arrive) {
				depart = arrive
			}
		}
		stops = append(stops, plan.StopTime{Stop: s.point(scheduled.stop), Arrive: arrive, Depart: depart})
		used = append(used, position)
	}
	if len(stops) < 2 {
		return plan.Ride{}, false
	}
	line := s.routes[t.route]
	ride := plan.Ride{
		Mode: plan.Bus,
		Line: &plan.Line{
			ID:    line.id,
			Name:  lineName(line),
			Color: line.color,
			Path:  s.linePath(t),
		},
		Headsign: headsign(t.headsign, line.shortName),
		Fare:     plan.Fare{Cents: fareCents, Group: fareGroup},
		Stops:    stops,
		Path: func(board, alight int) []geo.LatLon {
			return s.legPath(t, used[board], used[alight])
		},
	}
	if positions != nil {
		if vehicle, ok := positions.byTrip[number]; ok {
			ride.Vehicle = &vehicle
		}
	}
	return ride, true
}

func (s *schedule) point(number int32) plan.Point {
	found := s.stops[number]
	return plan.Point{ID: "bus:" + found.id, Name: found.name, At: found.at}
}

func lineName(r route) string {
	long := titleCase(r.longName)
	switch {
	case r.shortName == "":
		return long
	case long == "":
		return r.shortName
	default:
		return r.shortName + " " + long
	}
}

func headsign(value, shortName string) string {
	before, after, found := strings.Cut(value, " - ")
	if found && shortName != "" && strings.HasPrefix(before, strings.TrimRight(shortName, "*")) {
		return strings.TrimSpace(after)
	}
	return value
}

func titleCase(value string) string {
	runes := []rune(strings.ToLower(value))
	start := true
	for index, character := range runes {
		if start && unicode.IsLetter(character) {
			runes[index] = unicode.ToUpper(character)
		}
		start = !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '\''
	}
	return string(runes)
}
