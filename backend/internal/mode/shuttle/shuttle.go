package shuttle

import (
	"context"
	"log"
	"maps"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/streets"
)

const (
	routesInterval   = time.Minute
	stopsInterval    = time.Hour
	busesInterval    = 5 * time.Second
	etaWindow        = 30 * time.Second
	busesFreshFor    = 30 * time.Second
	etasFreshFor     = 90 * time.Second
	busReportFresh   = 60 * time.Second
	nearbyMeters     = 800.0
	nearbyStopsKept  = 6
	optionsKept      = 3
	idleETAWait      = 5 * time.Second
	walkRankingSlack = 1.1
)

type stopETAs struct {
	fetchedAt time.Time
	byBus     map[int]time.Time
}

type snapshot struct {
	routes  []route
	stops   map[int]stop
	lines   map[int]*line
	buses   []bus
	busesAt time.Time
	etas    map[int]stopETAs
}

type Mode struct {
	streets      streets.Streets
	source       *downtowner
	now          func() time.Time
	current      atomic.Pointer[snapshot]
	writer       sync.Mutex
	observations rideObservations
}

func New(s streets.Streets) *Mode {
	return newMode(s, downtownerURL)
}

func newMode(s streets.Streets, baseURL string) *Mode {
	m := &Mode{streets: s, source: newDowntowner(baseURL), now: time.Now}
	m.current.Store(&snapshot{})
	return m
}

func (m *Mode) Name() plan.ModeName {
	return plan.Shuttle
}

func (m *Mode) Run(ctx context.Context) {
	go every(ctx, stopsInterval, m.pollStops)
	go every(ctx, routesInterval, m.pollRoutes)
	go every(ctx, busesInterval, m.pollBuses)
	go m.pollETAs(ctx)
	<-ctx.Done()
}

