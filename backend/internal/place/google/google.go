package google

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/place"
)

const (
	defaultBase       = "https://places.googleapis.com/v1"
	searchFieldMask   = "places.id,places.displayName,places.formattedAddress,places.location,places.primaryType"
	lookupFieldMask   = "id,displayName,formattedAddress,location,primaryType"
	sameSpotMeters    = 60.0
	biasRadiusMeters  = 5000.0
	pageSize          = 5
	matchedConfidence = 0.7
	firstConfidence   = 0.5
	laterConfidence   = 0.4
	confidenceStep    = 0.05
	floorConfidence   = 0.1
)

type Resolver struct {
	key    string
	base   string
	client *http.Client
}

func New(key string) *Resolver {
	return &Resolver{key: key, base: defaultBase, client: &http.Client{Timeout: 5 * time.Second}}
}

func (r *Resolver) Source() place.Source {
	return place.Google
}

type latLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type circle struct {
	Center latLng  `json:"center"`
	Radius float64 `json:"radius"`
}

type locationBias struct {
	Circle circle `json:"circle"`
}

type searchRequest struct {
	TextQuery    string        `json:"textQuery"`
	PageSize     int           `json:"pageSize"`
	LocationBias *locationBias `json:"locationBias,omitempty"`
}

type result struct {
	ID          string `json:"id"`
	DisplayName struct {
		Text string `json:"text"`
	} `json:"displayName"`
	FormattedAddress string `json:"formattedAddress"`
	Location         latLng `json:"location"`
	PrimaryType      string `json:"primaryType"`
}

type searchResponse struct {
	Places []result `json:"places"`
}

func (r *Resolver) Resolve(ctx context.Context, query string, near geo.LatLon) ([]place.Candidate, error) {
	if r.key == "" || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	request := searchRequest{TextQuery: query, PageSize: pageSize}
	if near != (geo.LatLon{}) {
		request.LocationBias = &locationBias{Circle: circle{
			Center: latLng{Latitude: near.Lat, Longitude: near.Lon},
			Radius: biasRadiusMeters,
		}}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base+"/places:searchText", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Goog-FieldMask", searchFieldMask)
	var response searchResponse
	if err := r.do(httpRequest, &response); err != nil {
		return nil, err
	}
	merged := merge(response.Places)
	candidates := make([]place.Candidate, 0, len(merged))
	for index, found := range merged {
		candidates = append(candidates, place.Candidate{
			Place:      found,
			Confidence: confidence(index, len(merged), query, found.Name),
		})
	}
	return candidates, nil
}

func (r *Resolver) Lookup(ctx context.Context, id string) (place.Place, error) {
	placeID, ok := strings.CutPrefix(id, string(place.Google)+":")
	if r.key == "" || !ok || placeID == "" {
		return place.Place{}, place.ErrNotFound
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+"/places/"+url.PathEscape(placeID), nil)
	if err != nil {
		return place.Place{}, err
	}
	httpRequest.Header.Set("X-Goog-FieldMask", lookupFieldMask)
	var found result
	if err := r.do(httpRequest, &found); err != nil {
		return place.Place{}, err
	}
	if found.ID == "" {
		return place.Place{}, place.ErrNotFound
	}
	return found.place(), nil
}

func (r *Resolver) do(request *http.Request, into any) error {
	request.Header.Set("X-Goog-Api-Key", r.key)
	response, err := r.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return place.ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("google places: %s: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	return json.NewDecoder(response.Body).Decode(into)
}

func merge(results []result) []place.Place {
	var places []place.Place
	for _, found := range results {
		if found.ID == "" {
			continue
		}
		candidate := found.place()
		joined := false
		for i := range places {
			if geo.Distance(places[i].At, candidate.At) <= sameSpotMeters {
				if moreSpecific(candidate.Name, places[i].Name, places[i].Address) {
					places[i].Name = candidate.Name
				}
				joined = true
				break
			}
		}
		if !joined {
			places = append(places, candidate)
		}
	}
	return places
}

func moreSpecific(name, current, address string) bool {
	city := locality(address)
	if strings.EqualFold(name, city) {
		return false
	}
	if strings.EqualFold(current, city) || strings.HasSuffix(current, " Station") && strings.EqualFold(strings.TrimSuffix(current, " Station"), city) {
		return true
	}
	return len(strings.Fields(name)) > len(strings.Fields(current))
}

func locality(address string) string {
	parts := strings.Split(address, ", ")
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-3]
}

var stationTypes = map[string]bool{"train_station": true, "transit_station": true, "subway_station": true, "bus_station": true}

func (found result) name() string {
	name := found.DisplayName.Text
	if stationTypes[found.PrimaryType] && strings.EqualFold(name, locality(found.FormattedAddress)) {
		return name + " Station"
	}
	return name
}

func (found result) place() place.Place {
	return place.Place{
		ID:      string(place.Google) + ":" + found.ID,
		Name:    found.name(),
		Address: found.FormattedAddress,
		At:      geo.LatLon{Lat: found.Location.Latitude, Lon: found.Location.Longitude},
		Source:  place.Google,
	}
}

func confidence(index, total int, query, name string) float64 {
	if index == 0 {
		if total == 1 || closeMatch(query, name) {
			return matchedConfidence
		}
		return firstConfidence
	}
	return max(floorConfidence, laterConfidence-confidenceStep*float64(index-1))
}

func closeMatch(query, name string) bool {
	q, n := normalize(query), normalize(name)
	if q == "" || n == "" {
		return false
	}
	return q == n || strings.Contains(n, q) || strings.Contains(q, n)
}

func normalize(text string) string {
	text = strings.NewReplacer("'", "", "’", "").Replace(strings.ToLower(text))
	return strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}
