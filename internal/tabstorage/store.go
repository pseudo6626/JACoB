package tabstorage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultQuotaBytes int64 = 128 * 1024 * 1024
	MaxQuotaBytes     int64 = 1024 * 1024 * 1024
	MaxObjectBytes    int64 = 32 * 1024 * 1024
	ChunkBytes              = 512 * 1024
	MaxListItems            = 500
	MaxBatchItems           = 64
	MaxBatchBytes     int64 = 2 * 1024 * 1024
)

var (
	ErrNotFound       = errors.New("storage object not found")
	ErrQuotaExceeded  = errors.New("storage quota exceeded")
	ErrInvalidName    = errors.New("invalid storage object name")
	ErrObjectTooLarge = errors.New("storage object exceeds size limit")
	ErrUploadNotFound = errors.New("storage upload not found")
	ErrBadOffset      = errors.New("storage upload offset mismatch")
)

type Item struct {
	Name       string `json:"name"`
	SizeBytes  int64  `json:"sizeBytes"`
	Type       string `json:"type"`
	ModifiedAt string `json:"modifiedAt"`
}

type Info struct {
	UsedBytes      int64 `json:"usedBytes"`
	QuotaBytes     int64 `json:"quotaBytes"`
	Items          int   `json:"items"`
	MaxObjectBytes int64 `json:"maxObjectBytes"`
	ChunkBytes     int   `json:"chunkBytes"`
}

type ListResult struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	Truncated  bool   `json:"truncated"`
}

type ReadResult struct {
	Item   Item   `json:"item"`
	Offset int64  `json:"offset"`
	Data   []byte `json:"-"`
	EOF    bool   `json:"eof"`
}

type UploadInfo struct {
	UploadID       string `json:"uploadId"`
	ChunkBytes     int    `json:"chunkBytes"`
	MaxObjectBytes int64  `json:"maxObjectBytes"`
}

type configFile struct {
	Schema     int   `json:"schema"`
	QuotaBytes int64 `json:"quotaBytes"`
}

type objectMeta struct {
	Type string `json:"type,omitempty"`
}

type upload struct {
	tabID    string
	name     string
	typeName string
	tempPath string
	file     *os.File
	expected int64
	written  int64
	started  time.Time
}

type Store struct {
	mu      sync.Mutex
	root    string
	uploads map[string]*upload
}

func New(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, errors.New("data directory is required")
	}
	root := filepath.Join(dataDir, "tab-storage")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	s := &Store{root: root, uploads: map[string]*upload{}}
	_ = s.cleanupStaleUploads()
	return s, nil
}

func (s *Store) Path() string { return s.root }

