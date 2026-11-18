package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/place"
	"ymaps/backend/internal/plan"
)

type Places interface {
	Resolve(ctx context.Context, query string, near geo.LatLon) (*place.Place, []place.Candidate)
	Lookup(ctx context.Context, id string) (place.Place, error)
}

type Planner interface {
	Choose(ctx context.Context, trip plan.Trip) (plan.Choice, error)
}

type Routes struct {
	Places  Places
	Planner Planner
	Debug   http.Handler
	Now     func() time.Time
	store   routeStore
}

const routeLifetime = 3 * time.Hour

type storedRoute struct {
	place       place.Place
	preferences plan.Preferences
	kind        plan.Kind
	expires     time.Time
}

type routeStore struct {
	mutex  sync.Mutex
	routes map[string]storedRoute
}

func (s *routeStore) put(id string, entry storedRoute) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.routes == nil {
		s.routes = map[string]storedRoute{}
	}
	s.routes[id] = entry
}

func (s *routeStore) get(id string, now time.Time) (storedRoute, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	entry, found := s.routes[id]
	if !found {
		return storedRoute{}, false
	}
	if now.After(entry.expires) {
		delete(s.routes, id)
		return storedRoute{}, false
	}
	return entry, true
}

type CoordinateBody struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type PreferencesBody struct {
	MaxWalkMinutes *int     `json:"maxWalkMinutes,omitempty"`
	MaxCostCents   *int     `json:"maxCostCents,omitempty"`
	AvoidModes     []string `json:"avoidModes,omitempty"`
}

type RouteRequestBody struct {
	Origin      *CoordinateBody  `json:"origin"`
	Destination string           `json:"destination"`
	PlaceID     string           `json:"placeId,omitempty"`
	Preferences *PreferencesBody `json:"preferences,omitempty"`
}

type EntrywayBody struct {
	Name       string         `json:"name"`
	Coordinate CoordinateBody `json:"coordinate"`
}

type PlaceBody struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Address    string         `json:"address,omitempty"`
	Coordinate CoordinateBody `json:"coordinate"`
	Entryway   *EntrywayBody  `json:"entryway,omitempty"`
	Source     string         `json:"source"`
}

type UnresolvedBody struct {
	Destination string      `json:"destination"`
	Places      []PlaceBody `json:"places"`
}

type PointBody struct {
	ID         string         `json:"id,omitempty"`
	Name       string         `json:"name,omitempty"`
	Coordinate CoordinateBody `json:"coordinate"`
}

type LineBody struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Path  string `json:"path"`
}

type VehicleBody struct {
	ID         string         `json:"id"`
	Name       string         `json:"name,omitempty"`
	Coordinate CoordinateBody `json:"coordinate"`
	Heading    *int           `json:"heading,omitempty"`
}

type LegBody struct {
	Mode           string       `json:"mode"`
	From           PointBody    `json:"from"`
	To             PointBody    `json:"to"`
	DepartAt       time.Time    `json:"departAt"`
	ArriveAt       time.Time    `json:"arriveAt"`
	CostCents      int          `json:"costCents"`
	Path           string       `json:"path"`
	Line           *LineBody    `json:"line,omitempty"`
	Headsign       *string      `json:"headsign,omitempty"`
	StopCount      *int         `json:"stopCount,omitempty"`
	Vehicle        *VehicleBody `json:"vehicle,omitempty"`
	BikesAvailable *int         `json:"bikesAvailable,omitempty"`
	Form           string       `json:"form,omitempty"`
	RentalURI      string       `json:"rentalUri,omitempty"`
}

type RouteBody struct {
	ID           string      `json:"id"`
	Place        PlaceBody   `json:"place"`
	ComputedAt   time.Time   `json:"computedAt"`
	DepartAt     time.Time   `json:"departAt"`
	ArriveAt     time.Time   `json:"arriveAt"`
	CostCents    int         `json:"costCents"`
	Legs         []LegBody   `json:"legs"`
	Label        string      `json:"label,omitempty"`
	Kind         string      `json:"kind,omitempty"`
	Alternatives []RouteBody `json:"alternatives"`
}

