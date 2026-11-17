package yalemap

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type flexString string

func (s *flexString) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*s = flexString(text)
		return nil
	}
	*s = flexString(strings.TrimSpace(string(data)))
	return nil
}

type flexFloat float64

func (f *flexFloat) UnmarshalJSON(data []byte) error {
	var text flexString
	if err := text.UnmarshalJSON(data); err != nil {
		return err
	}
	if strings.TrimSpace(string(text)) == "" {
		return nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(string(text)), 64)
	if err != nil {
		return nil
	}
	*f = flexFloat(value)
	return nil
}

type flexList []string

func (l *flexList) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '[' {
		var items []flexString
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		for _, item := range items {
			*l = append(*l, string(item))
		}
		return nil
	}
	var text flexString
	if err := text.UnmarshalJSON(data); err != nil {
		return err
	}
	for _, item := range strings.Split(string(text), ",") {
		if item = strings.TrimSpace(item); item != "" {
			*l = append(*l, item)
		}
	}
	return nil
}

type rawLocation struct {
	ID           flexString      `json:"id"`
	CatID        flexString      `json:"catId"`
	Name         string          `json:"name"`
	Lat          flexFloat       `json:"lat"`
	Lng          flexFloat       `json:"lng"`
	LocationType string          `json:"locationType"`
	Shape        json.RawMessage `json:"shape"`
}

type rawCategory struct {
	CatID  flexString `json:"catId"`
	ID     flexString `json:"id"`
	Name   string     `json:"name"`
	Parent flexString `json:"parent"`
}

func (c rawCategory) key() string {
	if c.CatID != "" {
		return string(c.CatID)
	}
	return string(c.ID)
}

type rawDetail struct {
	Description string   `json:"description"`
	Keywords    flexList `json:"keywords"`
}

type detail struct {
	address       string
	abbreviations []string
	keywords      []string
}

func (r *Resolver) fetchAll(ctx context.Context) ([]rawLocation, []rawCategory, error) {
	var locations []rawLocation
	if err := r.get(ctx, "/locations", &locations); err != nil {
		return nil, nil, err
	}
	var categories []rawCategory
	if err := r.get(ctx, "/categories", &categories); err != nil {
		categories = nil
	}
	return locations, categories, nil
}

func (r *Resolver) fetchDetail(ctx context.Context, id string) (detail, error) {
	var raw rawDetail
	if err := r.get(ctx, "/locations/"+url.PathEscape(id), &raw); err != nil {
		return detail{}, err
	}
	return parseDetail(raw), nil
}

func (r *Resolver) get(ctx context.Context, path string, into any) error {
	query := url.Values{"map": {mapID}, "key": {r.key}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+path+"?"+query.Encode(), nil)
	if err != nil {
		return errorf(r.key, "yale map %s: %w", path, err)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return errorf(r.key, "yale map %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errorf(r.key, "yale map %s: %s", path, response.Status)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return errorf(r.key, "yale map %s: %w", path, err)
	}
	if err := decode(body, into); err != nil {
		return errorf(r.key, "yale map %s: %w", path, err)
	}
	return nil
}

func decode(body []byte, into any) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return io.ErrUnexpectedEOF
	}
	if _, wantsList := into.(*[]rawLocation); wantsList && trimmed[0] == '{' {
		var wrapped struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(trimmed, &wrapped); err == nil && len(wrapped.Data) > 0 {
			trimmed = wrapped.Data
		}
	}
	if _, wantsDetail := into.(*rawDetail); wantsDetail && trimmed[0] == '[' {
		var list []json.RawMessage
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return err
		}
		if len(list) == 0 {
			return nil
		}
		trimmed = list[0]
	}
	return json.Unmarshal(trimmed, into)
}
