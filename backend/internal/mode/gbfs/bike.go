package gbfs

import (
	"context"
	"math"
	"time"

	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/streets"
)

const rideNewHavenDiscovery = "https://gbfs.dropmobility.com/systems/newhaven/gbfs.json"

const (
	bikeSpeedKmh    = 13.0
	bikeUnlockCents = 100
	bikeMinuteCents = 25
)

var bikeProfile = streets.Profile{Kind: streets.Bicycle, SpeedKmh: bikeSpeedKmh}

type BikeMode struct {
	reader *Reader
	now    func() time.Time
}

func NewBike(key string) *BikeMode {
	return newBike(key, rideNewHavenDiscovery)
}

func newBike(key, discoveryURL string) *BikeMode {
	mode := &BikeMode{now: time.Now}
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

func (m *BikeMode) Network(_ context.Context, query plan.Query) (plan.Network, error) {
	if m.reader == nil {
		return plan.Network{}, nil
	}
	snapshot := m.reader.Fresh(m.now())
	if snapshot == nil {
		return plan.Network{}, nil
	}
	rental := plan.Rental{Mode: plan.Bike}
	for _, station := range snapshot.Stations {
		if !query.Covers(station.At) {
			continue
		}
		if station.Renting && station.BikesAvailable > 0 {
			bikes := station.BikesAvailable
			rental.Pickups = append(rental.Pickups, bikePickup(stationPoint(station), &bikes))
		}
		if station.Returning {
			rental.Dropoffs = append(rental.Dropoffs, stationPoint(station))
		}
	}
	for _, vehicle := range snapshot.Vehicles {
		if query.Covers(vehicle.At) {
			rental.Pickups = append(rental.Pickups, bikePickup(plan.Point{ID: "bike:" + vehicle.ID, Name: "Bike", At: vehicle.At}, nil))
		}
	}
	if len(rental.Pickups) == 0 || len(rental.Dropoffs) == 0 {
		return plan.Network{}, nil
	}
	return plan.Network{Rentals: []plan.Rental{rental}}, nil
}

func bikePickup(point plan.Point, bikes *int) plan.Pickup {
	return plan.Pickup{
		Point:          point,
		BikesAvailable: bikes,
		Profile:        bikeProfile,
		Unlock:         bikeUnlockAndPark,
		Cost:           bikeCents,
	}
}

func bikeCents(ride time.Duration) int {
	return bikeUnlockCents + bikeMinuteCents*int(math.Ceil(ride.Minutes()))
}

func stationPoint(station Station) plan.Point {
	return plan.Point{ID: "bike:" + station.ID, Name: station.Name, At: station.At}
}
