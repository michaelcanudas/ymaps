package shuttle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ymaps/backend/internal/geo"
)

const downtownerURL = "https://yale.downtownerapp.com"

type route struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	ShortName string    `json:"short_name"`
	Color     string    `json:"color"`
	Active    bool      `json:"active"`
	Stops     []int     `json:"stops"`
	Path      []float64 `json:"path"`
}

type stop struct {
	ID   int     `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

func (s stop) at() geo.LatLon {
	return geo.LatLon{Lat: s.Lat, Lon: s.Lon}
}

type bus struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Heading    int     `json:"heading"`
	Route      int     `json:"route"`
	LastStop   int     `json:"lastStop"`
	LastUpdate int64   `json:"lastUpdate"`
}

func (b bus) updatedAt() time.Time {
	return time.Unix(b.LastUpdate, 0)
}

type eta struct {
	Minutes int `json:"avg"`
	BusID   int `json:"bus_id"`
	Route   int `json:"route"`
}

type etaResponse struct {
	Etas map[string]struct {
		Etas []eta `json:"etas"`
	} `json:"etas"`
	CalculationTime int64 `json:"calculation_time"`
}

type downtowner struct {
	baseURL string
	http    *http.Client
	mutex   sync.Mutex
	etags   map[string]string
}

func newDowntowner(baseURL string) *downtowner {
	return &downtowner{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
		etags:   map[string]string{},
	}
}

func (d *downtowner) get(ctx context.Context, path string, into any) (bool, error) {
	url := d.baseURL + path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "ymaps-backend")
	d.mutex.Lock()
	etag := d.etags[url]
	d.mutex.Unlock()
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	response, err := d.http.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		return false, nil
	}
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("downtowner %s: %s", path, response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		return false, fmt.Errorf("downtowner %s: %w", path, err)
	}
	if tag := response.Header.Get("ETag"); tag != "" {
		d.mutex.Lock()
		d.etags[url] = tag
		d.mutex.Unlock()
	}
	return true, nil
}

func (d *downtowner) routes(ctx context.Context) ([]route, bool, error) {
	var routes []route
	changed, err := d.get(ctx, "/routes_routes.php?inactive=true", &routes)
	return routes, changed, err
}

func (d *downtowner) stops(ctx context.Context) ([]stop, bool, error) {
	var stops []stop
	changed, err := d.get(ctx, "/routes_stops.php", &stops)
	return stops, changed, err
}

func (d *downtowner) buses(ctx context.Context) ([]bus, bool, error) {
	var buses []bus
	changed, err := d.get(ctx, "/routes_buses.php", &buses)
	return buses, changed, err
}

func (d *downtowner) etas(ctx context.Context, stopID int) (etaResponse, bool, error) {
	var response etaResponse
	changed, err := d.get(ctx, "/routes_eta.php?stop="+strconv.Itoa(stopID), &response)
	return response, changed, err
}
