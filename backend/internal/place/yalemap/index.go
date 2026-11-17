package yalemap

import (
	"html"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"ymaps/backend/internal/geo"
)

const duplicateMeters = 100

type entry struct {
	id            string
	name          string
	normalized    string
	nameTokens    []string
	keywordTokens []string
	address       string
	at            geo.LatLon
	building      bool
	entrance      bool
}

type index struct {
	byID           map[string]*entry
	byAbbreviation map[string][]*entry
	byName         map[string][]*entry
	byKeyword      map[string][]*entry
	searchable     []*entry
	buildings      []*entry
	entrances      map[string][]*entry
}

func build(locations []rawLocation, categories []rawCategory, details map[string]detail) *index {
	buildingCategories := descendants(categories, func(name string) bool { return name == "buildings" })
	entranceCategories := descendants(categories, func(name string) bool { return strings.Contains(name, "entrance") })
	idx := &index{
		byID:           map[string]*entry{},
		byAbbreviation: map[string][]*entry{},
		byName:         map[string][]*entry{},
		byKeyword:      map[string][]*entry{},
		entrances:      map[string][]*entry{},
	}
	ordered := append([]rawLocation(nil), locations...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return buildingCategories[string(ordered[i].CatID)] && !buildingCategories[string(ordered[j].CatID)]
	})
	var entrances []*entry
	for _, location := range ordered {
		id := string(location.ID)
		at := geo.LatLon{Lat: float64(location.Lat), Lon: float64(location.Lng)}
		if id == "" || strings.TrimSpace(location.Name) == "" || at == (geo.LatLon{}) {
			continue
		}
		if _, dup := idx.byID[id]; dup {
			continue
		}
		e := &entry{
			id:         id,
			name:       strings.TrimSpace(location.Name),
			normalized: normalize(location.Name),
			nameTokens: withoutStopwords(tokens(location.Name)),
			at:         at,
		}
		category := string(location.CatID)
		e.entrance = entranceCategories[category] || location.LocationType == "access_point" || strings.HasSuffix(e.normalized, " entrance")
		if len(categories) > 0 {
			e.building = buildingCategories[category]
		} else {
			e.building = !e.entrance && location.LocationType == "standard" && hasShape(location.Shape)
		}
		if e.entrance {
			idx.byID[id] = e
			entrances = append(entrances, e)
			continue
		}
		if same := idx.sameAs(e); same != nil {
			idx.byID[id] = same
			continue
		}
		idx.byID[id] = e
		if d, ok := details[id]; ok {
			e.address = d.address
			for _, abbreviation := range d.abbreviations {
				idx.byAbbreviation[abbreviation] = append(idx.byAbbreviation[abbreviation], e)
			}
			for _, keyword := range d.keywords {
				normalizedKeyword := normalize(keyword)
				if normalizedKeyword == "" {
					continue
				}
				idx.byKeyword[normalizedKeyword] = append(idx.byKeyword[normalizedKeyword], e)
				e.keywordTokens = append(e.keywordTokens, tokens(keyword)...)
			}
		}
		idx.byName[e.normalized] = append(idx.byName[e.normalized], e)
		idx.searchable = append(idx.searchable, e)
		if e.building {
			idx.buildings = append(idx.buildings, e)
		}
	}
	for _, entrance := range entrances {
		idx.byName[entrance.normalized] = append(idx.byName[entrance.normalized], entrance)
		if owner := idx.owner(entrance); owner != nil {
			idx.entrances[owner.id] = append(idx.entrances[owner.id], entrance)
		}
	}
	return idx
}

func (idx *index) sameAs(e *entry) *entry {
	for _, existing := range idx.byName[e.normalized] {
		if geo.Distance(existing.at, e.at) <= duplicateMeters {
			return existing
		}
	}
	return nil
}

func (idx *index) owner(entrance *entry) *entry {
	var best *entry
	for _, e := range idx.searchable {
		if !strings.HasPrefix(entrance.normalized, e.normalized+" ") {
			continue
		}
		switch {
		case best == nil, len(e.normalized) > len(best.normalized):
			best = e
		case len(e.normalized) == len(best.normalized) && geo.Distance(entrance.at, e.at) < geo.Distance(entrance.at, best.at):
			best = e
		}
	}
	return best
}

func descendants(categories []rawCategory, matches func(string) bool) map[string]bool {
	children := map[string][]string{}
	var queue []string
	for _, category := range categories {
		key := category.key()
		children[string(category.Parent)] = append(children[string(category.Parent)], key)
		if matches(normalize(category.Name)) {
			queue = append(queue, key)
		}
	}
	found := map[string]bool{}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if key == "" || found[key] {
			continue
		}
		found[key] = true
		queue = append(queue, children[key]...)
	}
	return found
}

func hasShape(shape []byte) bool {
	trimmed := strings.TrimSpace(string(shape))
	return trimmed != "" && trimmed != "null" && trimmed != "[]" && trimmed != `""`
}

var (
	tag              = regexp.MustCompile(`<[^>]*>`)
	abbreviationLine = regexp.MustCompile(`(?i)abbreviations?\s*:\s*([^\n]*)`)
	addressLine      = regexp.MustCompile(`(?i)address\s*:\s*([^\n]*)`)
	abbreviationWord = regexp.MustCompile(`^[A-Z][A-Z0-9&]{1,7}$`)
)

func parseDetail(raw rawDetail) detail {
	text := html.UnescapeString(tag.ReplaceAllString(raw.Description, "\n"))
	text = strings.ReplaceAll(text, " ", " ")
	var d detail
	seen := map[string]bool{}
	add := func(abbreviation string) {
		if !seen[abbreviation] {
			seen[abbreviation] = true
			d.abbreviations = append(d.abbreviations, abbreviation)
		}
	}
	for _, match := range abbreviationLine.FindAllStringSubmatch(text, -1) {
		for _, word := range strings.FieldsFunc(match[1], func(r rune) bool { return r == ',' || r == '/' || r == ';' || unicode.IsSpace(r) }) {
			if !abbreviationWord.MatchString(word) {
				break
			}
			add(word)
		}
	}
	if match := addressLine.FindStringSubmatch(text); match != nil {
		d.address = strings.TrimSpace(match[1])
	}
	for _, keyword := range raw.Keywords {
		keyword = strings.TrimSpace(keyword)
		if abbreviationWord.MatchString(keyword) {
			add(keyword)
			continue
		}
		d.keywords = append(d.keywords, keyword)
	}
	return d
}

var stopwords = map[string]bool{"the": true, "of": true, "and": true, "at": true, "to": true, "a": true, "yale": true}

func tokens(text string) []string {
	text = strings.NewReplacer("'", "", "’", "").Replace(strings.ToLower(text))
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func normalize(text string) string {
	return strings.Join(tokens(text), " ")
}

func withoutStopwords(words []string) []string {
	var kept []string
	for _, word := range words {
		if !stopwords[word] {
			kept = append(kept, word)
		}
	}
	return kept
}
