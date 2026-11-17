package shuttle

import (
	"math"
	"sort"

	"ymaps/backend/internal/geo"
)

const (
	snapRadiusMeters        = 75.0
	backtrackToleranceMeter = 10.0
	loopToleranceFraction   = 0.01
)

type line struct {
	id        int
	name      string
	color     string
	stops     []int
	locations []geo.LatLon
	path      []geo.LatLon
	loop      []geo.LatLon
	offsets   []float64
	length    float64
	snapped   []float64
	gaps      []float64
	trusted   bool
}

type snapCandidate struct {
	offset float64
	meters float64
}

func newLine(r route, stops map[int]stop) *line {
	l := &line{id: r.ID, name: r.Name, color: r.Color, stops: r.Stops}
	for _, id := range r.Stops {
		s, ok := stops[id]
		if !ok {
			return nil
		}
		l.locations = append(l.locations, s.at())
	}
	for i := 0; i+1 < len(r.Path); i += 2 {
		l.path = append(l.path, geo.LatLon{Lat: r.Path[i], Lon: r.Path[i+1]})
	}
	l.buildLoop()
	l.snapStops()
	return l
}

func (l *line) buildLoop() {
	if len(l.path) < 2 {
		return
	}
	l.loop = append([]geo.LatLon{}, l.path...)
	if l.loop[0] != l.loop[len(l.loop)-1] {
		l.loop = append(l.loop, l.loop[0])
	}
	l.offsets = make([]float64, len(l.loop))
	for i := 1; i < len(l.loop); i++ {
		l.offsets[i] = l.offsets[i-1] + geo.Distance(l.loop[i-1], l.loop[i])
	}
	l.length = l.offsets[len(l.offsets)-1]
}

func (l *line) snapStops() {
	n := len(l.stops)
	if l.length <= 0 || n < 2 {
		return
	}
	candidates := make([][]snapCandidate, n)
	for k, location := range l.locations {
		candidates[k] = l.candidates(location)
		if len(candidates[k]) == 0 {
			return
		}
	}
	bestCost := math.Inf(1)
	var bestChoice []int
	for first := range candidates[0] {
		cost, choice := l.cheapestChoice(candidates, first)
		if cost < bestCost {
			bestCost, bestChoice = cost, choice
		}
	}
	if bestChoice == nil {
		return
	}
	l.snapped = make([]float64, n)
	for k, c := range bestChoice {
		l.snapped[k] = candidates[k][c].offset
	}
	l.gaps = make([]float64, n)
	total := 0.0
	for k := range l.snapped {
		l.gaps[k] = l.gap(l.snapped[k], l.snapped[(k+1)%n])
		total += l.gaps[k]
	}
	l.trusted = math.Abs(total-l.length) <= l.length*loopToleranceFraction
}

func (l *line) cheapestChoice(candidates [][]snapCandidate, first int) (float64, []int) {
	n := len(candidates)
	costs := []float64{}
	back := make([][]int, n)
	for c := range candidates[0] {
		if c == first {
			costs = append(costs, candidates[0][c].meters/2)
		} else {
			costs = append(costs, math.Inf(1))
		}
	}
	for k := 1; k < n; k++ {
		next := make([]float64, len(candidates[k]))
		back[k] = make([]int, len(candidates[k]))
		for c, current := range candidates[k] {
			next[c] = math.Inf(1)
			for p, previous := range candidates[k-1] {
				cost := costs[p] + l.gap(previous.offset, current.offset) + current.meters/2
				if cost < next[c] {
					next[c], back[k][c] = cost, p
				}
			}
		}
		costs = next
	}
	best, bestLast := math.Inf(1), -1
	for c, last := range candidates[n-1] {
		cost := costs[c] + l.gap(last.offset, candidates[0][first].offset)
		if cost < best {
			best, bestLast = cost, c
		}
	}
	if bestLast < 0 {
		return best, nil
	}
	choice := make([]int, n)
	choice[n-1] = bestLast
	for k := n - 1; k > 0; k-- {
		choice[k-1] = back[k][choice[k]]
	}
	return best, choice
}

