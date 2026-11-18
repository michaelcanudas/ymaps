package plan

import (
	"context"
	"testing"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/streets"
)

var noon = time.Date(2026, 11, 17, 12, 0, 0, 0, newHaven)

type fixedMode struct {
	name    ModeName
	network Network
}

func (f fixedMode) Name() ModeName                                  { return f.name }
func (f fixedMode) Run(context.Context)                             {}
func (f fixedMode) Network(context.Context, Query) (Network, error) { return f.network, nil }

func point(id string, lat, lon float64) Point {
	return Point{ID: id, Name: id, At: geo.LatLon{Lat: lat, Lon: lon}}
}

func ride(mode ModeName, line string, fare Fare, stops ...StopTime) Ride {
	return Ride{Mode: mode, Line: &Line{ID: line, Name: line}, Fare: fare, Stops: stops}
}

func at(p Point, minutes int) StopTime {
	t := noon.Add(time.Duration(minutes) * time.Minute)
	return StopTime{Stop: p, Arrive: t, Depart: t}
}

func plan(t *testing.T, trip Trip, modes ...Mode) Option {
	t.Helper()
	planner := Planner{Streets: streets.Estimate{}, Modes: modes}
	option, err := planner.Plan(context.Background(), trip)
	if err != nil {
		t.Fatal(err)
	}
	return option
}

func modes(option Option) []ModeName {
	var found []ModeName
	for _, leg := range option.Legs {
		found = append(found, leg.Mode)
	}
	return found
}

func TestTransferBetweenLinesEmerges(t *testing.T) {
	origin := point("origin", 41.3000, -72.9300)
	a := point("a", 41.3005, -72.9300)
	b := point("b", 41.3200, -72.9300)
	c := point("c", 41.3200, -72.9000)
	destination := point("destination", 41.3205, -72.9000)
	first := ride(Shuttle, "north", Fare{}, at(a, 3), at(b, 10))
	second := ride(Shuttle, "east", Fare{}, at(b, 12), at(c, 20))
	option := plan(t, Trip{Origin: origin, Destination: destination, DepartAt: noon}, fixedMode{Shuttle, Network{Rides: []Ride{first, second}}})
	got := modes(option)
	want := []ModeName{Walk, Shuttle, Shuttle, Walk}
	if len(got) != len(want) {
		t.Fatalf("modes %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("modes %v", got)
		}
	}
	if option.Legs[1].To != option.Legs[2].From {
		t.Fatal("transfer is not at the shared stop")
	}
	if !option.Legs[0].ArriveAt.Add(boardBuffer).Equal(option.Legs[1].DepartAt) {
		t.Fatal("first walk should leave just in time for the shuttle")
	}
}

func TestShortTripsWalk(t *testing.T) {
	origin := point("origin", 41.3000, -72.9300)
	a := point("a", 41.3001, -72.9300)
	b := point("b", 41.3035, -72.9300)
	destination := point("destination", 41.3036, -72.9300)
	quick := ride(Shuttle, "quick", Fare{}, at(a, 1), at(b, 4))
	option := plan(t, Trip{Origin: origin, Destination: destination, DepartAt: noon}, fixedMode{Shuttle, Network{Rides: []Ride{quick}}})
	if got := modes(option); len(got) != 1 || got[0] != Walk {
		t.Fatalf("modes %v", got)
	}
}

func longTrip() (Trip, Ride, Ride) {
	origin := point("origin", 41.3000, -72.9300)
	a := point("a", 41.3003, -72.9300)
	b := point("b", 41.3200, -72.9300)
	destination := point("destination", 41.3203, -72.9300)
	free := ride(Shuttle, "free", Fare{}, at(a, 4), at(b, 12))
	paid := ride(Bus, "paid", Fare{Cents: 175, Group: "cttransit"}, at(a, 3), at(b, 10))
	return Trip{Origin: origin, Destination: destination, DepartAt: noon}, free, paid
}

func TestFreeBeatsPaidInATie(t *testing.T) {
	trip, free, paid := longTrip()
	option := plan(t, trip, fixedMode{Shuttle, Network{Rides: []Ride{free}}}, fixedMode{Bus, Network{Rides: []Ride{paid}}})
	if option.Legs[1].Mode != Shuttle || option.CostCents() != 0 {
		t.Fatalf("modes %v cost %d", modes(option), option.CostCents())
	}
}

func TestPreferencesAreLimits(t *testing.T) {
	trip, free, paid := longTrip()
	trip.Preferences.Avoid = map[ModeName]bool{Shuttle: true}
	option := plan(t, trip, fixedMode{Shuttle, Network{Rides: []Ride{free}}}, fixedMode{Bus, Network{Rides: []Ride{paid}}})
	if option.Legs[1].Mode != Bus {
		t.Fatalf("modes %v", modes(option))
	}
	zero := 0
	trip.Preferences = Preferences{MaxCostCents: &zero}
	option = plan(t, trip, fixedMode{Bus, Network{Rides: []Ride{paid}}})
	if option.CostCents() != 0 || len(option.Legs) != 1 {
		t.Fatalf("free only gave %v", modes(option))
	}
}

