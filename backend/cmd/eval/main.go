package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed trips.json
var tripsJSON []byte

type trip struct {
	Name        string     `json:"name"`
	Origin      [2]float64 `json:"origin"`
	Destination string     `json:"destination"`
	Place       string     `json:"place"`
	Expect      []string   `json:"expect"`
}

type coordinate struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type point struct {
	Name       string     `json:"name"`
	Coordinate coordinate `json:"coordinate"`
}

type leg struct {
	Mode     string    `json:"mode"`
	From     point     `json:"from"`
	To       point     `json:"to"`
	DepartAt time.Time `json:"departAt"`
	ArriveAt time.Time `json:"arriveAt"`
	Cost     int       `json:"costCents"`
	Line     *struct {
		Name string `json:"name"`
	} `json:"line"`
}

type route struct {
	Place struct {
		Name       string     `json:"name"`
		Coordinate coordinate `json:"coordinate"`
		Entryway   *struct {
			Coordinate coordinate `json:"coordinate"`
		} `json:"entryway"`
	} `json:"place"`
	ComputedAt time.Time `json:"computedAt"`
	DepartAt   time.Time `json:"departAt"`
	ArriveAt   time.Time `json:"arriveAt"`
	Cost       int       `json:"costCents"`
	Legs       []leg     `json:"legs"`
}

type result struct {
	Trip     string   `json:"trip"`
	Summary  string   `json:"summary"`
	Failures []string `json:"failures"`
}

const (
	walkMetersPerSecond = 1.34
	walkDetour          = 1.3
	shortTripMeters     = 450.0
	stopReachMeters     = 450.0
)

func main() {
	base := flag.String("url", "http://localhost:8090", "backend base URL")
	only := flag.String("only", "", "run trips whose name contains this")
	output := flag.String("out", "", "write results as JSON to this file")
	flag.Parse()
	var trips []trip
	if err := json.Unmarshal(tripsJSON, &trips); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	shuttle := loadShuttle()
	results := make([]result, len(trips))
	var group sync.WaitGroup
	limit := make(chan struct{}, 4)
	for i, t := range trips {
		if *only != "" && !strings.Contains(t.Name, *only) {
			continue
		}
		group.Add(1)
		go func(i int, t trip) {
			defer group.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			results[i] = evaluate(*base, t, shuttle)
		}(i, t)
	}
	group.Wait()
	passed, ran := 0, 0
	var kept []result
	for _, r := range results {
		if r.Trip == "" {
			continue
		}
		ran++
		kept = append(kept, r)
		status := "PASS"
		if len(r.Failures) == 0 {
			passed++
		} else {
			status = "FAIL"
		}
		fmt.Printf("%s %-32s %s\n", status, r.Trip, r.Summary)
		for _, f := range r.Failures {
			fmt.Printf("     - %s\n", f)
		}
	}
	fmt.Printf("\n%d/%d passed at %s\n", passed, ran, time.Now().Format("Mon 15:04"))
	if *output != "" {
		data, _ := json.MarshalIndent(kept, "", "  ")
		os.WriteFile(*output, data, 0o644)
	}
	if passed != ran {
		os.Exit(1)
	}
}

