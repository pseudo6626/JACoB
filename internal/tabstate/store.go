package tabstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	maxKeyBytes      = 128
	maxValueBytes    = 512 * 1024
	maxTabStateBytes = 2 * 1024 * 1024
)

type fileData struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Tabs          map[string]map[string]any `json:"tabs"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	tabs map[string]map[string]any
}

func New(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "tab-state.json"), tabs: map[string]map[string]any{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Path() string { return s.path }

func validID(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 160 {
		return false
	}
	for _, r := range v {
		if !(r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

func validKey(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && len([]byte(v)) <= maxKeyBytes
}

func clone(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

func (s *Store) Get(tabID, key string) (any, bool) {
	if !validID(tabID) || !validKey(key) {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket := s.tabs[tabID]
	if bucket == nil {
		return nil, false
	}
	v, ok := bucket[key]
	if !ok {
		return nil, false
	}
	return clone(v), true
}

func (s *Store) Set(tabID, key string, value any) error {
	if !validID(tabID) {
		return errors.New("invalid tab id")
	}
	if !validKey(key) {
		return errors.New("state key is required and must be 128 bytes or fewer")
	}
	vb, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode state value: %w", err)
	}
	if len(vb) > maxValueBytes {
		return fmt.Errorf("state value exceeds %d byte limit", maxValueBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	old := cloneBucket(s.tabs[tabID])
	if s.tabs[tabID] == nil {
		s.tabs[tabID] = map[string]any{}
	}
	s.tabs[tabID][key] = clone(value)
	if err := s.checkTabLimitLocked(tabID); err != nil {
		if old == nil {
			delete(s.tabs, tabID)
		} else {
			s.tabs[tabID] = old
		}
		return err
	}
	if err := s.persistLocked(); err != nil {
		if old == nil {
			delete(s.tabs, tabID)
		} else {
			s.tabs[tabID] = old
		}
		return err
	}
	return nil
}

func (s *Store) Delete(tabID, key string) error {
	if !validID(tabID) || !validKey(key) {
		return errors.New("invalid tab id or state key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket := s.tabs[tabID]
	if bucket == nil {
		return nil
	}
	old := cloneBucket(bucket)
	delete(bucket, key)
	if len(bucket) == 0 {
		delete(s.tabs, tabID)
	}
	if err := s.persistLocked(); err != nil {
		s.tabs[tabID] = old
		return err
	}
	return nil
}

func (s *Store) Clear(tabID string) error {
	if !validID(tabID) {
		return errors.New("invalid tab id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := cloneBucket(s.tabs[tabID])
	delete(s.tabs, tabID)
	if err := s.persistLocked(); err != nil {
		if old != nil {
			s.tabs[tabID] = old
		}
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
		return fmt.Errorf("parse tab state store: %w", err)
	}
	if f.Tabs != nil {
		s.tabs = f.Tabs
	}
	return nil
}

func (s *Store) checkTabLimitLocked(tabID string) error {
	b, err := json.Marshal(s.tabs[tabID])
	if err != nil {
		return err
	}
	if len(b) > maxTabStateBytes {
		return fmt.Errorf("tab state exceeds %d byte limit", maxTabStateBytes)
	}
	return nil
}

func (s *Store) persistLocked() error {
	data := fileData{SchemaVersion: 1, Tabs: s.tabs}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	_ = os.Remove(s.path)
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func cloneBucket(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = clone(v)
	}
	return out
}
