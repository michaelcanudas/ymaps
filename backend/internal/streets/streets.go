package streets

import (
	"context"
	"time"

	"ymaps/backend/internal/geo"
)

type Kind string

const (
	Foot    Kind = "foot"
	Bicycle Kind = "bicycle"
)

type Profile struct {
	Kind     Kind
	SpeedKmh float64
}

var Walking = Profile{Kind: Foot, SpeedKmh: 4.8}

type Trace struct {
	Duration time.Duration
	Meters   float64
	Path     []geo.LatLon
}

type Streets interface {
	Route(ctx context.Context, from, to geo.LatLon, profile Profile) (Trace, error)
	Matrix(ctx context.Context, from, to []geo.LatLon, profile Profile) ([][]time.Duration, error)
}

const detourFactor = 1.3

type Estimate struct{}

func (Estimate) Route(_ context.Context, from, to geo.LatLon, profile Profile) (Trace, error) {
	meters := geo.Distance(from, to) * detourFactor
	return Trace{Duration: travelTime(meters, profile), Meters: meters, Path: []geo.LatLon{from, to}}, nil
}

func (Estimate) Matrix(_ context.Context, from, to []geo.LatLon, profile Profile) ([][]time.Duration, error) {
	durations := make([][]time.Duration, len(from))
	for i, origin := range from {
		durations[i] = make([]time.Duration, len(to))
		for j, destination := range to {
			durations[i][j] = travelTime(geo.Distance(origin, destination)*detourFactor, profile)
		}
	}
	return durations, nil
}

func travelTime(meters float64, profile Profile) time.Duration {
	speed := profile.SpeedKmh
	if speed <= 0 {
		speed = Walking.SpeedKmh
	}
	return time.Duration(meters / (speed / 3.6) * float64(time.Second))
}

type Fallback struct {
	Primary Streets
	Backup  Streets
}

func (f Fallback) Route(ctx context.Context, from, to geo.LatLon, profile Profile) (Trace, error) {
	trace, err := f.Primary.Route(ctx, from, to, profile)
	if err == nil {
		return trace, nil
	}
	return f.Backup.Route(ctx, from, to, profile)
}

func (f Fallback) Matrix(ctx context.Context, from, to []geo.LatLon, profile Profile) ([][]time.Duration, error) {
	durations, err := f.Primary.Matrix(ctx, from, to, profile)
	if err == nil {
		return durations, nil
	}
	return f.Backup.Matrix(ctx, from, to, profile)
}