type ErrorBody struct {
	Message string `json:"message"`
}

func (r *Routes) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Routes) plan(w http.ResponseWriter, request *http.Request) {
	var body RouteRequestBody
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorBody{Message: "malformed body"})
		return
	}
	if body.Origin == nil || strings.TrimSpace(body.Destination) == "" {
		writeJSON(w, http.StatusBadRequest, ErrorBody{Message: "origin and destination are required"})
		return
	}
	preferences, err := parsePreferences(body.Preferences)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorBody{Message: err.Error()})
		return
	}
	origin := geo.LatLon{Lat: body.Origin.Lat, Lon: body.Origin.Lon}
	ctx := request.Context()
	var resolved place.Place
	if body.PlaceID != "" {
		found, err := r.Places.Lookup(ctx, body.PlaceID)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, UnresolvedBody{Destination: body.Destination, Places: []PlaceBody{}})
			return
		}
		resolved = found
	} else {
		best, candidates := r.Places.Resolve(ctx, body.Destination, origin)
		if best == nil {
			places := make([]PlaceBody, 0, len(candidates))
			for _, candidate := range candidates {
				places = append(places, placeBody(candidate.Place))
			}
			writeJSON(w, http.StatusUnprocessableEntity, UnresolvedBody{Destination: body.Destination, Places: places})
			return
		}
		resolved = *best
	}
	id := newRouteID()
	route, err := r.route(ctx, id, origin, storedRoute{place: resolved, preferences: preferences})
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, UnresolvedBody{Destination: body.Destination, Places: []PlaceBody{}})
		return
	}
	writeJSON(w, http.StatusOK, route)
}

func (r *Routes) refresh(w http.ResponseWriter, request *http.Request) {
	id := request.PathValue("routeId")
	lat, latErr := strconv.ParseFloat(request.URL.Query().Get("lat"), 64)
	lon, lonErr := strconv.ParseFloat(request.URL.Query().Get("lon"), 64)
	if latErr != nil || lonErr != nil {
		writeJSON(w, http.StatusBadRequest, ErrorBody{Message: "lat and lon are required"})
		return
	}
	entry, found := r.store.get(id, r.now())
	if !found {
		writeJSON(w, http.StatusNotFound, ErrorBody{Message: "route not found"})
		return
	}
	route, err := r.route(request.Context(), id, geo.LatLon{Lat: lat, Lon: lon}, entry)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorBody{Message: "no route from here"})
		return
	}
	writeJSON(w, http.StatusOK, route)
}

func (r *Routes) route(ctx context.Context, id string, origin geo.LatLon, entry storedRoute) (RouteBody, error) {
	now := r.now()
	destination := entry.place
	trip := plan.Trip{
		Origin:      plan.Point{Name: "Current location", At: origin},
		Destination: plan.Point{ID: destination.ID, Name: destinationName(destination), At: destination.Arrival()},
		DepartAt:    now,
		Preferences: entry.preferences,
	}
	choice, err := r.Planner.Choose(ctx, trip)
	if err != nil {
		return RouteBody{}, err
	}
	if len(choice.Route.Legs) == 0 {
		return RouteBody{}, errors.New("empty route")
	}
	expires := now.Add(routeLifetime)
	r.store.put(id, storedRoute{place: destination, preferences: entry.preferences, kind: entry.kind, expires: expires})
	if entry.kind != "" {
		for _, alternative := range choice.Alternatives {
			if alternative.Kind == entry.kind {
				body := routeBody(id, destination, now, alternative.Option)
				body.Kind, body.Label = string(alternative.Kind), alternative.Label
				return body, nil
			}
		}
	}
	body := routeBody(id, destination, now, choice.Route)
	for _, alternative := range choice.Alternatives {
		alternativeID := newRouteID()
		r.store.put(alternativeID, storedRoute{place: destination, preferences: entry.preferences, kind: alternative.Kind, expires: expires})
		extra := routeBody(alternativeID, destination, now, alternative.Option)
		extra.Kind, extra.Label = string(alternative.Kind), alternative.Label
		body.Alternatives = append(body.Alternatives, extra)
	}
	return body, nil
}

