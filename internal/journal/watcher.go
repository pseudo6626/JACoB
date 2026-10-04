package journal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Kind string         `json:"kind"`
	Data map[string]any `json:"data"`
}

type Watcher struct {
	Dir         string
	Replay      bool
	OnEvent     func(Event)
	mu          sync.RWMutex
	status      map[string]any
	market      map[string]any
	eliteFiles  map[string]map[string]any
	fileNames   map[string]string
	fileUpdated map[string]string
	lastEvent   map[string]any
	context     map[string]any
	journal     string
}

func New(dir string, replay bool, onEvent func(Event)) *Watcher {
	return &Watcher{
		Dir: dir, Replay: replay, OnEvent: onEvent,
		status: map[string]any{}, market: map[string]any{}, context: map[string]any{},
		eliteFiles: map[string]map[string]any{}, fileNames: map[string]string{}, fileUpdated: map[string]string{},
	}
}

func (w *Watcher) Snapshot() map[string]any {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return map[string]any{
		"journalDir":       w.Dir,
		"journalFile":      w.journal,
		"status":           cloneMap(w.status),
		"market":           cloneMap(w.market),
		"eliteFiles":       cloneMapMap(w.eliteFiles),
		"eliteFileNames":   cloneStringMap(w.fileNames),
		"eliteFileUpdated": cloneStringMap(w.fileUpdated),
		"lastJournalEvent": cloneMap(w.lastEvent),
		"journalContext":   cloneMap(w.context),
	}
}

// EliteFiles returns the currently available JSON snapshots written by Elite
// into the journal directory. Only files inside the detected journal directory
// are ever read; this is not a general filesystem API.
func (w *Watcher) EliteFiles() map[string]map[string]any {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return cloneMapMap(w.eliteFiles)
}

func (w *Watcher) EliteFile(name string) (map[string]any, string, string, bool) {
	key := normalizeEliteFileLookup(name)
	w.mu.RLock()
	defer w.mu.RUnlock()
	data, ok := w.eliteFiles[key]
	if !ok {
		return nil, "", "", false
	}
	return cloneMap(data), w.fileNames[key], w.fileUpdated[key], true
}

func (w *Watcher) EliteFileIndex() []map[string]any {
	w.mu.RLock()
	defer w.mu.RUnlock()
	keys := make([]string, 0, len(w.eliteFiles))
	for key := range w.eliteFiles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		out = append(out, map[string]any{
			"name":    key,
			"file":    w.fileNames[key],
			"updated": w.fileUpdated[key],
		})
	}
	return out
}

func (w *Watcher) Run(ctx context.Context) error {
	if w.Dir == "" {
		<-ctx.Done()
		return ctx.Err()
	}
	fileTicker := time.NewTicker(250 * time.Millisecond)
	journalTicker := time.NewTicker(250 * time.Millisecond)
	defer fileTicker.Stop()
	defer journalTicker.Stop()

	fileMods := map[string]time.Time{}
	var currentPath string
	var file *os.File
	var reader *bufio.Reader
	defer func() {
		if file != nil {
			_ = file.Close()
		}
	}()

	openLatest := func() {
		latest := latestJournal(w.Dir)
		if latest == "" || latest == currentPath {
			return
		}
		if file != nil {
			_ = file.Close()
		}
		f, err := os.Open(latest)
		if err != nil {
			return
		}
		if !w.Replay {
			w.seedContext(latest)
			_, _ = f.Seek(0, io.SeekEnd)
		}
		file = f
		reader = bufio.NewReader(file)
		currentPath = latest
		w.mu.Lock()
		w.journal = filepath.Base(latest)
		w.mu.Unlock()
		w.emit(Event{Kind: "core.journalFile", Data: map[string]any{"path": latest}})
	}

	openLatest()
	_ = w.readEliteFiles(fileMods)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-fileTicker.C:
			_ = w.readEliteFiles(fileMods)
		case <-journalTicker.C:
			latest := latestJournal(w.Dir)
			if latest != "" && latest != currentPath {
				openLatest()
			}
			if reader == nil {
				continue
			}
			for {
				line, err := reader.ReadString('\n')
				if len(strings.TrimSpace(line)) > 0 {
					var obj map[string]any
					if json.Unmarshal([]byte(line), &obj) == nil {
						w.mu.Lock()
						w.lastEvent = cloneMap(obj)
						w.applyContextLocked(obj)
						w.mu.Unlock()
						w.emit(Event{Kind: "journal", Data: obj})
					}
				}
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					break
				}
			}
		}
	}
}

