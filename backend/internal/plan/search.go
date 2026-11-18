package plan

import (
	"context"
	"sort"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/streets"
)

const (
	accessRadiusMeters    = 1700.0
	accessNodesKept       = 160
	transferRadiusMeters  = 350.0
	boardBuffer           = time.Minute
	maxRides              = 4
	rentalAccessLimit     = 5 * time.Minute
	rentalProgressSlack   = 75.0
	minimumRentalMeters   = 400.0
	dropoffNearStopMeters = 250.0
	bagLimit              = 24
	pruneSlack            = 10 * time.Minute
	preferencePruneSlack  = 60 * time.Minute
	candidatesKept        = 14
	walkDetour            = 1.3
	minimumRide           = 2 * time.Minute
)

type stepKind int

const (
	starting stepKind = iota
	walking
	riding
	renting
)

type step struct {
	kind   stepKind
	from   int
	walk   time.Duration
	ride   *Ride
	board  int
	alight int
	pickup *Pickup
	mode   ModeName
	to     Point
}

type label struct {
	node   int
	arrive time.Time
	walk   time.Duration
	wait   time.Duration
	cost   int
	paid   uint32
	rides  int
	rented bool
	step   step
	prev   *label
}

func (a *label) dominates(b *label) bool {
	return !a.arrive.After(b.arrive) &&
		a.walk <= b.walk &&
		a.wait <= b.wait &&
		a.cost <= b.cost &&
		a.paid&b.paid == b.paid &&
		(!a.rented || b.rented)
}

type bag []*label

func (b *bag) add(l *label) bool {
	for _, existing := range *b {
		if existing.dominates(l) {
			return false
		}
	}
	kept := make(bag, 0, len(*b)+1)
	for _, existing := range *b {
		if !l.dominates(existing) {
			kept = append(kept, existing)
		}
	}
	kept = append(kept, l)
	if len(kept) > bagLimit {
		sort.Slice(kept, func(i, j int) bool { return kept[i].arrive.Before(kept[j].arrive) })
		kept = kept[:bagLimit]
	}
	*b = kept
	return true
}

type nodeKind uint8

const (
	stopNode nodeKind = 1 << iota
	pickupNode
	dropoffNode
)

type node struct {
	point     Point
	kind      nodeKind
	access    time.Duration
	hasAccess bool
	egress    time.Duration
	hasEgress bool
}

type neighbor struct {
	node int
	walk time.Duration
}

type search struct {
	streets      streets.Streets
	trip         Trip
	query        Query
	network      Network
	nodes        []node
	index        map[string]int
	rideNodes    [][]int
	pickupNodes  [][]int
	dropoffNodes [][]int
	fareBits     map[string]uint32
	neighbors    map[int][]neighbor
	best         map[int]*bag
	arrivals     bag
	direct       *label
	slack        time.Duration
}

const (
	originNode      = 0
	destinationNode = 1
)

func newSearch(s streets.Streets, trip Trip, network Network) *search {
	query := Query{Origin: trip.Origin.At, Destination: trip.Destination.At, DepartAt: trip.DepartAt}
	search := &search{
		streets:   s,
		trip:      trip,
		query:     query,
		network:   network,
		index:     map[string]int{},
		fareBits:  map[string]uint32{},
		neighbors: map[int][]neighbor{},
		best:      map[int]*bag{},
		slack:     pruneSlack,
	}
	if trip.Preferences.MaxWalk > 0 || trip.Preferences.MaxCostCents != nil {
		search.slack = preferencePruneSlack
	}
	search.nodes = []node{{point: trip.Origin}, {point: trip.Destination}}
	search.build()
	return search
}

func (s *search) nodeFor(p Point, kind nodeKind) int {
	if !s.query.Covers(p.At) {
		return -1
	}
	if p.ID != "" {
		if found, ok := s.index[p.ID]; ok {
			s.nodes[found].kind |= kind
			return found
		}
	}
	s.nodes = append(s.nodes, node{point: p, kind: kind})
	number := len(s.nodes) - 1
	if p.ID != "" {
		s.index[p.ID] = number
	}
	return number
}

