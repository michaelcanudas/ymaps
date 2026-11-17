package alias

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/place"
)

//go:embed places.json
var seed []byte

const exactConfidence = 0.95

type entry struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Address  string        `json:"address"`
	Lat      float64       `json:"lat"`
	Lon      float64       `json:"lon"`
	Terms    []string      `json:"terms"`
	Entryway *entrywayJSON `json:"entryway"`
}

type entrywayJSON struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

type Resolver struct {
	byTerm map[string]place.Place
	byID   map[string]place.Place
}

func New() *Resolver {
	resolver, err := parse(seed)
	if err != nil {
		panic(err)
	}
	return resolver
}

func parse(data []byte) (*Resolver, error) {
	var entries []entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("alias seed: %w", err)
	}
	resolver := &Resolver{byTerm: map[string]place.Place{}, byID: map[string]place.Place{}}
	for _, e := range entries {
		p := place.Place{
			ID:      string(place.Alias) + ":" + e.ID,
			Name:    e.Name,
			Address: e.Address,
			At:      geo.LatLon{Lat: e.Lat, Lon: e.Lon},
			Source:  place.Alias,
		}
		if e.Entryway != nil {
			p.Entryway = &place.Entryway{Name: e.Entryway.Name, At: geo.LatLon{Lat: e.Entryway.Lat, Lon: e.Entryway.Lon}}
		}
		if _, dup := resolver.byID[p.ID]; dup {
			return nil, fmt.Errorf("alias seed: duplicate id %s", e.ID)
		}
		resolver.byID[p.ID] = p
		for _, term := range append([]string{e.Name}, e.Terms...) {
			key := normalize(term)
			if existing, dup := resolver.byTerm[key]; dup && existing.ID != p.ID {
				return nil, fmt.Errorf("alias seed: term %q used by %s and %s", term, existing.ID, p.ID)
			}
			resolver.byTerm[key] = p
		}
	}
	return resolver, nil
}

func normalize(text string) string {
	text = strings.NewReplacer("'", "", "’", "").Replace(strings.ToLower(text))
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) > 1 && words[0] == "the" {
		words = words[1:]
	}
	return strings.Join(words, " ")
}

func (r *Resolver) Source() place.Source {
	return place.Alias
}

func (r *Resolver) Resolve(_ context.Context, query string, _ geo.LatLon) ([]place.Candidate, error) {
	p, ok := r.byTerm[normalize(query)]
	if !ok {
		return nil, nil
	}
	return []place.Candidate{{Place: p, Confidence: exactConfidence}}, nil
}

func (r *Resolver) Lookup(_ context.Context, id string) (place.Place, error) {
	p, ok := r.byID[id]
	if !ok {
		return place.Place{}, place.ErrNotFound
	}
	return p, nil
}
