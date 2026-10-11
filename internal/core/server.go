package core

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	"jacob/internal/catalog"
	"jacob/internal/control"
	"jacob/internal/customtabs"
	"jacob/internal/deviceauth"
	"jacob/internal/galaxy"
	"jacob/internal/journal"
	"jacob/internal/localization"
	"jacob/internal/macros"
	"jacob/internal/networkdiag"
	"jacob/internal/pathconfig"
	"jacob/internal/platform"
	"jacob/internal/spatial"
	"jacob/internal/tabstate"
	"jacob/internal/updater"
	"jacob/internal/vision"
	"jacob/internal/webfetch"
	"jacob/internal/webui"
)

type Config struct {
	Bind              string
	DataDir           string
	JournalDir        string
	BindingsDir       string
	ReplayJournal     bool
	EnableInput       bool
	LANEnabled        bool
	PairToken         string
	AutoBind          bool
	ManualPaths       pathconfig.Config
	ControlSchemeDirs []string
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
	spatial       *spatial.Mapper
	calibration   *vision.Calibration
	overlay       platform.OverlayDriver
	tabs          *customtabs.Store
	tabState      *tabstate.Store
	appearance    *appearance.Store
	locale        *localization.Store
	updater       *updater.Client
	webfetch      *webfetch.Client
	catalog       *catalog.Client
	clientsMu     sync.RWMutex
	recorderMu    sync.Mutex
	pairMu        sync.Mutex
	clients       map[*wsClient]struct{}
	clientDevices map[*wsClient]string
	pairFailures  map[string]pairFailure
	devices       *deviceauth.Store
	macros        *macros.Store
	macroMu       sync.Mutex
	macroRunning  bool
	recorderOwner *wsClient
	started       time.Time
	mediaToken    string
	autoBind      bindings.AutoBindReport
	shutdown      chan struct{}
	shutdownOnce  sync.Once
}