func (l *line) gap(from, to float64) float64 {
	forward := math.Mod(to-from, l.length)
	if forward < 0 {
		forward += l.length
	}
	if forward > l.length-backtrackToleranceMeter {
		forward -= l.length
	}
	return forward
}

func (l *line) candidates(location geo.LatLon) []snapCandidate {
	var found []snapCandidate
	for i := 0; i+1 < len(l.loop); i++ {
		fraction := projectionFraction(l.loop[i], l.loop[i+1], location)
		point := interpolate(l.loop[i], l.loop[i+1], fraction)
		meters := geo.Distance(point, location)
		if meters > snapRadiusMeters {
			continue
		}
		offset := l.offsets[i] + fraction*(l.offsets[i+1]-l.offsets[i])
		found = append(found, snapCandidate{offset: math.Mod(offset, l.length), meters: meters})
	}
	return found
}

func (l *line) rideMeters(board, alight int) float64 {
	n := len(l.stops)
	total := 0.0
	for k := board; k != alight; k = (k + 1) % n {
		if l.trusted {
			total += math.Max(0, l.gaps[k])
		} else {
			total += geo.Distance(l.locations[k], l.locations[(k+1)%n])
		}
	}
	return total
}

func (l *line) loopMeters() float64 {
	if l.trusted {
		return l.length
	}
	return l.straightLoopMeters()
}

func (l *line) straightLoopMeters() float64 {
	total := 0.0
	n := len(l.locations)
	for k := range l.locations {
		total += geo.Distance(l.locations[k], l.locations[(k+1)%n])
	}
	return total
}

func (l *line) legPath(board, alight int) []geo.LatLon {
	if !l.trusted {
		return []geo.LatLon{l.locations[board], l.locations[alight]}
	}
	return l.slice(l.snapped[board], l.rideMeters(board, alight))
}

func (l *line) slice(from, meters float64) []geo.LatLon {
	path := []geo.LatLon{l.pointAt(from)}
	if meters <= 0 {
		return append(path, path[0])
	}
	target := from + meters
	segments := len(l.loop) - 1
	start := sort.SearchFloat64s(l.offsets, from)
	if start < len(l.offsets) && l.offsets[start] <= from {
		start++
	}
	for k := start; ; k++ {
		vertex := k % segments
		unrolled := l.offsets[vertex] + float64(k/segments)*l.length
		if unrolled >= target {
			break
		}
		path = append(path, l.loop[vertex])
	}
	return append(path, l.pointAt(math.Mod(target, l.length)))
}

func (l *line) pointAt(offset float64) geo.LatLon {
	index := sort.SearchFloat64s(l.offsets, offset)
	if index <= 0 {
		return l.loop[0]
	}
	if index >= len(l.offsets) {
		return l.loop[len(l.loop)-1]
	}
	span := l.offsets[index] - l.offsets[index-1]
	if span <= 0 {
		return l.loop[index]
	}
	return interpolate(l.loop[index-1], l.loop[index], (offset-l.offsets[index-1])/span)
}

func projectionFraction(a, b, p geo.LatLon) float64 {
	scale := math.Cos(a.Lat * math.Pi / 180)
	dx, dy := (b.Lon-a.Lon)*scale, b.Lat-a.Lat
	px, py := (p.Lon-a.Lon)*scale, p.Lat-a.Lat
	squared := dx*dx + dy*dy
	if squared == 0 {
		return 0
	}
	return math.Max(0, math.Min(1, (px*dx+py*dy)/squared))
}

func interpolate(a, b geo.LatLon, fraction float64) geo.LatLon {
	return geo.LatLon{Lat: a.Lat + (b.Lat-a.Lat)*fraction, Lon: a.Lon + (b.Lon-a.Lon)*fraction}
}
