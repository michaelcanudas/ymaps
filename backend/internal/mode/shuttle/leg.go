package shuttle

import (
	"strconv"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
)

const (
	busSpeedMetersPerSecond = 5.0
	dwellPerStop            = 15 * time.Second
	loopsAhead              = 2
	minimumPace             = 0.6
	maximumPace             = 1.8
	anchorBacktrack         = 30 * time.Second
)

type etaLookup func(stopID, busID int) (time.Time, bool)

func (l *line) segment(k int) time.Duration {
	n := len(l.stops)
	return travelDuration(l.rideMeters(k, (k+1)%n), 1)
}

func travelDuration(meters float64, stops int) time.Duration {
	return time.Duration(meters/busSpeedMetersPerSecond*float64(time.Second)) + time.Duration(stops)*dwellPerStop
}

func (l *line) startIndex(b bus, etas etaLookup) int {
	n := len(l.stops)
	soonest, soonestAt := -1, time.Time{}
	for k, id := range l.stops {
		at, ok := etas(id, b.ID)
		if !ok {
			continue
		}
		if soonest < 0 || at.Before(soonestAt) || at.Equal(soonestAt) && l.stops[(k+n-1)%n] == b.LastStop {
			soonest, soonestAt = k, at
		}
	}
	if soonest >= 0 {
		best, bestScore := soonest, time.Duration(1<<62)
		for k, id := range l.stops {
			if id != l.stops[soonest] {
				continue
			}
			score := time.Duration(1 << 61)
			if next, ok := etas(l.stops[(k+1)%n], b.ID); ok {
				score = next.Sub(soonestAt)
				if score < 0 {
					score = time.Duration(1<<60) - score
				}
			}
			if l.stops[(k+n-1)%n] == b.LastStop {
				score -= time.Minute
			}
			if score < bestScore {
				best, bestScore = k, score
			}
		}
		return best
	}
	if b.LastStop != 0 {
		for k, id := range l.stops {
			if id == b.LastStop {
				return (k + 1) % n
			}
		}
	}
	return 0
}

type anchor struct {
	position int
	at       time.Time
}

func (l *line) schedule(b bus, etas etaLookup, now, until time.Time) ([]int, []time.Time) {
	n := len(l.stops)
	if n < 2 {
		return nil, nil
	}
	start := l.startIndex(b, etas)
	total := n * loopsAhead
	positions := make([]int, total)
	expected := make([]time.Duration, total)
	elapsed := time.Duration(0)
	for j := 0; j < total; j++ {
		positions[j] = (start + j) % n
		if j > 0 {
			elapsed += l.segment(positions[j-1])
		}
		expected[j] = elapsed
	}
	var anchors []anchor
	seen := map[int]bool{}
	for j := 0; j < n; j++ {
		id := l.stops[positions[j]]
		if seen[id] {
			continue
		}
		seen[id] = true
		at, ok := etas(id, b.ID)
		if !ok {
			continue
		}
		if len(anchors) > 0 && at.Before(anchors[len(anchors)-1].at.Add(-anchorBacktrack)) {
			continue
		}
		if len(anchors) > 0 && at.Before(anchors[len(anchors)-1].at) {
			at = anchors[len(anchors)-1].at
		}
		anchors = append(anchors, anchor{position: j, at: at})
	}
	pace := 1.0
	if len(anchors) >= 2 {
		first, last := anchors[0], anchors[len(anchors)-1]
		if span := expected[last.position] - expected[first.position]; span > 2*time.Minute {
			pace = clamp(last.at.Sub(first.at).Seconds()/span.Seconds(), minimumPace, maximumPace)
		}
	}
	times := make([]time.Time, total)
	scaled := func(from, to int) time.Duration {
		return time.Duration(pace * float64(expected[to]-expected[from]))
	}
	for j := 0; j < total; j++ {
		var before, after *anchor
		for k := range anchors {
			if anchors[k].position <= j {
				before = &anchors[k]
			}
			if anchors[k].position >= j && after == nil {
				after = &anchors[k]
			}
		}
		switch {
		case before != nil && after != nil && before.position != after.position:
			fraction := float64(expected[j]-expected[before.position]) / float64(expected[after.position]-expected[before.position])
			times[j] = before.at.Add(time.Duration(fraction * float64(after.at.Sub(before.at))))
		case before != nil:
			times[j] = before.at.Add(scaled(before.position, j))
		case after != nil:
			times[j] = after.at.Add(-scaled(j, after.position))
		default:
			times[j] = now.Add(scaled(0, j))
		}
	}
	for j := range times {
		if times[j].Before(now) {
			times[j] = now
		}
		if j > 0 && times[j].Before(times[j-1]) {
			times[j] = times[j-1]
		}
	}
	cut := total
	for j := range times {
		if times[j].After(until) {
			cut = j + 1
			break
		}
	}
	return positions[:cut], times[:cut]
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func (l *line) ride(b bus, stops map[int]stop, etas etaLookup, now, until time.Time) (plan.Ride, bool) {
	positions, times := l.schedule(b, etas, now, until)
	if len(positions) < 2 {
		return plan.Ride{}, false
	}
	stopTimes := make([]plan.StopTime, len(positions))
	for j, k := range positions {
		s, ok := stops[l.stops[k]]
		if !ok {
			return plan.Ride{}, false
		}
		stopTimes[j] = plan.StopTime{Stop: stopPoint(s), Arrive: times[j], Depart: times[j]}
	}
	heading := b.Heading
	return plan.Ride{
		Mode: plan.Shuttle,
		Line: &plan.Line{
			ID:    "shuttle:line:" + strconv.Itoa(l.id),
			Name:  l.name,
			Color: l.color,
			Path:  l.path,
		},
		Vehicle: &plan.Vehicle{
			ID:      "shuttle:bus:" + strconv.Itoa(b.ID),
			Name:    b.Name,
			At:      geo.LatLon{Lat: b.Lat, Lon: b.Lon},
			Heading: &heading,
		},
		Stops: stopTimes,
		Path: func(board, alight int) []geo.LatLon {
			var path []geo.LatLon
			for j := board; j < alight; j++ {
				piece := l.legPath(positions[j], positions[j+1])
				if len(path) > 0 && len(piece) > 0 {
					piece = piece[1:]
				}
				path = append(path, piece...)
			}
			return path
		},
	}, true
}

func stopPoint(s stop) plan.Point {
	return plan.Point{ID: "shuttle:" + strconv.Itoa(s.ID), Name: s.Name, At: s.at()}
}
