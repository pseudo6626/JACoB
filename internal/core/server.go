package core

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"jacob/internal/appearance"
	"jacob/internal/bindings"
	"jacob/internal/buildinfo"
	"jacob/internal/control"
	"jacob/internal/customtabs"
	"jacob/internal/journal"
	"jacob/internal/localization"
	"jacob/internal/platform"
	"jacob/internal/tabstate"
	"jacob/internal/updater"
	"jacob/internal/vision"
	"jacob/internal/webfetch"
	"jacob/internal/webui"
)

type Config struct {
	Bind          string
	DataDir       string
	JournalDir    string
	BindingsDir   string
	ReplayJournal bool
	EnableInput   bool
	LANEnabled    bool
	PairToken     string
	AutoBind      bool
}

type Server struct {
	cfg           Config
	input         platform.InputDriver
	watcher       *journal.Watcher
	bindings      *bindings.Store
	controls      *control.Bridge
	recorder      platform.InputRecorder
	capture       platform.CaptureDriver
	vision        *vision.Tracker
	overlay       platform.OverlayDriver
	tabs          *customtabs.Store
	tabState      *tabstate.Store
	appearance    *appearance.Store
	locale        *localization.Store
	updater       *updater.Client
	webfetch      *webfetch.Client
	clientsMu     sync.RWMutex
	recorderMu    sync.Mutex
	clients       map[*wsClient]struct{}
	recorderOwner *wsClient
	started       time.Time
	mediaToken    string
	autoBind      bindings.AutoBindReport
	shutdown      chan struct{}
	shutdownOnce  sync.Once
}

type envelope struct {
	Type   string         `json:"type"`
	ID     string         `json:"id,omitempty"`
	Method string         `json:"method,omitempty"`
	Event  string         `json:"event,omitempty"`
	Params map[string]any `json:"params,omitempty"`
	Data   any            `json:"data,omitempty"`
	OK     *bool          `json:"ok,omitempty"`
	Result any            `json:"result,omitempty"`
	Error  *apiError      `json:"error,omitempty"`
}
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(cfg Config) *Server {
	if cfg.LANEnabled && cfg.PairToken == "" {
		token, err := persistentPairToken(cfg.DataDir)
		if err != nil {
			log.Printf("JACoB LAN access disabled because a secure pairing token could not be generated: %v", err)
			cfg.LANEnabled = false
		} else {
			cfg.PairToken = token
		}
	} else if cfg.LANEnabled && len(strings.TrimSpace(cfg.PairToken)) < 16 {
		log.Printf("JACoB LAN access disabled: configured pairing token must be at least 16 characters")
		cfg.LANEnabled = false
	}
	s := &Server{cfg: cfg, input: platform.NewInputDriver(), recorder: platform.NewInputRecorder(), capture: platform.NewCaptureDriver(), overlay: platform.NewOverlayDriver(), updater: updater.New(), webfetch: webfetch.New(), clients: map[*wsClient]struct{}{}, started: time.Now(), shutdown: make(chan struct{})}
	mediaToken, mediaErr := secureRandomToken()
	if mediaErr != nil {
		log.Printf("JACoB local media authorization unavailable because a secure token could not be generated: %v", mediaErr)
	} else {
		s.mediaToken = mediaToken
	}
	s.vision = vision.New(s.capture)
	localeStore, localeErr := localization.New(cfg.DataDir)
	if localeErr != nil {
		log.Printf("JACoB locale store unavailable: %v", localeErr)
	} else {
		s.locale = localeStore
		log.Printf("Locale store: %s", localeStore.Path())
	}
	themeStore, themeErr := appearance.New(cfg.DataDir)
	if themeErr != nil {
		log.Printf("JACoB appearance store unavailable: %v", themeErr)
	} else {
		s.appearance = themeStore
	}
	tabStore, err := customtabs.New(cfg.DataDir)
	if err != nil {
		log.Printf("JACoB custom tab store unavailable: %v", err)
	} else {
		s.tabs = tabStore
		log.Printf("Custom tabs store: %s", tabStore.Path())
	}
	stateStore, stateErr := tabstate.New(cfg.DataDir)
	if stateErr != nil {
		log.Printf("JACoB tab state store unavailable: %v", stateErr)
	} else {
		s.tabState = stateStore
		log.Printf("Tab state store: %s", stateStore.Path())
	}
	s.bindings = bindings.New(cfg.BindingsDir)
	if cfg.AutoBind {
		if platform.GameRunning() {
			log.Printf("JACoB autobind refused: Elite Dangerous is running; close the game before modifying bindings")
		} else {
			health := s.healthReport()
			blockers, _ := health["blockers"].([]string)
			if len(blockers) > 0 {
				log.Printf("JACoB autobind refused by preflight: %s", strings.Join(blockers, "; "))
			} else {
				report, err := s.bindings.EnsureMissingPrimaryBindings()
				if err != nil {
					log.Printf("JACoB binding scan failed: %v", err)
				} else {
					s.autoBind = report
					if report.Assigned > 0 {
						log.Printf("JACoB created %d clone preset(s) and assigned %d fallback bindings; selector backup: %s", len(report.CreatedFiles), report.Assigned, report.BackupPath)
					} else {
						log.Printf("JACoB binding scan: no missing commands needed fallback bindings")
					}
				}
			}
		}
	}
	s.watcher = journal.New(cfg.JournalDir, cfg.ReplayJournal, func(e journal.Event) { s.broadcast(envelope{Type: "event", Event: e.Kind, Data: e.Data}) })
	s.controls = control.New(s.bindings, s.input)
	health := s.healthReport()
	if blockers, _ := health["blockers"].([]string); len(blockers) > 0 {
		for _, b := range blockers {
			log.Printf("JACoB preflight BLOCKED: %s", b)
		}
	} else if warnings, _ := health["warnings"].([]string); len(warnings) > 0 {
		for _, w := range warnings {
			log.Printf("JACoB preflight warning: %s", w)
		}
	} else {
		log.Printf("JACoB preflight: OK")
	}
	return s
}

