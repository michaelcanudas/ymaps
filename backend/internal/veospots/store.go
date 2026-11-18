package veospots

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const stateVersion = 1

type savedState struct {
	Version  int               `json:"version"`
	SavedAt  time.Time         `json:"savedAt"`
	LastAt   time.Time         `json:"lastAt"`
	Tracks   map[string]*Track `json:"tracks"`
	TripEnds []*Cluster        `json:"tripEnds"`
	Rests    []*Cluster        `json:"rests"`
	Recent   []TripEnd         `json:"recentTripEnds"`
}

func (c *Collector) Load() error {
	raw, err := os.ReadFile(c.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved savedState
	if err := json.Unmarshal(raw, &saved); err != nil {
		return fmt.Errorf("%s: %w", c.path, err)
	}
	if saved.Version != stateVersion {
		return fmt.Errorf("%s: version %d not supported", c.path, saved.Version)
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if saved.Tracks == nil {
		saved.Tracks = map[string]*Track{}
	}
	c.state = state{Tracks: saved.Tracks, TripEnds: saved.TripEnds, Rests: saved.Rests, Recent: saved.Recent}
	c.lastAt = saved.LastAt
	c.spots = c.activeSpots()
	return nil
}

func (c *Collector) Save() error {
	c.mutex.Lock()
	if !c.pending {
		c.mutex.Unlock()
		return nil
	}
	raw, err := json.Marshal(savedState{
		Version: stateVersion, SavedAt: time.Now(), LastAt: c.lastAt,
		Tracks: c.state.Tracks, TripEnds: c.state.TripEnds, Rests: c.state.Rests, Recent: c.state.Recent,
	})
	c.pending = false
	c.mutex.Unlock()
	if err == nil {
		err = c.write(raw)
	}
	if err != nil {
		c.mutex.Lock()
		c.pending = true
		c.mutex.Unlock()
	}
	return err
}

func (c *Collector) write(raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	temporary := c.path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, c.path)
}
