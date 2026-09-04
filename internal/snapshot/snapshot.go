// Package snapshot stores and loads saved window layouts as JSON files
// under $XDG_STATE_HOME/herd (default ~/.local/state/herd).
package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jcam3ron/herd/internal/backend"
)

type Snapshot struct {
	Backend string                  `json:"backend"`
	Name    string                  `json:"name"`
	SavedAt time.Time               `json:"saved_at"`
	Windows []backend.PlannedWindow `json:"windows"`
}

type Store struct {
	Dir string
}

func NewStore() (*Store, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	dir = filepath.Join(dir, "herd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

func (s *Store) Path(name string) string {
	return filepath.Join(s.Dir, name+".json")
}

// Exists reports whether a snapshot named name has already been saved.
func (s *Store) Exists(name string) bool {
	_, err := os.Stat(s.Path(name))
	return err == nil
}

func (s *Store) Save(snap Snapshot) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path(snap.Name), data, 0o644)
}

func (s *Store) Load(name string) (Snapshot, error) {
	data, err := os.ReadFile(s.Path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}, fmt.Errorf("no snapshot named %q (see: herd list)", name)
		}
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("decoding snapshot %q: %w", name, err)
	}
	return snap, nil
}

func (s *Store) List() ([]Snapshot, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	var snaps []Snapshot
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		snap, err := s.Load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		snaps = append(snaps, snap)
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Name < snaps[j].Name })
	return snaps, nil
}

func (s *Store) Remove(name string) error {
	if _, err := os.Stat(s.Path(name)); os.IsNotExist(err) {
		return fmt.Errorf("no snapshot named %q", name)
	}
	return os.Remove(s.Path(name))
}
