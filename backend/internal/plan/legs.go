package plan

import (
	"context"
	"sync"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/streets"
)

const (
	negligibleWalk   = 20 * time.Second
	negligibleMeters = 25.0
)

func materializeAll(ctx context.Context, s streets.Streets, trip Trip, labels []*label) []candidate {
	results := make([]*candidate, len(labels))
	var group sync.WaitGroup
	for i, l := range labels {
		group.Add(1)
		go func(i int, l *label) {
			defer group.Done()
			if c, ok := materialize(ctx, s, trip, l); ok {
				results[i] = &c
			}
		}(i, l)
	}
	group.Wait()
	var candidates []candidate
	for _, c := range results {
		if c != nil {
			candidates = append(candidates, *c)
		}
	}
	return candidates
}

func chain(l *label) []*label {
	var steps []*label
	for current := l; current != nil && current.step.kind != starting; current = current.prev {
		steps = append(steps, current)
	}
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	return steps
}

func materialize(ctx context.Context, s streets.Streets, trip Trip, final *label) (candidate, bool) {
	steps := chain(final)
	points := map[*label]Point{}
	location := func(l *label) Point {
		if l == nil || l.step.kind == starting {
			return trip.Origin
		}
		if p, ok := points[l]; ok {
			return p
		}
		return trip.Destination
	}
	for _, l := range steps {
		switch l.step.kind {
		case riding:
			points[l] = l.step.ride.Stops[l.step.alight].Stop
		case renting:
			points[l] = l.step.to
		}
	}
	for i, l := range steps {
		if l.step.kind != walking {
			continue
		}
		if i+1 < len(steps) {
			next := steps[i+1]
			switch next.step.kind {
			case riding:
				points[l] = next.step.ride.Stops[next.step.board].Stop
			case renting:
				points[l] = next.step.pickup.Point
			}
		} else {
			points[l] = trip.Destination
		}
	}
	c := candidate{modes: map[ModeName]bool{}}
	at := trip.DepartAt
	var legs []Leg
	rides := 0
	for i, l := range steps {
		from := location(l.prev)
		to := location(l)
		switch l.step.kind {
		case walking:
			if geo.Distance(from.At, to.At) <= negligibleMeters && l.step.walk <= negligibleWalk {
				continue
			}
			trace, err := s.Route(ctx, from.At, to.At, streets.Walking)
			if err != nil {
				return candidate{}, false
			}
			legs = append(legs, Leg{Mode: Walk, From: from, To: to, DepartAt: at, ArriveAt: at.Add(trace.Duration), Path: trace.Path})
			at = at.Add(trace.Duration)
			c.walk += trace.Duration
		case riding:
			ride := l.step.ride
			board := ride.Stops[l.step.board]
			alight := ride.Stops[l.step.alight]
			if at.After(board.Depart) {
				return candidate{}, false
			}
			if rides == 0 {
				if shift := board.Depart.Add(-boardBuffer).Sub(at); shift > 0 && len(legs) > 0 {
					for k := range legs {
						legs[k].DepartAt = legs[k].DepartAt.Add(shift)
						legs[k].ArriveAt = legs[k].ArriveAt.Add(shift)
					}
				}
			} else {
				c.wait += board.Depart.Sub(at)
			}
			arrive := alight.Arrive
			if arrive.Before(board.Depart) {
				arrive = board.Depart
			}
			leg := Leg{
				Mode:      ride.Mode,
				From:      board.Stop,
				To:        alight.Stop,
				DepartAt:  board.Depart,
				ArriveAt:  arrive,
				CostCents: rideCost(steps[:i], ride),
				Line:      ride.Line,
				Headsign:  ride.Headsign,
				StopCount: l.step.alight - l.step.board,
				Vehicle:   ride.Vehicle,
			}
			if ride.Path != nil {
				leg.Path = ride.Path(l.step.board, l.step.alight)
			}
			if len(leg.Path) < 2 {
				leg.Path = []geo.LatLon{board.Stop.At, alight.Stop.At}
			}
			legs = append(legs, leg)
			at = arrive
			rides++
		case renting:
			pickup := l.step.pickup
			trace, err := s.Route(ctx, pickup.Point.At, l.step.to.At, pickup.Profile)
			if err != nil || !pickup.Reaches(trace.Meters) {
				return candidate{}, false
			}
			duration := trace.Duration + pickup.Unlock
			legs = append(legs, Leg{
				Mode:           l.step.mode,
				From:           pickup.Point,
				To:             l.step.to,
				DepartAt:       at,
				ArriveAt:       at.Add(duration),
				CostCents:      pickup.Cost(duration),
				Path:           trace.Path,
				Vehicle:        pickup.Vehicle,
				BikesAvailable: pickup.BikesAvailable,
				Form:           pickup.Form,
				RentalURI:      pickup.RentalURI,
			})
			at = at.Add(duration)
			rides++
		}
	}
	if len(legs) == 0 {
		legs = []Leg{{Mode: Walk, From: trip.Origin, To: trip.Destination, DepartAt: trip.DepartAt, ArriveAt: trip.DepartAt, Path: []geo.LatLon{trip.Origin.At, trip.Destination.At}}}
	}
	c.option = Option{Legs: legs}
	c.arrive = c.option.ArriveAt()
	c.cost = c.option.CostCents()
	c.rides = rides
	for _, leg := range legs {
		if leg.Mode != Walk {
			c.modes[leg.Mode] = true
		}
	}
	return c, true
}

func rideCost(before []*label, ride *Ride) int {
	if ride.Fare.Group == "" {
		return ride.Fare.Cents
	}
	for _, l := range before {
		if l.step.kind == riding && l.step.ride.Fare.Group == ride.Fare.Group {
			return 0
		}
	}
	return ride.Fare.Cents
}