func (s *search) build() {
	s.rideNodes = make([][]int, len(s.network.Rides))
	for i, ride := range s.network.Rides {
		nodes := make([]int, len(ride.Stops))
		for j, stop := range ride.Stops {
			nodes[j] = s.nodeFor(stop.Stop, stopNode)
		}
		s.rideNodes[i] = nodes
		if group := ride.Fare.Group; group != "" {
			if _, ok := s.fareBits[group]; !ok && len(s.fareBits) < 32 {
				s.fareBits[group] = 1 << len(s.fareBits)
			}
		}
	}
	var stops []geo.LatLon
	for _, n := range s.nodes {
		if n.kind&stopNode != 0 {
			stops = append(stops, n.point.At)
		}
	}
	s.pickupNodes = make([][]int, len(s.network.Rentals))
	s.dropoffNodes = make([][]int, len(s.network.Rentals))
	for i, rental := range s.network.Rentals {
		pickups := make([]int, len(rental.Pickups))
		for j, pickup := range rental.Pickups {
			pickups[j] = -1
			if geo.Distance(pickup.Point.At, s.query.Origin) <= accessRadiusMeters || nearAny(pickup.Point.At, stops, transferRadiusMeters) {
				pickups[j] = s.nodeFor(pickup.Point, pickupNode)
			}
		}
		dropoffs := make([]int, len(rental.Dropoffs))
		for j, dropoff := range rental.Dropoffs {
			dropoffs[j] = -1
			if geo.Distance(dropoff.At, s.query.Destination) <= accessRadiusMeters || nearAny(dropoff.At, stops, dropoffNearStopMeters) {
				dropoffs[j] = s.nodeFor(dropoff, dropoffNode)
			}
		}
		s.pickupNodes[i] = pickups
		s.dropoffNodes[i] = dropoffs
	}
}

func nearAny(at geo.LatLon, points []geo.LatLon, meters float64) bool {
	for _, p := range points {
		if geo.Distance(at, p) <= meters {
			return true
		}
	}
	return false
}

func (s *search) closest(from geo.LatLon, kinds nodeKind) []int {
	var found []int
	for i := destinationNode + 1; i < len(s.nodes); i++ {
		if s.nodes[i].kind&kinds != 0 && geo.Distance(from, s.nodes[i].point.At) <= accessRadiusMeters {
			found = append(found, i)
		}
	}
	sort.Slice(found, func(a, b int) bool {
		return geo.Distance(from, s.nodes[found[a]].point.At) < geo.Distance(from, s.nodes[found[b]].point.At)
	})
	if len(found) > accessNodesKept {
		found = found[:accessNodesKept]
	}
	return found
}

func (s *search) walkTimes(ctx context.Context) error {
	access := s.closest(s.query.Origin, stopNode|pickupNode)
	targets := []geo.LatLon{s.query.Destination}
	for _, n := range access {
		targets = append(targets, s.nodes[n].point.At)
	}
	fromOrigin, err := s.streets.Matrix(ctx, []geo.LatLon{s.query.Origin}, targets, streets.Walking)
	if err != nil {
		return err
	}
	s.nodes[destinationNode].access, s.nodes[destinationNode].hasAccess = fromOrigin[0][0], true
	for i, n := range access {
		s.nodes[n].access, s.nodes[n].hasAccess = fromOrigin[0][i+1], true
	}
	egress := s.closest(s.query.Destination, stopNode|dropoffNode)
	if len(egress) == 0 {
		return nil
	}
	sources := make([]geo.LatLon, len(egress))
	for i, n := range egress {
		sources[i] = s.nodes[n].point.At
	}
	toDestination, err := s.streets.Matrix(ctx, sources, []geo.LatLon{s.query.Destination}, streets.Walking)
	if err != nil {
		return err
	}
	for i, n := range egress {
		s.nodes[n].egress, s.nodes[n].hasEgress = toDestination[i][0], true
	}
	return nil
}

func estimate(meters float64, profile streets.Profile) time.Duration {
	speed := profile.SpeedKmh
	if speed <= 0 {
		speed = streets.Walking.SpeedKmh
	}
	return time.Duration(meters * walkDetour / (speed / 3.6) * float64(time.Second))
}

func (s *search) neighborsOf(n int) []neighbor {
	if found, ok := s.neighbors[n]; ok {
		return found
	}
	var found []neighbor
	at := s.nodes[n].point.At
	for i := destinationNode + 1; i < len(s.nodes); i++ {
		if i == n || s.nodes[i].kind&(stopNode|pickupNode) == 0 {
			continue
		}
		if meters := geo.Distance(at, s.nodes[i].point.At); meters <= transferRadiusMeters {
			found = append(found, neighbor{node: i, walk: estimate(meters, streets.Walking)})
		}
	}
	s.neighbors[n] = found
	return found
}

