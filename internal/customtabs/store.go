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

const (
	maxHTMLBytes   = 4 * 1024 * 1024
	storeSchema    = 3
	contentDirName = "custom-tabs"
)

var defaultNavIDs = []string{"dashboard", "tabmanager", "tutorial", "settings"}
var hideableDefaultNavIDs = map[string]bool{"dashboard": true, "tutorial": true, "settings": true}

type Tab struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HTML      string `json:"html,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
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
	mu         sync.RWMutex
	dir        string
	path       string
	contentDir string
	tabs       map[string]Tab
	layout     Layout
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
	s := &Store{
		dir:        dir,
		path:       filepath.Join(dir, "custom-tabs.json"),
		contentDir: filepath.Join(dir, contentDirName),
		tabs:       map[string]Tab{},
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.contentDir, 0o700); err != nil {
		return nil, err
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Directory() string        { return s.dir }
func (s *Store) Path() string             { return s.path }
func (s *Store) ContentDirectory() string { return s.contentDir }

// List returns metadata only. HTML is intentionally omitted so navigation and
// Tab Manager refreshes stay small even when saved tabs are large.
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
			t.HTML = ""
			out = append(out, t)
			seen[id] = true
		}
	}
	remaining := make([]Tab, 0, len(s.tabs)-len(out))
	for id, t := range s.tabs {
		if !seen[id] {
			t.HTML = ""
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

// Get reads one tab body on demand. Normal listing/navigation never reads all
// saved HTML into a single response.
func (s *Store) Get(id string) (Tab, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tabs[id]
	if !ok || !validTabID(id) {
		return Tab{}, false
	}
	b, err := os.ReadFile(s.tabPath(id))
	if err != nil {
		return Tab{}, false
	}
	t.HTML = string(b)
	t.SizeBytes = int64(len(b))
	return t, true
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
	htmlBytes := []byte(html)
	if len(htmlBytes) > maxHTMLBytes {
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
		if err := replaceFileAtomic(s.tabPath(id), htmlBytes); err != nil {
			return Tab{}, err
		}
		t := Tab{ID: id, Name: name, SizeBytes: int64(len(htmlBytes)), CreatedAt: now, UpdatedAt: now}
		s.tabs[id] = t
		s.layout = s.normalizedLayoutLocked()
		if err := s.persistLocked(); err != nil {
			delete(s.tabs, id)
			_ = os.Remove(s.tabPath(id))
			return Tab{}, err
		}
		return t, nil
	}

	if !validTabID(id) {
		return Tab{}, errors.New("invalid saved tab id")
	}
	old, ok := s.tabs[id]
	if !ok {
		return Tab{}, errors.New("saved tab not found")
	}
	oldBody, readErr := os.ReadFile(s.tabPath(id))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return Tab{}, readErr
	}
	if err := replaceFileAtomic(s.tabPath(id), htmlBytes); err != nil {
		return Tab{}, err
	}
	t := old
	t.Name = name
	t.SizeBytes = int64(len(htmlBytes))
	t.UpdatedAt = now
	t.HTML = ""
	s.tabs[id] = t
	if err := s.persistLocked(); err != nil {
		s.tabs[id] = old
		if readErr == nil {
			_ = replaceFileAtomic(s.tabPath(id), oldBody)
		} else {
			_ = os.Remove(s.tabPath(id))
		}
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
	if !validTabID(id) {
		return errors.New("invalid saved tab id")
	}
	oldLayout := s.layout
	path := s.tabPath(id)
	backup := path + ".delete"
	_ = os.Remove(backup)
	hadBody := false
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			return err
		}
		hadBody = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(s.tabs, id)
	s.layout = s.normalizedLayoutLocked()
	if err := s.persistLocked(); err != nil {
		s.tabs[id] = old
		s.layout = oldLayout
		if hadBody {
			_ = os.Rename(backup, path)
		}
		return err
	}
	if hadBody {
		_ = os.Remove(backup)
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
	migrated := f.SchemaVersion < storeSchema
	for _, raw := range f.Tabs {
		if strings.TrimSpace(raw.ID) == "" || strings.TrimSpace(raw.Name) == "" || !validTabID(raw.ID) {
			continue
		}
		t := raw
		legacyHTML := t.HTML
		t.HTML = ""
		if strings.TrimSpace(legacyHTML) != "" {
			body := []byte(legacyHTML)
			if len(body) > maxHTMLBytes {
				return fmt.Errorf("saved tab %q exceeds %d byte limit", t.Name, maxHTMLBytes)
			}
			if err := replaceFileAtomic(s.tabPath(t.ID), body); err != nil {
				return fmt.Errorf("migrate saved tab %q: %w", t.Name, err)
			}
			t.SizeBytes = int64(len(body))
			migrated = true
		} else {
			info, statErr := os.Stat(s.tabPath(t.ID))
			if statErr != nil {
				// A metadata entry without its content file cannot be opened safely.
				continue
			}
			t.SizeBytes = info.Size()
		}
		s.tabs[t.ID] = t
	}
	s.layout = f.Layout
	s.layout = s.normalizedLayoutLocked()
	if migrated {
		if err := s.persistLocked(); err != nil {
			return fmt.Errorf("persist migrated custom tabs: %w", err)
		}
	}
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
	tabs := s.listLocked()
	for i := range tabs {
		tabs[i].HTML = ""
	}
	b, err := json.MarshalIndent(fileData{SchemaVersion: storeSchema, Tabs: tabs, Layout: s.normalizedLayoutLocked()}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return replaceFileAtomic(s.path, b)
}

func (s *Store) tabPath(id string) string {
	return filepath.Join(s.contentDir, id+".html")
}

func validTabID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return id != "." && id != ".."
}

func replaceFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".jacob-*.tmp")
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
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	backup := path + ".bak"
	_ = os.Remove(backup)
	hadOld := false
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			return err
		}
		hadOld = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if hadOld {
			_ = os.Rename(backup, path)
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
