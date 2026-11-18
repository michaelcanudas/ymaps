package plan

import (
	"fmt"
	"math"
	"strings"
	"time"
	_ "time/tzdata"
)

type policy struct {
	tieWindow       time.Duration
	walkWeight      float64
	longWalkWeight  float64
	waitWeight      float64
	transferMinutes float64
	freeSlack       float64
	equalSlack      float64
}

const (
	longWalk         = 10 * time.Minute
	shortWalk        = 8 * time.Minute
	minimumRideShare = 0.25
)

var (
	dayPolicy = policy{
		tieWindow:       3 * time.Minute,
		walkWeight:      1,
		longWalkWeight:  1,
		waitWeight:      1.5,
		transferMinutes: 3,
		freeSlack:       4,
		equalSlack:      1,
	}
	nightPolicy = policy{
		tieWindow:       5 * time.Minute,
		walkWeight:      1.5,
		longWalkWeight:  1.5,
		waitWeight:      2.5,
		transferMinutes: 3,
		freeSlack:       6,
		equalSlack:      1,
	}
	newHaven = mustLocation("America/New_York")
	dusk     = [12]int{17, 18, 19, 20, 20, 21, 21, 20, 19, 18, 17, 17}
	dawn     = [12]int{7, 7, 7, 6, 6, 5, 6, 6, 7, 7, 7, 7}
)

func mustLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

func IsNight(at time.Time) bool {
	local := at.In(newHaven)
	month := int(local.Month()) - 1
	return local.Hour() >= dusk[month] || local.Hour() < dawn[month]
}

func policyAt(at time.Time) policy {
	if IsNight(at) {
		return nightPolicy
	}
	return dayPolicy
}

type candidate struct {
	option Option
	arrive time.Time
	walk   time.Duration
	wait   time.Duration
	cost   int
	rides  int
	riding time.Duration
	modes  map[ModeName]bool
}

func (c candidate) worthRiding() bool {
	if c.rides == 0 {
		return true
	}
	total := c.arrive.Sub(c.option.DepartAt())
	return total > 0 && c.riding.Seconds() >= minimumRideShare*total.Seconds()
}

func (c candidate) walkOnly() bool {
	return c.rides == 0
}

func (p policy) burden(c candidate) float64 {
	walk := c.walk.Minutes()
	burden := walk*p.walkWeight + math.Max(0, walk-longWalk.Minutes())*p.longWalkWeight + c.wait.Minutes()*p.waitWeight
	if c.rides > 1 {
		burden += float64(c.rides-1) * p.transferMinutes
	}
	return burden
}

func allowed(c candidate, prefs Preferences) bool {
	if prefs.MaxWalk > 0 && c.walk > prefs.MaxWalk {
		return false
	}
	if prefs.MaxCostCents != nil && c.cost > *prefs.MaxCostCents {
		return false
	}
	for mode := range c.modes {
		if prefs.Avoid[mode] {
			return false
		}
	}
	return true
}

func (p policy) choose(candidates []candidate, prefs Preferences) (candidate, bool) {
	var usable []candidate
	for _, c := range candidates {
		if allowed(c, prefs) && c.worthRiding() {
			usable = append(usable, c)
		}
	}
	if len(usable) == 0 {
		return candidate{}, false
	}
	for _, c := range usable {
		if c.walkOnly() && c.walk <= shortWalk {
			return c, true
		}
	}
	earliest := usable[0].arrive
	for _, c := range usable {
		if c.arrive.Before(earliest) {
			earliest = c.arrive
		}
	}
	var tied []candidate
	lightest := math.Inf(1)
	for _, c := range usable {
		if !c.arrive.After(earliest.Add(p.tieWindow)) {
			tied = append(tied, c)
			lightest = math.Min(lightest, p.burden(c))
		}
	}
	if pick, ok := p.lightest(tied, lightest+p.freeSlack, true); ok {
		return pick, true
	}
	return p.lightest(tied, lightest+p.equalSlack, false)
}

func (p policy) lightest(tied []candidate, ceiling float64, freeOnly bool) (candidate, bool) {
	var pick candidate
	found := false
	for _, c := range tied {
		burden := p.burden(c)
		if burden > ceiling || (freeOnly && c.cost > 0) {
			continue
		}
		if !found || p.better(c, pick) {
			pick, found = c, true
		}
	}
	return pick, found
}

func (p policy) better(a, b candidate) bool {
	if a.cost != b.cost {
		return a.cost < b.cost
	}
	if burdenA, burdenB := p.burden(a), p.burden(b); burdenA != burdenB {
		return burdenA < burdenB
	}
	return a.arrive.Before(b.arrive)
}

func (p policy) describe(c candidate, departAt time.Time) string {
	var parts []string
	for _, leg := range c.option.Legs {
		label := string(leg.Mode)
		if leg.Line != nil {
			label += "(" + leg.Line.Name + ")"
		}
		parts = append(parts, fmt.Sprintf("%s %.0fm", label, leg.ArriveAt.Sub(leg.DepartAt).Minutes()))
	}
	return fmt.Sprintf("arrive +%.1fm walk %.1fm wait %.1fm cost %d burden %.1f | %s", c.arrive.Sub(departAt).Minutes(), c.walk.Minutes(), c.wait.Minutes(), c.cost, p.burden(c), strings.Join(parts, ", "))
}
