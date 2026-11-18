package plan

import (
	"fmt"
	"math"
	"time"
)

type Kind string

const (
	Free        Kind = "free"
	Faster      Kind = "faster"
	LessWalking Kind = "lessWalking"
	Cheaper     Kind = "cheaper"
	ByShuttle   Kind = "shuttle"
)

type Alternative struct {
	Option Option
	Kind   Kind
	Label  string
}

type Choice struct {
	Route        Option
	Alternatives []Alternative
}

const (
	alternativesKept     = 2
	fasterBy             = 3 * time.Minute
	lessWalkingBy        = 5 * time.Minute
	cheaperBy            = 100
	cheaperSlowerLimit   = 8 * time.Minute
	alternativeLateLimit = 15 * time.Minute
)

func (p policy) alternatives(main candidate, candidates []candidate, prefs Preferences) []Alternative {
	var picked []Alternative
	var pickedCandidates []candidate
	if shuttle, ok := p.shuttleAlternative(main, candidates, prefs); ok {
		picked = append(picked, Alternative{Option: shuttle.option, Kind: ByShuttle, Label: describeTradeOff(ByShuttle, shuttle, main)})
		pickedCandidates = append(pickedCandidates, shuttle)
	}
	var usable []candidate
	for _, c := range candidates {
		if c.option.ArriveAt().Equal(main.option.ArriveAt()) && sameModes(c, main) && c.cost == main.cost {
			continue
		}
		if !allowed(c, prefs) || !c.worthRiding() || c.arrive.After(main.arrive.Add(alternativeLateLimit)) {
			continue
		}
		if !c.arrive.Before(main.arrive) && c.walk >= main.walk && c.cost >= main.cost {
			continue
		}
		if !meaningfullyDifferent(c, main) {
			continue
		}
		usable = append(usable, c)
	}
	offer := func(kind Kind, matches func(candidate) bool, better func(a, b candidate) bool) {
		if len(picked) == alternativesKept {
			return
		}
		var best *candidate
		for i := range usable {
			c := usable[i]
			if !matches(c) {
				continue
			}
			duplicate := false
			for _, other := range pickedCandidates {
				duplicate = duplicate || !meaningfullyDifferent(c, other)
			}
			if duplicate {
				continue
			}
			if best == nil || better(c, *best) {
				best = &usable[i]
			}
		}
		if best != nil {
			picked = append(picked, Alternative{Option: best.option, Kind: kind, Label: describeTradeOff(kind, *best, main)})
			pickedCandidates = append(pickedCandidates, *best)
		}
	}
	earlier := func(a, b candidate) bool { return a.arrive.Before(b.arrive) }
	lighter := func(a, b candidate) bool { return p.burden(a) < p.burden(b) }
	cheaper := func(a, b candidate) bool { return a.cost < b.cost || a.cost == b.cost && a.arrive.Before(b.arrive) }
	if main.cost > 0 {
		offer(Free, func(c candidate) bool { return c.cost == 0 }, earlier)
	}
	offer(Faster, func(c candidate) bool { return !c.arrive.After(main.arrive.Add(-fasterBy)) }, earlier)
	offer(LessWalking, func(c candidate) bool { return c.walk <= main.walk-lessWalkingBy }, lighter)
	offer(Cheaper, func(c candidate) bool {
		return c.cost > 0 && c.cost <= main.cost-cheaperBy && !c.arrive.After(main.arrive.Add(cheaperSlowerLimit))
	}, cheaper)
	return picked
}

func (p policy) shuttleAlternative(main candidate, candidates []candidate, prefs Preferences) (candidate, bool) {
	if main.modes[Shuttle] {
		return candidate{}, false
	}
	var best *candidate
	for i := range candidates {
		c := candidates[i]
		if !c.modes[Shuttle] || len(c.modes) != 1 || !allowed(c, prefs) || !c.worthRiding() || c.arrive.After(main.arrive.Add(alternativeLateLimit)) {
			continue
		}
		if best == nil || c.arrive.Before(best.arrive) || c.arrive.Equal(best.arrive) && p.burden(c) < p.burden(*best) {
			best = &candidates[i]
		}
	}
	if best == nil {
		return candidate{}, false
	}
	return *best, true
}

func sameModes(a, b candidate) bool {
	if len(a.modes) != len(b.modes) {
		return false
	}
	for mode := range a.modes {
		if !b.modes[mode] {
			return false
		}
	}
	return true
}

func meaningfullyDifferent(a, b candidate) bool {
	if !sameModes(a, b) {
		return true
	}
	return absDuration(a.arrive.Sub(b.arrive)) >= fasterBy ||
		int(math.Abs(float64(a.cost-b.cost))) >= cheaperBy ||
		absDuration(a.walk-b.walk) >= lessWalkingBy
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func money(cents int) string {
	if cents == 0 {
		return "Free"
	}
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func timing(c, main candidate) string {
	minutes := int(math.Round(c.arrive.Sub(main.arrive).Minutes()))
	switch {
	case minutes > 0:
		return fmt.Sprintf("%d min slower", minutes)
	case minutes < 0:
		return fmt.Sprintf("%d min faster", -minutes)
	default:
		return "Same time"
	}
}

func describeTradeOff(kind Kind, c, main candidate) string {
	switch kind {
	case ByShuttle:
		return "Yale Shuttle · " + timing(c, main)
	case Free:
		return "Free · " + timing(c, main)
	case Faster:
		return fmt.Sprintf("%d min faster · %s", int(math.Round(main.arrive.Sub(c.arrive).Minutes())), money(c.cost))
	case LessWalking:
		walked := int(math.Round((main.walk - c.walk).Minutes()))
		return fmt.Sprintf("%d min less walking · %s", walked, money(c.cost))
	default:
		return money(c.cost) + " · " + timing(c, main)
	}
}
