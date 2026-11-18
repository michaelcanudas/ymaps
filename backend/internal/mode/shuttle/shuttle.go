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
)

const (
	routesInterval   = time.Minute
	stopsInterval    = time.Hour
	busesInterval    = 5 * time.Second
	etaWindow        = 45 * time.Second
	busesFreshFor    = 30 * time.Second
	etasFreshFor     = 120 * time.Second
	busReportFresh   = 60 * time.Second
	idleETAWait      = 5 * time.Second
	requestETAAge    = 20 * time.Second
	requestETAMeters = 900.0
	requestETAStops  = 8
	requestETAWait   = 1500 * time.Millisecond
)

var downtownerBias = []time.Duration{45 * time.Second, 30 * time.Second, -20 * time.Second, 0, 0}

type stopETAs struct {
	fetchedAt time.Time
	byBus     map[int]time.Time
	routes    map[int]int
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
	source   *downtowner
	now      func() time.Time
	current  atomic.Pointer[snapshot]
	writer   sync.Mutex
	accuracy *Accuracy
}

func New() *Mode {
	return newMode(downtownerURL)
}

func newMode(baseURL string) *Mode {
	m := &Mode{source: newDowntowner(baseURL), now: time.Now, accuracy: NewAccuracy(downtownerBias)}
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
	if changed {
		for _, b := range buses {
			if b.LastStop != 0 {
				m.accuracy.Passed(b.ID, b.LastStop, b.updatedAt())
			}
		}
	}
	m.update(func(next *snapshot) {
		if changed {
			next.buses = buses
		}
		next.busesAt = fetchedAt
	})
	return nil
}

func (m *Mode) Accuracy() *Accuracy {
	return m.accuracy
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
		entry := stopETAs{fetchedAt: fetchedAt, byBus: previous.byBus, routes: previous.routes}
		if changed || !known {
			entry.byBus = arrivals(response, stopID)
			routes := busRoutes(response, stopID)
			entry.routes = routes
			calculatedAt := time.Unix(response.CalculationTime, 0)
			for busID, at := range entry.byBus {
				m.accuracy.Predicted(routes[busID], busID, stopID, calculatedAt, at)
			}
		}
		next.etas = maps.Clone(next.etas)
		if next.etas == nil {
			next.etas = map[int]stopETAs{}
		}
		next.etas[stopID] = entry
	})
	return nil
}

func busRoutes(response etaResponse, stopID int) map[int]int {
	routes := map[int]int{}
	for _, e := range response.Etas[strconv.Itoa(stopID)].Etas {
		routes[e.BusID] = e.Route
	}
	return routes
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

func (s *snapshot) lookup(now time.Time, accuracy *Accuracy) etaLookup {
	return func(stopID, busID int) (time.Time, bool) {
		entry, ok := s.etas[stopID]
		if !ok || now.Sub(entry.fetchedAt) > etasFreshFor {
			return time.Time{}, false
		}
		at, ok := entry.byBus[busID]
		if !ok {
			return time.Time{}, false
		}
		return at.Add(accuracy.Correction(entry.routes[busID], at.Sub(entry.fetchedAt))), true
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

func (m *Mode) Network(ctx context.Context, query plan.Query) (plan.Network, error) {
	now := m.now()
	snap := m.current.Load()
	fresh := snap.freshBuses(now)
	if len(fresh) == 0 {
		return plan.Network{}, nil
	}
	m.refreshNear(ctx, snap, fresh, now, query.Origin, query.Destination)
	snap = m.current.Load()
	if !snap.etasFresh(now) {
		return plan.Network{}, nil
	}
	etas := snap.lookup(now, m.accuracy)
	var network plan.Network
	for routeID, buses := range fresh {
		l := snap.lines[routeID]
		for _, b := range buses {
			if ride, ok := l.ride(b, snap.stops, etas, now, query.Until); ok {
				network.Rides = append(network.Rides, ride)
			}
		}
	}
	return network, nil
}

func (m *Mode) refreshNear(ctx context.Context, snap *snapshot, fresh map[int][]bus, now time.Time, places ...geo.LatLon) {
	served := map[int]bool{}
	for routeID := range fresh {
		for _, id := range snap.lines[routeID].stops {
			served[id] = true
		}
	}
	wanted := map[int]bool{}
	for _, at := range places {
		for _, id := range nearestStops(snap.stops, served, at) {
			if entry, ok := snap.etas[id]; !ok || now.Sub(entry.fetchedAt) > requestETAAge {
				wanted[id] = true
			}
		}
	}
	if len(wanted) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, requestETAWait)
	defer cancel()
	var group sync.WaitGroup
	for id := range wanted {
		group.Add(1)
		go func(id int) {
			defer group.Done()
			if err := m.pollStopETAs(ctx, id); err != nil && ctx.Err() == nil {
				log.Printf("shuttle: eta %d: %v", id, err)
			}
		}(id)
	}
	group.Wait()
}

func nearestStops(stops map[int]stop, served map[int]bool, to geo.LatLon) []int {
	type near struct {
		id     int
		meters float64
	}
	var found []near
	for id := range served {
		if s, ok := stops[id]; ok {
			if meters := geo.Distance(s.at(), to); meters <= requestETAMeters {
				found = append(found, near{id: id, meters: meters})
			}
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].meters < found[j].meters })
	if len(found) > requestETAStops {
		found = found[:requestETAStops]
	}
	ids := make([]int, len(found))
	for i, f := range found {
		ids[i] = f.id
	}
	return ids
}