func (w *Watcher) seedContext(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	s.Buffer(buf, 2*1024*1024)
	seed := map[string]any{}
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}
		applyContext(seed, obj)
	}
	if len(seed) == 0 {
		return
	}
	w.mu.Lock()
	for k, v := range seed {
		w.context[k] = v
	}
	w.mu.Unlock()
}

func (w *Watcher) applyContextLocked(obj map[string]any) {
	applyContext(w.context, obj)
}

func applyContext(dst map[string]any, obj map[string]any) {
	if dst == nil || obj == nil {
		return
	}
	event, _ := obj["event"].(string)
	switch event {
	case "Location", "FSDJump", "CarrierJump":
		if system, ok := obj["StarSystem"].(string); ok && strings.TrimSpace(system) != "" {
			dst["currentSystem"] = system
		}
	case "Loadout":
		if r, ok := numberValue(obj["MaxJumpRange"]); ok && r > 0 {
			dst["maxJumpRange"] = r
		}
		for _, key := range []string{"Ship", "ShipName", "ShipIdent"} {
			if v, ok := obj[key].(string); ok && strings.TrimSpace(v) != "" {
				dst[strings.ToLower(key[:1])+key[1:]] = v
			}
		}
	case "CarrierStats":
		if v, ok := numberValue(obj["CarrierID"]); ok {
			dst["carrierID"] = v
		}
		for _, pair := range [][2]string{{"Callsign", "carrierCallsign"}, {"Name", "carrierName"}} {
			if v, ok := obj[pair[0]].(string); ok && strings.TrimSpace(v) != "" {
				dst[pair[1]] = v
			}
		}
		if su, ok := obj["SpaceUsage"].(map[string]any); ok {
			for _, pair := range [][2]string{{"TotalCapacity", "carrierTotalCapacity"}, {"Cargo", "carrierCargo"}, {"CargoSpaceReserved", "carrierCargoSpaceReserved"}, {"FreeSpace", "carrierFreeSpace"}} {
				if v, ok := numberValue(su[pair[0]]); ok {
					dst[pair[1]] = v
				}
			}
		}
		if finance, ok := obj["Finance"].(map[string]any); ok {
			if v, ok := numberValue(finance["AvailableBalance"]); ok {
				dst["carrierAvailableBalance"] = v
			}
		}
	}
}

func numberValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// readEliteFiles watches every JSON snapshot in Elite's journal directory.
// Known snapshots receive stable camel-case keys; any future JSON files are
// exposed with a normalized filename key automatically.
func (w *Watcher) readEliteFiles(lastMods map[string]time.Time) error {
	matches, err := filepath.Glob(filepath.Join(w.Dir, "*.json"))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, path := range matches {
		st, statErr := os.Lstat(path)
		if statErr != nil || st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		base := filepath.Base(path)
		seen[base] = true
		if last, ok := lastMods[base]; ok && !st.ModTime().After(last) {
			continue
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var obj map[string]any
		if json.Unmarshal(b, &obj) != nil {
			// Elite can rewrite these files while we are polling. Leave the old
			// mod-time in place so the next tick retries rather than discarding a
			// temporarily partial file.
			continue
		}
		key := eliteFileKey(base)
		updated := st.ModTime().UTC().Format(time.RFC3339Nano)
		lastMods[base] = st.ModTime()
		w.mu.Lock()
		w.eliteFiles[key] = cloneMap(obj)
		w.fileNames[key] = base
		w.fileUpdated[key] = updated
		if key == "status" {
			w.status = cloneMap(obj)
		}
		if key == "market" {
			w.market = cloneMap(obj)
		}
		w.mu.Unlock()

		w.emit(Event{Kind: "eliteFile", Data: map[string]any{"name": key, "file": base, "updated": updated, "available": true, "data": obj}})
		if key == "status" {
			w.emit(Event{Kind: "status", Data: obj})
		}
		if key == "market" {
			w.emit(Event{Kind: "market", Data: obj})
		}
	}

	// A deleted snapshot is removed from the exposed set. Elite normally keeps
	// these files around, but handling deletion makes the state truthful when a
	// commander or cleanup utility removes one.
	for base := range lastMods {
		if seen[base] {
			continue
		}
		delete(lastMods, base)
		key := eliteFileKey(base)
		w.mu.Lock()
		delete(w.eliteFiles, key)
		delete(w.fileNames, key)
		delete(w.fileUpdated, key)
		if key == "status" {
			w.status = map[string]any{}
		}
		if key == "market" {
			w.market = map[string]any{}
		}
		w.mu.Unlock()
		w.emit(Event{Kind: "eliteFile", Data: map[string]any{"name": key, "file": base, "available": false}})
	}
	return nil
}

func eliteFileKey(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	known := map[string]string{
		"status":      "status",
		"market":      "market",
		"outfitting":  "outfitting",
		"shipyard":    "shipyard",
		"modulesinfo": "modulesInfo",
		"cargo":       "cargo",
		"navroute":    "navRoute",
		"backpack":    "backpack",
		"shiplocker":  "shipLocker",
		"fcmaterials": "fcMaterials",
	}
	if key, ok := known[strings.ToLower(base)]; ok {
		return key
	}
	if base == "" {
		return "unknown"
	}
	var b strings.Builder
	upperNext := false
	for i, r := range base {
		if r == '-' || r == '_' || r == ' ' || r == '.' {
			upperNext = true
			continue
		}
		if b.Len() == 0 {
			b.WriteString(strings.ToLower(string(r)))
			continue
		}
		if upperNext {
			b.WriteString(strings.ToUpper(string(r)))
			upperNext = false
			continue
		}
		if i > 0 {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func normalizeEliteFileLookup(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(name), ".json") {
		return eliteFileKey(name)
	}
	// Accept exact canonical keys first.
	for _, key := range []string{"status", "market", "outfitting", "shipyard", "modulesInfo", "cargo", "navRoute", "backpack", "shipLocker", "fcMaterials"} {
		if strings.EqualFold(name, key) {
			return key
		}
	}
	return eliteFileKey(name + ".json")
}

func (w *Watcher) JournalFiles() []map[string]any {
	matches, _ := filepath.Glob(filepath.Join(w.Dir, "Journal*.log"))
	type item struct {
		name string
		mod  time.Time
		size int64
	}
	items := make([]item, 0, len(matches))
	for _, path := range matches {
		if st, err := os.Lstat(path); err == nil && !st.IsDir() && st.Mode()&os.ModeSymlink == 0 {
			items = append(items, item{name: filepath.Base(path), mod: st.ModTime(), size: st.Size()})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{"file": it.name, "updated": it.mod.UTC().Format(time.RFC3339Nano), "bytes": it.size})
	}
	return out
}

// ReadJournal reads parsed events from one journal session. The filename must
// be a basename from the detected journal directory. Results are bounded so a
// custom tab cannot request an unbounded WebSocket payload.
func (w *Watcher) ReadJournal(name, eventName string, offset, limit int) ([]map[string]any, map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		path := latestJournal(w.Dir)
		if path == "" {
			return nil, nil, errors.New("no journal file is available")
		}
		name = filepath.Base(path)
	}
	if filepath.Base(name) != name || !strings.HasPrefix(strings.ToLower(name), "journal") || !strings.HasSuffix(strings.ToLower(name), ".log") {
		return nil, nil, errors.New("invalid journal filename")
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 1000
	}
	if limit > 5000 {
		limit = 5000
	}
	path := filepath.Join(w.Dir, name)
	st, err := os.Lstat(path)
	if err != nil || st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("journal file not found")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	out := make([]map[string]any, 0, minInt(limit, 256))
	truncated := false
	filter := strings.TrimSpace(eventName)
	matched := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}
		if filter != "" && filter != "*" {
			ev, _ := obj["event"].(string)
			if !strings.EqualFold(ev, filter) {
				continue
			}
		}
		if matched < offset {
			matched++
			continue
		}
		matched++
		if len(out) >= limit {
			truncated = true
			break
		}
		out = append(out, obj)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	info := map[string]any{"file": name, "updated": st.ModTime().UTC().Format(time.RFC3339Nano), "bytes": st.Size(), "count": len(out), "offset": offset, "limit": limit, "truncated": truncated}
	if truncated {
		info["nextOffset"] = offset + len(out)
	}
	if filter != "" {
		info["event"] = filter
	}
	return out, info, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (w *Watcher) emit(e Event) {
	if w.OnEvent != nil {
		w.OnEvent(e)
	}
}

func latestJournal(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "Journal*.log"))
	type item struct {
		path string
		mod  time.Time
	}
	items := make([]item, 0, len(matches))
	for _, p := range matches {
		if st, err := os.Stat(p); err == nil {
			items = append(items, item{p, st.ModTime()})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	if len(items) == 0 {
		return ""
	}
	return items[0].path
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	b, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func cloneMapMap(in map[string]map[string]any) map[string]map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneMap(v)
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
