package gbfs

import (
	"context"
	"math"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/streets"
)

const rideNewHavenDiscovery = "https://gbfs.dropmobility.com/systems/newhaven/gbfs.json"

const (
	bikeSpeedKmh      = 15.0
	bikeLateSpeedKmh  = 12.0
	bikeUnlockCents   = 100
	bikeMinuteCents   = 25
	minimumRideMeters = 100.0
)

var bikeProfile = streets.Profile{Kind: streets.Bicycle, SpeedKmh: bikeSpeedKmh}

type BikeMode struct {
	streets streets.Streets
	reader  *Reader
	now     func() time.Time
}

func NewBike(s streets.Streets, key string) *BikeMode {
	return newBike(s, key, rideNewHavenDiscovery)
}

func newBike(s streets.Streets, key, discoveryURL string) *BikeMode {
	mode := &BikeMode{streets: s, now: time.Now}
	if key == "" {
		return mode
	}
	mode.reader = NewReader(Config{
		Name:         "ride-new-haven",
		DiscoveryURL: discoveryURL,
		Headers:      map[string]string{"Authorization": "Bearer " + key},
		Feeds: []Feed{
			{Name: StationInformation, Required: true},
			{Name: StationStatus, Required: true},
			{Name: FreeBikeStatus},
			{Name: VehicleTypes},
		},
	})
	return mode
}

func (m *BikeMode) Name() plan.ModeName {
	return plan.Bike
}

func (m *BikeMode) Run(ctx context.Context) {
	if m.reader == nil {
		<-ctx.Done()
		return
	}
	m.reader.Run(ctx)
}

type bikePickup struct {
	point plan.Point
	bikes int
}

func (m *BikeMode) Options(ctx context.Context, trip plan.Trip) ([]plan.Option, error) {
	if m.reader == nil {
		return nil, nil
	}
	snapshot := m.reader.Fresh(m.now())
	if snapshot == nil {
		return nil, nil
	}
	pickups := nearestPickups(snapshot, trip.Origin.At)
	dropoffs := nearestReturns(snapshot, trip.Destination.At)
	if len(pickups) == 0 || len(dropoffs) == 0 {
		return nil, nil
	}
	pickupPoints := make([]geo.LatLon, len(pickups))
	for i, pickup := range pickups {
		pickupPoints[i] = pickup.point.At
	}
	dropoffPoints := make([]geo.LatLon, len(dropoffs))
	for i, dropoff := range dropoffs {
		dropoffPoints[i] = dropoff.At
	}
	walkIn, walkOut, err := walkTimes(ctx, m.streets, trip.Origin.At, pickupPoints, dropoffPoints, trip.Destination.At)
	if err != nil {
		return nil, err
	}
	rides, err := m.streets.Matrix(ctx, pickupPoints, dropoffPoints, bikeProfile)
	if err != nil {
		return nil, err
	}
	pairs := bestPairs(len(pickups), len(dropoffs), func(i, j int) (time.Duration, bool) {
		if geo.Distance(pickupPoints[i], dropoffPoints[j]) < minimumRideMeters {
			return 0, false
		}
		late := scaleDuration(walkIn[i], estimatedLateWalk) + bikeLate(rides[i][j]) + loneVehicleRisk(walkIn[i], pickups[i].bikes) + scaleDuration(walkOut[j], estimatedLateWalk)
		return late, true
	})
	var options []plan.Option
	for _, chosen := range pairs {
		pickup := pickups[chosen.pickup]
		dropoff := dropoffs[chosen.dropoff]
		trace, err := m.streets.Route(ctx, pickup.point.At, dropoff.At, bikeProfile)
		if err != nil {
			continue
		}
		rideP50 := trace.Duration + unlockAndParkP50
		rideP90 := bikeLate(trace.Duration) + loneVehicleRisk(walkIn[chosen.pickup], pickup.bikes)
		bikes := pickup.bikes
		leg := plan.Leg{
			Mode:           plan.Bike,
			From:           pickup.point,
			To:             dropoff,
			CostCents:      bikeCents(rideP50),
			Path:           trace.Path,
			BikesAvailable: &bikes,
		}
		option, err := assemble(ctx, m.streets, trip, leg, rideP50, rideP90)
		if err != nil {
			continue
		}
		options = append(options, option)
	}
	return options, nil
}

func bikeLate(ride time.Duration) time.Duration {
	return scaleDuration(ride, bikeSpeedKmh/bikeLateSpeedKmh) + unlockAndParkP90
}

func bikeCents(ride time.Duration) int {
	return bikeUnlockCents + bikeMinuteCents*int(math.Ceil(ride.Minutes()))
}

func nearestPickups(snapshot *Snapshot, origin geo.LatLon) []bikePickup {
	var candidates []bikePickup
	for _, station := range snapshot.Stations {
		if station.Renting && station.BikesAvailable > 0 {
			candidates = append(candidates, bikePickup{point: stationPoint(station), bikes: station.BikesAvailable})
		}
	}
	for _, vehicle := range snapshot.Vehicles {
		candidates = append(candidates, bikePickup{point: plan.Point{ID: "bike:" + vehicle.ID, Name: "Bike", At: vehicle.At}, bikes: 1})
	}
	points := make([]geo.LatLon, len(candidates))
	for i, candidate := range candidates {
		points[i] = candidate.point.At
	}
	var chosen []bikePickup
	for _, index := range nearestIndices(points, origin, candidatesPerEnd) {
		chosen = append(chosen, candidates[index])
	}
	return chosen
}

func nearestReturns(snapshot *Snapshot, destination geo.LatLon) []plan.Point {
	var candidates []plan.Point
	for _, station := range snapshot.Stations {
		if station.Returning {
			candidates = append(candidates, stationPoint(station))
		}
	}
	points := make([]geo.LatLon, len(candidates))
	for i, candidate := range candidates {
		points[i] = candidate.At
	}
	var chosen []plan.Point
	for _, index := range nearestIndices(points, destination, candidatesPerEnd) {
		chosen = append(chosen, candidates[index])
	}
	return chosen
}

func stationPoint(station Station) plan.Point {
	return plan.Point{ID: "bike:" + station.ID, Name: station.Name, At: station.At}
}
