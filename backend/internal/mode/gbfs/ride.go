package gbfs

import (
	"context"
	"sort"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/streets"
)

const (
	searchRadiusMeters  = 1200.0
	candidatesPerEnd    = 4
	optionsReturned     = 3
	estimatedLateWalk   = 1.1
	unlockAndParkP50    = time.Minute
	unlockAndParkP90    = 150 * time.Second
	loneVehicleRiskRate = 5
)

type pair struct {
	pickup  int
	dropoff int
	score   time.Duration
}

func nearestIndices(points []geo.LatLon, target geo.LatLon, limit int) []int {
	var indices []int
	for i, point := range points {
		if geo.Distance(point, target) <= searchRadiusMeters {
			indices = append(indices, i)
		}
	}
	sort.SliceStable(indices, func(a, b int) bool {
		return geo.Distance(points[indices[a]], target) < geo.Distance(points[indices[b]], target)
	})
	if len(indices) > limit {
		indices = indices[:limit]
	}
	return indices
}

func walkTimes(ctx context.Context, s streets.Streets, origin geo.LatLon, pickups []geo.LatLon, dropoffs []geo.LatLon, destination geo.LatLon) ([]time.Duration, []time.Duration, error) {
	inbound, err := s.Matrix(ctx, []geo.LatLon{origin}, pickups, streets.Walking)
	if err != nil {
		return nil, nil, err
	}
	outbound, err := s.Matrix(ctx, dropoffs, []geo.LatLon{destination}, streets.Walking)
	if err != nil {
		return nil, nil, err
	}
	walkOut := make([]time.Duration, len(dropoffs))
	for i := range dropoffs {
		walkOut[i] = outbound[i][0]
	}
	return inbound[0], walkOut, nil
}

func loneVehicleRisk(walk time.Duration, vehicles int) time.Duration {
	if vehicles > 1 {
		return 0
	}
	return walk / loneVehicleRiskRate
}

func bestPairs(pickups, dropoffs int, score func(pickup, dropoff int) (time.Duration, bool)) []pair {
	var pairs []pair
	for i := 0; i < pickups; i++ {
		for j := 0; j < dropoffs; j++ {
			if value, ok := score(i, j); ok {
				pairs = append(pairs, pair{pickup: i, dropoff: j, score: value})
			}
		}
	}
	sort.SliceStable(pairs, func(a, b int) bool { return pairs[a].score < pairs[b].score })
	if len(pairs) > optionsReturned {
		pairs = pairs[:optionsReturned]
	}
	return pairs
}

func scaleDuration(duration time.Duration, factor float64) time.Duration {
	return time.Duration(float64(duration) * factor)
}

func assemble(ctx context.Context, s streets.Streets, trip plan.Trip, ride plan.Leg, rideP50, rideP90 time.Duration) (plan.Option, error) {
	first, err := plan.WalkLeg(ctx, s, trip.Origin, ride.From, trip.DepartAt)
	if err != nil {
		return plan.Option{}, err
	}
	ride.DepartAt = first.ArriveAt
	ride.ArriveAt = first.ArriveAt.Add(rideP50)
	ride.ArriveAtLate = first.ArriveAtLate.Add(rideP90)
	last, err := plan.WalkLeg(ctx, s, ride.To, trip.Destination, ride.ArriveAt)
	if err != nil {
		return plan.Option{}, err
	}
	last.ArriveAtLate = ride.ArriveAtLate.Add(last.ArriveAtLate.Sub(last.DepartAt))
	return plan.Option{Legs: []plan.Leg{first, ride, last}}, nil
}
