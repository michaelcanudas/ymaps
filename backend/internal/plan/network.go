package plan

import (
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/streets"
)

type Query struct {
	Origin      geo.LatLon
	Destination geo.LatLon
	DepartAt    time.Time
	Until       time.Time
}

const corridorSlackMeters = 2500.0

func (q Query) Covers(at geo.LatLon) bool {
	direct := geo.Distance(q.Origin, q.Destination)
	return geo.Distance(q.Origin, at)+geo.Distance(at, q.Destination) <= direct+corridorSlackMeters
}

type StopTime struct {
	Stop   Point
	Arrive time.Time
	Depart time.Time
}

type Fare struct {
	Cents int
	Group string
}

type Ride struct {
	Mode     ModeName
	Line     *Line
	Headsign string
	Vehicle  *Vehicle
	Fare     Fare
	Stops    []StopTime
	Path     func(board, alight int) []geo.LatLon
}

type Pickup struct {
	Point          Point
	Vehicle        *Vehicle
	Form           string
	RentalURI      string
	BikesAvailable *int
	Profile        streets.Profile
	RangeMeters    *float64
	Unlock         time.Duration
	Cost           func(ride time.Duration) int
}

const (
	rangeSafetyFactor  = 1.3
	rangeReserveMeters = 500.0
)

func (p Pickup) Reaches(meters float64) bool {
	if p.RangeMeters == nil {
		return true
	}
	return *p.RangeMeters >= rangeSafetyFactor*meters+rangeReserveMeters
}

type Rental struct {
	Mode     ModeName
	Pickups  []Pickup
	Dropoffs []Point
}

type Network struct {
	Rides   []Ride
	Rentals []Rental
}