func evaluate(base string, t trip, shuttle *shuttleState) result {
	r := result{Trip: t.Name}
	fail := func(format string, args ...any) { r.Failures = append(r.Failures, fmt.Sprintf(format, args...)) }
	origin := coordinate{Lat: t.Origin[0], Lon: t.Origin[1]}
	body, _ := json.Marshal(map[string]any{"origin": origin, "destination": t.Destination})
	response, err := (&http.Client{Timeout: 30 * time.Second}).Post(base+"/v1/routes", "application/json", bytes.NewReader(body))
	if err != nil {
		fail("request: %v", err)
		return r
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var unresolved struct {
			Places []struct {
				Name string `json:"name"`
			} `json:"places"`
		}
		json.NewDecoder(response.Body).Decode(&unresolved)
		var names []string
		for _, p := range unresolved.Places {
			names = append(names, p.Name)
		}
		r.Summary = fmt.Sprintf("status %d %v", response.StatusCode, names)
		fail("status %d", response.StatusCode)
		return r
	}
	var got route
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		fail("decode: %v", err)
		return r
	}
	destination := got.Place.Coordinate
	if got.Place.Entryway != nil {
		destination = got.Place.Entryway.Coordinate
	}
	r.Summary = summarize(got)
	if t.Place != "" && !strings.Contains(strings.ToLower(got.Place.Name), strings.ToLower(t.Place)) {
		fail("resolved to %q, want %q", got.Place.Name, t.Place)
	}
	straight := distance(origin, destination)
	walkOnly := walkEstimate(straight)
	modes := map[string]bool{}
	walking := time.Duration(0)
	for i, l := range got.Legs {
		modes[l.Mode] = true
		if l.ArriveAt.Before(l.DepartAt) {
			fail("leg %d arrives before it departs", i)
		}
		if i > 0 {
			previous := got.Legs[i-1]
			if gap := distance(previous.To.Coordinate, l.From.Coordinate); gap > 30 {
				fail("legs %d and %d are %.0f m apart", i-1, i, gap)
			}
			if l.DepartAt.Before(previous.ArriveAt.Add(-time.Second)) {
				fail("leg %d departs before leg %d arrives", i, i-1)
			}
			if l.Mode == "walk" && previous.Mode == "walk" {
				fail("two walk legs in a row")
			}
		}
		if l.Mode == "walk" {
			walking += l.ArriveAt.Sub(l.DepartAt)
		}
		if l.Mode == "bike" || l.Mode == "veo" {
			pickupToDestination := distance(l.From.Coordinate, destination)
			start := origin
			if i > 0 {
				start = got.Legs[i-1].From.Coordinate
			}
			if pickupToDestination > distance(start, destination)+75 {
				fail("walks away from the destination to reach a %s (%.0f m farther)", l.Mode, pickupToDestination-distance(start, destination))
			}
			if i > 0 && got.Legs[i-1].Mode == "walk" {
				if access := got.Legs[i-1].ArriveAt.Sub(got.Legs[i-1].DepartAt); access > 6*time.Minute {
					fail("walks %s to reach a %s", access.Round(time.Second), l.Mode)
				}
			}
			if ride := l.ArriveAt.Sub(l.DepartAt); ride < 2*time.Minute {
				fail("%s ride of only %s", l.Mode, ride.Round(time.Second))
			}
		}
	}
	walkOnlyRoute := len(got.Legs) == 1 && got.Legs[0].Mode == "walk"
	riding := time.Duration(0)
	for _, l := range got.Legs {
		if l.Mode != "walk" {
			riding += l.ArriveAt.Sub(l.DepartAt)
		}
	}
	if total := got.ArriveAt.Sub(got.DepartAt); !walkOnlyRoute && riding < total/4 {
		fail("rides only %s of a %s trip", riding.Round(time.Second), total.Round(time.Second))
	}
	if straight <= shortTripMeters && !walkOnlyRoute {
		fail("trip is %.0f m but the route is not a walk", straight)
	}
	if contains(t.Expect, "walk") && !walkOnlyRoute {
		fail("expected a walk")
	}
	if !walkOnlyRoute && walking > walkOnly+3*time.Minute {
		fail("walks %s, more than walking there (%s)", walking.Round(time.Second), walkOnly.Round(time.Second))
	}
	if limit := got.ComputedAt.Add(walkOnly + 6*time.Minute); got.ArriveAt.After(limit) && !walkOnlyRoute {
		fail("arrives %s after just walking would", got.ArriveAt.Sub(got.ComputedAt.Add(walkOnly)).Round(time.Second))
	}
	if contains(t.Expect, "shuttle_if_obvious") && !modes["shuttle"] {
		if reason, obvious := shuttle.obvious(origin, destination, walkOnly); obvious {
			fail("shuttle is the obvious choice: %s", reason)
		}
	}
	return r
}

func summarize(r route) string {
	var parts []string
	for _, l := range r.Legs {
		minutes := int(math.Round(l.ArriveAt.Sub(l.DepartAt).Minutes()))
		label := l.Mode
		if l.Line != nil {
			label += "(" + l.Line.Name + ")"
		}
		parts = append(parts, fmt.Sprintf("%s %dm", label, minutes))
	}
	total := int(math.Round(r.ArriveAt.Sub(r.ComputedAt).Minutes()))
	return fmt.Sprintf("%dm $%.2f → %s | %s", total, float64(r.Cost)/100, r.Place.Name, strings.Join(parts, ", "))
}