func TestBusTransfersShareOneFare(t *testing.T) {
	origin := point("origin", 41.3000, -72.9300)
	a := point("a", 41.3003, -72.9300)
	b := point("b", 41.3300, -72.9300)
	c := point("c", 41.3300, -72.8900)
	destination := point("destination", 41.3303, -72.8900)
	fare := Fare{Cents: 175, Group: "cttransit"}
	option := plan(t, Trip{Origin: origin, Destination: destination, DepartAt: noon}, fixedMode{Bus, Network{Rides: []Ride{
		ride(Bus, "one", fare, at(a, 3), at(b, 12)),
		ride(Bus, "two", fare, at(b, 14), at(c, 22)),
	}}})
	if option.CostCents() != 175 {
		t.Fatalf("cost %d for %v", option.CostCents(), modes(option))
	}
}

func TestRentalNeverWalksAway(t *testing.T) {
	origin := point("origin", 41.3000, -72.9300)
	behind := point("behind", 41.2985, -72.9300)
	destination := point("destination", 41.3200, -72.9300)
	dock := point("dock", 41.3195, -72.9300)
	rental := Rental{Mode: Bike, Pickups: []Pickup{{
		Point:   behind,
		Profile: streets.Profile{Kind: streets.Bicycle, SpeedKmh: 30},
		Cost:    func(time.Duration) int { return 100 },
	}}, Dropoffs: []Point{dock}}
	option := plan(t, Trip{Origin: origin, Destination: destination, DepartAt: noon}, fixedMode{Bike, Network{Rentals: []Rental{rental}}})
	for _, leg := range option.Legs {
		if leg.Mode == Bike {
			t.Fatalf("walked away from the destination to a bike: %v", modes(option))
		}
	}
}

func TestNightWeighsWalkingMore(t *testing.T) {
	c := candidate{walk: 12 * time.Minute}
	if dayPolicy.burden(c) >= nightPolicy.burden(c) {
		t.Fatal("night should weigh walking more")
	}
	if !IsNight(time.Date(2026, 11, 17, 19, 0, 0, 0, newHaven)) || IsNight(noon) {
		t.Fatal("night boundaries")
	}
}

func TestAlternativesOfferAFreeRoute(t *testing.T) {
	trip, free, paid := longTrip()
	paid.Stops[1] = at(paid.Stops[1].Stop, 6)
	free.Stops[0] = at(free.Stops[0].Stop, 5)
	free.Stops[1] = at(free.Stops[1].Stop, 13)
	planner := Planner{Streets: streets.Estimate{}, Modes: []Mode{fixedMode{Shuttle, Network{Rides: []Ride{free}}}, fixedMode{Bus, Network{Rides: []Ride{paid}}}}}
	choice, err := planner.Choose(context.Background(), trip)
	if err != nil {
		t.Fatal(err)
	}
	if choice.Route.CostCents() == 0 {
		t.Fatalf("main %v", modes(choice.Route))
	}
	if len(choice.Alternatives) == 0 || choice.Alternatives[0].Option.CostCents() != 0 {
		t.Fatalf("alternatives %+v", choice.Alternatives)
	}
}

func TestAlternativesIncludeAShuttle(t *testing.T) {
	trip, free, paid := longTrip()
	paid.Stops[1] = at(paid.Stops[1].Stop, 6)
	free.Stops[0] = at(free.Stops[0].Stop, 5)
	free.Stops[1] = at(free.Stops[1].Stop, 13)
	planner := Planner{Streets: streets.Estimate{}, Modes: []Mode{fixedMode{Shuttle, Network{Rides: []Ride{free}}}, fixedMode{Bus, Network{Rides: []Ride{paid}}}}}
	choice, err := planner.Choose(context.Background(), trip)
	if err != nil {
		t.Fatal(err)
	}
	if len(choice.Alternatives) == 0 || choice.Alternatives[0].Kind != ByShuttle || choice.Alternatives[0].Label != "Yale Shuttle · 7 min slower" {
		t.Fatalf("alternatives %+v", choice.Alternatives)
	}
	for _, alternative := range choice.Alternatives[1:] {
		if alternative.Option.CostCents() == 0 && alternative.Kind == Free {
			t.Fatal("the shuttle was offered twice")
		}
	}
}

func TestLongWalksLoseToASlightlySlowerRide(t *testing.T) {
	origin := point("origin", 41.3000, -72.9300)
	a := point("a", 41.3003, -72.9300)
	b := point("b", 41.3240, -72.9300)
	destination := point("destination", 41.3243, -72.9300)
	trip := Trip{Origin: origin, Destination: destination, DepartAt: noon}
	walk := plan(t, trip)
	minutes := int(walk.ArriveAt().Sub(noon).Minutes())
	slow := ride(Bus, "slow", Fare{Cents: 175, Group: "cttransit"}, at(a, minutes-2), at(b, minutes+5))
	option := plan(t, trip, fixedMode{Bus, Network{Rides: []Ride{slow}}})
	if option.Legs[1].Mode != Bus {
		t.Fatalf("a %d minute walk beat a bus 6 minutes slower: %v", minutes, modes(option))
	}
}
