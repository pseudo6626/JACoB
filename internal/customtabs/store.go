package customtabs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxHTMLBytes = 1024 * 1024

var defaultNavIDs = []string{"dashboard", "tabmanager", "tutorial", "settings"}
var hideableDefaultNavIDs = map[string]bool{"dashboard": true, "tutorial": true, "settings": true}

type Tab struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HTML      string `json:"html"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Layout struct {
	Order          []string `json:"order,omitempty"`
	HiddenDefaults []string `json:"hiddenDefaults,omitempty"`
}

type fileData struct {
	SchemaVersion int    `json:"schemaVersion"`
	Tabs          []Tab  `json:"tabs"`
	Layout        Layout `json:"layout,omitempty"`
}

type Store struct {
	mu     sync.RWMutex
	dir    string
	path   string
	tabs   map[string]Tab
	layout Layout
}

func DefaultDirectory() string {
	if v := strings.TrimSpace(os.Getenv("JACOB_DATA_DIR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("EDBRIDGE_DATA_DIR")); v != "" {
		return v
	}
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		return filepath.Join(d, "JACoB")
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".jacob")
	}
	return ".jacob"
}

func New(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		dir = DefaultDirectory()
	}
	s := &Store{dir: dir, path: filepath.Join(dir, "custom-tabs.json"), tabs: map[string]Tab{}}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Directory() string { return s.dir }
func (s *Store) Path() string      { return s.path }

func (s *Store) List() []Tab {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listLocked()
}

func (s *Store) listLocked() []Tab {
	out := make([]Tab, 0, len(s.tabs))
	seen := map[string]bool{}
	for _, navID := range s.normalizedLayoutLocked().Order {
		if !strings.HasPrefix(navID, "custom-") {
			continue
		}
		id := strings.TrimPrefix(navID, "custom-")
		if t, ok := s.tabs[id]; ok {
			out = append(out, t)
			seen[id] = true
		}
	}
	remaining := make([]Tab, 0, len(s.tabs)-len(out))
	for id, t := range s.tabs {
		if !seen[id] {
			remaining = append(remaining, t)
		}
	}
	sort.Slice(remaining, func(i, j int) bool {
		if remaining[i].CreatedAt == remaining[j].CreatedAt {
			return strings.ToLower(remaining[i].Name) < strings.ToLower(remaining[j].Name)
		}
		return remaining[i].CreatedAt < remaining[j].CreatedAt
	})
	return append(out, remaining...)
}

func (s *Store) Get(id string) (Tab, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tabs[id]
	return t, ok
}

func (s *Store) Layout() Layout {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneLayout(s.normalizedLayoutLocked())
}

func (s *Store) SaveLayout(order, hiddenDefaults []string) (Layout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.layout
	s.layout = Layout{Order: append([]string(nil), order...), HiddenDefaults: append([]string(nil), hiddenDefaults...)}
	s.layout = s.normalizedLayoutLocked()
	if err := s.persistLocked(); err != nil {
		s.layout = old
		return Layout{}, err
	}
	return cloneLayout(s.layout), nil
}

func (s *Store) Save(id, name, html string) (Tab, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Tab{}, errors.New("tab name is required")
	}
	if len(name) > 80 {
		return Tab{}, errors.New("tab name must be 80 characters or fewer")
	}
	if strings.TrimSpace(html) == "" {
		return Tab{}, errors.New("tab HTML is required")
	}
	if len([]byte(html)) > maxHTMLBytes {
		return Tab{}, fmt.Errorf("tab HTML exceeds %d byte limit", maxHTMLBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if id == "" {
		id = randomID()
		for {
			if _, exists := s.tabs[id]; !exists {
				break
			}
			id = randomID()
		}
		t := Tab{ID: id, Name: name, HTML: html, CreatedAt: now, UpdatedAt: now}
		s.tabs[id] = t
		s.layout = s.normalizedLayoutLocked()
		if err := s.persistLocked(); err != nil {
			delete(s.tabs, id)
			return Tab{}, err
		}
		return t, nil
	}

	old, ok := s.tabs[id]
	if !ok {
		return Tab{}, errors.New("saved tab not found")
	}
	t := old
	t.Name = name
	t.HTML = html
	t.UpdatedAt = now
	s.tabs[id] = t
	if err := s.persistLocked(); err != nil {
		s.tabs[id] = old
		return Tab{}, err
	}
	return t, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.tabs[id]
	if !ok {
		return errors.New("saved tab not found")
	}
	oldLayout := s.layout
	delete(s.tabs, id)
	s.layout = s.normalizedLayoutLocked()
	if err := s.persistLocked(); err != nil {
		s.tabs[id] = old
		s.layout = oldLayout
		return err
	}
	return nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var f fileData
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("parse custom tabs store: %w", err)
	}
	for _, t := range f.Tabs {
		if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.HTML) == "" {
			continue
		}
		s.tabs[t.ID] = t
	}
	s.layout = f.Layout
	s.layout = s.normalizedLayoutLocked()
	return nil
}

func (s *Store) normalizedLayoutLocked() Layout {
	valid := map[string]bool{}
	for _, id := range defaultNavIDs {
		valid[id] = true
	}
	for id := range s.tabs {
		valid["custom-"+id] = true
	}

	order := make([]string, 0, len(valid))
	seen := map[string]bool{}
	for _, id := range s.layout.Order {
		id = strings.TrimSpace(id)
		if valid[id] && !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	// New installs keep custom tools between Home and the management pages.
	if len(order) == 0 {
		order = append(order, "dashboard")
		seen["dashboard"] = true
		ids := make([]string, 0, len(s.tabs))
		for id := range s.tabs {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return s.tabs[ids[i]].CreatedAt < s.tabs[ids[j]].CreatedAt })
		for _, id := range ids {
			navID := "custom-" + id
			order = append(order, navID)
			seen[navID] = true
		}
	}
	// Append anything not yet represented. This also handles tabs created after a saved layout.
	remainingCustom := make([]Tab, 0)
	for id, t := range s.tabs {
		if !seen["custom-"+id] {
			remainingCustom = append(remainingCustom, t)
		}
	}
	sort.Slice(remainingCustom, func(i, j int) bool { return remainingCustom[i].CreatedAt < remainingCustom[j].CreatedAt })
	for _, t := range remainingCustom {
		navID := "custom-" + t.ID
		order = append(order, navID)
		seen[navID] = true
	}
	for _, id := range defaultNavIDs {
		if !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}

	hidden := make([]string, 0, len(s.layout.HiddenDefaults))
	hiddenSeen := map[string]bool{}
	for _, id := range s.layout.HiddenDefaults {
		id = strings.TrimSpace(id)
		if hideableDefaultNavIDs[id] && !hiddenSeen[id] {
			hidden = append(hidden, id)
			hiddenSeen[id] = true
		}
	}
	return Layout{Order: order, HiddenDefaults: hidden}
}

func cloneLayout(in Layout) Layout {
	return Layout{Order: append([]string(nil), in.Order...), HiddenDefaults: append([]string(nil), in.HiddenDefaults...)}
}

func (s *Store) persistLocked() error {
	tabs := make([]Tab, 0, len(s.tabs))
	for _, t := range s.tabs {
		tabs = append(tabs, t)
	}
	sort.Slice(tabs, func(i, j int) bool { return tabs[i].CreatedAt < tabs[j].CreatedAt })
	b, err := json.MarshalIndent(fileData{SchemaVersion: 2, Tabs: tabs, Layout: s.normalizedLayoutLocked()}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(s.dir, "custom-tabs-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	backup := s.path + ".bak"
	_ = os.Remove(backup)
	hadOld := false
	if _, err := os.Stat(s.path); err == nil {
		if err := os.Rename(s.path, backup); err != nil {
			return err
		}
		hadOld = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		if hadOld {
			_ = os.Rename(backup, s.path)
		}
		return err
	}
	if hadOld {
		_ = os.Remove(backup)
	}
	ok = true
	return nil
}

func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("tab-%d", time.Now().UnixNano())
	}
	return "tab-" + hex.EncodeToString(b[:])
}
