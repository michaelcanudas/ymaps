package place

import (
	"context"
	"errors"
	"log"
	"sort"
	"strings"

	"ymaps/backend/internal/geo"
)

type Source string

const (
	Alias  Source = "alias"
	Yale   Source = "yale"
	Google Source = "google"
)

type Entryway struct {
	Name string
	At   geo.LatLon
}

type Place struct {
	ID       string
	Name     string
	Address  string
	At       geo.LatLon
	Entryway *Entryway
	Source   Source
}

func (p Place) Arrival() geo.LatLon {
	if p.Entryway != nil {
		return p.Entryway.At
	}
	return p.At
}

type Candidate struct {
	Place      Place
	Confidence float64
}

var ErrNotFound = errors.New("place not found")

type Resolver interface {
	Source() Source
	Resolve(ctx context.Context, query string, near geo.LatLon) ([]Candidate, error)
	Lookup(ctx context.Context, id string) (Place, error)
}

const (
	sureConfidence  = 0.8
	clearConfidence = 0.6
	clearMargin     = 0.15
	maxCandidates   = 5
)

type Chain []Resolver

func (c Chain) Resolve(ctx context.Context, query string, near geo.LatLon) (*Place, []Candidate) {
	var all []Candidate
	for _, resolver := range c {
		found, err := resolver.Resolve(ctx, query, near)
		if err != nil {
			log.Printf("resolver %s: %v", resolver.Source(), err)
			continue
		}
		all = append(all, found...)
		if best, ok := decided(found); ok && best.Confidence >= sureConfidence {
			return &best.Place, rank(all, near)
		}
	}
	ranked := rank(all, near)
	if best, ok := decided(ranked); ok {
		return &best.Place, ranked
	}
	return nil, ranked
}

func (c Chain) Lookup(ctx context.Context, id string) (Place, error) {
	prefix, _, _ := strings.Cut(id, ":")
	for _, resolver := range c {
		if string(resolver.Source()) == prefix {
			return resolver.Lookup(ctx, id)
		}
	}
	return Place{}, ErrNotFound
}

func decided(candidates []Candidate) (Candidate, bool) {
	if len(candidates) == 0 {
		return Candidate{}, false
	}
	sorted := append([]Candidate(nil), candidates...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Confidence > sorted[j].Confidence })
	best := sorted[0]
	if best.Confidence < clearConfidence {
		return Candidate{}, false
	}
	for _, other := range sorted[1:] {
		if best.Confidence-other.Confidence >= clearMargin {
			break
		}
		if !same(best.Place, other.Place) {
			return Candidate{}, false
		}
	}
	return best, true
}

const sameMeters = 150.0

func same(a, b Place) bool {
	if a.ID == b.ID {
		return true
	}
	if geo.Distance(a.At, b.At) > sameMeters {
		return false
	}
	x, y := strings.ToLower(a.Name), strings.ToLower(b.Name)
	return x == y || strings.Contains(x, y) || strings.Contains(y, x)
}

func rank(candidates []Candidate, near geo.LatLon) []Candidate {
	var unique []Candidate
	for _, candidate := range candidates {
		duplicate := false
		for i, kept := range unique {
			if same(kept.Place, candidate.Place) {
				duplicate = true
				if candidate.Confidence > kept.Confidence {
					unique[i].Confidence = candidate.Confidence
				}
				break
			}
		}
		if !duplicate {
			unique = append(unique, candidate)
		}
	}
	sort.SliceStable(unique, func(i, j int) bool {
		if unique[i].Confidence != unique[j].Confidence {
			return unique[i].Confidence > unique[j].Confidence
		}
		return geo.Distance(near, unique[i].Place.At) < geo.Distance(near, unique[j].Place.At)
	})
	if len(unique) > maxCandidates {
		unique = unique[:maxCandidates]
	}
	return unique
}