func every(ctx context.Context, interval time.Duration, poll func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := poll(ctx); err != nil && ctx.Err() == nil {
			log.Printf("shuttle: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Mode) update(change func(next *snapshot)) {
	m.writer.Lock()
	defer m.writer.Unlock()
	next := *m.current.Load()
	change(&next)
	m.current.Store(&next)
}

func (m *Mode) pollStops(ctx context.Context) error {
	stops, changed, err := m.source.stops(ctx)
	if err != nil || !changed {
		return err
	}
	byID := make(map[int]stop, len(stops))
	for _, s := range stops {
		byID[s.ID] = s
	}
	m.update(func(next *snapshot) {
		next.stops = byID
		next.lines = buildLines(next.routes, byID)
	})
	return nil
}

func (m *Mode) pollRoutes(ctx context.Context) error {
	routes, changed, err := m.source.routes(ctx)
	if err != nil || !changed {
		return err
	}
	m.update(func(next *snapshot) {
		next.routes = routes
		next.lines = buildLines(routes, next.stops)
	})
	return nil
}

func (m *Mode) pollBuses(ctx context.Context) error {
	buses, changed, err := m.source.buses(ctx)
	if err != nil {
		return err
	}
	fetchedAt := m.now()
	m.update(func(next *snapshot) {
		if changed {
			next.buses = buses
		}
		next.busesAt = fetchedAt
	})
	return nil
}

func (m *Mode) pollETAs(ctx context.Context) {
	for ctx.Err() == nil {
		stops := m.current.Load().stopsWithFreshBuses(m.now())
		if len(stops) == 0 {
			if !sleep(ctx, idleETAWait) {
				return
			}
			continue
		}
		spacing := etaWindow / time.Duration(len(stops))
		for _, id := range stops {
			if err := m.pollStopETAs(ctx, id); err != nil && ctx.Err() == nil {
				log.Printf("shuttle: eta %d: %v", id, err)
			}
			if !sleep(ctx, spacing) {
				return
			}
		}
	}
}

func (m *Mode) pollStopETAs(ctx context.Context, stopID int) error {
	response, changed, err := m.source.etas(ctx, stopID)
	if err != nil {
		return err
	}
	fetchedAt := m.now()
	m.update(func(next *snapshot) {
		previous, known := next.etas[stopID]
		entry := stopETAs{fetchedAt: fetchedAt, byBus: previous.byBus}
		if changed || !known {
			entry.byBus = arrivals(response, stopID)
		}
		next.etas = maps.Clone(next.etas)
		if next.etas == nil {
			next.etas = map[int]stopETAs{}
		}
		next.etas[stopID] = entry
	})
	return nil
}

func arrivals(response etaResponse, stopID int) map[int]time.Time {
	calculatedAt := time.Unix(response.CalculationTime, 0)
	byBus := map[int]time.Time{}
	for _, e := range response.Etas[strconv.Itoa(stopID)].Etas {
		byBus[e.BusID] = calculatedAt.Add(time.Duration(e.Minutes) * time.Minute)
	}
	return byBus
}

func sleep(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func buildLines(routes []route, stops map[int]stop) map[int]*line {
	if len(stops) == 0 {
		return nil
	}
	lines := make(map[int]*line, len(routes))
	for _, r := range routes {
		if l := newLine(r, stops); l != nil {
			lines[r.ID] = l
		}
	}
	return lines
}

func (s *snapshot) freshBuses(now time.Time) map[int][]bus {
	if now.Sub(s.busesAt) > busesFreshFor {
		return nil
	}
	byRoute := map[int][]bus{}
	for _, b := range s.buses {
		if _, ok := s.lines[b.Route]; ok && now.Sub(b.updatedAt()) <= busReportFresh {
			byRoute[b.Route] = append(byRoute[b.Route], b)
		}
	}
	return byRoute
}

func (s *snapshot) stopsWithFreshBuses(now time.Time) []int {
	seen := map[int]bool{}
	var stops []int
	for routeID := range s.freshBuses(now) {
		for _, id := range s.lines[routeID].stops {
			if !seen[id] {
				seen[id] = true
				stops = append(stops, id)
			}
		}
	}
	sort.Ints(stops)
	return stops
}

func (s *snapshot) lookup(now time.Time) etaLookup {
	return func(stopID, busID int) (time.Time, bool) {
		entry, ok := s.etas[stopID]
		if !ok || now.Sub(entry.fetchedAt) > etasFreshFor {
			return time.Time{}, false
		}
		at, ok := entry.byBus[busID]
		return at, ok
	}
}

func (s *snapshot) etasFresh(now time.Time) bool {
	for _, entry := range s.etas {
		if now.Sub(entry.fetchedAt) <= etasFreshFor {
			return true
		}
	}
	return false
}

type nearbyStop struct {
	stop   stop
	meters float64
}

func nearest(stops map[int]stop, served map[int]bool, to geo.LatLon) []stop {
	var nearby []nearbyStop
	for id := range served {
		s, ok := stops[id]
		if !ok {
			continue
		}
		if meters := geo.Distance(s.at(), to); meters <= nearbyMeters {
			nearby = append(nearby, nearbyStop{stop: s, meters: meters})
		}
	}
	sort.Slice(nearby, func(i, j int) bool {
		if nearby[i].meters != nearby[j].meters {
			return nearby[i].meters < nearby[j].meters
		}
		return nearby[i].stop.ID < nearby[j].stop.ID
	})
	if len(nearby) > nearbyStopsKept {
		nearby = nearby[:nearbyStopsKept]
	}
	found := make([]stop, len(nearby))
	for i, n := range nearby {
		found[i] = n.stop
	}
	return found
}

type candidate struct {
	line   *line
	board  stop
	alight stop
	pass   pass
	score  time.Time
}

func (m *Mode) Options(ctx context.Context, trip plan.Trip) ([]plan.Option, error) {
	now := m.now()
	snap := m.current.Load()
	fresh := snap.freshBuses(now)
	if len(fresh) == 0 || !snap.etasFresh(now) {
		return nil, nil
	}
	served := map[int]bool{}
	for routeID := range fresh {
		for _, id := range snap.lines[routeID].stops {
			served[id] = true
		}
	}
	boards := nearest(snap.stops, served, trip.Origin.At)
	alights := nearest(snap.stops, served, trip.Destination.At)
	if len(boards) == 0 || len(alights) == 0 {
		return nil, nil
	}
	from := []geo.LatLon{trip.Origin.At}
	for _, s := range alights {
		from = append(from, s.at())
	}
	var to []geo.LatLon
	for _, s := range boards {
		to = append(to, s.at())
	}
	to = append(to, trip.Destination.At)
	walks, err := m.streets.Matrix(ctx, from, to, streets.Walking)
	if err != nil {
		return nil, err
	}
	etas := snap.lookup(now)
	best := map[int]candidate{}
	for routeID, buses := range fresh {
		l := snap.lines[routeID]
		for bi, board := range boards {
			walkArrival := trip.DepartAt.Add(walks[0][bi])
			for ai, alight := range alights {
				if board.ID == alight.ID {
					continue
				}
				egress := time.Duration(float64(walks[ai+1][len(boards)]) * walkRankingSlack)
				for _, b := range buses {
					p, ok := l.plan(b, board.ID, alight.ID, etas, &m.observations, walkArrival, now)
					if !ok {
						continue
					}
					c := candidate{line: l, board: board, alight: alight, pass: p, score: p.alightAt.Add(p.lateBy).Add(egress)}
					if existing, ok := best[routeID]; !ok || c.score.Before(existing.score) {
						best[routeID] = c
					}
				}
			}
		}
	}
	ranked := make([]candidate, 0, len(best))
	for _, c := range best {
		ranked = append(ranked, c)
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score.Before(ranked[j].score) })
	if len(ranked) > optionsKept {
		ranked = ranked[:optionsKept]
	}
	var options []plan.Option
	for _, c := range ranked {
		option, ok, err := m.option(ctx, trip, c, etas, now)
		if err != nil {
			return options, err
		}
		if ok {
			options = append(options, option)
		}
	}
	return options, nil
}

func (m *Mode) option(ctx context.Context, trip plan.Trip, c candidate, etas etaLookup, now time.Time) (plan.Option, bool, error) {
	boardPoint := stopPoint(c.board)
	alightPoint := stopPoint(c.alight)
	access, err := plan.WalkLeg(ctx, m.streets, trip.Origin, boardPoint, trip.DepartAt)
	if err != nil {
		return plan.Option{}, false, err
	}
	p, ok := c.line.plan(c.pass.bus, c.board.ID, c.alight.ID, etas, &m.observations, access.ArriveAt, now)
	if !ok {
		return plan.Option{}, false, nil
	}
	ride := c.line.leg(p, boardPoint, alightPoint)
	egress, err := plan.WalkLeg(ctx, m.streets, alightPoint, trip.Destination, ride.ArriveAt)
	if err != nil {
		return plan.Option{}, false, err
	}
	egress.ArriveAtLate = egress.ArriveAtLate.Add(p.lateBy)
	return plan.Option{Legs: []plan.Leg{access, ride, egress}}, true, nil
}

func (l *line) leg(p pass, from, to plan.Point) plan.Leg {
	heading := p.bus.Heading
	return plan.Leg{
		Mode:         plan.Shuttle,
		From:         from,
		To:           to,
		DepartAt:     p.boardAt,
		ArriveAt:     p.alightAt,
		ArriveAtLate: p.alightAt.Add(p.lateBy),
		Path:         l.legPath(p.passage.board, p.passage.alight),
		Line: &plan.Line{
			ID:    "shuttle:line:" + strconv.Itoa(l.id),
			Name:  l.name,
			Color: l.color,
			Path:  l.path,
		},
		StopCount: p.passage.stopCount,
		Vehicle: &plan.Vehicle{
			ID:      "shuttle:bus:" + strconv.Itoa(p.bus.ID),
			Name:    p.bus.Name,
			At:      geo.LatLon{Lat: p.bus.Lat, Lon: p.bus.Lon},
			Heading: &heading,
		},
	}
}

func stopPoint(s stop) plan.Point {
	return plan.Point{ID: "shuttle:" + strconv.Itoa(s.ID), Name: s.Name, At: s.at()}
}
