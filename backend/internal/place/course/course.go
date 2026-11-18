package course

import (
	"context"
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/place"
)

//go:embed courses.json
var seed []byte

const (
	courseConfidence  = 0.92
	subjectConfidence = 0.7
	buildingMinimum   = 0.85
)

type Yale interface {
	Resolve(ctx context.Context, query string, near geo.LatLon) ([]place.Candidate, error)
	Lookup(ctx context.Context, id string) (place.Place, error)
}

type meeting struct {
	MapID    string `json:"m"`
	Building string `json:"b"`
	Room     string `json:"r"`
}

type table struct {
	Courses  map[string]meeting `json:"courses"`
	Subjects map[string]string  `json:"subjects"`
}

type Resolver struct {
	yale  Yale
	table table
}

func New(yale Yale) *Resolver {
	r := &Resolver{yale: yale}
	if err := json.Unmarshal(seed, &r.table); err != nil {
		panic(err)
	}
	return r
}

func (r *Resolver) Source() place.Source {
	return place.Yale
}

var courseCode = regexp.MustCompile(`^([A-Za-z&]{2,5})\s*-?\s*(\d{3,4})([A-Za-z]?)$`)

func codes(query string) (string, []string, bool) {
	match := courseCode.FindStringSubmatch(strings.TrimSpace(query))
	if match == nil {
		return "", nil, false
	}
	subject := strings.ToUpper(match[1])
	number := match[2]
	suffix := strings.ToUpper(match[3])
	found := []string{subject + " " + number + suffix}
	switch {
	case len(number) == 4 && strings.HasSuffix(number, "0"):
		found = append(found, subject+" "+number[:3]+suffix)
	case len(number) == 3:
		found = append(found, subject+" "+number+"0"+suffix)
	}
	return subject, found, true
}

func (r *Resolver) Resolve(ctx context.Context, query string, near geo.LatLon) ([]place.Candidate, error) {
	subject, candidates, ok := codes(query)
	if !ok {
		return nil, nil
	}
	for _, code := range candidates {
		if m, ok := r.table.Courses[code]; ok {
			if p, ok := r.meetingPlace(ctx, m, near); ok {
				return []place.Candidate{{Place: p, Confidence: courseConfidence}}, nil
			}
		}
	}
	if building, ok := r.table.Subjects[subject]; ok {
		if p, ok := r.meetingPlace(ctx, meeting{MapID: numeric(building), Building: building}, near); ok {
			return []place.Candidate{{Place: p, Confidence: subjectConfidence}}, nil
		}
	}
	return nil, nil
}

func numeric(value string) string {
	for _, r := range value {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return value
}

func (r *Resolver) meetingPlace(ctx context.Context, m meeting, near geo.LatLon) (place.Place, bool) {
	if m.MapID != "" {
		if p, err := r.yale.Lookup(ctx, string(place.Yale)+":"+m.MapID); err == nil {
			return p, true
		}
	}
	if m.Building == "" || numeric(m.Building) != "" {
		return place.Place{}, false
	}
	found, err := r.yale.Resolve(ctx, m.Building, near)
	if err != nil || len(found) == 0 || found[0].Confidence < buildingMinimum {
		return place.Place{}, false
	}
	return found[0].Place, true
}

func (r *Resolver) Lookup(ctx context.Context, id string) (place.Place, error) {
	return r.yale.Lookup(ctx, id)
}