type round map[int]*bag

func (r round) add(l *label) {
	b, ok := r[l.node]
	if !ok {
		b = &bag{}
		r[l.node] = b
	}
	b.add(l)
}

func (s *search) bestArrival() (time.Time, bool) {
	if len(s.arrivals) == 0 {
		return time.Time{}, false
	}
	earliest := s.arrivals[0].arrive
	for _, l := range s.arrivals[1:] {
		if l.arrive.Before(earliest) {
			earliest = l.arrive
		}
	}
	return earliest, true
}

func (s *search) pruned(l *label) bool {
	if earliest, ok := s.bestArrival(); ok && l.arrive.After(earliest.Add(s.slack)) {
		return true
	}
	for _, a := range s.arrivals {
		if !a.arrive.After(l.arrive) && a.walk <= l.walk && a.wait <= l.wait && a.cost <= l.cost {
			return true
		}
	}
	return false
}

func (s *search) offer(r round, l *label) bool {
	if s.pruned(l) {
		return false
	}
	b, ok := s.best[l.node]
	if !ok {
		b = &bag{}
		s.best[l.node] = b
	}
	if !b.add(l) {
		return false
	}
	r.add(l)
	return true
}

func (s *search) reach(from *label) {
	n := s.nodes[from.node]
	if !n.hasEgress {
		return
	}
	s.arrivals.add(&label{
		node:   destinationNode,
		arrive: from.arrive.Add(n.egress),
		walk:   from.walk + n.egress,
		wait:   from.wait,
		cost:   from.cost,
		paid:   from.paid,
		rides:  from.rides,
		rented: from.rented,
		step:   step{kind: walking, from: from.node, walk: n.egress},
		prev:   from,
	})
}

func (s *search) run(ctx context.Context) ([]*label, error) {
	if err := s.walkTimes(ctx); err != nil {
		return nil, err
	}
	start := &label{node: originNode, arrive: s.trip.DepartAt, step: step{kind: starting}}
	if d := s.nodes[destinationNode]; d.hasAccess {
		s.direct = &label{
			node:   destinationNode,
			arrive: start.arrive.Add(d.access),
			walk:   d.access,
			step:   step{kind: walking, from: originNode, walk: d.access},
			prev:   start,
		}
		s.arrivals.add(s.direct)
	}
	previous := round{}
	for i := destinationNode + 1; i < len(s.nodes); i++ {
		if n := s.nodes[i]; n.hasAccess {
			s.offer(previous, &label{
				node:   i,
				arrive: start.arrive.Add(n.access),
				walk:   n.access,
				step:   step{kind: walking, from: originNode, walk: n.access},
				prev:   start,
			})
		}
	}
	for k := 1; k <= maxRides && len(previous) > 0; k++ {
		if ctx.Err() != nil {
			break
		}
		current := round{}
		s.scanRides(previous, current)
		s.scanRentals(previous, current)
		s.transfer(current)
		previous = current
	}
	found := append([]*label{}, s.arrivals...)
	sort.Slice(found, func(i, j int) bool { return found[i].arrive.Before(found[j].arrive) })
	if len(found) > candidatesKept {
		found = found[:candidatesKept]
	}
	if s.direct != nil {
		kept := false
		for _, l := range found {
			kept = kept || l == s.direct
		}
		if !kept {
			found = append(found, s.direct)
		}
	}
	return found, nil
}

type carry struct {
	base  *label
	board int
	wait  time.Duration
	cost  int
	paid  uint32
}

func (a carry) dominates(b carry) bool {
	return a.base.walk <= b.base.walk && a.wait <= b.wait && a.cost <= b.cost && a.paid&b.paid == b.paid && (!a.base.rented || b.base.rented)
}

func addCarry(carried []carry, c carry) []carry {
	for _, existing := range carried {
		if existing.dominates(c) {
			return carried
		}
	}
	kept := carried[:0:0]
	for _, existing := range carried {
		if !c.dominates(existing) {
			kept = append(kept, existing)
		}
	}
	return append(kept, c)
}

