package yalemap

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/place"
)

const (
	defaultBase       = "https://api.concept3d.com"
	mapID             = "1910"
	refreshEvery      = 24 * time.Hour
	retryAfter        = time.Minute
	detailConcurrency = 6
	maxResults        = 5

	linkConfidence         = 0.95
	abbreviationConfidence = 0.95
	courseConfidence       = 0.9
	nameConfidence         = 0.9
	keywordConfidence      = 0.85
	prefixConfidence       = 0.75
	tokenBaseConfidence    = 0.5
	tokenCoverageWeight    = 0.2
	partialWeight          = 0.3
	phraseConfidence       = 0.9
	phraseExtraWordPenalty = 0.08
	phraseFloor            = 0.55
)

type Resolver struct {
	key    string
	base   string
	client *http.Client
	now    func() time.Time

	mu              sync.Mutex
	current         *index
	locations       []rawLocation
	categories      []rawCategory
	details         map[string]detail
	loadedAt        time.Time
	failedAt        time.Time
	failure         error
	refreshing      bool
	fetchingDetails bool
	background      sync.WaitGroup
}

func New(key string) *Resolver {
	return &Resolver{
		key:     key,
		base:    defaultBase,
		client:  &http.Client{Timeout: 20 * time.Second},
		now:     time.Now,
		details: map[string]detail{},
	}
}

func (r *Resolver) Warm(ctx context.Context) error {
	if r.key == "" {
		return nil
	}
	_, err := r.index(ctx)
	return err
}

func (r *Resolver) Source() place.Source {
	return place.Yale
}

var mapLink = regexp.MustCompile(`m/(\d+)`)

func (r *Resolver) Resolve(ctx context.Context, query string, near geo.LatLon) ([]place.Candidate, error) {
	if r.key == "" || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	idx, err := r.index(ctx)
	if err != nil {
		return nil, err
	}
	scores := map[*entry]float64{}
	offer := func(e *entry, confidence float64) {
		if confidence > scores[e] {
			scores[e] = confidence
		}
	}
	if match := mapLink.FindStringSubmatch(query); match != nil {
		if e, ok := idx.byID[match[1]]; ok {
			offer(e, linkConfidence)
		}
	}
	words := tokens(query)
	if len(words) == 0 && len(scores) == 0 {
		return nil, nil
	}
	if len(words) == 1 {
		for _, e := range idx.byAbbreviation[strings.ToUpper(words[0])] {
			offer(e, abbreviationConfidence)
		}
	}
	if code, ok := courseBuilding(query, words); ok {
		for _, e := range idx.byAbbreviation[code] {
			offer(e, courseConfidence)
		}
	}
	normalized := strings.Join(words, " ")
	for _, e := range idx.byName[normalized] {
		offer(e, nameConfidence)
	}
	for _, e := range idx.byKeyword[normalized] {
		offer(e, keywordConfidence)
	}
	for _, e := range idx.searchable {
		if confidence := fuzzy(e, normalized, words); confidence > 0 {
			offer(e, confidence)
		}
	}
	return idx.candidates(scores, near), nil
}

func (r *Resolver) Lookup(ctx context.Context, id string) (place.Place, error) {
	locationID, ok := strings.CutPrefix(id, string(place.Yale)+":")
	if r.key == "" || !ok || locationID == "" {
		return place.Place{}, place.ErrNotFound
	}
	idx, err := r.index(ctx)
	if err != nil {
		return place.Place{}, err
	}
	e, ok := idx.byID[locationID]
	if !ok {
		return place.Place{}, place.ErrNotFound
	}
	return idx.place(e, e.at), nil
}

func (r *Resolver) index(ctx context.Context) (*index, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		if !r.failedAt.IsZero() && r.now().Sub(r.failedAt) < retryAfter {
			return nil, r.failure
		}
		locations, categories, err := r.fetchAll(ctx)
		if err != nil {
			if ctx.Err() == nil {
				r.failedAt, r.failure = r.now(), err
			}
			return nil, err
		}
		r.installLocked(locations, categories)
		return r.current, nil
	}
	if r.now().Sub(r.loadedAt) >= refreshEvery && !r.refreshing {
		r.refreshing = true
		r.background.Add(1)
		go r.refresh()
	}
	return r.current, nil
}

func (r *Resolver) refresh() {
	defer r.background.Done()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	locations, categories, err := r.fetchAll(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshing = false
	if err != nil {
		log.Printf("yale map refresh: %v", err)
		r.loadedAt = r.now().Add(retryAfter - refreshEvery)
		return
	}
	r.installLocked(locations, categories)
}

func (r *Resolver) installLocked(locations []rawLocation, categories []rawCategory) {
	r.locations, r.categories = locations, categories
	r.loadedAt = r.now()
	r.failedAt, r.failure = time.Time{}, nil
	r.current = build(r.locations, r.categories, r.details)
	r.startDetailsLocked()
}

func (r *Resolver) startDetailsLocked() {
	if r.fetchingDetails {
		return
	}
	var missing []string
	for _, e := range r.current.buildings {
		if _, ok := r.details[e.id]; !ok {
			missing = append(missing, e.id)
		}
	}
	if len(missing) == 0 {
		return
	}
	r.fetchingDetails = true
	r.background.Add(1)
	go func() {
		defer r.background.Done()
		fetched := r.fetchDetails(missing)
		r.mu.Lock()
		defer r.mu.Unlock()
		for id, d := range fetched {
			r.details[id] = d
		}
		r.fetchingDetails = false
		r.current = build(r.locations, r.categories, r.details)
	}()
}

func (r *Resolver) fetchDetails(ids []string) map[string]detail {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		failures int
		lastErr  error
	)
	fetched := map[string]detail{}
	slots := make(chan struct{}, detailConcurrency)
	for _, id := range ids {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			d, err := r.fetchDetail(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
				lastErr = err
				return
			}
			fetched[id] = d
		}()
	}
	wg.Wait()
	if failures > 0 {
		log.Printf("yale map details: %d of %d failed, last: %v", failures, len(ids), lastErr)
	}
	return fetched
}

