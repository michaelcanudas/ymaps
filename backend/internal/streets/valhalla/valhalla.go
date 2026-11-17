package valhalla

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/streets"
)

const (
	requestTimeout   = 2 * time.Second
	shapePrecision   = 1e6
	bicycleUseRoads  = 0.5
	bicycleType      = "Hybrid"
	errorBodyLimit   = 512
	metersPerKm      = 1000
	pedestrianCosted = "pedestrian"
	bicycleCosted    = "bicycle"
	unreachable      = 24 * time.Hour
)

var errMalformedResponse = errors.New("valhalla: malformed response")

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: requestTimeout},
	}
}

type location struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type bicycleOptions struct {
	CyclingSpeed float64 `json:"cycling_speed,omitempty"`
	UseRoads     float64 `json:"use_roads"`
	BicycleType  string  `json:"bicycle_type"`
}

type pedestrianOptions struct {
	WalkingSpeed float64 `json:"walking_speed"`
}

type costingOptions struct {
	Pedestrian *pedestrianOptions `json:"pedestrian,omitempty"`
	Bicycle    *bicycleOptions    `json:"bicycle,omitempty"`
}

type routeRequest struct {
	Locations      []location      `json:"locations"`
	Costing        string          `json:"costing"`
	CostingOptions *costingOptions `json:"costing_options,omitempty"`
	Units          string          `json:"units"`
}

type matrixRequest struct {
	Sources        []location      `json:"sources"`
	Targets        []location      `json:"targets"`
	Costing        string          `json:"costing"`
	CostingOptions *costingOptions `json:"costing_options,omitempty"`
	Units          string          `json:"units"`
}

type summary struct {
	Time   float64 `json:"time"`
	Length float64 `json:"length"`
}

type routeResponse struct {
	Trip struct {
		Summary summary `json:"summary"`
		Legs    []struct {
			Shape string `json:"shape"`
		} `json:"legs"`
	} `json:"trip"`
}

type matrixCell struct {
	Time *float64 `json:"time"`
}

type matrixResponse struct {
	SourcesToTargets [][]matrixCell `json:"sources_to_targets"`
}

func (c *Client) Route(ctx context.Context, from, to geo.LatLon, profile streets.Profile) (streets.Trace, error) {
	costing, options := costingFor(profile)
	request := routeRequest{
		Locations:      []location{toLocation(from), toLocation(to)},
		Costing:        costing,
		CostingOptions: options,
		Units:          "kilometers",
	}
	var response routeResponse
	if err := c.post(ctx, "/route", request, &response); err != nil {
		return streets.Trace{}, err
	}
	if len(response.Trip.Legs) == 0 {
		return streets.Trace{}, errMalformedResponse
	}
	var path []geo.LatLon
	for _, leg := range response.Trip.Legs {
		points := decodeShape(leg.Shape)
		if len(path) > 0 && len(points) > 0 && path[len(path)-1] == points[0] {
			points = points[1:]
		}
		path = append(path, points...)
	}
	return streets.Trace{
		Duration: seconds(response.Trip.Summary.Time),
		Meters:   response.Trip.Summary.Length * metersPerKm,
		Path:     path,
	}, nil
}

func (c *Client) Matrix(ctx context.Context, from, to []geo.LatLon, profile streets.Profile) ([][]time.Duration, error) {
	if len(from) == 0 || len(to) == 0 {
		durations := make([][]time.Duration, len(from))
		for i := range durations {
			durations[i] = []time.Duration{}
		}
		return durations, nil
	}
	costing, options := costingFor(profile)
	request := matrixRequest{
		Sources:        toLocations(from),
		Targets:        toLocations(to),
		Costing:        costing,
		CostingOptions: options,
		Units:          "kilometers",
	}
	var response matrixResponse
	if err := c.post(ctx, "/sources_to_targets", request, &response); err != nil {
		return nil, err
	}
	if len(response.SourcesToTargets) != len(from) {
		return nil, errMalformedResponse
	}
	durations := make([][]time.Duration, len(from))
	for i, row := range response.SourcesToTargets {
		if len(row) != len(to) {
			return nil, errMalformedResponse
		}
		durations[i] = make([]time.Duration, len(to))
		for j, cell := range row {
			if cell.Time == nil {
				durations[i][j] = unreachable
				continue
			}
			durations[i][j] = seconds(*cell.Time)
		}
	}
	return durations, nil
}

func (c *Client) post(ctx context.Context, path string, body, into any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, errorBodyLimit))
		return fmt.Errorf("valhalla %s: status %d: %s", path, response.StatusCode, strings.TrimSpace(string(detail)))
	}
	return json.NewDecoder(response.Body).Decode(into)
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: requestTimeout}
}

func costingFor(profile streets.Profile) (string, *costingOptions) {
	if profile.Kind != streets.Bicycle {
		if profile.SpeedKmh <= 0 {
			return pedestrianCosted, nil
		}
		return pedestrianCosted, &costingOptions{Pedestrian: &pedestrianOptions{WalkingSpeed: profile.SpeedKmh}}
	}
	return bicycleCosted, &costingOptions{Bicycle: &bicycleOptions{
		CyclingSpeed: profile.SpeedKmh,
		UseRoads:     bicycleUseRoads,
		BicycleType:  bicycleType,
	}}
}

func toLocation(point geo.LatLon) location {
	return location{Lat: point.Lat, Lon: point.Lon}
}

func toLocations(points []geo.LatLon) []location {
	locations := make([]location, len(points))
	for i, point := range points {
		locations[i] = toLocation(point)
	}
	return locations
}

func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}

func decodeShape(encoded string) []geo.LatLon {
	var path []geo.LatLon
	lat, lon := 0, 0
	for index := 0; index < len(encoded); {
		var deltaLat, deltaLon int
		deltaLat, index = readSigned(encoded, index)
		deltaLon, index = readSigned(encoded, index)
		lat += deltaLat
		lon += deltaLon
		path = append(path, geo.LatLon{Lat: float64(lat) / shapePrecision, Lon: float64(lon) / shapePrecision})
	}
	return path
}

func readSigned(encoded string, index int) (int, int) {
	result, shift := 0, 0
	for index < len(encoded) {
		chunk := int(encoded[index]) - 63
		index++
		result |= (chunk & 0x1f) << shift
		shift += 5
		if chunk < 0x20 {
			break
		}
	}
	if result&1 != 0 {
		return ^(result >> 1), index
	}
	return result >> 1, index
}