func routeBody(id string, destination place.Place, now time.Time, option plan.Option) RouteBody {
	legs := make([]LegBody, 0, len(option.Legs))
	for _, leg := range option.Legs {
		legs = append(legs, legBody(leg))
	}
	return RouteBody{
		ID:           id,
		Place:        placeBody(destination),
		ComputedAt:   now.UTC(),
		DepartAt:     option.DepartAt().UTC(),
		ArriveAt:     option.ArriveAt().UTC(),
		CostCents:    option.CostCents(),
		Legs:         legs,
		Alternatives: []RouteBody{},
	}
}

var avoidable = map[string]plan.ModeName{
	string(plan.Shuttle): plan.Shuttle,
	string(plan.Bus):     plan.Bus,
	string(plan.Bike):    plan.Bike,
	string(plan.Veo):     plan.Veo,
}

func parsePreferences(body *PreferencesBody) (plan.Preferences, error) {
	var preferences plan.Preferences
	if body == nil {
		return preferences, nil
	}
	if body.MaxWalkMinutes != nil {
		if *body.MaxWalkMinutes < 1 {
			return preferences, errors.New("maxWalkMinutes must be at least 1")
		}
		preferences.MaxWalk = time.Duration(*body.MaxWalkMinutes) * time.Minute
	}
	if body.MaxCostCents != nil {
		if *body.MaxCostCents < 0 {
			return preferences, errors.New("maxCostCents must not be negative")
		}
		limit := *body.MaxCostCents
		preferences.MaxCostCents = &limit
	}
	for _, name := range body.AvoidModes {
		mode, ok := avoidable[name]
		if !ok {
			return preferences, errors.New("unknown mode in avoidModes: " + name)
		}
		if preferences.Avoid == nil {
			preferences.Avoid = map[plan.ModeName]bool{}
		}
		preferences.Avoid[mode] = true
	}
	return preferences, nil
}

func destinationName(p place.Place) string {
	if p.Entryway != nil {
		return p.Entryway.Name
	}
	return p.Name
}

func legBody(leg plan.Leg) LegBody {
	body := LegBody{
		Mode:      string(leg.Mode),
		From:      pointBody(leg.From),
		To:        pointBody(leg.To),
		DepartAt:  leg.DepartAt.UTC(),
		ArriveAt:  leg.ArriveAt.UTC(),
		CostCents: leg.CostCents,
		Path:      geo.Encode(leg.Path),
	}
	if leg.Vehicle != nil {
		body.Vehicle = &VehicleBody{ID: leg.Vehicle.ID, Name: leg.Vehicle.Name, Coordinate: coordinate(leg.Vehicle.At), Heading: leg.Vehicle.Heading}
	}
	switch leg.Mode {
	case plan.Shuttle, plan.Bus:
		if leg.Line != nil {
			body.Line = &LineBody{ID: leg.Line.ID, Name: leg.Line.Name, Color: leg.Line.Color, Path: geo.Encode(leg.Line.Path)}
		}
		stopCount := leg.StopCount
		body.StopCount = &stopCount
		if leg.Mode == plan.Bus {
			headsign := leg.Headsign
			body.Headsign = &headsign
		}
	case plan.Bike:
		body.BikesAvailable = leg.BikesAvailable
	case plan.Veo:
		body.Form = leg.Form
		body.RentalURI = leg.RentalURI
	}
	return body
}

func pointBody(p plan.Point) PointBody {
	return PointBody{ID: p.ID, Name: p.Name, Coordinate: coordinate(p.At)}
}

func placeBody(p place.Place) PlaceBody {
	body := PlaceBody{ID: p.ID, Name: p.Name, Address: p.Address, Coordinate: coordinate(p.At), Source: string(p.Source)}
	if p.Entryway != nil {
		body.Entryway = &EntrywayBody{Name: p.Entryway.Name, Coordinate: coordinate(p.Entryway.At)}
	}
	return body
}

func coordinate(at geo.LatLon) CoordinateBody {
	return CoordinateBody{Lat: at.Lat, Lon: at.Lon}
}

func newRouteID() string {
	bytes := make([]byte, 12)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
