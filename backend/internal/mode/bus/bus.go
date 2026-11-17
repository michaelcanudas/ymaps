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
	"ymaps/backend/internal/streets"
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

	stopRadiusMeters   = 600
	nearbyStopLimit    = 6
	departureWindow    = 60 * time.Minute
	earlySlack         = 30 * time.Minute
	boardingBuffer     = 2 * time.Minute
	lateMarginRealtime = 3 * time.Minute
	lateMarginSchedule = 6 * time.Minute
	walkLateFactor     = 1.1
	optionLimit        = 4
	fareCents          = 175
)

type Mode struct {
	streets             streets.Streets
	client              *http.Client
	scheduleURL         string
	tripUpdatesURL      string
	vehiclePositionsURL string
	now                 func() time.Time
	schedule            atomic.Pointer[schedule]
	updates             atomic.Pointer[tripUpdates]
	positions           atomic.Pointer[vehiclePositions]
}

func New(s streets.Streets) *Mode {
	return &Mode{
		streets:             s,
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

type nearbyStop struct {
	stop int32
	walk time.Duration
}

type ride struct {
	trip     int32
	date     int32
	board    nearbyStop
	alight   nearbyStop
	boardAt  int32
	alightAt int32
	departAt time.Time
	arriveAt time.Time
	late     time.Time
	live     bool
}

func (m *Mode) Options(ctx context.Context, trip plan.Trip) ([]plan.Option, error) {
	s := m.schedule.Load()
	if s == nil {
		return nil, nil
	}
	departAt := trip.DepartAt
	if departAt.IsZero() {
		departAt = m.now()
	}
	boards, err := m.walkTimes(ctx, s, trip.Origin.At, true)
	if err != nil || len(boards) == 0 {
		return nil, err
	}
	alights, err := m.walkTimes(ctx, s, trip.Destination.At, false)
	if err != nil || len(alights) == 0 {
		return nil, err
	}
	rides := s.findRides(departAt, boards, alights, m.freshUpdates(s))
	positions := m.freshPositions(s)
	walksTo := map[int32]plan.Leg{}
	walksFrom := map[int32]plan.Leg{}
	var options []plan.Option
	for _, found := range rides {
		if len(options) == optionLimit {
			break
		}
		option, ok := m.buildOption(ctx, s, trip, departAt, found, positions, walksTo, walksFrom)
		if ok {
			options = append(options, option)
		}
	}
	return options, nil
}

func (m *Mode) walkTimes(ctx context.Context, s *schedule, at geo.LatLon, toStops bool) ([]nearbyStop, error) {
	stops := s.nearbyStops(at)
	if len(stops) == 0 {
		return nil, nil
	}
	points := make([]geo.LatLon, len(stops))
	for index, number := range stops {
		points[index] = s.stops[number].at
	}
	nearby := make([]nearbyStop, len(stops))
	if toStops {
		matrix, err := m.streets.Matrix(ctx, []geo.LatLon{at}, points, streets.Walking)
		if err != nil {
			return nil, err
		}
		for index, number := range stops {
			nearby[index] = nearbyStop{stop: number, walk: matrix[0][index]}
		}
		return nearby, nil
	}
	matrix, err := m.streets.Matrix(ctx, points, []geo.LatLon{at}, streets.Walking)
	if err != nil {
		return nil, err
	}
	for index, number := range stops {
		nearby[index] = nearbyStop{stop: number, walk: matrix[index][0]}
	}
	return nearby, nil
}

func (s *schedule) nearbyStops(at geo.LatLon) []int32 {
	type candidate struct {
		stop     int32
		distance float64
	}
	var candidates []candidate
	for number, found := range s.stops {
		if found.at == (geo.LatLon{}) || s.visitStart[number] == s.visitStart[number+1] {
			continue
		}
		if distance := geo.Distance(at, found.at); distance <= stopRadiusMeters {
			candidates = append(candidates, candidate{stop: int32(number), distance: distance})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].distance < candidates[j].distance })
	if len(candidates) > nearbyStopLimit {
		candidates = candidates[:nearbyStopLimit]
	}
	stops := make([]int32, len(candidates))
	for index, found := range candidates {
		stops[index] = found.stop
	}
	return stops
}

func (s *schedule) findRides(departAt time.Time, boards, alights []nearbyStop, updates *tripUpdates) []ride {
	alightWalks := make(map[int32]nearbyStop, len(alights))
	for _, found := range alights {
		alightWalks[found.stop] = found
	}
	latest := departAt.Add(departureWindow)
	type rideKey struct {
		trip int32
		date int32
	}
	best := map[rideKey]ride{}
	for _, day := range []serviceDay{serviceDayOf(departAt, -1), serviceDayOf(departAt, 0)} {
		for _, board := range boards {
			earliest := departAt.Add(board.walk)
			if earliest.After(latest) {
				continue
			}
			low := int32(earliest.Add(-earlySlack).Sub(day.base) / time.Second)
			high := int32(latest.Sub(day.base) / time.Second)
			visits := s.stopVisits(board.stop)
			start := sort.Search(len(visits), func(i int) bool { return s.times[visits[i].position].departure >= low })
			for _, found := range visits[start:] {
				boarding := s.times[found.position]
				if boarding.departure > high {
					break
				}
				if boarding.flags&noPickup != 0 {
					continue
				}
				t := s.trips[found.trip]
				if !s.active(t.service, day) {
					continue
				}
				delay, live := tripDelay{}, false
				if updates != nil {
					if known, ok := updates.trips[found.trip]; ok && known.date == day.date {
						if known.canceled {
							continue
						}
						delay, live = known, true
					}
				}
				_, departureDelay, usable := delay.at(boarding.sequence)
				if !usable {
					continue
				}
				leaves := day.at(boarding.departure + departureDelay)
				if leaves.Before(earliest) || leaves.After(latest) {
					continue
				}
				margin := lateMarginSchedule
				if live {
					margin = lateMarginRealtime
				}
				for position := found.position + 1; position < t.first+t.count; position++ {
					alighting := s.times[position]
					if alighting.flags&noDropOff != 0 || alighting.stop == board.stop {
						continue
					}
					alight, ok := alightWalks[alighting.stop]
					if !ok {
						continue
					}
					arrivalDelay, _, usable := delay.at(alighting.sequence)
					if !usable {
						continue
					}
					arrives := day.at(alighting.arrival + arrivalDelay)
					if arrives.Before(leaves) {
						arrives = leaves
					}
					candidate := ride{
						trip:     found.trip,
						date:     day.date,
						board:    board,
						alight:   alight,
						boardAt:  found.position,
						alightAt: position,
						departAt: leaves,
						arriveAt: arrives,
						late:     arrives.Add(margin).Add(time.Duration(float64(alight.walk) * walkLateFactor)),
						live:     live,
					}
					key := rideKey{trip: found.trip, date: day.date}
					if existing, ok := best[key]; !ok || betterRide(candidate, existing) {
						best[key] = candidate
					}
				}
			}
		}
	}
	rides := make([]ride, 0, len(best))
	for _, found := range best {
		rides = append(rides, found)
	}
	sort.Slice(rides, func(i, j int) bool { return betterRide(rides[i], rides[j]) })
	return rides
}

func betterRide(a, b ride) bool {
	if !a.late.Equal(b.late) {
		return a.late.Before(b.late)
	}
	if walkA, walkB := a.board.walk+a.alight.walk, b.board.walk+b.alight.walk; walkA != walkB {
		return walkA < walkB
	}
	if !a.departAt.Equal(b.departAt) {
		return a.departAt.After(b.departAt)
	}
	return a.trip < b.trip
}

func (m *Mode) buildOption(ctx context.Context, s *schedule, trip plan.Trip, departAt time.Time, found ride, positions *vehiclePositions, walksTo, walksFrom map[int32]plan.Leg) (plan.Option, bool) {
	t := s.trips[found.trip]
	board := s.point(found.board.stop)
	alight := s.point(found.alight.stop)

	walkTo, ok := walksTo[found.board.stop]
	if !ok {
		leg, err := plan.WalkLeg(ctx, m.streets, trip.Origin, board, departAt)
		if err != nil {
			return plan.Option{}, false
		}
		walkTo, walksTo[found.board.stop] = leg, leg
	}
	slack := found.departAt.Sub(walkTo.ArriveAt)
	if slack < 0 {
		return plan.Option{}, false
	}
	if shift := slack - boardingBuffer; shift > 0 {
		walkTo = shiftLeg(walkTo, shift)
	}

	walkFrom, ok := walksFrom[found.alight.stop]
	if !ok {
		leg, err := plan.WalkLeg(ctx, m.streets, alight, trip.Destination, departAt)
		if err != nil {
			return plan.Option{}, false
		}
		walkFrom, walksFrom[found.alight.stop] = leg, leg
	}

	margin := lateMarginSchedule
	if found.live {
		margin = lateMarginRealtime
	}
	line := s.routes[t.route]
	bus := plan.Leg{
		Mode:         plan.Bus,
		From:         board,
		To:           alight,
		DepartAt:     found.departAt,
		ArriveAt:     found.arriveAt,
		ArriveAtLate: found.arriveAt.Add(margin),
		CostCents:    fareCents,
		Path:         s.legPath(t, found.boardAt, found.alightAt),
		Line: &plan.Line{
			ID:    line.id,
			Name:  lineName(line),
			Color: line.color,
			Path:  s.linePath(t),
		},
		Headsign:  headsign(t.headsign, line.shortName),
		StopCount: int(found.alightAt - found.boardAt),
	}
	if positions != nil {
		if vehicle, ok := positions.byTrip[found.trip]; ok {
			bus.Vehicle = &vehicle
		}
	}

	walkFrom = shiftLeg(walkFrom, found.arriveAt.Sub(walkFrom.DepartAt))
	walkFrom.ArriveAtLate = bus.ArriveAtLate.Add(walkFrom.ArriveAtLate.Sub(walkFrom.DepartAt))
	return plan.Option{Legs: []plan.Leg{walkTo, bus, walkFrom}}, true
}

func (s *schedule) point(number int32) plan.Point {
	found := s.stops[number]
	return plan.Point{ID: "bus:" + found.id, Name: found.name, At: found.at}
}

func shiftLeg(leg plan.Leg, by time.Duration) plan.Leg {
	leg.DepartAt = leg.DepartAt.Add(by)
	leg.ArriveAt = leg.ArriveAt.Add(by)
	leg.ArriveAtLate = leg.ArriveAtLate.Add(by)
	return leg
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
