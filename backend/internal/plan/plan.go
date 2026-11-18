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

func (o Option) DepartAt() time.Time { return o.Legs[0].DepartAt }
func (o Option) ArriveAt() time.Time { return o.Legs[len(o.Legs)-1].ArriveAt }

func (o Option) CostCents() int {
	total := 0
	for _, leg := range o.Legs {
		total += leg.CostCents
	}
	return total
}

type Preferences struct {
	MaxWalk      time.Duration
	MaxCostCents *int
	Avoid        map[ModeName]bool
}

type Trip struct {
	Origin      Point
	Destination Point
	DepartAt    time.Time
	Preferences Preferences
}

type Mode interface {
	Name() ModeName
	Run(ctx context.Context)
	Network(ctx context.Context, query Query) (Network, error)
}

type Planner struct {
	Modes       []Mode
	Streets     streets.Streets
	ModeTimeout time.Duration
	Horizon     time.Duration
}

var ErrNoRoute = errors.New("no route")

const defaultHorizon = 90 * time.Minute

func (p Planner) Run(ctx context.Context) {
	for _, mode := range p.Modes {
		go mode.Run(ctx)
	}
}

func (p Planner) Plan(ctx context.Context, trip Trip) (Option, error) {
	choice, err := p.Choose(ctx, trip)
	return choice.Route, err
}

func (p Planner) Choose(ctx context.Context, trip Trip) (Choice, error) {
	horizon := p.Horizon
	if horizon <= 0 {
		horizon = defaultHorizon
	}
	query := Query{
		Origin:      trip.Origin.At,
		Destination: trip.Destination.At,
		DepartAt:    trip.DepartAt,
		Until:       trip.DepartAt.Add(horizon),
	}
	network := p.collect(ctx, query, trip.Preferences.Avoid)
	s := newSearch(p.Streets, trip, network)
	candidates, err := s.run(ctx)
	if err != nil {
		return Choice{}, err
	}
	options := materializeAll(ctx, p.Streets, trip, candidates)
	policy := policyAt(trip.DepartAt)
	for _, c := range options {
		log.Printf("candidate %s", policy.describe(c, trip.DepartAt))
	}
	prefs := trip.Preferences
	best, ok := policy.choose(options, prefs)
	if !ok {
		prefs = Preferences{}
		best, ok = policy.choose(options, prefs)
	}
	if !ok {
		return Choice{}, ErrNoRoute
	}
	return Choice{Route: best.option, Alternatives: policy.alternatives(best, options, prefs)}, nil
}

func (p Planner) collect(ctx context.Context, query Query, avoid map[ModeName]bool) Network {
	timeout := p.ModeTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	modeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var mutex sync.Mutex
	var group sync.WaitGroup
	var network Network
	for _, mode := range p.Modes {
		if avoid[mode.Name()] {
			continue
		}
		group.Add(1)
		go func(mode Mode) {
			defer group.Done()
			started := time.Now()
			found, err := mode.Network(modeCtx, query)
			if err != nil {
				log.Printf("mode %s: %v after %s", mode.Name(), err, time.Since(started).Round(time.Millisecond))
				return
			}
			log.Printf("mode %s: %d rides, %d rentals in %s", mode.Name(), len(found.Rides), len(found.Rentals), time.Since(started).Round(time.Millisecond))
			mutex.Lock()
			network.Rides = append(network.Rides, found.Rides...)
			network.Rentals = append(network.Rentals, found.Rentals...)
			mutex.Unlock()
		}(mode)
	}
	group.Wait()
	return network
}
