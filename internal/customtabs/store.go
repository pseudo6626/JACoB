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

type Tab struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HTML      string `json:"html"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type fileData struct {
	SchemaVersion int   `json:"schemaVersion"`
	Tabs          []Tab `json:"tabs"`
}

type Store struct {
	mu   sync.RWMutex
	dir  string
	path string
	tabs map[string]Tab
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
	out := make([]Tab, 0, len(s.tabs))
	for _, t := range s.tabs {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		leftName, rightName := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if leftName == rightName {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return leftName < rightName
	})
	return out
}

func (s *Store) Get(id string) (Tab, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tabs[id]
	return t, ok
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
	delete(s.tabs, id)
	if err := s.persistLocked(); err != nil {
		s.tabs[id] = old
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
	return nil
}

func (s *Store) persistLocked() error {
	tabs := make([]Tab, 0, len(s.tabs))
	for _, t := range s.tabs {
		tabs = append(tabs, t)
	}
	sort.Slice(tabs, func(i, j int) bool { return tabs[i].CreatedAt < tabs[j].CreatedAt })
	b, err := json.MarshalIndent(fileData{SchemaVersion: 1, Tabs: tabs}, "", "  ")
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
