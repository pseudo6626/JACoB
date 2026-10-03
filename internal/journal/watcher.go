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
	Dir       string
	Replay    bool
	OnEvent   func(Event)
	mu        sync.RWMutex
	status    map[string]any
	lastEvent map[string]any
	journal   string
}

func New(dir string, replay bool, onEvent func(Event)) *Watcher {
	return &Watcher{Dir: dir, Replay: replay, OnEvent: onEvent, status: map[string]any{}}
}

func (w *Watcher) Snapshot() map[string]any {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return map[string]any{
		"journalDir":       w.Dir,
		"journalFile":      w.journal,
		"status":           cloneMap(w.status),
		"lastJournalEvent": cloneMap(w.lastEvent),
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	if w.Dir == "" {
		<-ctx.Done()
		return ctx.Err()
	}
	statusTicker := time.NewTicker(250 * time.Millisecond)
	journalTicker := time.NewTicker(250 * time.Millisecond)
	defer statusTicker.Stop()
	defer journalTicker.Stop()

	var statusMod time.Time
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
	_ = w.readStatus(&statusMod)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-statusTicker.C:
			_ = w.readStatus(&statusMod)
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

func (w *Watcher) readStatus(lastMod *time.Time) error {
	path := filepath.Join(w.Dir, "Status.json")
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !st.ModTime().After(*lastMod) {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	*lastMod = st.ModTime()
	w.mu.Lock()
	w.status = cloneMap(obj)
	w.mu.Unlock()
	w.emit(Event{Kind: "status", Data: obj})
	return nil
}

func (w *Watcher) emit(e Event) {
	if w.OnEvent != nil {
		w.OnEvent(e)
	}
}

func latestJournal(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "Journal.*.log"))
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
