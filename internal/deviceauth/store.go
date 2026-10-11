package deviceauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	schemaVersion       = 1
	temporaryLifetime   = 24 * time.Hour
	rememberedLifetime  = 180 * 24 * time.Hour
	maxDeviceNameLength = 80
)

type Device struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Remember   bool      `json:"remember"`
	Admin      bool      `json:"admin"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

type record struct {
	Device
	SecretHash string `json:"secretHash"`
}

type diskFile struct {
	SchemaVersion int      `json:"schemaVersion"`
	Devices       []record `json:"devices"`
}

type Store struct {
	mu      sync.Mutex
	path    string
	devices map[string]record
}

func FilePath(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Join(dataDir, "devices.json")
}

func New(dataDir string) (*Store, error) {
	s := &Store{path: FilePath(dataDir), devices: map[string]record{}}
	if s.path == "" {
		return s, nil
	}
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var disk diskFile
	if err := json.Unmarshal(b, &disk); err != nil {
		return nil, fmt.Errorf("read remembered devices: %w", err)
	}
	if disk.SchemaVersion != 0 && disk.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("unsupported remembered-device schema %d", disk.SchemaVersion)
	}
	now := time.Now().UTC()
	for _, rec := range disk.Devices {
		if strings.TrimSpace(rec.ID) == "" || strings.TrimSpace(rec.SecretHash) == "" || !rec.Remember {
			continue
		}
		if expired(rec, now) {
			continue
		}
		rec.Name = normalizeName(rec.Name)
		s.devices[rec.ID] = rec
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Enroll(name string) (Device, string, error) {
	if s == nil {
		return Device{}, "", fmt.Errorf("device store unavailable")
	}
	id, err := randomToken(16)
	if err != nil {
		return Device{}, "", err
	}
	secret, err := randomToken(32)
	if err != nil {
		return Device{}, "", err
	}
	now := time.Now().UTC()
	rec := record{Device: Device{ID: id, Name: normalizeName(name), CreatedAt: now, LastSeenAt: now}, SecretHash: secretDigest(secret)}
	s.mu.Lock()
	s.devices[id] = rec
	s.mu.Unlock()
	return rec.Device, id + "." + secret, nil
}

func (s *Store) Authenticate(credential string) (Device, bool) {
	if s == nil {
		return Device{}, false
	}
	id, secret, ok := splitCredential(credential)
	if !ok {
		return Device{}, false
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[id]
	if !ok || expired(rec, now) {
		if ok {
			delete(s.devices, id)
			if rec.Remember {
				_ = s.saveLocked()
			}
		}
		return Device{}, false
	}
	got := secretDigest(secret)
	if subtle.ConstantTimeCompare([]byte(got), []byte(rec.SecretHash)) != 1 {
		return Device{}, false
	}
	// Persist remembered last-seen timestamps at most once every five minutes.
	persist := rec.Remember && (rec.LastSeenAt.IsZero() || now.Sub(rec.LastSeenAt) >= 5*time.Minute)
	rec.LastSeenAt = now
	s.devices[id] = rec
	if persist {
		_ = s.saveLocked()
	}
	return rec.Device, true
}

func (s *Store) Get(id string) (Device, bool) {
	if s == nil {
		return Device{}, false
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[id]
	if !ok || expired(rec, now) {
		if ok {
			delete(s.devices, id)
			if rec.Remember {
				_ = s.saveLocked()
			}
		}
		return Device{}, false
	}
	return rec.Device, true
}

func (s *Store) List() []Device {
	if s == nil {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, 0, len(s.devices))
	changedPersistent := false
	for id, rec := range s.devices {
		if expired(rec, now) {
			delete(s.devices, id)
			changedPersistent = changedPersistent || rec.Remember
			continue
		}
		out = append(out, rec.Device)
	}
	if changedPersistent {
		_ = s.saveLocked()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastSeenAt.Equal(out[j].LastSeenAt) {
			return out[i].Name < out[j].Name
		}
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	return out
}

func (s *Store) SetRemember(id string, value bool) (Device, error) {
	if s == nil {
		return Device{}, fmt.Errorf("device store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[id]
	if !ok {
		return Device{}, fmt.Errorf("paired device was not found")
	}
	before := rec
	rec.Remember = value
	s.devices[id] = rec
	if err := s.saveLocked(); err != nil {
		s.devices[id] = before
		return Device{}, err
	}
	return rec.Device, nil
}

func (s *Store) SetAdmin(id string, value bool) (Device, error) {
	if s == nil {
		return Device{}, fmt.Errorf("device store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[id]
	if !ok {
		return Device{}, fmt.Errorf("paired device was not found")
	}
	before := rec
	rec.Admin = value
	s.devices[id] = rec
	if rec.Remember {
		if err := s.saveLocked(); err != nil {
			s.devices[id] = before
			return Device{}, err
		}
	}
	return rec.Device, nil
}

func (s *Store) Revoke(id string) error {
	if s == nil {
		return fmt.Errorf("device store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[id]
	if !ok {
		return nil
	}
	delete(s.devices, id)
	if rec.Remember {
		if err := s.saveLocked(); err != nil {
			s.devices[id] = rec
			return err
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	remembered := make([]record, 0, len(s.devices))
	for _, rec := range s.devices {
		if rec.Remember {
			remembered = append(remembered, rec)
		}
	}
	sort.Slice(remembered, func(i, j int) bool { return remembered[i].CreatedAt.Before(remembered[j].CreatedAt) })
	b, err := json.MarshalIndent(diskFile{SchemaVersion: schemaVersion, Devices: remembered}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := s.path + ".tmp"
	backup := s.path + ".previous"
	_ = os.Remove(tmp)
	_ = os.Remove(backup)
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	hadPrevious := false
	if _, err := os.Stat(s.path); err == nil {
		if err := os.Rename(s.path, backup); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		hadPrevious = true
	} else if !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		if hadPrevious {
			_ = os.Rename(backup, s.path)
		}
		_ = os.Remove(tmp)
		return err
	}
	if hadPrevious {
		_ = os.Remove(backup)
	}
	return nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func secretDigest(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func splitCredential(v string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(v), ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func normalizeName(v string) string {
	v = strings.Join(strings.Fields(strings.TrimSpace(v)), " ")
	if v == "" {
		v = "Browser device"
	}
	runes := []rune(v)
	if len(runes) > maxDeviceNameLength {
		v = string(runes[:maxDeviceNameLength])
	}
	return v
}

func expired(rec record, now time.Time) bool {
	last := rec.LastSeenAt
	if last.IsZero() {
		last = rec.CreatedAt
	}
	if last.IsZero() {
		return true
	}
	limit := temporaryLifetime
	if rec.Remember {
		limit = rememberedLifetime
	}
	return now.Sub(last) > limit
}
