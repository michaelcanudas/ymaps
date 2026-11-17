package plan

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/streets"
)

type ModeName string

const (
	Walk    ModeName = "walk"
	Shuttle ModeName = "shuttle"
	Bus     ModeName = "bus"
	Bike    ModeName = "bike"
	Veo     ModeName = "veo"
)

type Point struct {
	ID   string
	Name string
	At   geo.LatLon
}

type Line struct {
	ID    string
	Name  string
	Color string
	Path  []geo.LatLon
}

type Vehicle struct {
	ID      string
	Name    string
	At      geo.LatLon
	Heading *int
}

type Leg struct {
	Mode           ModeName
	From           Point
	To             Point
	DepartAt       time.Time
	ArriveAt       time.Time
	ArriveAtLate   time.Time
	CostCents      int
	Path           []geo.LatLon
	Line           *Line
	Headsign       string
	StopCount      int
	Vehicle        *Vehicle
	BikesAvailable *int
	Form           string
	RentalURI      string
}

type Option struct {
	Legs []Leg
}

func (o Option) DepartAt() time.Time     { return o.Legs[0].DepartAt }
func (o Option) ArriveAt() time.Time     { return o.Legs[len(o.Legs)-1].ArriveAt }
func (o Option) ArriveAtLate() time.Time { return o.Legs[len(o.Legs)-1].ArriveAtLate }

func (o Option) CostCents() int {
	total := 0
	for _, leg := range o.Legs {
		total += leg.CostCents
	}
	return total
}

type Trip struct {
	Origin      Point
	Destination Point
	DepartAt    time.Time
}

type Mode interface {
	Name() ModeName
	Run(ctx context.Context)
	Options(ctx context.Context, trip Trip) ([]Option, error)
}

const walkLateFactor = 1.1

func WalkLeg(ctx context.Context, s streets.Streets, from, to Point, departAt time.Time) (Leg, error) {
	trace, err := s.Route(ctx, from.At, to.At, streets.Walking)
	if err != nil {
		return Leg{}, err
	}
	return Leg{
		Mode:         Walk,
		From:         from,
		To:           to,
		DepartAt:     departAt,
		ArriveAt:     departAt.Add(trace.Duration),
		ArriveAtLate: departAt.Add(time.Duration(float64(trace.Duration) * walkLateFactor)),
		Path:         trace.Path,
	}, nil
}

type Planner struct {
	Modes          []Mode
	Streets        streets.Streets
	CentsPerMinute float64
	ModeTimeout    time.Duration
}

var ErrNoRoute = errors.New("no route")

func (p Planner) Run(ctx context.Context) {
	for _, mode := range p.Modes {
		go mode.Run(ctx)
	}
}

func (p Planner) Plan(ctx context.Context, trip Trip) (Option, error) {
	options := p.collect(ctx, trip)
	if walk, err := WalkLeg(ctx, p.Streets, trip.Origin, trip.Destination, trip.DepartAt); err == nil {
		options = append(options, Option{Legs: []Leg{walk}})
	}
	var best Option
	bestScore := 0.0
	for _, option := range options {
		if len(option.Legs) == 0 {
			continue
		}
		score := p.score(option, trip.DepartAt)
		if len(best.Legs) == 0 || score < bestScore {
			best, bestScore = option, score
		}
	}
	if len(best.Legs) == 0 {
		return Option{}, ErrNoRoute
	}
	return best, nil
}

func (p Planner) score(option Option, departAt time.Time) float64 {
	minutes := option.ArriveAtLate().Sub(departAt).Minutes()
	if p.CentsPerMinute <= 0 {
		return minutes
	}
	return minutes + float64(option.CostCents())/p.CentsPerMinute
}

func (p Planner) collect(ctx context.Context, trip Trip) []Option {
	timeout := p.ModeTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	modeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var mutex sync.Mutex
	var group sync.WaitGroup
	var options []Option
	for _, mode := range p.Modes {
		group.Add(1)
		go func(mode Mode) {
			defer group.Done()
			started := time.Now()
			found, err := mode.Options(modeCtx, trip)
			if err != nil {
				log.Printf("mode %s: %v after %s", mode.Name(), err, time.Since(started).Round(time.Millisecond))
				return
			}
			log.Printf("mode %s: %d options in %s", mode.Name(), len(found), time.Since(started).Round(time.Millisecond))
			mutex.Lock()
			options = append(options, found...)
			mutex.Unlock()
		}(mode)
	}
	group.Wait()
	return options
}
