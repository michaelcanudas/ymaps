package shuttle

import (
	"math"
	"sort"
	"sync"
	"time"
)

const (
	busSpeedMetersPerSecond = 4.5
	dwellPerStop            = 20 * time.Second
	minimumLateDelay        = 2 * time.Minute
	lateFraction            = 0.25
	maxMissedLoops          = 3
	observationsKept        = 20
)

type passage struct {
	board     int
	alight    int
	stopCount int
}

func passages(stops []int, board, alight int) []passage {
	n := len(stops)
	var found []passage
	for i, id := range stops {
		if id != board {
			continue
		}
		for distance := 1; distance < n; distance++ {
			j := (i + distance) % n
			if stops[j] == alight {
				found = append(found, passage{board: i, alight: j, stopCount: distance})
				break
			}
		}
	}
	return found
}

type etaLookup func(stopID, busID int) (time.Time, bool)

type pass struct {
	bus      bus
	passage  passage
	boardAt  time.Time
	alightAt time.Time
	lateBy   time.Duration
}

type rideKey struct {
	route  int
	board  int
	alight int
}

type rideObservations struct {
	mutex  sync.Mutex
	byPair map[rideKey][]time.Duration
}

func (o *rideObservations) record(key rideKey, ride time.Duration) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if o.byPair == nil {
		o.byPair = map[rideKey][]time.Duration{}
	}
	recent := append(o.byPair[key], ride)
	if len(recent) > observationsKept {
		recent = recent[len(recent)-observationsKept:]
	}
	o.byPair[key] = recent
}

func (o *rideObservations) median(key rideKey) (time.Duration, bool) {
	o.mutex.Lock()
	recent := append([]time.Duration{}, o.byPair[key]...)
	o.mutex.Unlock()
	if len(recent) == 0 {
		return 0, false
	}
	sort.Slice(recent, func(i, j int) bool { return recent[i] < recent[j] })
	return recent[len(recent)/2], true
}

func (l *line) estimatedRide(p passage) time.Duration {
	return travelDuration(l.rideMeters(p.board, p.alight), p.stopCount)
}

func (l *line) estimatedLoop() time.Duration {
	return travelDuration(l.loopMeters(), len(l.stops))
}

func travelDuration(meters float64, stops int) time.Duration {
	return time.Duration(meters/busSpeedMetersPerSecond*float64(time.Second)) + time.Duration(stops)*dwellPerStop
}

func (l *line) plan(b bus, boardStop, alightStop int, etas etaLookup, observations *rideObservations, walkArrival, now time.Time) (pass, bool) {
	options := passages(l.stops, boardStop, alightStop)
	if len(options) == 0 {
		return pass{}, false
	}
	boardAt, ok := etas(boardStop, b.ID)
	if !ok {
		return pass{}, false
	}
	key := rideKey{route: l.id, board: boardStop, alight: alightStop}
	chosen, ride := options[0], time.Duration(0)
	if alightAt, ok := etas(alightStop, b.ID); ok && alightAt.After(boardAt) {
		ride = alightAt.Sub(boardAt)
		observations.record(key, ride)
		chosen = l.closestPassage(options, ride)
	} else {
		chosen = shortestPassage(options)
		if median, ok := observations.median(key); ok {
			ride = median
		} else {
			ride = l.estimatedRide(chosen)
		}
	}
	loop := l.loopDuration(b, etas, now)
	for missed := 0; boardAt.Before(walkArrival); missed++ {
		if missed == maxMissedLoops || loop <= 0 {
			return pass{}, false
		}
		boardAt = boardAt.Add(loop)
	}
	alightAt := boardAt.Add(ride)
	return pass{bus: b, passage: chosen, boardAt: boardAt, alightAt: alightAt, lateBy: lateDelay(alightAt.Sub(now))}, true
}

func (l *line) closestPassage(options []passage, ride time.Duration) passage {
	best, bestError := options[0], time.Duration(math.MaxInt64)
	for _, option := range options {
		difference := l.estimatedRide(option) - ride
		if difference < 0 {
			difference = -difference
		}
		if difference < bestError {
			best, bestError = option, difference
		}
	}
	return best
}

func shortestPassage(options []passage) passage {
	best := options[0]
	for _, option := range options[1:] {
		if option.stopCount < best.stopCount {
			best = option
		}
	}
	return best
}

func (l *line) loopDuration(b bus, etas etaLookup, now time.Time) time.Duration {
	estimate := l.estimatedLoop()
	if b.LastStop == 0 {
		return estimate
	}
	if at, ok := etas(b.LastStop, b.ID); ok {
		if observed := at.Sub(now); observed >= estimate/2 {
			return observed
		}
	}
	return estimate
}

func lateDelay(untilArrival time.Duration) time.Duration {
	delay := time.Duration(float64(untilArrival) * lateFraction)
	if delay < minimumLateDelay {
		return minimumLateDelay
	}
	return delay
}
