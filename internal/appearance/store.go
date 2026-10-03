package appearance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxThemeBytes = 512 * 1024

type Store struct {
	mu   sync.RWMutex
	path string
	html string
}

func New(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "ui-theme.html")}
	b, err := os.ReadFile(s.path)
	if err == nil {
		s.html = string(b)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

func (s *Store) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.html
}

func (s *Store) Save(html string) error {
	if len([]byte(html)) > maxThemeBytes {
		return errors.New("theme HTML is too large")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.WriteFile(s.path+".tmp", []byte(html), 0o600); err != nil {
		return err
	}
	_ = os.Remove(s.path)
	if err := os.Rename(s.path+".tmp", s.path); err != nil {
		_ = os.Remove(s.path + ".tmp")
		return err
	}
	s.html = html
	return nil
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.html = ""
	return nil
}