func validTabID(v string) bool {
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

func validObjectName(name string) bool {
	if name == "" || name != strings.TrimSpace(name) || len([]byte(name)) > 512 || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.ContainsRune(name, 0) {
		return false
	}
	clean := path.Clean(name)
	if clean != name || clean == "." || strings.HasPrefix(clean, "../") {
		return false
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." || len([]byte(part)) > 180 || strings.HasPrefix(strings.ToLower(part), ".jacob") || strings.ContainsAny(part, `<>:"|?*`) || strings.HasSuffix(part, " ") || strings.HasSuffix(part, ".") || windowsReserved(part) {
			return false
		}
	}
	return true
}

func windowsReserved(part string) bool {
	base := strings.ToUpper(strings.TrimSuffix(part, filepath.Ext(part)))
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return true
	}
	for _, prefix := range []string{"COM", "LPT"} {
		if strings.HasPrefix(base, prefix) && len(base) == 4 && base[3] >= '1' && base[3] <= '9' {
			return true
		}
	}
	return false
}

func (s *Store) tabRoot(tabID string, create bool) (string, error) {
	if !validTabID(tabID) {
		return "", errors.New("invalid tab id")
	}
	root := filepath.Join(s.root, tabID)
	if create {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return "", err
		}
	}
	if info, err := os.Lstat(root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", errors.New("tab storage root is not a directory")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if !create {
			return "", os.ErrNotExist
		}
	} else {
		return "", err
	}
	return root, nil
}

func (s *Store) objectPath(tabID, name string, createParents bool) (string, error) {
	if !validObjectName(name) {
		return "", ErrInvalidName
	}
	root, err := s.tabRoot(tabID, createParents)
	if err != nil {
		return "", err
	}
	parts := strings.Split(name, "/")
	cur := root
	for _, part := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, part)
		info, statErr := os.Lstat(cur)
		if errors.Is(statErr, os.ErrNotExist) && createParents {
			if err := os.Mkdir(cur, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return "", err
			}
			info, statErr = os.Lstat(cur)
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", errors.New("storage path contains a non-directory or symlink")
		}
	}
	return filepath.Join(root, filepath.FromSlash(name)), nil
}

func metaPath(objectPath string) string { return objectPath + ".jacobmeta" }
func configPath(root string) string     { return filepath.Join(root, ".jacob-storage.json") }

func normalizeType(v, name string) string {
	v = strings.TrimSpace(v)
	if len(v) > 160 || strings.ContainsAny(v, "\r\n") {
		v = ""
	}
	if v == "" {
		v = mime.TypeByExtension(filepath.Ext(name))
	}
	if v == "" {
		v = "application/octet-stream"
	}
	return v
}

func (s *Store) quotaLocked(tabID string) (int64, error) {
	root, err := s.tabRoot(tabID, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultQuotaBytes, nil
		}
		return 0, err
	}
	b, err := os.ReadFile(configPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return DefaultQuotaBytes, nil
	}
	if err != nil {
		return 0, err
	}
	var cfg configFile
	if json.Unmarshal(b, &cfg) != nil || !allowedQuota(cfg.QuotaBytes) {
		return DefaultQuotaBytes, nil
	}
	return cfg.QuotaBytes, nil
}

func allowedQuota(v int64) bool {
	switch v {
	case 128 * 1024 * 1024, 256 * 1024 * 1024, 512 * 1024 * 1024, 1024 * 1024 * 1024:
		return true
	default:
		return false
	}
}

func (s *Store) writeConfigLocked(tabID string, quota int64) error {
	root, err := s.tabRoot(tabID, true)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(configFile{Schema: 1, QuotaBytes: quota}, "", "  ")
	tmp := configPath(root) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, configPath(root)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func isInternal(rel string) bool {
	base := filepath.Base(rel)
	return strings.HasPrefix(base, ".jacob") || strings.Contains(base, ".jacobmeta") || strings.HasSuffix(base, ".jacob-backup") || strings.HasPrefix(rel, ".uploads/") || rel == ".uploads" || strings.Contains(base, ".jacob-upload-")
}

func (s *Store) usageLocked(tabID string) (int64, int, error) {
	root, err := s.tabRoot(tabID, false)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	var used int64
	count := 0
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".uploads" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || isInternal(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		used += info.Size()
		count++
		return nil
	})
	return used, count, err
}

func (s *Store) Info(tabID string) (Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	used, count, err := s.usageLocked(tabID)
	if err != nil {
		return Info{}, err
	}
	quota, err := s.quotaLocked(tabID)
	if err != nil {
		return Info{}, err
	}
	return Info{UsedBytes: used, QuotaBytes: quota, Items: count, MaxObjectBytes: MaxObjectBytes, ChunkBytes: ChunkBytes}, nil
}

func (s *Store) SetQuota(tabID string, quota int64) (Info, error) {
	if !allowedQuota(quota) || quota > MaxQuotaBytes {
		return Info{}, errors.New("storage quota must be 128, 256, 512, or 1024 MiB")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	used, count, err := s.usageLocked(tabID)
	if err != nil {
		return Info{}, err
	}
	if used > quota {
		return Info{}, fmt.Errorf("%w: current data uses %d bytes", ErrQuotaExceeded, used)
	}
	if err := s.writeConfigLocked(tabID, quota); err != nil {
		return Info{}, err
	}
	return Info{UsedBytes: used, QuotaBytes: quota, Items: count, MaxObjectBytes: MaxObjectBytes, ChunkBytes: ChunkBytes}, nil
}

func (s *Store) itemLocked(tabID, name string) (Item, error) {
	p, err := s.objectPath(tabID, name, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Item{}, ErrNotFound
		}
		return Item{}, err
	}
	info, err := os.Lstat(p)
	if errors.Is(err, os.ErrNotExist) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Item{}, ErrNotFound
	}
	typeName := ""
	if b, err := os.ReadFile(metaPath(p)); err == nil {
		var meta objectMeta
		if json.Unmarshal(b, &meta) == nil {
			typeName = meta.Type
		}
	}
	return Item{Name: name, SizeBytes: info.Size(), Type: normalizeType(typeName, name), ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano)}, nil
}

func (s *Store) Read(tabID, name string, offset int64, limit int) (ReadResult, error) {
	if offset < 0 {
		return ReadResult{}, ErrBadOffset
	}
	if limit <= 0 || limit > ChunkBytes {
		limit = ChunkBytes
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.itemLocked(tabID, name)
	if err != nil {
		return ReadResult{}, err
	}
	if offset > item.SizeBytes {
		return ReadResult{}, ErrBadOffset
	}
	p, _ := s.objectPath(tabID, name, false)
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ReadResult{}, ErrNotFound
		}
		return ReadResult{}, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ReadResult{}, err
	}
	want := int64(limit)
	if remain := item.SizeBytes - offset; remain < want {
		want = remain
	}
	buf := make([]byte, int(want))
	if want > 0 {
		if _, err := io.ReadFull(f, buf); err != nil {
			return ReadResult{}, err
		}
	}
	return ReadResult{Item: item, Offset: offset, Data: buf, EOF: offset+int64(len(buf)) >= item.SizeBytes}, nil
}

func (s *Store) List(tabID, prefix, cursor string, limit int) (ListResult, error) {
	if prefix != "" && (strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "\\") || strings.Contains(prefix, "..")) {
		return ListResult{}, ErrInvalidName
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > MaxListItems {
		limit = MaxListItems
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.tabRoot(tabID, false)
	if errors.Is(err, os.ErrNotExist) {
		return ListResult{Items: []Item{}}, nil
	}
	if err != nil {
		return ListResult{}, err
	}
	names := []string{}
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".uploads" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || isInternal(rel) || !strings.HasPrefix(rel, prefix) || (cursor != "" && rel <= cursor) {
			return nil
		}
		names = append(names, rel)
		return nil
	})
	if err != nil {
		return ListResult{}, err
	}
	sort.Strings(names)
	truncated := len(names) > limit
	if truncated {
		names = names[:limit]
	}
	items := make([]Item, 0, len(names))
	for _, name := range names {
		item, err := s.itemLocked(tabID, name)
		if err == nil {
			items = append(items, item)
		}
	}
	result := ListResult{Items: items, Truncated: truncated}
	if truncated && len(items) > 0 {
		result.NextCursor = items[len(items)-1].Name
	}
	return result, nil
}

func (s *Store) Begin(tabID, name, typeName string, expected int64) (UploadInfo, error) {
	if expected < 0 || expected > MaxObjectBytes {
		return UploadInfo{}, ErrObjectTooLarge
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked()
	p, err := s.objectPath(tabID, name, true)
	if err != nil {
		return UploadInfo{}, err
	}
	used, _, err := s.usageLocked(tabID)
	if err != nil {
		return UploadInfo{}, err
	}
	quota, err := s.quotaLocked(tabID)
	if err != nil {
		return UploadInfo{}, err
	}
	oldSize := int64(0)
	if info, err := os.Lstat(p); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		oldSize = info.Size()
	}
	if used-oldSize+expected > quota {
		return UploadInfo{}, fmt.Errorf("%w: write would use %d of %d bytes", ErrQuotaExceeded, used-oldSize+expected, quota)
	}
	root, _ := s.tabRoot(tabID, true)
	uploadDir := filepath.Join(root, ".uploads")
	if err := os.MkdirAll(uploadDir, 0o700); err != nil {
		return UploadInfo{}, err
	}
	id, err := randomID()
	if err != nil {
		return UploadInfo{}, err
	}
	temp := filepath.Join(uploadDir, id+".part")
	f, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return UploadInfo{}, err
	}
	s.uploads[id] = &upload{tabID: tabID, name: name, typeName: normalizeType(typeName, name), tempPath: temp, file: f, expected: expected, started: time.Now()}
	return UploadInfo{UploadID: id, ChunkBytes: ChunkBytes, MaxObjectBytes: MaxObjectBytes}, nil
}

func (s *Store) WriteChunk(tabID, uploadID string, offset int64, data []byte) error {
	if len(data) > ChunkBytes {
		return ErrObjectTooLarge
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.uploads[uploadID]
	if u == nil || u.tabID != tabID {
		return ErrUploadNotFound
	}
	if offset != u.written {
		return ErrBadOffset
	}
	if u.written+int64(len(data)) > u.expected || u.written+int64(len(data)) > MaxObjectBytes {
		return ErrObjectTooLarge
	}
	if len(data) > 0 {
		n, err := u.file.Write(data)
		if err != nil {
			return err
		}
		if n != len(data) {
			return io.ErrShortWrite
		}
		u.written += int64(n)
	}
	return nil
}

func (s *Store) Commit(tabID, uploadID string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.uploads[uploadID]
	if u == nil || u.tabID != tabID {
		return Item{}, ErrUploadNotFound
	}
	delete(s.uploads, uploadID)
	cleanup := func() { _ = u.file.Close(); _ = os.Remove(u.tempPath) }
	if u.written != u.expected {
		cleanup()
		return Item{}, fmt.Errorf("%w: received %d of %d bytes", ErrBadOffset, u.written, u.expected)
	}
	if err := u.file.Sync(); err != nil {
		cleanup()
		return Item{}, err
	}
	if err := u.file.Close(); err != nil {
		_ = os.Remove(u.tempPath)
		return Item{}, err
	}
	dest, err := s.objectPath(tabID, u.name, true)
	if err != nil {
		_ = os.Remove(u.tempPath)
		return Item{}, err
	}
	used, _, err := s.usageLocked(tabID)
	if err != nil {
		_ = os.Remove(u.tempPath)
		return Item{}, err
	}
	quota, err := s.quotaLocked(tabID)
	if err != nil {
		_ = os.Remove(u.tempPath)
		return Item{}, err
	}
	oldSize := int64(0)
	if info, err := os.Lstat(dest); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		oldSize = info.Size()
	}
	if used-oldSize+u.expected > quota {
		_ = os.Remove(u.tempPath)
		return Item{}, fmt.Errorf("%w: write would use %d of %d bytes", ErrQuotaExceeded, used-oldSize+u.expected, quota)
	}
	if err := replaceFile(u.tempPath, dest); err != nil {
		_ = os.Remove(u.tempPath)
		return Item{}, err
	}
	metaBytes, _ := json.Marshal(objectMeta{Type: u.typeName})
	metaTmp := metaPath(dest) + ".tmp"
	if err := os.WriteFile(metaTmp, metaBytes, 0o600); err == nil {
		if err := replaceFile(metaTmp, metaPath(dest)); err != nil {
			_ = os.Remove(metaTmp)
		}
	} else {
		_ = os.Remove(metaTmp)
	}
	return s.itemLocked(tabID, u.name)
}

func replaceFile(temp, dest string) error {
	if err := os.Rename(temp, dest); err == nil {
		return nil
	}
	backup := dest + ".jacob-backup"
	_ = os.Remove(backup)
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
		if err := os.Rename(temp, dest); err != nil {
			_ = os.Rename(backup, dest)
			return err
		}
		_ = os.Remove(backup)
		return nil
	}
	return os.Rename(temp, dest)
}

func (s *Store) Abort(tabID, uploadID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.uploads[uploadID]
	if u == nil || u.tabID != tabID {
		return ErrUploadNotFound
	}
	delete(s.uploads, uploadID)
	_ = u.file.Close()
	return os.Remove(u.tempPath)
}

func (s *Store) PutBytes(tabID, name, typeName string, data []byte) (Item, error) {
	if int64(len(data)) > MaxObjectBytes {
		return Item{}, ErrObjectTooLarge
	}
	begin, err := s.Begin(tabID, name, typeName, int64(len(data)))
	if err != nil {
		return Item{}, err
	}
	for offset := 0; offset < len(data); {
		end := offset + ChunkBytes
		if end > len(data) {
			end = len(data)
		}
		if err := s.WriteChunk(tabID, begin.UploadID, int64(offset), data[offset:end]); err != nil {
			_ = s.Abort(tabID, begin.UploadID)
			return Item{}, err
		}
		offset = end
	}
	return s.Commit(tabID, begin.UploadID)
}

func (s *Store) Delete(tabID, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.objectPath(tabID, name, false)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, ErrNotFound
	}
	if err := os.Remove(p); err != nil {
		return false, err
	}
	_ = os.Remove(metaPath(p))
	return true, nil
}

func (s *Store) Clear(tabID string) error {
	if !validTabID(tabID) {
		return errors.New("invalid tab id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, u := range s.uploads {
		if u.tabID == tabID {
			_ = u.file.Close()
			_ = os.Remove(u.tempPath)
			delete(s.uploads, id)
		}
	}
	return os.RemoveAll(filepath.Join(s.root, tabID))
}

func (s *Store) cleanupExpiredLocked() {
	cutoff := time.Now().Add(-15 * time.Minute)
	for id, u := range s.uploads {
		if u.started.Before(cutoff) {
			_ = u.file.Close()
			_ = os.Remove(u.tempPath)
			delete(s.uploads, id)
		}
	}
}

func (s *Store) cleanupStaleUploads() error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validTabID(entry.Name()) {
			continue
		}
		root := filepath.Join(s.root, entry.Name())
		_ = os.RemoveAll(filepath.Join(root, ".uploads"))
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jacob-backup") {
				return nil
			}
			original := strings.TrimSuffix(p, ".jacob-backup")
			if _, err := os.Lstat(original); errors.Is(err, os.ErrNotExist) {
				_ = os.Rename(p, original)
			} else {
				_ = os.Remove(p)
			}
			return nil
		})
	}
	return nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