func courseBuilding(query string, words []string) (string, bool) {
	if before, _, ok := strings.Cut(query, " - "); ok {
		if code := tokens(before); len(code) >= 1 {
			return strings.ToUpper(code[0]), true
		}
	}
	if len(words) >= 2 && strings.ContainsAny(words[1], "0123456789") {
		return strings.ToUpper(words[0]), true
	}
	return "", false
}

func fuzzy(e *entry, normalized string, words []string) float64 {
	if len(normalized) >= 3 && strings.HasPrefix(e.normalized, normalized+" ") {
		return prefixConfidence
	}
	if len(words) >= 2 && strings.Contains(" "+e.normalized+" ", " "+normalized+" ") {
		extra := len(strings.Fields(e.normalized)) - len(words)
		return max(phraseFloor, phraseConfidence-phraseExtraWordPenalty*float64(extra))
	}
	meaningful := withoutStopwords(words)
	if len(meaningful) == 0 {
		return 0
	}
	initials := len(meaningful) >= 2
	matched := 0
	for _, word := range meaningful {
		if matchesAny(word, e.nameTokens, initials) || matchesAny(word, e.keywordTokens, initials) {
			matched++
		}
	}
	if matched == len(meaningful) {
		coveredName := 0
		for _, nameToken := range e.nameTokens {
			if matchesAny(nameToken, meaningful, false) || matchesAnyPrefix(nameToken, meaningful, initials) {
				coveredName++
			}
		}
		coverage := 0.0
		if len(e.nameTokens) > 0 {
			coverage = float64(coveredName) / float64(len(e.nameTokens))
		}
		return tokenBaseConfidence + tokenCoverageWeight*coverage
	}
	fraction := float64(matched) / float64(len(meaningful))
	if fraction < 0.5 {
		return 0
	}
	return partialWeight * fraction
}

func matchesAny(word string, candidates []string, initials bool) bool {
	for _, candidate := range candidates {
		if candidate == word || (len(word) >= 2 && strings.HasPrefix(candidate, word)) || (initials && isInitial(candidate, word)) {
			return true
		}
	}
	return false
}

func matchesAnyPrefix(nameToken string, words []string, initials bool) bool {
	for _, word := range words {
		if (len(word) >= 2 && strings.HasPrefix(nameToken, word)) || (initials && isInitial(nameToken, word)) {
			return true
		}
	}
	return false
}

func isInitial(initial, word string) bool {
	return len(initial) == 1 && len(word) > 1 && unicode.IsLetter(rune(initial[0])) && word[0] == initial[0]
}

func (idx *index) candidates(scores map[*entry]float64, near geo.LatLon) []place.Candidate {
	ranked := make([]*entry, 0, len(scores))
	for e := range scores {
		ranked = append(ranked, e)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if scores[ranked[i]] != scores[ranked[j]] {
			return scores[ranked[i]] > scores[ranked[j]]
		}
		di, dj := geo.Distance(near, ranked[i].at), geo.Distance(near, ranked[j].at)
		if di != dj {
			return di < dj
		}
		return ranked[i].id < ranked[j].id
	})
	if len(ranked) > maxResults {
		ranked = ranked[:maxResults]
	}
	candidates := make([]place.Candidate, 0, len(ranked))
	for _, e := range ranked {
		from := near
		if near == (geo.LatLon{}) {
			from = e.at
		}
		candidates = append(candidates, place.Candidate{Place: idx.place(e, from), Confidence: scores[e]})
	}
	return candidates
}

func (idx *index) place(e *entry, from geo.LatLon) place.Place {
	p := place.Place{
		ID:      string(place.Yale) + ":" + e.id,
		Name:    e.name,
		Address: e.address,
		At:      e.at,
		Source:  place.Yale,
	}
	var best *entry
	for _, entrance := range idx.entrances[e.id] {
		if best == nil || geo.Distance(from, entrance.at) < geo.Distance(from, best.at) {
			best = entrance
		}
	}
	if best != nil {
		p.Entryway = &place.Entryway{Name: best.name, At: best.at}
	}
	return p
}

func errorf(key, format string, args ...any) error {
	err := fmt.Errorf(format, args...)
	if key == "" {
		return err
	}
	return redacted{message: strings.ReplaceAll(err.Error(), key, "REDACTED"), cause: err}
}

type redacted struct {
	message string
	cause   error
}

func (e redacted) Error() string { return e.message }

func (e redacted) Unwrap() error { return e.cause }