func walkEstimate(meters float64) time.Duration {
	return time.Duration(meters * walkDetour / walkMetersPerSecond * float64(time.Second))
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func distance(a, b coordinate) float64 {
	rad := math.Pi / 180
	dLat := (b.Lat - a.Lat) * rad
	dLon := (b.Lon - a.Lon) * rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * 6371008.8 * math.Asin(math.Min(1, math.Sqrt(h)))
}

type shuttleStop struct {
	ID   int     `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

type shuttleRoute struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
	Stops  []int  `json:"stops"`
}

type shuttleBus struct {
	ID         int   `json:"id"`
	Route      int   `json:"route"`
	LastUpdate int64 `json:"lastUpdate"`
}

type shuttleState struct {
	mutex  sync.Mutex
	stops  map[int]shuttleStop
	routes []shuttleRoute
	buses  []shuttleBus
	etas   map[int]map[int]time.Time
}

const downtowner = "https://yale.downtownerapp.com"

func getJSON(url string, into any) error {
	response, err := (&http.Client{Timeout: 15 * time.Second}).Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return json.NewDecoder(response.Body).Decode(into)
}

func loadShuttle() *shuttleState {
	s := &shuttleState{stops: map[int]shuttleStop{}, etas: map[int]map[int]time.Time{}}
	var stops []shuttleStop
	getJSON(downtowner+"/routes_stops.php", &stops)
	for _, stop := range stops {
		s.stops[stop.ID] = stop
	}
	getJSON(downtowner+"/routes_routes.php", &s.routes)
	getJSON(downtowner+"/routes_buses.php", &s.buses)
	return s
}

func (s *shuttleState) etasAt(stop int) map[int]time.Time {
	s.mutex.Lock()
	if found, ok := s.etas[stop]; ok {
		s.mutex.Unlock()
		return found
	}
	s.mutex.Unlock()
	var response struct {
		Etas map[string]struct {
			Etas []struct {
				Minutes int `json:"avg"`
				BusID   int `json:"bus_id"`
			} `json:"etas"`
		} `json:"etas"`
		CalculationTime int64 `json:"calculation_time"`
	}
	byBus := map[int]time.Time{}
	if getJSON(downtowner+"/routes_eta.php?stop="+strconv.Itoa(stop), &response) == nil {
		at := time.Unix(response.CalculationTime, 0)
		for _, e := range response.Etas[strconv.Itoa(stop)].Etas {
			byBus[e.BusID] = at.Add(time.Duration(e.Minutes) * time.Minute)
		}
	}
	s.mutex.Lock()
	s.etas[stop] = byBus
	s.mutex.Unlock()
	return byBus
}

func (s *shuttleState) obvious(origin, destination coordinate, walkOnly time.Duration) (string, bool) {
	if walkOnly < 9*time.Minute {
		return "", false
	}
	now := time.Now()
	for _, r := range s.routes {
		if !r.Active {
			continue
		}
		var buses []int
		for _, b := range s.buses {
			if b.Route == r.ID && now.Sub(time.Unix(b.LastUpdate, 0)) < 2*time.Minute {
				buses = append(buses, b.ID)
			}
		}
		if len(buses) == 0 {
			continue
		}
		n := len(r.Stops)
		for i, boardID := range r.Stops {
			board, ok := s.stops[boardID]
			if !ok {
				continue
			}
			boardAt := coordinate{Lat: board.Lat, Lon: board.Lon}
			reach := distance(origin, boardAt)
			if reach > stopReachMeters {
				continue
			}
			for hop := 1; hop <= n*6/10; hop++ {
				alight, ok := s.stops[r.Stops[(i+hop)%n]]
				if !ok {
					continue
				}
				alightAt := coordinate{Lat: alight.Lat, Lon: alight.Lon}
				egressMeters := distance(alightAt, destination)
				if egressMeters > stopReachMeters {
					continue
				}
				access := walkEstimate(reach)
				egress := walkEstimate(egressMeters)
				boardETAs := s.etasAt(board.ID)
				alightETAs := s.etasAt(alight.ID)
				for _, bus := range buses {
					leaves, ok := boardETAs[bus]
					if !ok || leaves.Before(now.Add(access)) || leaves.After(now.Add(access+8*time.Minute)) {
						continue
					}
					arrives, ok := alightETAs[bus]
					if !ok || !arrives.After(leaves) {
						continue
					}
					total := arrives.Add(egress).Sub(now)
					if total <= walkOnly-3*time.Minute {
						return fmt.Sprintf("%s from %s to %s in %s vs walking %s", r.Name, board.Name, alight.Name, total.Round(time.Minute), walkOnly.Round(time.Minute)), true
					}
				}
			}
		}
	}
	return "", false
}