func (s *search) pay(base *label, fare Fare) (int, uint32) {
	bit := s.fareBits[fare.Group]
	if bit != 0 && base.paid&bit != 0 {
		return base.cost, base.paid
	}
	return base.cost + fare.Cents, base.paid | bit
}

func (s *search) scanRides(previous, current round) {
	for ri := range s.network.Rides {
		ride := &s.network.Rides[ri]
		nodes := s.rideNodes[ri]
		first := -1
		for i, n := range nodes {
			if n >= 0 && previous[n] != nil {
				first = i
				break
			}
		}
		if first < 0 {
			continue
		}
		var carried []carry
		for i := first; i < len(nodes); i++ {
			n := nodes[i]
			if n < 0 {
				continue
			}
			stop := ride.Stops[i]
			for _, c := range carried {
				if stop.Arrive.Sub(ride.Stops[c.board].Depart) < minimumRide {
					continue
				}
				arrive := stop.Arrive
				if departed := ride.Stops[c.board].Depart; arrive.Before(departed) {
					arrive = departed
				}
				s.offer(current, &label{
					node:   n,
					arrive: arrive,
					walk:   c.base.walk,
					wait:   c.wait,
					cost:   c.cost,
					paid:   c.paid,
					rides:  c.base.rides + 1,
					rented: c.base.rented,
					step:   step{kind: riding, ride: ride, board: c.board, alight: i},
					prev:   c.base,
				})
			}
			b := previous[n]
			if b == nil {
				continue
			}
			for _, base := range *b {
				if base.step.kind == riding && base.step.ride == ride {
					continue
				}
				if base.arrive.Add(boardBuffer).After(stop.Depart) {
					continue
				}
				wait := time.Duration(0)
				if base.rides > 0 {
					wait = stop.Depart.Sub(base.arrive)
				}
				cost, paid := s.pay(base, ride.Fare)
				carried = addCarry(carried, carry{base: base, board: i, wait: base.wait + wait, cost: cost, paid: paid})
			}
		}
	}
}

func (s *search) scanRentals(previous, current round) {
	destination := s.query.Destination
	for ri := range s.network.Rentals {
		rental := &s.network.Rentals[ri]
		for pi := range rental.Pickups {
			n := s.pickupNodes[ri][pi]
			if n < 0 || previous[n] == nil {
				continue
			}
			pickup := &rental.Pickups[pi]
			toDestination := geo.Distance(pickup.Point.At, destination)
			for _, base := range *previous[n] {
				if base.rented || base.step.kind != walking || base.step.walk > rentalAccessLimit {
					continue
				}
				start := s.nodes[base.step.from].point.At
				if toDestination > geo.Distance(start, destination)+rentalProgressSlack {
					continue
				}
				for di, dropoff := range rental.Dropoffs {
					m := s.dropoffNodes[ri][di]
					if m < 0 || m == n {
						continue
					}
					meters := geo.Distance(pickup.Point.At, dropoff.At)
					if meters < minimumRentalMeters || !pickup.Reaches(meters*walkDetour) {
						continue
					}
					if geo.Distance(dropoff.At, destination) > toDestination-minimumRentalMeters/2 {
						continue
					}
					ride := estimate(meters, pickup.Profile) + pickup.Unlock
					s.offer(current, &label{
						node:   m,
						arrive: base.arrive.Add(ride),
						walk:   base.walk,
						wait:   base.wait,
						cost:   base.cost + pickup.Cost(ride),
						paid:   base.paid,
						rides:  base.rides + 1,
						rented: true,
						step:   step{kind: renting, pickup: pickup, mode: rental.Mode, to: dropoff},
						prev:   base,
					})
				}
			}
		}
	}
}

func (s *search) transfer(current round) {
	var arrived []*label
	for _, b := range current {
		arrived = append(arrived, *b...)
	}
	for _, l := range arrived {
		s.reach(l)
	}
	for _, l := range arrived {
		for _, next := range s.neighborsOf(l.node) {
			s.offer(current, &label{
				node:   next.node,
				arrive: l.arrive.Add(next.walk),
				walk:   l.walk + next.walk,
				wait:   l.wait,
				cost:   l.cost,
				paid:   l.paid,
				rides:  l.rides,
				rented: l.rented,
				step:   step{kind: walking, from: l.node, walk: next.walk},
				prev:   l,
			})
		}
	}
}
