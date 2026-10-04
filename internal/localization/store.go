package localization

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const DefaultLanguage = "en"

type Language struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	NativeName string `json:"nativeName"`
}

var supported = []Language{
	{Code: "en", Name: "English", NativeName: "English"},
	{Code: "ru", Name: "Russian", NativeName: "Русский"},
	{Code: "de", Name: "German", NativeName: "Deutsch"},
	{Code: "fr", Name: "French", NativeName: "Français"},
	{Code: "zh-CN", Name: "Simplified Chinese", NativeName: "简体中文"},
	{Code: "es", Name: "Spanish", NativeName: "Español"},
}

type fileData struct {
	SchemaVersion int    `json:"schemaVersion"`
	Language      string `json:"language"`
}

type Store struct {
	mu       sync.RWMutex
	path     string
	language string
}

func Supported() []Language {
	out := make([]Language, len(supported))
	copy(out, supported)
	return out
}

func Normalize(code string) string {
	code = strings.TrimSpace(code)
	for _, lang := range supported {
		if strings.EqualFold(lang.Code, code) {
			return lang.Code
		}
	}
	switch strings.ToLower(code) {
	case "zh", "zh-cn", "zh_hans", "zh-hans", "zh_hans_cn", "zh-hans-cn":
		return "zh-CN"
	case "es-es", "es_es":
		return "es"
	}
	return ""
}

func New(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "locale.json"), language: DefaultLanguage}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f fileData
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if lang := Normalize(f.Language); lang != "" {
		s.language = lang
	}
	return s, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.language
}

func (s *Store) Set(code string) error {
	code = Normalize(code)
	if code == "" {
		return errors.New("unsupported language")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(fileData{SchemaVersion: 1, Language: code}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	_ = os.Remove(s.path)
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	s.language = code
	return nil
}

func (s *Store) Info() map[string]any {
	language := DefaultLanguage
	if s != nil {
		language = s.Get()
	}
	return map[string]any{"language": language, "supported": Supported()}
}
