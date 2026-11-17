package gbfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

const (
	StationInformation = "station_information"
	StationStatus      = "station_status"
	FreeBikeStatus     = "free_bike_status"
	VehicleTypes       = "vehicle_types"
	SystemPricingPlans = "system_pricing_plans"
)

const (
	defaultFastEvery = 30 * time.Second
	defaultSlowEvery = time.Hour
	staleAfter       = 120 * time.Second
	requestTimeout   = 10 * time.Second
)

var fastFeeds = map[string]bool{StationStatus: true, FreeBikeStatus: true}

type Feed struct {
	Name     string
	Required bool
}

type Config struct {
	Name         string
	DiscoveryURL string
	Headers      map[string]string
	Feeds        []Feed
	FastEvery    time.Duration
	SlowEvery    time.Duration
	Client       *http.Client
	Derive       func(*Snapshot)
}

type Snapshot struct {
	FetchedAt time.Time
	Stations  []Station
	Vehicles  []FreeVehicle
	Types     map[string]VehicleType
	Plans     map[string]PricingPlan
	Spots     []Spot
}

type slowData struct {
	information map[string]Station
	types       map[string]VehicleType
	plans       map[string]PricingPlan
}

type Reader struct {
	config   Config
	now      func() time.Time
	current  atomic.Pointer[Snapshot]
	feedURLs map[string]string
	slow     *slowData
}

var errFeedNotFound = errors.New("feed not found")

func NewReader(config Config) *Reader {
	if config.FastEvery <= 0 {
		config.FastEvery = defaultFastEvery
	}
	if config.SlowEvery <= 0 {
		config.SlowEvery = defaultSlowEvery
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: requestTimeout}
	}
	return &Reader{config: config, now: time.Now}
}

func (r *Reader) Current() *Snapshot {
	return r.current.Load()
}

func (r *Reader) Fresh(now time.Time) *Snapshot {
	snapshot := r.current.Load()
	if snapshot == nil || now.Sub(snapshot.FetchedAt) > staleAfter {
		return nil
	}
	return snapshot
}

func (r *Reader) Run(ctx context.Context) {
	r.Refresh(ctx, true)
	fast := time.NewTicker(r.config.FastEvery)
	defer fast.Stop()
	slow := time.NewTicker(r.config.SlowEvery)
	defer slow.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-slow.C:
			r.Refresh(ctx, true)
		case <-fast.C:
			r.Refresh(ctx, false)
		}
	}
}

func (r *Reader) Refresh(ctx context.Context, includeSlow bool) {
	if err := r.refresh(ctx, includeSlow); err != nil {
		log.Printf("gbfs %s: %v", r.config.Name, err)
	}
}

func (r *Reader) refresh(ctx context.Context, includeSlow bool) error {
	if includeSlow || r.feedURLs == nil {
		if err := r.discover(ctx); err != nil && r.feedURLs == nil {
			return err
		}
	}
	if includeSlow || r.slow == nil {
		slow, err := r.pollSlow(ctx)
		if err != nil && r.slow == nil {
			return err
		}
		if err == nil {
			r.slow = slow
		}
	}
	snapshot, err := r.pollFast(ctx)
	if err != nil {
		return err
	}
	if r.config.Derive != nil {
		r.config.Derive(snapshot)
	}
	r.current.Store(snapshot)
	return nil
}

func (r *Reader) discover(ctx context.Context) error {
	var body envelope
	if err := r.fetch(ctx, r.config.DiscoveryURL, &body); err != nil {
		return fmt.Errorf("discovery: %w", err)
	}
	var languages map[string]discoveryLanguage
	if err := json.Unmarshal(body.Data, &languages); err != nil {
		return fmt.Errorf("discovery: %w", err)
	}
	language, found := languages["en"]
	if !found {
		for _, other := range languages {
			language = other
			break
		}
	}
	urls := make(map[string]string, len(language.Feeds))
	for _, feed := range language.Feeds {
		urls[feed.Name] = feed.URL
	}
	for _, feed := range r.config.Feeds {
		if feed.Required && urls[feed.Name] == "" {
			return fmt.Errorf("discovery: required feed %s not listed", feed.Name)
		}
	}
	r.feedURLs = urls
	return nil
}

func (r *Reader) pollSlow(ctx context.Context) (*slowData, error) {
	slow := &slowData{information: map[string]Station{}, types: map[string]VehicleType{}, plans: map[string]PricingPlan{}}
	for _, feed := range r.config.Feeds {
		if fastFeeds[feed.Name] {
			continue
		}
		raw, err := r.feedData(ctx, feed)
		if err != nil {
			return nil, err
		}
		if raw == nil {
			continue
		}
		switch feed.Name {
		case StationInformation:
			slow.information, err = decodeStationInformation(raw)
		case VehicleTypes:
			slow.types, err = decodeVehicleTypes(raw)
		case SystemPricingPlans:
			slow.plans, err = decodePricingPlans(raw)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", feed.Name, err)
		}
	}
	return slow, nil
}

func (r *Reader) pollFast(ctx context.Context) (*Snapshot, error) {
	snapshot := &Snapshot{Types: r.slow.types, Plans: r.slow.plans}
	for _, feed := range r.config.Feeds {
		if !fastFeeds[feed.Name] {
			continue
		}
		raw, err := r.feedData(ctx, feed)
		if err != nil {
			return nil, err
		}
		if raw == nil {
			continue
		}
		switch feed.Name {
		case StationStatus:
			snapshot.Stations, err = decodeStationStatus(raw, r.slow.information)
		case FreeBikeStatus:
			snapshot.Vehicles, err = decodeFreeBikeStatus(raw)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", feed.Name, err)
		}
	}
	snapshot.FetchedAt = r.now()
	return snapshot, nil
}

func (r *Reader) feedData(ctx context.Context, feed Feed) (json.RawMessage, error) {
	url := r.feedURLs[feed.Name]
	if url == "" {
		if feed.Required {
			return nil, fmt.Errorf("%s: not listed", feed.Name)
		}
		return nil, nil
	}
	var body envelope
	err := r.fetch(ctx, url, &body)
	if errors.Is(err, errFeedNotFound) && !feed.Required {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", feed.Name, err)
	}
	return body.Data, nil
}

func (r *Reader) fetch(ctx context.Context, url string, into any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for name, value := range r.config.Headers {
		request.Header.Set(name, value)
	}
	request.Header.Set("Accept", "application/json")
	response, err := r.config.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		io.Copy(io.Discard, response.Body)
		return errFeedNotFound
	}
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, response.Body)
		return fmt.Errorf("status %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(into)
}