func (s *Server) Run(ctx context.Context) error {
	go func() { _ = s.watcher.Run(ctx) }()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/video/frame.jpg", s.handleVideoFrame)
	mux.HandleFunc("/api/video.mjpeg", s.handleVideoMJPEG)
	mux.Handle("/", webui.Handler())
	httpServer := &http.Server{
		Addr:              s.cfg.Bind,
		Handler:           withHeaders(hostGuard(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		select {
		case <-ctx.Done():
		case <-s.shutdown:
		}
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(c)
	}()
	log.Printf("JACoB %s listening on http://%s", buildinfo.Display, s.cfg.Bind)
	if s.cfg.LANEnabled {
		log.Printf("LAN pairing token: %s", s.cfg.PairToken)
	}
	return httpServer.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	local := isLoopbackRequest(r)
	if !local && !s.authorizeRemoteToken(r) {
		http.Error(w, "pairing token required", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(s.systemInfo(local))
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if err := validateWebSocketOrigin(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	local := isLoopbackRequest(r)
	if !local {
		if !s.cfg.LANEnabled {
			http.Error(w, "LAN access disabled", http.StatusForbidden)
			return
		}
		got := r.URL.Query().Get("token")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.PairToken)) != 1 {
			http.Error(w, "invalid pairing token", http.StatusUnauthorized)
			return
		}
	}
	client, err := upgradeWebSocket(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	client.local = local
	s.clientsMu.Lock()
	s.clients[client] = struct{}{}
	s.clientsMu.Unlock()
	defer func() {
		s.stopRecorderOwnedBy(client)
		s.clientsMu.Lock()
		delete(s.clients, client)
		s.clientsMu.Unlock()
		_ = client.Close()
	}()
	_ = s.send(client, envelope{Type: "event", Event: "core.hello", Data: s.systemInfo(local)})
	_ = s.send(client, envelope{Type: "event", Event: "state", Data: s.watcher.Snapshot()})
	for {
		op, payload, err := client.ReadMessage()
		if err != nil {
			return
		}
		switch op {
		case 0x8:
			return
		case 0x9:
			_ = client.WritePong(payload)
		case 0x1:
			var req envelope
			if err := json.Unmarshal(payload, &req); err != nil {
				_ = s.sendError(client, "", "BAD_JSON", err.Error())
				continue
			}
			s.handleRequest(client, req)
		}
	}
}

func (s *Server) requireInput(c *wsClient, id string) bool {
	if !s.cfg.EnableInput {
		_ = s.sendError(c, id, "INPUT_DISABLED", "input is disabled; start JACoB with JACOB_ENABLE_INPUT=1")
		return false
	}
	if !s.input.Available() {
		_ = s.sendError(c, id, "INPUT_UNAVAILABLE", fmt.Sprintf("input driver %s is not available on this platform build", s.input.Name()))
		return false
	}
	return true
}

func (s *Server) requireLocal(c *wsClient, id, action string) bool {
	if c != nil && c.local {
		return true
	}
	_ = s.sendError(c, id, "LOCAL_ONLY", action+" is only available from a browser running on the host computer")
	return false
}

func (s *Server) handleRequest(c *wsClient, req envelope) {
	if req.Type != "request" {
		_ = s.sendError(c, req.ID, "BAD_MESSAGE", "expected type=request")
		return
	}
	switch req.Method {
	case "core.ping":
		s.sendResult(c, req.ID, map[string]any{"pong": true, "time": time.Now().UTC().Format(time.RFC3339Nano)})
	case "core.shutdown":
		if !c.local {
			_ = s.sendError(c, req.ID, "LOCAL_ONLY", "JACoB can only be closed from a browser running on the host computer")
			return
		}
		if s.recorder.Status().Recording {
			_, _ = s.recorder.Stop()
		}
		if s.overlay != nil {
			_ = s.overlay.ClearAll()
		}
		s.sendResult(c, req.ID, map[string]any{"closing": true})
		s.broadcast(envelope{Type: "event", Event: "core.shutdown", Data: map[string]any{"closing": true}})
		go func() {
			time.Sleep(120 * time.Millisecond)
			s.shutdownOnce.Do(func() { close(s.shutdown) })
		}()
	case "state.get":
		s.sendResult(c, req.ID, s.watcher.Snapshot())
	case "elitefiles.list":
		s.sendResult(c, req.ID, map[string]any{"files": s.watcher.EliteFileIndex()})
	case "elitefiles.get":
		name, _ := req.Params["name"].(string)
		if strings.TrimSpace(name) == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "name is required")
			return
		}
		data, fileName, updated, found := s.watcher.EliteFile(name)
		s.sendResult(c, req.ID, map[string]any{"found": found, "name": name, "file": fileName, "updated": updated, "data": data})
	case "journal.files":
		s.sendResult(c, req.ID, map[string]any{"files": s.watcher.JournalFiles()})
	case "journal.read":
		fileName, _ := req.Params["file"].(string)
		eventName, _ := req.Params["event"].(string)
		offset := intParam(req.Params, "offset", 0)
		limit := intParam(req.Params, "limit", 1000)
		events, info, err := s.watcher.ReadJournal(fileName, eventName, offset, limit)
		if err != nil {
			_ = s.sendError(c, req.ID, "JOURNAL_READ_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"events": events, "info": info})
	case "system.health":
		s.sendResult(c, req.ID, s.healthForClient(c.local))
	case "locale.get":
		if s.locale == nil {
			s.sendResult(c, req.ID, map[string]any{"language": localization.DefaultLanguage, "supported": localization.Supported()})
			return
		}
		s.sendResult(c, req.ID, s.locale.Info())
	case "locale.save":
		if s.locale == nil {
			_ = s.sendError(c, req.ID, "LOCALE_UNAVAILABLE", "language storage is unavailable")
			return
		}
		language, _ := req.Params["language"].(string)
		if err := s.locale.Set(language); err != nil {
			_ = s.sendError(c, req.ID, "BAD_LANGUAGE", err.Error())
			return
		}
		info := s.locale.Info()
		s.sendResult(c, req.ID, info)
		s.broadcast(envelope{Type: "event", Event: "locale.changed", Data: info})
	case "appearance.get":
		if s.appearance == nil {
			s.sendResult(c, req.ID, map[string]any{"html": "", "custom": false})
			return
		}
		html := s.appearance.Get()
		s.sendResult(c, req.ID, map[string]any{"html": html, "custom": strings.TrimSpace(html) != ""})
	case "appearance.save":
		if !s.requireLocal(c, req.ID, "appearance changes") {
			return
		}
		if s.appearance == nil {
			_ = s.sendError(c, req.ID, "APPEARANCE_UNAVAILABLE", "appearance storage is unavailable")
			return
		}
		html, _ := req.Params["html"].(string)
		if err := s.appearance.Save(html); err != nil {
			_ = s.sendError(c, req.ID, "APPEARANCE_SAVE_FAILED", err.Error())
			return
		}
		s.broadcast(envelope{Type: "event", Event: "appearance.changed", Data: map[string]any{"custom": strings.TrimSpace(html) != ""}})
		s.sendResult(c, req.ID, map[string]any{"saved": true, "custom": strings.TrimSpace(html) != ""})
	case "appearance.reset":
		if !s.requireLocal(c, req.ID, "appearance changes") {
			return
		}
		if s.appearance == nil {
			s.sendResult(c, req.ID, map[string]any{"reset": true})
			return
		}
		if err := s.appearance.Reset(); err != nil {
			_ = s.sendError(c, req.ID, "APPEARANCE_RESET_FAILED", err.Error())
			return
		}
		s.broadcast(envelope{Type: "event", Event: "appearance.changed", Data: map[string]any{"custom": false}})
		s.sendResult(c, req.ID, map[string]any{"reset": true})
	case "tabs.list":
		if s.tabs == nil {
			_ = s.sendError(c, req.ID, "TAB_STORE_UNAVAILABLE", "custom tab storage is unavailable")
			return
		}
		s.sendResult(c, req.ID, map[string]any{"directory": s.tabs.Directory(), "tabs": s.tabs.List(), "layout": s.tabs.Layout()})
	case "tabs.layout.save":
		if s.tabs == nil {
			_ = s.sendError(c, req.ID, "TAB_STORE_UNAVAILABLE", "custom tab storage is unavailable")
			return
		}
		order := stringSliceParam(req.Params, "order")
		hidden := stringSliceParam(req.Params, "hiddenDefaults")
		layout, err := s.tabs.SaveLayout(order, hidden)
		if err != nil {
			_ = s.sendError(c, req.ID, "TAB_LAYOUT_SAVE_FAILED", err.Error())
			return
		}
		s.broadcast(envelope{Type: "event", Event: "tabs.changed", Data: map[string]any{"action": "layout", "layout": layout}})
		s.sendResult(c, req.ID, layout)
	case "tabs.get":
		if s.tabs == nil {
			_ = s.sendError(c, req.ID, "TAB_STORE_UNAVAILABLE", "custom tab storage is unavailable")
			return
		}
		id, _ := req.Params["id"].(string)
		if id == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.id is required")
			return
		}
		tab, ok := s.tabs.Get(id)
		if !ok {
			_ = s.sendError(c, req.ID, "TAB_NOT_FOUND", "saved tab not found")
			return
		}
		s.sendResult(c, req.ID, tab)
	case "tabs.save":
		if !s.requireLocal(c, req.ID, "custom-tab installation and editing") {
			return
		}
		if s.tabs == nil {
			_ = s.sendError(c, req.ID, "TAB_STORE_UNAVAILABLE", "custom tab storage is unavailable")
			return
		}
		id, _ := req.Params["id"].(string)
		name, _ := req.Params["name"].(string)
		html, _ := req.Params["html"].(string)
		tab, err := s.tabs.Save(id, name, html)
		if err != nil {
			_ = s.sendError(c, req.ID, "TAB_SAVE_FAILED", err.Error())
			return
		}
		s.broadcast(envelope{Type: "event", Event: "tabs.changed", Data: map[string]any{"action": "saved", "tab": tab}})
		s.sendResult(c, req.ID, tab)
	case "tabs.delete":
		if !s.requireLocal(c, req.ID, "custom-tab removal") {
			return
		}
		if s.tabs == nil {
			_ = s.sendError(c, req.ID, "TAB_STORE_UNAVAILABLE", "custom tab storage is unavailable")
			return
		}
		id, _ := req.Params["id"].(string)
		if id == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.id is required")
			return
		}
		if err := s.tabs.Delete(id); err != nil {
			_ = s.sendError(c, req.ID, "TAB_DELETE_FAILED", err.Error())
			return
		}
		if s.overlay != nil {
			_ = s.overlay.ClearLayer("tab:" + id)
		}
		if s.tabState != nil {
			_ = s.tabState.Clear(id)
		}
		s.broadcast(envelope{Type: "event", Event: "tabs.changed", Data: map[string]any{"action": "deleted", "id": id}})
		s.sendResult(c, req.ID, map[string]any{"id": id, "deleted": true})
	case "tabstate.get":
		if s.tabState == nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_UNAVAILABLE", "persistent custom-tab state is unavailable")
			return
		}
		tabID, _ := req.Params["tabId"].(string)
		key, _ := req.Params["key"].(string)
		if tabID == "" || key == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "tab id and state key are required")
			return
		}
		value, ok := s.tabState.Get(tabID, key)
		s.sendResult(c, req.ID, map[string]any{"found": ok, "value": value})
	case "tabstate.set":
		if s.tabState == nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_UNAVAILABLE", "persistent custom-tab state is unavailable")
			return
		}
		tabID, _ := req.Params["tabId"].(string)
		key, _ := req.Params["key"].(string)
		if tabID == "" || key == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "tab id and state key are required")
			return
		}
		if err := s.tabState.Set(tabID, key, req.Params["value"]); err != nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_SAVE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"saved": true, "key": key})
	case "tabstate.delete":
		if s.tabState == nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_UNAVAILABLE", "persistent custom-tab state is unavailable")
			return
		}
		tabID, _ := req.Params["tabId"].(string)
		key, _ := req.Params["key"].(string)
		if err := s.tabState.Delete(tabID, key); err != nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_DELETE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"deleted": true, "key": key})
	case "tabstate.clear":
		if s.tabState == nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_UNAVAILABLE", "persistent custom-tab state is unavailable")
			return
		}
		tabID, _ := req.Params["tabId"].(string)
		if err := s.tabState.Clear(tabID); err != nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_CLEAR_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"cleared": true})
	case "update.check":
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		info, err := s.updater.Check(ctx)
		if err != nil {
			_ = s.sendError(c, req.ID, "UPDATE_CHECK_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, info)
	case "update.install":
		if !c.local {
			_ = s.sendError(c, req.ID, "LOCAL_ONLY", "software updates can only be installed from a browser running on the host computer")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		info, err := s.updater.Check(ctx)
		if err != nil {
			_ = s.sendError(c, req.ID, "UPDATE_CHECK_FAILED", err.Error())
			return
		}
		if !info.Available {
			_ = s.sendError(c, req.ID, "NO_UPDATE", "this JACoB build is already current")
			return
		}
		if !info.Installable || info.Asset == nil {
			msg := info.InstallNote
			if msg == "" {
				msg = "automatic installation is unavailable for this release"
			}
			_ = s.sendError(c, req.ID, "UPDATE_NOT_INSTALLABLE", msg)
			return
		}
		path, err := s.updater.Download(ctx, *info.Asset, filepath.Join(s.cfg.DataDir, "updates"))
		if err != nil {
			_ = s.sendError(c, req.ID, "UPDATE_DOWNLOAD_FAILED", err.Error())
			return
		}
		switch runtime.GOOS {
		case "windows":
			args := []string{"--update", "--wait-pid", strconv.Itoa(os.Getpid()), fmt.Sprintf("--recorder=%t", s.recorder.Available())}
			cmd := exec.Command(path, args...)
			if err := cmd.Start(); err != nil {
				_ = s.sendError(c, req.ID, "UPDATE_LAUNCH_FAILED", err.Error())
				return
			}
		case "linux":
			if _, err := updater.ReplaceLinuxExecutable(path); err != nil {
				_ = s.sendError(c, req.ID, "UPDATE_INSTALL_FAILED", err.Error())
				return
			}
		default:
			_ = s.sendError(c, req.ID, "UPDATE_NOT_INSTALLABLE", "automatic installation is unavailable on this operating system")
			return
		}
		s.sendResult(c, req.ID, map[string]any{"installing": true, "version": info.LatestVersion, "asset": info.Asset.Name})
		s.broadcast(envelope{Type: "event", Event: "core.update", Data: map[string]any{"installing": true, "version": info.LatestVersion}})
		go func() {
			time.Sleep(180 * time.Millisecond)
			s.shutdownOnce.Do(func() { close(s.shutdown) })
		}()
	case "net.fetch":
		if s.webfetch == nil {
			_ = s.sendError(c, req.ID, "NET_FETCH_UNAVAILABLE", "outbound API access is unavailable")
			return
		}
		urlValue, _ := req.Params["url"].(string)
		method, _ := req.Params["method"].(string)
		body, _ := req.Params["body"].(string)
		headers := stringMapParam(req.Params, "headers")
		ctx, cancel := context.WithTimeout(context.Background(), 22*time.Second)
		defer cancel()
		result, err := s.webfetch.Fetch(ctx, webfetch.Request{URL: urlValue, Method: method, Headers: headers, Body: body})
		if err != nil {
			_ = s.sendError(c, req.ID, "NET_FETCH_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "bindings.diagnostics":
		s.sendResult(c, req.ID, s.bindings.Diagnostics())
	case "bindings.list":
		s.sendResult(c, req.ID, map[string]any{"directory": s.bindings.Directory(), "activeFile": s.bindings.ActiveFile(), "activeFiles": s.bindings.ActiveFiles(), "activeSource": s.bindings.ActiveSource(), "files": s.bindings.ListFiles(), "actions": s.bindings.ListActions(), "autoBind": s.autoBind, "diagnostics": s.bindings.Diagnostics()})
	case "bindings.autofill":
		if !s.requireLocal(c, req.ID, "binding-file modification") {
			return
		}
		if platform.GameRunning() {
			_ = s.sendError(c, req.ID, "GAME_RUNNING", "close Elite Dangerous before JACoB modifies binding files")
			return
		}
		health := s.healthReport()
		if blockers, _ := health["blockers"].([]string); len(blockers) > 0 {
			_ = s.sendError(c, req.ID, "BINDINGS_PREFLIGHT_FAILED", strings.Join(blockers, "; "))
			return
		}
		report, err := s.bindings.EnsureMissingPrimaryBindings()
		if err != nil {
			_ = s.sendError(c, req.ID, "BINDINGS_AUTOFILL_FAILED", err.Error())
			return
		}
		s.autoBind = report
		s.sendResult(c, req.ID, report)
	case "bindings.reload":
		if err := s.bindings.Reload(); err != nil {
			_ = s.sendError(c, req.ID, "BINDINGS_RELOAD_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"activeFile": s.bindings.ActiveFile(), "actionCount": len(s.bindings.ListActions())})
	case "bindings.get":
		name, _ := req.Params["name"].(string)
		if name == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.name is required")
			return
		}
		a, ok := s.bindings.Get(name)
		if !ok {
			_ = s.sendError(c, req.ID, "BINDING_NOT_FOUND", fmt.Sprintf("binding %q not found", name))
			return
		}
		s.sendResult(c, req.ID, a)
	case "input.tap":
		if !s.requireInput(c, req.ID) {
			return
		}
		key, _ := req.Params["key"].(string)
		if key == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.key is required")
			return
		}
		delayMs := intParam(req.Params, "delayMs", 0)
		if delayMs < 0 || delayMs > 5000 {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.delayMs must be between 0 and 5000")
			return
		}
		if delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
		if err := s.input.FocusGame(); err != nil {
			_ = s.sendError(c, req.ID, "GAME_FOCUS_FAILED", err.Error())
			return
		}
		modifiers := stringSliceParam(req.Params, "modifiers")
		if err := s.input.TapChord(key, modifiers); err != nil {
			_ = s.sendError(c, req.ID, "INPUT_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"sent": true, "key": key, "modifiers": modifiers, "delayMs": delayMs})
	case "input.text":
		if !s.requireInput(c, req.ID) {
			return
		}
		text, _ := req.Params["text"].(string)
		if text == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.text is required")
			return
		}
		intervalMs := intParam(req.Params, "intervalMs", 15)
		result, err := s.controls.TypeText(text, intervalMs)
		if err != nil {
			_ = s.sendError(c, req.ID, "TEXT_INPUT_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "input.hold":
		if !s.requireInput(c, req.ID) {
			return
		}
		key, _ := req.Params["key"].(string)
		if key == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.key is required")
			return
		}
		durationMs := intParam(req.Params, "durationMs", 250)
		if durationMs < 20 || durationMs > 10000 {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.durationMs must be between 20 and 10000")
			return
		}
		modifiers := stringSliceParam(req.Params, "modifiers")
		if err := s.input.FocusGame(); err != nil {
			_ = s.sendError(c, req.ID, "GAME_FOCUS_FAILED", err.Error())
			return
		}
		if err := s.input.HoldChord(key, modifiers, durationMs); err != nil {
			_ = s.sendError(c, req.ID, "INPUT_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"sent": true, "key": key, "modifiers": modifiers, "durationMs": durationMs})
	case "recorder.status":
		s.sendResult(c, req.ID, s.recorder.Status())
	case "recorder.start":
		if !s.recorder.Available() {
			_ = s.sendError(c, req.ID, "RECORDER_UNAVAILABLE", "input recorder is unavailable on this platform build")
			return
		}
		if err := s.recorder.Start(func(ev platform.RecordedInputEvent) {
			s.broadcast(envelope{Type: "event", Event: "recorder.input", Data: s.enrichRecordedInput(ev)})
		}); err != nil {
			_ = s.sendError(c, req.ID, "RECORDER_START_FAILED", err.Error())
			return
		}
		s.recorderMu.Lock()
		s.recorderOwner = c
		s.recorderMu.Unlock()
		s.sendResult(c, req.ID, s.recorder.Status())
	case "recorder.stop":
		events, err := s.recorder.Stop()
		if err != nil {
			_ = s.sendError(c, req.ID, "RECORDER_STOP_FAILED", err.Error())
			return
		}
		s.recorderMu.Lock()
		s.recorderOwner = nil
		s.recorderMu.Unlock()
		enriched := make([]any, 0, len(events))
		for _, ev := range events {
			enriched = append(enriched, s.enrichRecordedInput(ev))
		}
		s.sendResult(c, req.ID, map[string]any{"status": s.recorder.Status(), "events": enriched})
	case "overlay.info":
		if s.overlay == nil {
			s.sendResult(c, req.ID, platform.OverlayInfo{Available: false, Driver: "overlay-unavailable"})
			return
		}
		s.sendResult(c, req.ID, s.overlay.Info())
	case "overlay.set":
		if s.overlay == nil || !s.overlay.Available() {
			driver := "overlay-unavailable"
			if s.overlay != nil {
				driver = s.overlay.Name()
			}
			_ = s.sendError(c, req.ID, "OVERLAY_UNAVAILABLE", "overlay driver is unavailable: "+driver)
			return
		}
		layer, _ := req.Params["layer"].(string)
		if layer == "" || len(layer) > 160 {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.layer is required and must be <= 160 characters")
			return
		}
		raw, ok := req.Params["scene"]
		if !ok {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.scene is required")
			return
		}
		b, err := json.Marshal(raw)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var scene platform.OverlayScene
		if err := json.Unmarshal(b, &scene); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		if err := platform.ValidateOverlayScene(&scene); err != nil {
			_ = s.sendError(c, req.ID, "OVERLAY_SCENE_INVALID", err.Error())
			return
		}
		if err := s.overlay.SetLayer(layer, scene); err != nil {
			_ = s.sendError(c, req.ID, "OVERLAY_RENDER_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"layer": layer, "items": len(scene.Items), "overlay": s.overlay.Info()})
	case "overlay.clear":
		if s.overlay == nil {
			s.sendResult(c, req.ID, map[string]any{"cleared": true})
			return
		}
		layer, _ := req.Params["layer"].(string)
		if layer == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.layer is required")
			return
		}
		if err := s.overlay.ClearLayer(layer); err != nil {
			_ = s.sendError(c, req.ID, "OVERLAY_CLEAR_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"layer": layer, "cleared": true, "overlay": s.overlay.Info()})
	case "video.info":
		s.sendResult(c, req.ID, map[string]any{"available": s.capture.Available(), "driver": s.capture.Name(), "eliteOnly": true, "foregroundOnly": true, "rawFramesExposed": false})
	case "vision.info":
		if s.vision == nil {
			s.sendResult(c, req.ID, map[string]any{"available": false, "eliteOnly": true, "foregroundOnly": true, "rawFramesExposed": false})
			return
		}
		s.sendResult(c, req.ID, s.vision.Info())
	case "vision.sample":
		if s.vision == nil {
			_ = s.sendError(c, req.ID, "VISION_UNAVAILABLE", "derived game vision is unavailable")
			return
		}
		width := intParam(req.Params, "width", 320)
		if width < 160 {
			width = 160
		}
		if width > 640 {
			width = 640
		}
		sample, err := s.vision.Sample(width)
		if err != nil {
			_ = s.sendError(c, req.ID, "VISION_SAMPLE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, sample)
	case "binding.press":
		if !s.requireInput(c, req.ID) {
			return
		}
		action, _ := req.Params["action"].(string)
		if action == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.action is required")
			return
		}
		result, err := s.controls.PressBinding(action)
		if err != nil {
			_ = s.sendError(c, req.ID, "BINDING_PRESS_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "binding.down":
		if !s.requireInput(c, req.ID) {
			return
		}
		action, _ := req.Params["action"].(string)
		if action == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.action is required")
			return
		}
		result, err := s.controls.DownBinding(action)
		if err != nil {
			_ = s.sendError(c, req.ID, "BINDING_DOWN_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "binding.up":
		if !s.requireInput(c, req.ID) {
			return
		}
		action, _ := req.Params["action"].(string)
		if action == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.action is required")
			return
		}
		result, err := s.controls.UpBinding(action)
		if err != nil {
			_ = s.sendError(c, req.ID, "BINDING_UP_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "binding.hold":
		if !s.requireInput(c, req.ID) {
			return
		}
		action, _ := req.Params["action"].(string)
		if action == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.action is required")
			return
		}
		durationMs := intParam(req.Params, "durationMs", 1000)
		if durationMs < 20 || durationMs > 10000 {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.durationMs must be between 20 and 10000")
			return
		}
		result, err := s.controls.HoldBinding(action, durationMs)
		if err != nil {
			_ = s.sendError(c, req.ID, "BINDING_HOLD_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	default:
		_ = s.sendError(c, req.ID, "NO_SUCH_METHOD", fmt.Sprintf("unknown method %q", req.Method))
	}
}

func stringMapParam(params map[string]any, key string) map[string]string {
	out := map[string]string{}
	raw, ok := params[key].(map[string]any)
	if !ok {
		return out
	}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func (s *Server) systemInfo(includeSecret bool) map[string]any {
	host, _ := os.Hostname()
	lan := map[string]any{"enabled": s.cfg.LANEnabled}
	if s.cfg.LANEnabled {
		lan["port"] = portOf(s.cfg.Bind)
		lan["addresses"] = localIPv4s()
		if includeSecret {
			lan["pairToken"] = s.cfg.PairToken
		}
	}
	journalDir, bindingsDir, bindingsFile := "", "", ""
	bindingsFiles := []string{}
	customTabsDir := ""
	if includeSecret {
		journalDir = s.cfg.JournalDir
		bindingsDir = s.bindings.Directory()
		bindingsFile = s.bindings.ActiveFile()
		bindingsFiles = s.bindings.ActiveFiles()
		if s.tabs != nil {
			customTabsDir = s.tabs.Directory()
		}
	} else {
		host = "paired-client"
	}
	// Scoped media authorization is intentionally separate from the LAN pairing
	// token. A tab with Video permission must never receive a WebSocket credential.
	mediaToken := s.mediaToken
	return map[string]any{
		"prototype": buildinfo.Display, "version": buildinfo.Version, "product": "JACoB", "name": "Journal Aligned Control Bridge", "apiVersion": 9, "os": runtime.GOOS, "arch": runtime.GOARCH, "goRuntime": runtime.Version(), "host": host, "uptimeSeconds": int(time.Since(s.started).Seconds()), "mediaToken": mediaToken,
		"journalDir": journalDir, "bindingsDir": bindingsDir, "bindingsFile": bindingsFile, "bindingsFiles": bindingsFiles, "bindingsSource": s.bindings.ActiveSource(), "bindingsCount": len(s.bindings.ListActions()), "autoBind": s.autoBind,
		"input": map[string]any{"enabled": s.cfg.EnableInput, "available": s.input.Available(), "driver": s.input.Name()}, "recorder": s.recorder.Status(), "capture": map[string]any{"available": s.capture.Available(), "driver": s.capture.Name(), "eliteOnly": true, "foregroundOnly": true}, "vision": func() map[string]any {
			if s.vision != nil {
				return s.vision.Info()
			}
			return map[string]any{"available": false}
		}(), "overlay": s.overlay.Info(), "lan": lan,
		"health":      s.healthForClient(includeSecret),
		"localClient": includeSecret,
		"appearance":  map[string]any{"available": s.appearance != nil, "custom": s.appearance != nil && strings.TrimSpace(s.appearance.Get()) != ""},
		"locale": func() map[string]any {
			if s.locale != nil {
				return s.locale.Info()
			}
			return map[string]any{"language": localization.DefaultLanguage, "supported": localization.Supported()}
		}(),
		"updates":    map[string]any{"repository": buildinfo.Repository, "checkAvailable": true, "installAvailable": runtime.GOOS == "windows" || runtime.GOOS == "linux"},
		"network":    map[string]any{"fetchAvailable": s.webfetch != nil, "publicHTTPOnly": true},
		"customTabs": map[string]any{"available": s.tabs != nil, "directory": customTabsDir},
	}
}

func (s *Server) healthForClient(local bool) map[string]any {
	h := s.healthReport()
	if local {
		return h
	}
	return map[string]any{
		"status":      h["status"],
		"blockers":    h["blockers"],
		"warnings":    h["warnings"],
		"gameRunning": h["gameRunning"],
	}
}

func (s *Server) healthReport() map[string]any {
	diag := s.bindings.Diagnostics()
	devices := platform.ProbeInputDevices()
	blockers := []string{}
	warnings := []string{}

	if diag.ActiveFile == "" {
		blockers = append(blockers, "no active Elite bindings preset could be resolved")
	}
	if diag.ParseError != "" {
		blockers = append(blockers, "active bindings preset does not parse cleanly: "+diag.ParseError)
	}
	blockingDuplicates := bindings.FilterBlockingDuplicateBindings(diag.DuplicateElements)
	if len(blockingDuplicates) > 0 {
		blockers = append(blockers, "active preset contains duplicate binding elements: "+strings.Join(blockingDuplicates, ", "))
	}
	if nonFatal := bindings.FilterNonFatalDuplicateBindings(diag.DuplicateElements); len(nonFatal) > 0 {
		warnings = append(warnings, "active preset contains known non-fatal duplicate binding elements: "+strings.Join(nonFatal, ", ")+" (Elite uses the first entry)")
	}
	if diag.Loader.Relevant && diag.Loader.HasErrors {
		if len(diag.Loader.MissingDevices) > 0 {
			blockers = append(blockers, "Elite binding loader reports missing devices: "+strings.Join(diag.Loader.MissingDevices, ", "))
		}
		blockingLoaderDuplicates := bindings.FilterBlockingDuplicateBindings(diag.Loader.DuplicateBindings)
		if len(blockingLoaderDuplicates) > 0 {
			blockers = append(blockers, "Elite binding loader reports duplicate bindings: "+strings.Join(blockingLoaderDuplicates, ", "))
		}
		if nonFatal := bindings.FilterNonFatalDuplicateBindings(diag.Loader.DuplicateBindings); len(nonFatal) > 0 {
			warnings = append(warnings, "Elite binding loader reports known non-fatal duplicates: "+strings.Join(nonFatal, ", ")+" (Elite uses the first entry)")
		}
	}
	needsMouse, needsKeyboard := false, false
	for _, d := range diag.RequiredDevices {
		if strings.EqualFold(d, "Mouse") {
			needsMouse = true
		}
		if strings.EqualFold(d, "Keyboard") {
			needsKeyboard = true
		}
	}
	if needsMouse && devices.MouseCount == 0 && devices.Error == "" {
		blockers = append(blockers, "active preset references Mouse, but the host exposes no Raw Input mouse device")
	}
	if needsKeyboard && devices.KeyboardCount == 0 && devices.Error == "" {
		blockers = append(blockers, "active preset references Keyboard, but the host exposes no Raw Input keyboard device")
	}
	if devices.Error != "" {
		warnings = append(warnings, "input-device probe failed: "+devices.Error)
	}
	if diag.ReadOnly {
		warnings = append(warnings, "active preset is a shipped ControlSchemes preset; JACoB will clone it into a user preset before autofill")
	}
	if diag.Loader.HasErrors && !diag.Loader.Relevant {
		warnings = append(warnings, "BindingLoadingErrors.log contains older/non-active errors; they are shown for diagnostics but do not block operation")
	}
	if len(diag.SelectorOdyssey) == 0 && len(diag.SelectorLegacy) == 0 {
		warnings = append(warnings, "no StartPreset selector file was found; JACoB is using preset fallback detection")
	}
	status := "ok"
	if len(warnings) > 0 {
		status = "warning"
	}
	if len(blockers) > 0 {
		status = "blocked"
	}
	return map[string]any{
		"status":      status,
		"blockers":    blockers,
		"warnings":    warnings,
		"devices":     devices,
		"bindings":    diag,
		"gameRunning": platform.GameRunning(),
	}
}

func (s *Server) authorizeRemoteToken(r *http.Request) bool {
	if !s.cfg.LANEnabled {
		return false
	}
	got := r.URL.Query().Get("token")
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.PairToken)) == 1
}

func (s *Server) authorizeMedia(r *http.Request) bool {
	// Media uses a dedicated per-process bearer token on both loopback and LAN.
	// Remote browsers receive it only after authenticating the WebSocket with the
	// LAN pairing token. This keeps the higher-privilege pairing credential out
	// of video URLs returned to sandboxed custom tabs.
	got := r.URL.Query().Get("token")
	return s.mediaToken != "" && got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.mediaToken)) == 1
}

func validateWebSocketOrigin(r *http.Request) error {
	if !validJACoBHost(r.Host) {
		return fmt.Errorf("unrecognized JACoB host")
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Non-browser clients may omit Origin. Browser WebSockets always send it,
		// so this still blocks cross-site WebSocket hijacking while preserving CLI tools.
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid websocket Origin")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("websocket Origin scheme is not allowed")
	}
	if !strings.EqualFold(u.Host, r.Host) {
		return fmt.Errorf("cross-origin websocket connections are blocked")
	}
	return nil
}

func videoParams(r *http.Request) (int, int, int) {
	q := r.URL.Query()
	width := 960
	quality := 60
	fps := 5
	if v, err := strconv.Atoi(q.Get("width")); err == nil && v >= 160 && v <= 1920 {
		width = v
	}
	if v, err := strconv.Atoi(q.Get("quality")); err == nil && v >= 20 && v <= 95 {
		quality = v
	}
	if v, err := strconv.Atoi(q.Get("fps")); err == nil && v >= 1 && v <= 15 {
		fps = v
	}
	return width, quality, fps
}

func (s *Server) handleVideoFrame(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeMedia(r) {
		http.Error(w, "media authorization required", http.StatusUnauthorized)
		return
	}
	if !s.capture.Available() {
		http.Error(w, "capture unavailable: "+s.capture.Name(), http.StatusNotImplemented)
		return
	}
	width, quality, _ := videoParams(r)
	b, info, err := s.capture.CaptureJPEG(width, quality)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-JACoB-Capture", info.Driver)
	if info.Blanked {
		w.Header().Set("X-JACoB-Blanked", "true")
		w.Header().Set("X-JACoB-Blank-Reason", info.Reason)
	}
	_, _ = w.Write(b)
}

func (s *Server) handleVideoMJPEG(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeMedia(r) {
		http.Error(w, "media authorization required", http.StatusUnauthorized)
		return
	}
	if !s.capture.Available() {
		http.Error(w, "capture unavailable: "+s.capture.Name(), http.StatusNotImplemented)
		return
	}
	width, quality, fps := videoParams(r)
	boundary := "jacobframe"
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "close")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	mw := multipart.NewWriter(w)
	_ = mw.SetBoundary(boundary)
	defer mw.Close()
	tick := time.NewTicker(time.Second / time.Duration(fps))
	defer tick.Stop()
	for {
		b, info, err := s.capture.CaptureJPEG(width, quality)
		if err != nil {
			return
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Type", "image/jpeg")
		h.Set("Content-Length", strconv.Itoa(len(b)))
		if info.Blanked {
			h.Set("X-JACoB-Blanked", "true")
			h.Set("X-JACoB-Blank-Reason", info.Reason)
		}
		part, err := mw.CreatePart(h)
		if err != nil {
			return
		}
		if _, err = part.Write(b); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) stopRecorderOwnedBy(c *wsClient) {
	s.recorderMu.Lock()
	owned := s.recorderOwner == c
	if owned {
		s.recorderOwner = nil
	}
	s.recorderMu.Unlock()
	if owned && s.recorder.Status().Recording {
		_, _ = s.recorder.Stop()
		log.Printf("JACoB recorder stopped because the controlling browser disconnected")
	}
}

func (s *Server) broadcast(v envelope) {
	b, _ := json.Marshal(v)
	s.clientsMu.RLock()
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.clientsMu.RUnlock()
	for _, c := range clients {
		_ = c.WriteText(b)
	}
}
func (s *Server) send(c *wsClient, v envelope) error { b, _ := json.Marshal(v); return c.WriteText(b) }
func (s *Server) sendResult(c *wsClient, id string, result any) {
	ok := true
	_ = s.send(c, envelope{Type: "response", ID: id, OK: &ok, Result: result})
}
func (s *Server) sendError(c *wsClient, id, code, msg string) error {
	ok := false
	return s.send(c, envelope{Type: "response", ID: id, OK: &ok, Error: &apiError{Code: code, Message: msg}})
}
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validJACoBHost(r.Host) {
			http.Error(w, "unrecognized JACoB host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// validJACoBHost rejects arbitrary DNS names even when they resolve to this
// machine. Browser same-origin checks alone do not stop DNS-rebinding attacks:
// an attacker-controlled hostname could otherwise resolve to 127.0.0.1 and
// present a matching Host and Origin. JACoB's browser surface is therefore
// reachable only through localhost/loopback literals or IP addresses actually
// assigned to this host.
func validJACoBHost(hostport string) bool {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return false
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	} else if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]")
	} else if strings.Count(hostport, ":") > 1 {
		// Bare IPv6 Host values are malformed for HTTP and should not be accepted.
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		var local net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			local = v.IP
		case *net.IPAddr:
			local = v.IP
		}
		if local != nil && ip.Equal(local) {
			return true
		}
	}
	return false
}

func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), display-capture=(), payment=(), usb=(), serial=(), bluetooth=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; frame-src 'self' blob:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; object-src 'none'")
		next.ServeHTTP(w, r)
	})
}

func stringSliceParam(m map[string]any, key string) []string {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	out := []string{}
	switch v := raw.(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, v...)
	}
	return out
}

type recordedBindingMatch struct {
	Action string `json:"action"`
	Slot   string `json:"slot"`
}

func canonicalRecordedChord(key string, mods []string) string {
	cp := append([]string(nil), mods...)
	for i := range cp {
		cp[i] = strings.ToUpper(strings.TrimSpace(cp[i]))
	}
	sort.Strings(cp)
	return strings.Join(append(cp, strings.ToUpper(strings.TrimSpace(key))), "+")
}

func (s *Server) enrichRecordedInput(ev platform.RecordedInputEvent) map[string]any {
	matches := []recordedBindingMatch{}
	if !ev.IsModifier {
		want := canonicalRecordedChord(ev.Key, ev.Modifiers)
		seen := map[string]bool{}
		for _, action := range s.bindings.ListActions() {
			if key, mods, ok := bindings.KeyboardChord(action.Secondary); ok && canonicalRecordedChord(key, mods) == want {
				id := action.Name + "|secondary"
				if !seen[id] {
					matches = append(matches, recordedBindingMatch{Action: action.Name, Slot: "secondary"})
					seen[id] = true
				}
			}
			if key, mods, ok := bindings.KeyboardChord(action.Primary); ok && canonicalRecordedChord(key, mods) == want {
				id := action.Name + "|primary"
				if !seen[id] {
					matches = append(matches, recordedBindingMatch{Action: action.Name, Slot: "primary"})
					seen[id] = true
				}
			}
		}
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].Slot != matches[j].Slot {
				return matches[i].Slot == "secondary"
			}
			return matches[i].Action < matches[j].Action
		})
	}
	return map[string]any{
		"pressId": ev.PressID, "type": ev.Type, "key": ev.Key, "modifiers": ev.Modifiers,
		"atMs": ev.AtMs, "deltaMs": ev.DeltaMs, "durationMs": ev.DurationMs, "isModifier": ev.IsModifier,
		"matches": matches,
	}
}

func intParam(m map[string]any, k string, d int) int {
	if raw, ok := m[k].(float64); ok {
		return int(raw)
	}
	return d
}
func secureRandomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b[:])), nil
}
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func portOf(bind string) string {
	_, p, err := net.SplitHostPort(bind)
	if err == nil {
		return p
	}
	return "6626"
}

func localIPv4s() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			s := ip.String()
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

func persistentPairToken(dataDir string) (string, error) {
	path := filepath.Join(dataDir, "pairing-token.txt")
	if b, err := os.ReadFile(path); err == nil {
		token := strings.TrimSpace(string(b))
		if len(token) >= 16 {
			return token, nil
		}
	}
	token, err := secureRandomToken()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(dataDir) == "" {
		return token, nil
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return token, nil
}