type pairFailure struct {
	WindowStart  time.Time
	Failures     int
	BlockedUntil time.Time
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
	if strings.TrimSpace(cfg.DataDir) != "" {
		_ = os.Remove(filepath.Join(cfg.DataDir, "pairing-token.txt"))
	}
	if cfg.LANEnabled && cfg.PairToken == "" {
		token, err := sessionPairToken(cfg.DataDir)
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
	s := &Server{cfg: cfg, input: platform.NewInputDriver(), recorder: platform.NewInputRecorder(), capture: platform.NewCaptureDriver(), overlay: platform.NewOverlayDriver(), updater: updater.New(), webfetch: webfetch.New(), clients: map[*wsClient]struct{}{}, clientDevices: map[*wsClient]string{}, pairFailures: map[string]pairFailure{}, started: time.Now(), shutdown: make(chan struct{})}
	log.Printf("core/init: bind=%q lan=%t inputEnabled=%t replayJournal=%t autoBind=%t journalDir=%q bindingsDir=%q controlSchemeDirs=%d", cfg.Bind, cfg.LANEnabled, cfg.EnableInput, cfg.ReplayJournal, cfg.AutoBind, cfg.JournalDir, cfg.BindingsDir, len(cfg.ControlSchemeDirs))
	log.Printf("core/platform: input=%s available=%t recorder=%s available=%t capture=%s available=%t overlay=%s available=%t", s.input.Name(), s.input.Available(), s.recorder.Name(), s.recorder.Available(), s.capture.Name(), s.capture.Available(), s.overlay.Name(), s.overlay.Available())
	deviceStore, deviceErr := deviceauth.New(cfg.DataDir)
	if deviceErr != nil {
		log.Printf("JACoB remembered-device store unavailable; LAN pairing is disabled until the store is repaired: %v", deviceErr)
		s.cfg.LANEnabled = false
	} else {
		s.devices = deviceStore
		log.Printf("security/devices: store ready path=%q remembered=%d", deviceauth.FilePath(cfg.DataDir), len(deviceStore.List()))
	}
	macroStore, macroErr := macros.New(cfg.DataDir)
	if macroErr != nil {
		log.Printf("JACoB macro store unavailable; built-in macros will be available for this session only: %v", macroErr)
		macroStore, _ = macros.New("")
	}
	s.macros = macroStore
	if macroStore != nil {
		log.Printf("macros/store: ready path=%q definitions=%d", macroStore.Path(), len(macroStore.List()))
	}
	s.catalog = catalog.New(s.webfetch)
	mediaToken, mediaErr := secureRandomToken()
	if mediaErr != nil {
		log.Printf("JACoB local media authorization unavailable because a secure token could not be generated: %v", mediaErr)
	} else {
		s.mediaToken = mediaToken
	}
	s.vision = vision.New(s.capture)
	s.spatial = spatial.New(s.vision)
	s.calibration = vision.NewCalibration(s.capture)
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
	s.bindings = bindings.NewWithControlDirs(cfg.BindingsDir, cfg.ControlSchemeDirs)
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
	mux.HandleFunc("/api/pair", s.handlePair)
	mux.HandleFunc("/api/device/refresh", s.handleDeviceRefresh)
	mux.HandleFunc("/api/device/logout", s.handleDeviceLogout)
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/video/frame.jpg", s.handleVideoFrame)
	mux.HandleFunc("/api/video.mjpeg", s.handleVideoMJPEG)
	mux.HandleFunc("/api/vision/calibration.jpg", s.handleVisionCalibrationFrame)
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
	log.Printf("http/start: %s configured on http://%s lan=%t", buildinfo.Display, s.cfg.Bind, s.cfg.LANEnabled)
	if s.cfg.LANEnabled {
		log.Printf("security/pairing: LAN pairing enabled; enrollment key is available only in host Settings")
	}
	go func() {
		time.Sleep(750 * time.Millisecond)
		s.logNetworkSnapshot("startup")
	}()
	err := httpServer.ListenAndServe()
	log.Printf("http/stop: bind=%q err=%v", s.cfg.Bind, err)
	return err
}

const deviceCookieName = "jacob_device"

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := validateWebSocketOrigin(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if isLoopbackRequest(r) {
		http.Error(w, "the host browser is already trusted", http.StatusBadRequest)
		return
	}
	if !s.cfg.LANEnabled || s.devices == nil {
		http.Error(w, "LAN pairing is unavailable", http.StatusForbidden)
		return
	}
	key := remoteAddressKey(r)
	if ok, wait := s.pairAttemptAllowed(key); !ok {
		log.Printf("security/pairing: rate-limited remote=%q retryAfter=%s", key, wait.Round(time.Second))
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		http.Error(w, "too many failed pairing attempts", http.StatusTooManyRequests)
		return
	}
	var body struct {
		PairKey string `json:"pairKey"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "invalid pairing request", http.StatusBadRequest)
		return
	}
	want := strings.TrimSpace(s.cfg.PairToken)
	got := strings.TrimSpace(body.PairKey)
	if want == "" || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		s.recordPairFailure(key)
		log.Printf("security/pairing: rejected enrollment remote=%q", key)
		time.Sleep(200 * time.Millisecond)
		http.Error(w, "pair key was not accepted", http.StatusUnauthorized)
		return
	}
	s.clearPairFailures(key)
	device, credential, err := s.devices.Enroll(body.Name)
	if err != nil {
		http.Error(w, "could not enroll device", http.StatusInternalServerError)
		return
	}
	setDeviceCookie(w, r, credential, false)
	log.Printf("security/pairing: enrolled device id=%q name=%q remote=%q", device.ID, device.Name, key)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"paired": true, "device": device})
	s.broadcastDevicesChanged()
}

func (s *Server) handleDeviceRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := validateWebSocketOrigin(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	cookie, err := r.Cookie(deviceCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		http.Error(w, "paired device credential required", http.StatusUnauthorized)
		return
	}
	device, ok := s.devices.Authenticate(cookie.Value)
	if !ok {
		clearDeviceCookie(w, r)
		http.Error(w, "paired device credential is no longer valid", http.StatusUnauthorized)
		return
	}
	setDeviceCookie(w, r, cookie.Value, device.Remember)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "device": device})
}

func (s *Server) handleDeviceLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := validateWebSocketOrigin(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	clearDeviceCookie(w, r)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func setDeviceCookie(w http.ResponseWriter, r *http.Request, credential string, remember bool) {
	cookie := &http.Cookie{Name: deviceCookieName, Value: credential, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil}
	if remember {
		cookie.MaxAge = int((180 * 24 * time.Hour) / time.Second)
		cookie.Expires = time.Now().Add(180 * 24 * time.Hour)
	}
	http.SetCookie(w, cookie)
}

func clearDeviceCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: deviceCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func (s *Server) authenticateRemoteRequest(r *http.Request) (deviceauth.Device, bool) {
	if !s.cfg.LANEnabled || s.devices == nil {
		return deviceauth.Device{}, false
	}
	cookie, err := r.Cookie(deviceCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return deviceauth.Device{}, false
	}
	return s.devices.Authenticate(cookie.Value)
}

func remoteAddressKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (s *Server) pairAttemptAllowed(key string) (bool, time.Duration) {
	now := time.Now()
	s.pairMu.Lock()
	defer s.pairMu.Unlock()
	if s.pairFailures == nil {
		s.pairFailures = map[string]pairFailure{}
	}
	f := s.pairFailures[key]
	if now.Before(f.BlockedUntil) {
		return false, time.Until(f.BlockedUntil)
	}
	if f.WindowStart.IsZero() || now.Sub(f.WindowStart) > time.Minute {
		f = pairFailure{WindowStart: now}
		s.pairFailures[key] = f
	}
	return true, 0
}

func (s *Server) recordPairFailure(key string) {
	now := time.Now()
	s.pairMu.Lock()
	defer s.pairMu.Unlock()
	if s.pairFailures == nil {
		s.pairFailures = map[string]pairFailure{}
	}
	f := s.pairFailures[key]
	if f.WindowStart.IsZero() || now.Sub(f.WindowStart) > time.Minute {
		f = pairFailure{WindowStart: now}
	}
	f.Failures++
	if f.Failures >= 5 {
		f.BlockedUntil = now.Add(5 * time.Minute)
	}
	s.pairFailures[key] = f
}

func (s *Server) clearPairFailures(key string) {
	s.pairMu.Lock()
	delete(s.pairFailures, key)
	s.pairMu.Unlock()
}

func (s *Server) registerClient(c *wsClient, deviceID string) {
	s.clientsMu.Lock()
	s.clients[c] = struct{}{}
	if deviceID != "" {
		s.clientDevices[c] = deviceID
	}
	s.clientsMu.Unlock()
	if deviceID != "" {
		s.broadcastDevicesChanged()
	}
}

func (s *Server) unregisterClient(c *wsClient) {
	s.clientsMu.Lock()
	deviceID := s.clientDevices[c]
	delete(s.clientDevices, c)
	delete(s.clients, c)
	s.clientsMu.Unlock()
	if deviceID == "" {
		return
	}
	s.broadcastDevicesChanged()
	go func(id string) {
		time.Sleep(60 * time.Second)
		if s.deviceConnectionCount(id) != 0 || s.devices == nil {
			return
		}
		device, ok := s.devices.Get(id)
		if ok && !device.Remember {
			_ = s.devices.Revoke(id)
			s.broadcastDevicesChanged()
		}
	}(deviceID)
}

func (s *Server) clientDevice(c *wsClient) (deviceauth.Device, bool) {
	if c == nil || s.devices == nil {
		return deviceauth.Device{}, false
	}
	s.clientsMu.RLock()
	id := s.clientDevices[c]
	s.clientsMu.RUnlock()
	if id == "" {
		return deviceauth.Device{}, false
	}
	return s.devices.Get(id)
}

func (s *Server) clientPrivileged(c *wsClient) bool {
	if c != nil && c.local {
		return true
	}
	device, ok := s.clientDevice(c)
	return ok && device.Admin
}

func (s *Server) deviceConnectionCount(id string) int {
	if id == "" {
		return 0
	}
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	n := 0
	for _, deviceID := range s.clientDevices {
		if deviceID == id {
			n++
		}
	}
	return n
}

func (s *Server) deviceList() []map[string]any {
	if s.devices == nil {
		return nil
	}
	devices := s.devices.List()
	out := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		connections := s.deviceConnectionCount(device.ID)
		out = append(out, map[string]any{
			"id": device.ID, "name": device.Name, "remember": device.Remember, "admin": device.Admin,
			"createdAt": device.CreatedAt, "lastSeenAt": device.LastSeenAt,
			"connected": connections > 0, "connections": connections,
		})
	}
	return out
}

func (s *Server) notifyDevicePermissions(id string) {
	if s.devices == nil {
		return
	}
	device, ok := s.devices.Get(id)
	if !ok {
		return
	}
	s.clientsMu.RLock()
	clients := make([]*wsClient, 0)
	for client, deviceID := range s.clientDevices {
		if deviceID == id {
			clients = append(clients, client)
		}
	}
	s.clientsMu.RUnlock()
	for _, client := range clients {
		_ = s.send(client, envelope{Type: "event", Event: "device.permissions", Data: map[string]any{"device": device, "admin": device.Admin, "remember": device.Remember, "privileged": device.Admin}})
	}
}

func (s *Server) broadcastDevicesChanged() {
	s.broadcast(envelope{Type: "event", Event: "devices.changed", Data: map[string]any{"changed": true}})
}

func (s *Server) revokeDevice(id string) error {
	if s.devices == nil {
		return fmt.Errorf("device store unavailable")
	}
	if err := s.devices.Revoke(id); err != nil {
		return err
	}
	s.clientsMu.RLock()
	clients := make([]*wsClient, 0)
	for client, deviceID := range s.clientDevices {
		if deviceID == id {
			clients = append(clients, client)
		}
	}
	s.clientsMu.RUnlock()
	for _, client := range clients {
		_ = s.send(client, envelope{Type: "event", Event: "device.revoked", Data: map[string]any{"id": id}})
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		for _, client := range clients {
			_ = client.Close()
		}
	}()
	s.broadcastDevicesChanged()
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	local := isLoopbackRequest(r)
	if !local {
		if _, ok := s.authenticateRemoteRequest(r); !ok {
			http.Error(w, "paired device credential required", http.StatusUnauthorized)
			return
		}
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
	deviceID := ""
	if !local {
		if !s.cfg.LANEnabled {
			http.Error(w, "LAN access disabled", http.StatusForbidden)
			return
		}
		device, ok := s.authenticateRemoteRequest(r)
		if !ok {
			http.Error(w, "paired device credential required", http.StatusUnauthorized)
			return
		}
		deviceID = device.ID
	}
	client, err := upgradeWebSocket(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	client.local = local
	s.registerClient(client, deviceID)
	log.Printf("ws/connect: local=%t remote=%q device=%q", local, remoteAddressKey(r), deviceID)
	defer func() {
		log.Printf("ws/disconnect: local=%t remote=%q device=%q", local, remoteAddressKey(r), deviceID)
		s.stopRecorderOwnedBy(client)
		s.unregisterClient(client)
		_ = client.Close()
	}()
	hello := s.systemInfo(local)
	hello["adminClient"] = false
	hello["privilegedClient"] = local
	if !local {
		if device, ok := s.clientDevice(client); ok {
			hello["device"] = device
			hello["adminClient"] = device.Admin
			hello["privilegedClient"] = device.Admin
			if device.Admin {
				hello["health"] = s.healthForClient(true)
			}
		}
	}
	_ = s.send(client, envelope{Type: "event", Event: "core.hello", Data: hello})
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
	if s.clientPrivileged(c) {
		return true
	}
	_ = s.sendError(c, id, "LOCAL_ONLY", action+" requires the host browser or a host-approved admin device")
	return false
}

func (s *Server) requirePhysicalHost(c *wsClient, id, action string) bool {
	if c != nil && c.local {
		return true
	}
	_ = s.sendError(c, id, "HOST_ONLY", action+" can only be changed from the browser running on the JACoB host")
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
		if !s.clientPrivileged(c) {
			_ = s.sendError(c, req.ID, "LOCAL_ONLY", "JACoB can only be closed from the host browser or a host-approved admin device")
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
		s.sendResult(c, req.ID, s.healthForClient(s.clientPrivileged(c)))
	case "network.diagnostics":
		if !s.requireLocal(c, req.ID, "network diagnostics") {
			return
		}
		s.sendResult(c, req.ID, s.networkDiagnostics())
	case "network.repair":
		if !s.requireLocal(c, req.ID, "network repair") {
			return
		}
		exe, _ := os.Executable()
		port, _ := strconv.Atoi(portOf(s.cfg.Bind))
		if err := networkdiag.EnsureInteractive(exe, port); err != nil {
			_ = s.sendError(c, req.ID, "NETWORK_REPAIR_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, s.networkDiagnostics())
	case "macros.policy":
		if s.macros == nil {
			_ = s.sendError(c, req.ID, "MACROS_UNAVAILABLE", "macro store is unavailable")
			return
		}
		s.sendResult(c, req.ID, s.macros.Policy())
	case "macros.policy.accept":
		if !s.requirePhysicalHost(c, req.ID, "macro use agreement") {
			return
		}
		if s.macros == nil {
			_ = s.sendError(c, req.ID, "MACROS_UNAVAILABLE", "macro store is unavailable")
			return
		}
		state, err := s.macros.AcceptPolicy()
		if err != nil {
			_ = s.sendError(c, req.ID, "MACRO_POLICY_SAVE_FAILED", err.Error())
			return
		}
		log.Printf("macro policy agreement accepted version=%d", state.Version)
		s.broadcast(envelope{Type: "event", Event: "macros.changed", Data: map[string]any{"action": "policy"}})
		s.sendResult(c, req.ID, state)
	case "macros.list":
		if s.macros == nil {
			_ = s.sendError(c, req.ID, "MACROS_UNAVAILABLE", "macro store is unavailable")
			return
		}
		s.sendResult(c, req.ID, map[string]any{"macros": s.macros.List()})
	case "macros.get":
		if s.macros == nil {
			_ = s.sendError(c, req.ID, "MACROS_UNAVAILABLE", "macro store is unavailable")
			return
		}
		name, _ := req.Params["name"].(string)
		def, ok := s.macros.Resolve(name)
		if !ok {
			_ = s.sendError(c, req.ID, "MACRO_NOT_FOUND", "macro was not found")
			return
		}
		s.sendResult(c, req.ID, def)
	case "macros.run":
		if !s.requireInput(c, req.ID) {
			return
		}
		if s.macros == nil {
			_ = s.sendError(c, req.ID, "MACROS_UNAVAILABLE", "macro store is unavailable")
			return
		}
		name, _ := req.Params["name"].(string)
		def, ok := s.macros.Resolve(name)
		if !ok {
			_ = s.sendError(c, req.ID, "MACRO_NOT_FOUND", fmt.Sprintf("macro %q was not found", name))
			return
		}
		if !def.BuiltIn && !s.macros.PolicyAccepted() {
			_ = s.sendError(c, req.ID, "MACRO_POLICY_REQUIRED", "accept the macro use agreement on the JACoB host before running custom macros")
			return
		}
		result, err := s.runMacro(def, macroArgs(req.Params["args"]))
		if err != nil {
			_ = s.sendError(c, req.ID, "MACRO_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "macros.save":
		if !s.requireLocal(c, req.ID, "macro editing") {
			return
		}
		if s.macros != nil && !s.macros.PolicyAccepted() {
			_ = s.sendError(c, req.ID, "MACRO_POLICY_REQUIRED", "accept the macro use agreement on the JACoB host before creating or modifying macros")
			return
		}
		if s.macros == nil {
			_ = s.sendError(c, req.ID, "MACROS_UNAVAILABLE", "macro store is unavailable")
			return
		}
		raw, ok := req.Params["macro"]
		if !ok {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "macro is required")
			return
		}
		blob, err := json.Marshal(raw)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var def macros.Definition
		if err := json.Unmarshal(blob, &def); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		saved, err := s.macros.Save(def)
		if err != nil {
			_ = s.sendError(c, req.ID, "MACRO_SAVE_FAILED", err.Error())
			return
		}
		s.broadcast(envelope{Type: "event", Event: "macros.changed", Data: map[string]any{"id": saved.ID, "action": "saved"}})
		s.sendResult(c, req.ID, saved)
	case "macros.delete":
		if !s.requireLocal(c, req.ID, "macro editing") {
			return
		}
		if s.macros != nil && !s.macros.PolicyAccepted() {
			_ = s.sendError(c, req.ID, "MACRO_POLICY_REQUIRED", "accept the macro use agreement on the JACoB host before creating or modifying macros")
			return
		}
		id, _ := req.Params["id"].(string)
		if s.macros == nil {
			_ = s.sendError(c, req.ID, "MACROS_UNAVAILABLE", "macro store is unavailable")
			return
		}
		if err := s.macros.Delete(id); err != nil {
			_ = s.sendError(c, req.ID, "MACRO_DELETE_FAILED", err.Error())
			return
		}
		s.broadcast(envelope{Type: "event", Event: "macros.changed", Data: map[string]any{"id": id, "action": "deleted"}})
		s.sendResult(c, req.ID, map[string]any{"deleted": true, "id": id})
	case "macros.reset":
		if !s.requireLocal(c, req.ID, "macro editing") {
			return
		}
		if s.macros != nil && !s.macros.PolicyAccepted() {
			_ = s.sendError(c, req.ID, "MACRO_POLICY_REQUIRED", "accept the macro use agreement on the JACoB host before creating or modifying macros")
			return
		}
		id, _ := req.Params["id"].(string)
		if s.macros == nil {
			_ = s.sendError(c, req.ID, "MACROS_UNAVAILABLE", "macro store is unavailable")
			return
		}
		def, err := s.macros.Reset(id)
		if err != nil {
			_ = s.sendError(c, req.ID, "MACRO_RESET_FAILED", err.Error())
			return
		}
		s.broadcast(envelope{Type: "event", Event: "macros.changed", Data: map[string]any{"id": id, "action": "reset"}})
		s.sendResult(c, req.ID, def)
	case "devices.list":
		if !s.requireLocal(c, req.ID, "connected-device list") {
			return
		}
		s.sendResult(c, req.ID, map[string]any{"devices": s.deviceList()})
	case "devices.setRemember":
		if !s.requirePhysicalHost(c, req.ID, "Remember me") {
			return
		}
		id, _ := req.Params["id"].(string)
		value, _ := req.Params["value"].(bool)
		device, err := s.devices.SetRemember(id, value)
		if err != nil {
			_ = s.sendError(c, req.ID, "DEVICE_UPDATE_FAILED", err.Error())
			return
		}
		log.Printf("security/device: remember changed id=%q value=%t", id, value)
		if !value && s.deviceConnectionCount(id) == 0 {
			_ = s.devices.Revoke(id)
		} else {
			s.notifyDevicePermissions(id)
		}
		s.broadcastDevicesChanged()
		s.sendResult(c, req.ID, device)
	case "devices.setAdmin":
		if !s.requirePhysicalHost(c, req.ID, "device administrator permission") {
			return
		}
		id, _ := req.Params["id"].(string)
		value, _ := req.Params["value"].(bool)
		device, err := s.devices.SetAdmin(id, value)
		if err != nil {
			_ = s.sendError(c, req.ID, "DEVICE_UPDATE_FAILED", err.Error())
			return
		}
		log.Printf("security/device: admin changed id=%q value=%t", id, value)
		s.notifyDevicePermissions(id)
		s.broadcastDevicesChanged()
		s.sendResult(c, req.ID, device)
	case "devices.revoke":
		if !s.requirePhysicalHost(c, req.ID, "device revocation") {
			return
		}
		id, _ := req.Params["id"].(string)
		if strings.TrimSpace(id) == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "device id is required")
			return
		}
		log.Printf("security/device: revoke requested id=%q", id)
		if err := s.revokeDevice(id); err != nil {
			_ = s.sendError(c, req.ID, "DEVICE_REVOKE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"revoked": true, "id": id})
	case "paths.get":
		if !s.requireLocal(c, req.ID, "Elite data path settings") {
			return
		}
		saved, err := pathconfig.Load(s.cfg.DataDir)
		if err != nil {
			_ = s.sendError(c, req.ID, "PATH_SETTINGS_READ_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, s.pathSettingsInfo(saved))
	case "paths.save":
		if !s.requireLocal(c, req.ID, "Elite data path settings") {
			return
		}
		journalDir, _ := req.Params["journalDir"].(string)
		bindingsDir, _ := req.Params["bindingsDir"].(string)
		next := pathconfig.Normalize(pathconfig.Config{
			JournalDir:        journalDir,
			BindingsDir:       bindingsDir,
			ControlSchemeDirs: stringSliceParam(req.Params, "controlSchemeDirs"),
		})
		if err := pathconfig.Save(s.cfg.DataDir, next); err != nil {
			_ = s.sendError(c, req.ID, "PATH_SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, s.pathSettingsInfo(next))
	case "paths.reset":
		if !s.requireLocal(c, req.ID, "Elite data path settings") {
			return
		}
		next := pathconfig.Config{}
		if err := pathconfig.Save(s.cfg.DataDir, next); err != nil {
			_ = s.sendError(c, req.ID, "PATH_SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, s.pathSettingsInfo(next))
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
			_ = s.overlay.ClearPrefix("tab:" + id)
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
	case "tabstate.cas":
		if s.tabState == nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_UNAVAILABLE", "persistent custom-tab state is unavailable")
			return
		}
		tabID, _ := req.Params["tabId"].(string)
		key, _ := req.Params["key"].(string)
		expectedFound, _ := req.Params["expectedFound"].(bool)
		if tabID == "" || key == "" {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "tabId and key are required")
			return
		}
		result, err := s.tabState.CompareSet(tabID, key, expectedFound, req.Params["expected"], req.Params["value"])
		if err != nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_CAS_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "tabstate.batch":
		if s.tabState == nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_UNAVAILABLE", "persistent custom-tab state is unavailable")
			return
		}
		tabID, _ := req.Params["tabId"].(string)
		raw, err := json.Marshal(req.Params["operations"])
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var operations []tabstate.BatchOperation
		if err := json.Unmarshal(raw, &operations); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		result, err := s.tabState.Batch(tabID, operations)
		if err != nil {
			_ = s.sendError(c, req.ID, "TAB_STATE_BATCH_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
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
		if !s.clientPrivileged(c) {
			_ = s.sendError(c, req.ID, "LOCAL_ONLY", "software updates can only be installed from the host browser or a host-approved admin device")
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
			args := []string{"--update", "--wait-pid", strconv.Itoa(os.Getpid()), "--data-dir", s.cfg.DataDir, "--health-port", portOf(s.cfg.Bind), fmt.Sprintf("--recorder=%t", s.recorder.Available())}
			if sourceExe, exeErr := os.Executable(); exeErr == nil && strings.TrimSpace(sourceExe) != "" {
				args = append(args, "--source-exe", sourceExe)
			}
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
	case "galaxy.parseSystemName":
		name, _ := req.Params["name"].(string)
		result, err := galaxy.ParseSystemName(name)
		if err != nil {
			_ = s.sendError(c, req.ID, "GALAXY_PARSE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "galaxy.decodeAddress":
		id64, _ := req.Params["id64"].(string)
		result, err := galaxy.DecodeAddress(id64)
		if err != nil {
			_ = s.sendError(c, req.ID, "GALAXY_ADDRESS_INVALID", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "galaxy.encodeAddress":
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var spec galaxy.EncodeRequest
		if err := json.Unmarshal(raw, &spec); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		result, err := galaxy.EncodeAddress(spec)
		if err != nil {
			_ = s.sendError(c, req.ID, "GALAXY_ADDRESS_INVALID", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"id64": result})
	case "galaxy.addressForSequence":
		id64, _ := req.Params["id64"].(string)
		sequence := intParam(req.Params, "sequence", -1)
		if sequence < 0 {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "sequence must be a non-negative integer")
			return
		}
		result, err := galaxy.AddressForSequence(id64, uint64(sequence))
		if err != nil {
			_ = s.sendError(c, req.ID, "GALAXY_ADDRESS_INVALID", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"id64": result})
	case "galaxy.boxel":
		id64, _ := req.Params["id64"].(string)
		result, err := galaxy.Boxel(id64)
		if err != nil {
			_ = s.sendError(c, req.ID, "GALAXY_ADDRESS_INVALID", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "galaxy.boxelHierarchy":
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var spec galaxy.HierarchyRequest
		if err := json.Unmarshal(raw, &spec); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		result, err := galaxy.BoxelHierarchy(spec)
		if err != nil {
			_ = s.sendError(c, req.ID, "GALAXY_BOXEL_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "catalog.system.get":
		if s.catalog == nil {
			_ = s.sendError(c, req.ID, "CATALOG_UNAVAILABLE", "system catalog is unavailable")
			return
		}
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var spec catalog.SystemRequest
		if err := json.Unmarshal(raw, &spec); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := s.catalog.SystemGet(ctx, spec)
		if err != nil {
			_ = s.sendError(c, req.ID, "CATALOG_LOOKUP_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "net.fetchQueued":
		if s.webfetch == nil {
			_ = s.sendError(c, req.ID, "NET_FETCH_UNAVAILABLE", "outbound API access is unavailable")
			return
		}
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var spec webfetch.QueueRequest
		if err := json.Unmarshal(raw, &spec); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		result, err := s.webfetch.FetchQueued(ctx, spec)
		if err != nil {
			_ = s.sendError(c, req.ID, "NET_FETCH_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "net.fetchBatch":
		if s.webfetch == nil {
			_ = s.sendError(c, req.ID, "NET_FETCH_UNAVAILABLE", "outbound API access is unavailable")
			return
		}
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var spec webfetch.BatchRequest
		if err := json.Unmarshal(raw, &spec); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		result, err := s.webfetch.FetchBatch(ctx, spec)
		if err != nil {
			_ = s.sendError(c, req.ID, "NET_FETCH_BATCH_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
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
		log.Printf("recorder/start: requested driver=%s available=%t", s.recorder.Name(), s.recorder.Available())
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
		log.Printf("recorder/start: active driver=%s", s.recorder.Name())
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
		log.Printf("recorder/stop: driver=%s capturedEvents=%d", s.recorder.Name(), len(events))
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
	case "overlay.clearPrefix":
		if !s.requireLocal(c, req.ID, "tab overlay cleanup") {
			return
		}
		if s.overlay == nil {
			s.sendResult(c, req.ID, map[string]any{"cleared": true})
			return
		}
		prefix, _ := req.Params["prefix"].(string)
		if !strings.HasPrefix(prefix, "tab:") || len(prefix) > 160 {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.prefix must be a tab overlay namespace")
			return
		}
		if err := s.overlay.ClearPrefix(prefix); err != nil {
			_ = s.sendError(c, req.ID, "OVERLAY_CLEAR_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"prefix": prefix, "cleared": true, "overlay": s.overlay.Info()})
	case "video.info":
		_, privacyMatte := s.capture.(platform.PrivacyMatteCaptureDriver)
		s.sendResult(c, req.ID, map[string]any{"available": s.capture.Available(), "driver": s.capture.Name(), "eliteOnly": true, "foregroundOnly": true, "rawFramesExposed": false, "privacyMatte": privacyMatte, "frameModel": "canonical-elite-client-v1", "desktopFallback": false})
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
	case "vision.inspect":
		if s.vision == nil {
			_ = s.sendError(c, req.ID, "VISION_UNAVAILABLE", "derived game vision is unavailable")
			return
		}
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var inspectReq vision.InspectRequest
		if err := json.Unmarshal(raw, &inspectReq); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		result, err := s.vision.Inspect(inspectReq)
		if err != nil {
			_ = s.sendError(c, req.ID, "VISION_INSPECT_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "vision.calibration.arm":
		if s.calibration == nil {
			_ = s.sendError(c, req.ID, "VISION_UNAVAILABLE", "Vision calibration is unavailable")
			return
		}
		timeoutMs := intParam(req.Params, "timeoutMs", 30000)
		if timeoutMs < 5000 {
			timeoutMs = 5000
		}
		if timeoutMs > 60000 {
			timeoutMs = 60000
		}
		if err := s.calibration.Arm(time.Duration(timeoutMs) * time.Millisecond); err != nil {
			_ = s.sendError(c, req.ID, "VISION_UNAVAILABLE", err.Error())
			return
		}
		s.sendResult(c, req.ID, s.calibration.Status())
	case "vision.calibration.status":
		if s.calibration == nil {
			s.sendResult(c, req.ID, map[string]any{"available": false, "armed": false, "ready": false})
			return
		}
		s.sendResult(c, req.ID, s.calibration.Status())
	case "vision.calibration.clear":
		if s.calibration != nil {
			s.calibration.Clear()
		}
		s.sendResult(c, req.ID, map[string]any{"cleared": true})
	case "spatial.info":
		if s.spatial == nil {
			s.sendResult(c, req.ID, map[string]any{"available": false, "eliteOnly": true, "rawFramesExposed": false})
			return
		}
		s.sendResult(c, req.ID, s.spatial.Info())
	case "spatial.begin":
		if s.spatial == nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UNAVAILABLE", "spatial mapper is unavailable")
			return
		}
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var in spatial.BeginRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		result, err := s.spatial.Begin(in)
		if err != nil {
			_ = s.sendError(c, req.ID, "SPATIAL_BEGIN_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "spatial.update":
		if s.spatial == nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UNAVAILABLE", "spatial mapper is unavailable")
			return
		}
		scene, _ := req.Params["scene"].(string)
		result, err := s.spatial.Update(scene)
		if err != nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UPDATE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "spatial.observe":
		if s.spatial == nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UNAVAILABLE", "spatial mapper is unavailable")
			return
		}
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var in spatial.ObserveRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		result, err := s.spatial.Observe(in)
		if err != nil {
			_ = s.sendError(c, req.ID, "SPATIAL_OBSERVE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "spatial.pose":
		if s.spatial == nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UNAVAILABLE", "spatial mapper is unavailable")
			return
		}
		scene, _ := req.Params["scene"].(string)
		result, err := s.spatial.Pose(scene)
		if err != nil {
			_ = s.sendError(c, req.ID, "SPATIAL_POSE_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "spatial.landmarks":
		if s.spatial == nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UNAVAILABLE", "spatial mapper is unavailable")
			return
		}
		scene, _ := req.Params["scene"].(string)
		result, err := s.spatial.Landmarks(scene)
		if err != nil {
			_ = s.sendError(c, req.ID, "SPATIAL_LANDMARKS_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, map[string]any{"scene": scene, "landmarks": result})
	case "spatial.project":
		if s.spatial == nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UNAVAILABLE", "spatial mapper is unavailable")
			return
		}
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var in spatial.ProjectRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		result, err := s.spatial.Project(in)
		if err != nil {
			_ = s.sendError(c, req.ID, "SPATIAL_PROJECT_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "spatial.export":
		if s.spatial == nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UNAVAILABLE", "spatial mapper is unavailable")
			return
		}
		scene, _ := req.Params["scene"].(string)
		result, err := s.spatial.Export(scene)
		if err != nil {
			_ = s.sendError(c, req.ID, "SPATIAL_EXPORT_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "spatial.import":
		if s.spatial == nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UNAVAILABLE", "spatial mapper is unavailable")
			return
		}
		raw, err := json.Marshal(req.Params)
		if err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		var in spatial.ImportRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			_ = s.sendError(c, req.ID, "BAD_PARAMS", err.Error())
			return
		}
		result, err := s.spatial.Import(in)
		if err != nil {
			_ = s.sendError(c, req.ID, "SPATIAL_IMPORT_FAILED", err.Error())
			return
		}
		s.sendResult(c, req.ID, result)
	case "spatial.end":
		if s.spatial == nil {
			_ = s.sendError(c, req.ID, "SPATIAL_UNAVAILABLE", "spatial mapper is unavailable")
			return
		}
		scene, _ := req.Params["scene"].(string)
		s.sendResult(c, req.ID, map[string]any{"scene": scene, "ended": s.spatial.End(scene)})
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

func (s *Server) handleVisionCalibrationFrame(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeMedia(r) {
		http.Error(w, "media authorization required", http.StatusUnauthorized)
		return
	}
	if s.calibration == nil {
		http.Error(w, "Vision calibration unavailable", http.StatusServiceUnavailable)
		return
	}
	frame, _, ok := s.calibration.Frame()
	if !ok {
		http.Error(w, "no captured calibration frame", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	_, _ = w.Write(frame)
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

func macroArgs(raw any) map[string]string {
	out := map[string]string{}
	values, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			out[key] = v
		case float64, bool, json.Number:
			out[key] = fmt.Sprint(v)
		}
	}
	return out
}

func (s *Server) runMacro(def macros.Definition, args map[string]string) (map[string]any, error) {
	for _, name := range def.Parameters {
		if strings.TrimSpace(args[name]) == "" {
			return nil, fmt.Errorf("macro parameter %q is required", name)
		}
	}
	if s.recorder != nil && s.recorder.Status().Recording {
		return nil, fmt.Errorf("stop the Action Recorder before running a macro")
	}
	s.macroMu.Lock()
	if s.macroRunning {
		s.macroMu.Unlock()
		return nil, fmt.Errorf("another macro is already running")
	}
	s.macroRunning = true
	s.macroMu.Unlock()
	defer func() {
		s.macroMu.Lock()
		s.macroRunning = false
		s.macroMu.Unlock()
	}()

	started := time.Now()
	log.Printf("macro/run-start: id=%q name=%q steps=%d parameters=%d", def.ID, def.Name, len(def.Steps), len(def.Parameters))
	steps := make([]map[string]any, 0, len(def.Steps))
	for index, step := range def.Steps {
		if step.DelayMs > 0 {
			time.Sleep(time.Duration(step.DelayMs) * time.Millisecond)
		}
		stepStarted := time.Now()
		switch step.Type {
		case "binding":
			var err error
			if step.HoldMs > 0 {
				_, err = s.controls.HoldBinding(step.Action, step.HoldMs)
			} else {
				_, err = s.controls.PressBinding(step.Action)
			}
			if err != nil {
				return nil, fmt.Errorf("step %d (%s): %w", index+1, step.Action, err)
			}
		case "raw":
			if err := platform.ValidateSafeChord(step.Key, step.Modifiers); err != nil {
				return nil, fmt.Errorf("step %d: %w", index+1, err)
			}
			if err := s.input.FocusGame(); err != nil {
				return nil, fmt.Errorf("step %d could not target Elite Dangerous: %w", index+1, err)
			}
			var err error
			if step.HoldMs > 0 {
				err = s.input.HoldChord(step.Key, step.Modifiers, step.HoldMs)
			} else {
				err = s.input.TapChord(step.Key, step.Modifiers)
			}
			if err != nil {
				return nil, fmt.Errorf("step %d (%s): %w", index+1, step.Key, err)
			}
		case "text":
			text, err := macros.ExpandText(step.Text, args)
			if err != nil {
				return nil, fmt.Errorf("step %d: %w", index+1, err)
			}
			if _, err := s.controls.TypeText(text, step.IntervalMs); err != nil {
				return nil, fmt.Errorf("step %d text input: %w", index+1, err)
			}
		default:
			return nil, fmt.Errorf("step %d has unsupported type %q", index+1, step.Type)
		}
		steps = append(steps, map[string]any{
			"index":      index + 1,
			"type":       step.Type,
			"durationMs": time.Since(stepStarted).Milliseconds(),
		})
		if time.Since(started) > 90*time.Second {
			return nil, fmt.Errorf("macro exceeded the 90-second execution limit")
		}
	}
	log.Printf("macro/run-complete: id=%q steps=%d duration=%s", def.ID, len(def.Steps), time.Since(started).Round(time.Millisecond))
	result := map[string]any{
		"id":         def.ID,
		"name":       def.Name,
		"steps":      steps,
		"durationMs": time.Since(started).Milliseconds(),
	}
	return result, nil
}

func firstPathEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func sameConfiguredPath(effective, candidate string) bool {
	effective = strings.TrimSpace(effective)
	candidate = strings.TrimSpace(candidate)
	if effective == "" || candidate == "" {
		return false
	}
	effective = filepath.Clean(effective)
	candidate = filepath.Clean(candidate)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(effective, candidate)
	}
	return effective == candidate
}

func pathSettingSource(effective, envValue, manualValue string) string {
	if sameConfiguredPath(effective, envValue) {
		return "environment"
	}
	if sameConfiguredPath(effective, manualValue) {
		return "manual"
	}
	return "auto"
}

func anyConfiguredPath(effective []string, candidates []string) bool {
	for _, e := range effective {
		for _, c := range candidates {
			if sameConfiguredPath(e, c) {
				return true
			}
		}
	}
	return false
}

func (s *Server) pathSettingsInfo(saved pathconfig.Config) map[string]any {
	saved = pathconfig.Normalize(saved)
	journalEnv := firstPathEnv("JACOB_JOURNAL_DIR", "EDBRIDGE_JOURNAL_DIR")
	bindingsEnv := firstPathEnv("JACOB_BINDINGS_DIR", "EDBRIDGE_BINDINGS_DIR")
	controlsEnv := firstPathEnv("JACOB_CONTROL_SCHEMES_DIR", "EDBRIDGE_CONTROL_SCHEMES_DIR")
	controlSource := "auto"
	if anyConfiguredPath(s.cfg.ControlSchemeDirs, []string{controlsEnv}) {
		controlSource = "environment"
	} else if anyConfiguredPath(s.cfg.ControlSchemeDirs, saved.ControlSchemeDirs) {
		controlSource = "manual"
	}
	return map[string]any{
		"saved": saved,
		"effective": map[string]any{
			"journalDir":        s.cfg.JournalDir,
			"bindingsDir":       s.bindings.Directory(),
			"controlSchemeDirs": append([]string(nil), s.cfg.ControlSchemeDirs...),
		},
		"source": map[string]any{
			"journalDir":        pathSettingSource(s.cfg.JournalDir, journalEnv, saved.JournalDir),
			"bindingsDir":       pathSettingSource(s.bindings.Directory(), bindingsEnv, saved.BindingsDir),
			"controlSchemeDirs": controlSource,
		},
		"environment": map[string]any{
			"journalDir":        journalEnv,
			"bindingsDir":       bindingsEnv,
			"controlSchemeDirs": controlsEnv,
		},
		"configFile":      pathconfig.FilePath(s.cfg.DataDir),
		"restartRequired": !pathconfig.Equal(saved, s.cfg.ManualPaths),
	}
}

func (s *Server) logNetworkSnapshot(reason string) {
	d := s.networkDiagnostics()
	host, _ := d["host"].(networkdiag.Report)
	profiles := make([]string, 0, len(host.Profiles))
	for _, p := range host.Profiles {
		profiles = append(profiles, p.InterfaceAlias+":"+p.NetworkCategory+":"+p.IPv4Connectivity)
	}
	log.Printf("network/%s: lan=%t bind=%q bindOK=%v addresses=%v preferred=%q firewallPresent=%t firewallOK=%t firewallEnabled=%t firewallProfiles=%q firewallPort=%q firewallRemote=%v profiles=%v inspectError=%q warnings=%v",
		reason, s.cfg.LANEnabled, s.cfg.Bind, d["bindOK"], d["addresses"], d["preferredAddress"], host.FirewallRulePresent, host.FirewallOK, host.FirewallEnabled, host.FirewallProfiles, host.FirewallPort, host.FirewallRemote, profiles, host.Error, d["warnings"])
}

func (s *Server) networkDiagnostics() map[string]any {
	addresses := localIPv4s()
	preferred := preferredLocalIPv4(addresses)
	port := portOf(s.cfg.Bind)
	preferredURL := ""
	if preferred != "" {
		preferredURL = "http://" + preferred + ":" + port + "/"
	}
	warnings := []string{}
	bindHost, _, err := net.SplitHostPort(s.cfg.Bind)
	bindOK := err == nil
	if err != nil {
		warnings = append(warnings, "configured bind address is invalid: "+s.cfg.Bind)
	} else if s.cfg.LANEnabled && net.ParseIP(bindHost) != nil && net.ParseIP(bindHost).IsLoopback() {
		bindOK = false
		warnings = append(warnings, "LAN is enabled but JACoB is bound only to loopback")
	} else if !s.cfg.LANEnabled && (bindHost == "0.0.0.0" || bindHost == "::") {
		bindOK = false
		warnings = append(warnings, "LAN is disabled but JACoB is listening on all interfaces")
	}
	if s.cfg.LANEnabled && preferred == "" {
		warnings = append(warnings, "no usable IPv4 LAN address was found")
	}
	exe, _ := os.Executable()
	portNum, _ := strconv.Atoi(port)
	host := networkdiag.Inspect(exe, portNum)
	for _, p := range host.Profiles {
		if strings.EqualFold(p.NetworkCategory, "Public") {
			warnings = append(warnings, "Windows reports an active Public network profile; JACoB intentionally opens its managed firewall rule only on Private/Domain networks")
		}
	}
	return map[string]any{
		"lanEnabled":       s.cfg.LANEnabled,
		"bind":             s.cfg.Bind,
		"bindOK":           bindOK,
		"port":             port,
		"addresses":        addresses,
		"preferredAddress": preferred,
		"preferredURL":     preferredURL,
		"host":             host,
		"warnings":         warnings,
	}
}

func (s *Server) systemInfo(includeSecret bool) map[string]any {
	host, _ := os.Hostname()
	lan := map[string]any{"enabled": s.cfg.LANEnabled}
	if s.cfg.LANEnabled {
		addresses := localIPv4s()
		lan["port"] = portOf(s.cfg.Bind)
		lan["addresses"] = addresses
		lan["preferredAddress"] = preferredLocalIPv4(addresses)
		lan["effectiveBind"] = s.cfg.Bind
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
		"prototype": buildinfo.Display, "version": buildinfo.Version, "product": "JACoB", "name": "Journal Aligned Control Bridge", "apiVersion": 12, "pid": os.Getpid(), "os": runtime.GOOS, "arch": runtime.GOARCH, "goRuntime": runtime.Version(), "host": host, "uptimeSeconds": int(time.Since(s.started).Seconds()), "mediaToken": mediaToken,
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
	_, ok := s.authenticateRemoteRequest(r)
	return ok
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

func videoParams(r *http.Request) (int, int, int, bool) {
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
	matteRaw := strings.ToLower(strings.TrimSpace(q.Get("matte")))
	matte := matteRaw == "1" || matteRaw == "true" || matteRaw == "yes" || matteRaw == "on"
	return width, quality, fps, matte
}

func captureVideoJPEG(c platform.CaptureDriver, maxWidth, quality int, matte bool) ([]byte, platform.CaptureInfo, error) {
	if matte {
		if m, ok := c.(platform.PrivacyMatteCaptureDriver); ok {
			return m.CapturePrivacyMatteJPEG(maxWidth, quality)
		}
	}
	return c.CaptureJPEG(maxWidth, quality)
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
	width, quality, _, matte := videoParams(r)
	b, info, err := captureVideoJPEG(s.capture, width, quality, matte)
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
	width, quality, fps, matte := videoParams(r)
	streamStarted := time.Now()
	framesSent := 0
	remote := remoteAddressKey(r)
	log.Printf("video/stream-start: remote=%q local=%t driver=%s width=%d quality=%d fps=%d matte=%t", remote, isLoopbackRequest(r), s.capture.Name(), width, quality, fps, matte)
	defer func() {
		log.Printf("video/stream-stop: remote=%q frames=%d duration=%s", remote, framesSent, time.Since(streamStarted).Round(time.Millisecond))
	}()
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
		b, info, err := captureVideoJPEG(s.capture, width, quality, matte)
		if err != nil {
			log.Printf("video/stream-error: remote=%q capture=%v", remote, err)
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
			if r.Context().Err() == nil {
				log.Printf("video/stream-error: remote=%q write=%v", remote, err)
			}
			return
		}
		framesSent++
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
	log.Printf("api/error: code=%s message=%q", code, truncateLogText(msg, 500))
	ok := false
	return s.send(c, envelope{Type: "response", ID: id, OK: &ok, Error: &apiError{Code: code, Message: msg}})
}

func truncateLogText(v string, max int) string {
	v = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(v), "\r", " "), "\n", " ")
	if max > 0 && len(v) > max {
		return v[:max] + "…"
	}
	return v
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

func isRFC1918(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 10 || (ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) || (ip4[0] == 192 && ip4[1] == 168)
}

func preferredRouteIPv4() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if a, ok := conn.LocalAddr().(*net.UDPAddr); ok && a.IP != nil && a.IP.To4() != nil {
		return a.IP.String()
	}
	return ""
}

func preferredLocalIPv4(addresses []string) string {
	if len(addresses) == 0 {
		return ""
	}
	route := preferredRouteIPv4()
	for _, address := range addresses {
		if address == route {
			return address
		}
	}
	for _, address := range addresses {
		if isRFC1918(net.ParseIP(address)) {
			return address
		}
	}
	return addresses[0]
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
			if ip == nil || ip.IsLoopback() || ip.To4() == nil || ip.IsLinkLocalUnicast() {
				continue
			}
			value := ip.String()
			if !seen[value] {
				seen[value] = true
				out = append(out, value)
			}
		}
	}
	route := preferredRouteIPv4()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i] == route {
			return true
		}
		if out[j] == route {
			return false
		}
		ai, aj := isRFC1918(net.ParseIP(out[i])), isRFC1918(net.ParseIP(out[j]))
		if ai != aj {
			return ai
		}
		return out[i] < out[j]
	})
	return out
}

func sessionPairToken(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) != "" {
		_ = os.Remove(filepath.Join(dataDir, "pairing-token.txt"))
	}
	return secureRandomToken()
}
