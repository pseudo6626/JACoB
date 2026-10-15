//go:build linux && !nocapture && cgo

package platform

/*
#cgo LDFLAGS: -ldl
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>
#include <poll.h>
#include <dlfcn.h>
#include <time.h>

struct wl_display;
struct wl_proxy;
struct wl_registry;
struct wl_interface;

struct wl_message {
	const char *name;
	const char *signature;
	const struct wl_interface **types;
};

struct wl_interface {
	const char *name;
	int version;
	int method_count;
	const struct wl_message *methods;
	int event_count;
	const struct wl_message *events;
};

struct wl_array {
	size_t size;
	size_t alloc;
	void *data;
};

static const struct wl_message jacob_registry_requests[] = {
	{"bind", "usun", NULL},
};

static const struct wl_message jacob_registry_events[] = {
	{"global", "usu", NULL},
	{"global_remove", "u", NULL},
};

static const struct wl_interface jacob_registry_interface = {
	"wl_registry", 1,
	1, jacob_registry_requests,
	2, jacob_registry_events
};

static const struct wl_message jacob_gamescope_requests[] = {
	{"destroy", "", NULL},
	{"set_app_target_refresh_cycle", "2uu", NULL},
	{"take_screenshot", "3suu", NULL},
};

static const struct wl_message jacob_gamescope_events[] = {
	{"feature_support", "uuu", NULL},
	{"active_display_info", "2sssua", NULL},
	{"screenshot_taken", "3s", NULL},
};

static const struct wl_interface jacob_gamescope_interface = {
	"gamescope_control", 3,
	3, jacob_gamescope_requests,
	3, jacob_gamescope_events
};

typedef struct wl_display* (*jacob_wl_display_connect_fn)(const char*);
typedef void (*jacob_wl_display_disconnect_fn)(struct wl_display*);
typedef int (*jacob_wl_display_roundtrip_fn)(struct wl_display*);
typedef int (*jacob_wl_display_flush_fn)(struct wl_display*);
typedef int (*jacob_wl_display_get_fd_fn)(struct wl_display*);
typedef int (*jacob_wl_display_prepare_read_fn)(struct wl_display*);
typedef int (*jacob_wl_display_read_events_fn)(struct wl_display*);
typedef void (*jacob_wl_display_cancel_read_fn)(struct wl_display*);
typedef int (*jacob_wl_display_dispatch_pending_fn)(struct wl_display*);
typedef struct wl_proxy* (*jacob_wl_proxy_marshal_flags_fn)(
	struct wl_proxy*, uint32_t, const struct wl_interface*, uint32_t, uint32_t, ...);
typedef int (*jacob_wl_proxy_add_listener_fn)(
	struct wl_proxy*, void (**)(void), void*);
typedef uint32_t (*jacob_wl_proxy_get_version_fn)(struct wl_proxy*);
typedef void (*jacob_wl_proxy_destroy_fn)(struct wl_proxy*);

typedef struct {
	void *library;
	jacob_wl_display_connect_fn display_connect;
	jacob_wl_display_disconnect_fn display_disconnect;
	jacob_wl_display_roundtrip_fn display_roundtrip;
	jacob_wl_display_flush_fn display_flush;
	jacob_wl_display_get_fd_fn display_get_fd;
	jacob_wl_display_prepare_read_fn display_prepare_read;
	jacob_wl_display_read_events_fn display_read_events;
	jacob_wl_display_cancel_read_fn display_cancel_read;
	jacob_wl_display_dispatch_pending_fn display_dispatch_pending;
	jacob_wl_proxy_marshal_flags_fn proxy_marshal_flags;
	jacob_wl_proxy_add_listener_fn proxy_add_listener;
	jacob_wl_proxy_get_version_fn proxy_get_version;
	jacob_wl_proxy_destroy_fn proxy_destroy;
} JacobWaylandAPI;

typedef struct {
	JacobWaylandAPI api;
	struct wl_display *display;
	struct wl_proxy *registry;
	struct wl_proxy *control;
	int screenshot_taken;
} JacobGamescopeCapture;

static void jacob_wayland_unload(JacobWaylandAPI *api) {
	if (!api) return;
	if (api->library) dlclose(api->library);
	memset(api, 0, sizeof(*api));
}

static int jacob_wayland_load(JacobWaylandAPI *api) {
	if (!api) return 0;
	memset(api, 0, sizeof(*api));

	api->library = dlopen("libwayland-client.so.0", RTLD_NOW | RTLD_LOCAL);
	if (!api->library) return 0;

	*(void**)(&api->display_connect) =
		dlsym(api->library, "wl_display_connect");
	*(void**)(&api->display_disconnect) =
		dlsym(api->library, "wl_display_disconnect");
	*(void**)(&api->display_roundtrip) =
		dlsym(api->library, "wl_display_roundtrip");
	*(void**)(&api->display_flush) =
		dlsym(api->library, "wl_display_flush");
	*(void**)(&api->display_get_fd) =
		dlsym(api->library, "wl_display_get_fd");
	*(void**)(&api->display_prepare_read) =
		dlsym(api->library, "wl_display_prepare_read");
	*(void**)(&api->display_read_events) =
		dlsym(api->library, "wl_display_read_events");
	*(void**)(&api->display_cancel_read) =
		dlsym(api->library, "wl_display_cancel_read");
	*(void**)(&api->display_dispatch_pending) =
		dlsym(api->library, "wl_display_dispatch_pending");
	*(void**)(&api->proxy_marshal_flags) =
		dlsym(api->library, "wl_proxy_marshal_flags");
	*(void**)(&api->proxy_add_listener) =
		dlsym(api->library, "wl_proxy_add_listener");
	*(void**)(&api->proxy_get_version) =
		dlsym(api->library, "wl_proxy_get_version");
	*(void**)(&api->proxy_destroy) =
		dlsym(api->library, "wl_proxy_destroy");

	if (!api->display_connect ||
		!api->display_disconnect ||
		!api->display_roundtrip ||
		!api->display_flush ||
		!api->display_get_fd ||
		!api->display_prepare_read ||
		!api->display_read_events ||
		!api->display_cancel_read ||
		!api->display_dispatch_pending ||
		!api->proxy_marshal_flags ||
		!api->proxy_add_listener ||
		!api->proxy_get_version ||
		!api->proxy_destroy) {
		jacob_wayland_unload(api);
		return 0;
	}

	return 1;
}

static int jacob_gamescope_library_available(void) {
	JacobWaylandAPI api;
	if (!jacob_wayland_load(&api)) return 0;
	jacob_wayland_unload(&api);
	return 1;
}

static void jacob_gamescope_feature_support(
	void *data, struct wl_proxy *control,
	uint32_t feature, uint32_t version, uint32_t flags) {
	(void)data; (void)control; (void)feature; (void)version; (void)flags;
}

static void jacob_gamescope_active_display_info(
	void *data, struct wl_proxy *control,
	const char *connector_name, const char *display_make,
	const char *display_model, uint32_t display_flags,
	struct wl_array *refresh_rates) {
	(void)data; (void)control; (void)connector_name; (void)display_make;
	(void)display_model; (void)display_flags; (void)refresh_rates;
}

static void jacob_gamescope_screenshot_taken(
	void *data, struct wl_proxy *control, const char *path) {
	(void)control; (void)path;
	JacobGamescopeCapture *ctx = (JacobGamescopeCapture*)data;
	if (ctx) ctx->screenshot_taken = 1;
}

static void (*jacob_gamescope_listener[])(void) = {
	(void (*)(void))jacob_gamescope_feature_support,
	(void (*)(void))jacob_gamescope_active_display_info,
	(void (*)(void))jacob_gamescope_screenshot_taken,
};

static void jacob_registry_global_remove(
	void *data, struct wl_registry *registry, uint32_t name) {
	(void)data; (void)registry; (void)name;
}

static void jacob_registry_global(
	void *data, struct wl_registry *registry, uint32_t name,
	const char *interface_name, uint32_t version) {
	JacobGamescopeCapture *ctx = (JacobGamescopeCapture*)data;
	if (!ctx || ctx->control || !interface_name) return;
	if (strcmp(interface_name, "gamescope_control") != 0) return;

	uint32_t bind_version = version < 3 ? version : 3;
	if (bind_version < 3) return;

	ctx->control = ctx->api.proxy_marshal_flags(
		(struct wl_proxy*)registry,
		0,
		&jacob_gamescope_interface,
		bind_version,
		0,
		name,
		jacob_gamescope_interface.name,
		bind_version,
		NULL);

	if (ctx->control) {
		ctx->api.proxy_add_listener(
			ctx->control, jacob_gamescope_listener, ctx);
	}
}

static void (*jacob_registry_listener[])(void) = {
	(void (*)(void))jacob_registry_global,
	(void (*)(void))jacob_registry_global_remove,
};

static void jacob_gamescope_close(JacobGamescopeCapture *ctx) {
	if (!ctx) return;

	if (ctx->control && ctx->api.proxy_destroy) {
		ctx->api.proxy_destroy(ctx->control);
		ctx->control = NULL;
	}
	if (ctx->registry && ctx->api.proxy_destroy) {
		ctx->api.proxy_destroy(ctx->registry);
		ctx->registry = NULL;
	}
	if (ctx->display && ctx->api.display_disconnect) {
		ctx->api.display_disconnect(ctx->display);
		ctx->display = NULL;
	}

	jacob_wayland_unload(&ctx->api);
	free(ctx);
}

static JacobGamescopeCapture* jacob_gamescope_open(const char *socket_name) {
	JacobGamescopeCapture *ctx =
		(JacobGamescopeCapture*)calloc(1, sizeof(JacobGamescopeCapture));
	if (!ctx) return NULL;

	if (!jacob_wayland_load(&ctx->api)) {
		jacob_gamescope_close(ctx);
		return NULL;
	}

	ctx->display = ctx->api.display_connect(
		socket_name && *socket_name ? socket_name : "gamescope-0");
	if (!ctx->display) {
		jacob_gamescope_close(ctx);
		return NULL;
	}

	// wl_display.get_registry is opcode 1 and constructs wl_registry.
	ctx->registry = ctx->api.proxy_marshal_flags(
		(struct wl_proxy*)ctx->display,
		1,
		&jacob_registry_interface,
		1,
		0,
		NULL);
	if (!ctx->registry) {
		jacob_gamescope_close(ctx);
		return NULL;
	}

	if (ctx->api.proxy_add_listener(
			ctx->registry, jacob_registry_listener, ctx) != 0) {
		jacob_gamescope_close(ctx);
		return NULL;
	}

	// First roundtrip receives globals and binds gamescope_control.
	// Second roundtrip finishes initial control events.
	if (ctx->api.display_roundtrip(ctx->display) < 0 ||
		ctx->api.display_roundtrip(ctx->display) < 0 ||
		!ctx->control) {
		jacob_gamescope_close(ctx);
		return NULL;
	}

	return ctx;
}

static int64_t jacob_monotonic_ms(void) {
	struct timespec ts;
	if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0) return 0;
	return (int64_t)ts.tv_sec * 1000LL + (int64_t)(ts.tv_nsec / 1000000LL);
}

static int jacob_gamescope_take_base_plane_png(
	JacobGamescopeCapture *ctx, const char *path, int timeout_ms) {
	if (!ctx || !ctx->display || !ctx->control || !path || !*path) return 0;
	if (timeout_ms < 50) timeout_ms = 50;

	ctx->screenshot_taken = 0;

	// gamescope_control.take_screenshot opcode 2:
	// type 1 = base_plane_only ("Just the game"), flags 0.
	ctx->api.proxy_marshal_flags(
		ctx->control,
		2,
		NULL,
		ctx->api.proxy_get_version(ctx->control),
		0,
		path,
		(uint32_t)1,
		(uint32_t)0);

	if (ctx->api.display_flush(ctx->display) < 0) return 0;

	int fd = ctx->api.display_get_fd(ctx->display);
	if (fd < 0) return 0;

	int64_t deadline = jacob_monotonic_ms() + timeout_ms;

	while (!ctx->screenshot_taken) {
		while (ctx->api.display_prepare_read(ctx->display) != 0) {
			if (ctx->api.display_dispatch_pending(ctx->display) < 0) return 0;
			if (ctx->screenshot_taken) return 1;
		}

		if (ctx->api.display_flush(ctx->display) < 0) {
			ctx->api.display_cancel_read(ctx->display);
			return 0;
		}

		int64_t now = jacob_monotonic_ms();
		int remaining = (int)(deadline - now);
		if (remaining <= 0) {
			ctx->api.display_cancel_read(ctx->display);
			return -1;
		}

		struct pollfd pfd;
		memset(&pfd, 0, sizeof(pfd));
		pfd.fd = fd;
		pfd.events = POLLIN;

		int pr = poll(&pfd, 1, remaining);
		if (pr == 0) {
			ctx->api.display_cancel_read(ctx->display);
			return -1;
		}
		if (pr < 0) {
			ctx->api.display_cancel_read(ctx->display);
			return 0;
		}
		if (!(pfd.revents & POLLIN)) {
			ctx->api.display_cancel_read(ctx->display);
			return 0;
		}

		if (ctx->api.display_read_events(ctx->display) < 0) return 0;
		if (ctx->api.display_dispatch_pending(ctx->display) < 0) return 0;
	}

	return 1;
}
*/
import "C"

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const gamescopeCaptureDriverName = "linux-gamescope-base-plane-v1"

type gamescopeCapture struct {
	mu          sync.Mutex
	native      *C.JacobGamescopeCapture
	socket      string
	cached      GameFrame
	captures    uint64
	cacheHits   uint64
	focusReject uint64
	failures    uint64
	timeouts    uint64
	blankFrames uint64
	lastLatency time.Duration
}

type linuxCaptureMux struct {
	gamescope *gamescopeCapture
	x11       *linuxCapture
}

func newLinuxCaptureDriver() CaptureDriver {
	return &linuxCaptureMux{
		gamescope: &gamescopeCapture{},
		x11:       &linuxCapture{},
	}
}

func (m *linuxCaptureMux) gamescopeActive() bool {
	return m.gamescope != nil && m.gamescope.Available()
}

func (m *linuxCaptureMux) Name() string {
	if m.gamescopeActive() {
		return m.gamescope.Name()
	}
	return m.x11.Name()
}

func (m *linuxCaptureMux) Available() bool {
	return m.gamescopeActive() || m.x11.Available()
}

func (m *linuxCaptureMux) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	if m.gamescopeActive() {
		return m.gamescope.CaptureFrame(maxWidth, intent)
	}
	return m.x11.CaptureFrame(maxWidth, intent)
}

func (m *linuxCaptureMux) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	if m.gamescopeActive() {
		return m.gamescope.CaptureJPEG(maxWidth, quality)
	}
	return m.x11.CaptureJPEG(maxWidth, quality)
}

func (m *linuxCaptureMux) CaptureStats() map[string]any {
	if m.gamescopeActive() {
		return map[string]any{
			"activeBackend": "gamescope",
			"gamescope":     m.gamescope.CaptureStats(),
			"x11Fallback":   m.x11.CaptureStats(),
		}
	}
	return map[string]any{
		"activeBackend": "x11",
		"x11Fallback":   m.x11.CaptureStats(),
	}
}

func (g *gamescopeCapture) Name() string {
	return gamescopeCaptureDriverName
}

func (g *gamescopeCapture) socketName() string {
	if v := strings.TrimSpace(os.Getenv("GAMESCOPE_WAYLAND_DISPLAY")); v != "" {
		return filepath.Base(v)
	}
	return "gamescope-0"
}

func (g *gamescopeCapture) socketPath() string {
	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(runtimeDir, g.socketName())
}

func (g *gamescopeCapture) Available() bool {
	if C.jacob_gamescope_library_available() == 0 {
		return false
	}
	info, err := os.Stat(g.socketPath())
	return err == nil && info.Mode()&os.ModeSocket != 0
}

func (g *gamescopeCapture) ensureNativeLocked() error {
	socket := g.socketName()
	if g.native != nil && g.socket == socket {
		return nil
	}
	if g.native != nil {
		C.jacob_gamescope_close(g.native)
		g.native = nil
		g.socket = ""
	}

	cs := C.CString(socket)
	native := C.jacob_gamescope_open(cs)
	C.free(unsafe.Pointer(cs))
	if native == nil {
		return fmt.Errorf("Gamescope control protocol v3+ is unavailable on %s", socket)
	}

	g.native = native
	g.socket = socket
	return nil
}

func (g *gamescopeCapture) dropNativeLocked() {
	if g.native != nil {
		C.jacob_gamescope_close(g.native)
		g.native = nil
		g.socket = ""
	}
}

func linuxGamescopeEliteDirectlyFocused() bool {
	displays, probes := linuxFocusSnapshot()
	if len(probes) == 0 {
		return false
	}

	for _, probe := range probes {
		if probe.focusedGFX == 359320 {
			_, _, ok := findEliteFocusedWindowTarget(probe.window, displays)
			return ok
		}
	}
	for _, probe := range probes {
		if probe.focusedGFX != 0 {
			return false
		}
	}

	for _, probe := range probes {
		if probe.focusedApp == 359320 {
			_, _, ok := findEliteFocusedWindowTarget(probe.window, displays)
			return ok
		}
	}
	return false
}

func (g *gamescopeCapture) blankDimensionsLocked() (int, int) {
	if g.cached.SourceWidth > 0 && g.cached.SourceHeight > 0 {
		return g.cached.SourceWidth, g.cached.SourceHeight
	}
	return 1280, 800
}

func (g *gamescopeCapture) focusBlankLocked(reason string) GameFrame {
	w, h := g.blankDimensionsLocked()
	frame := blankGameFrame(g.Name(), w, h, reason)
	g.cached = frame
	g.focusReject++
	return frame
}

func (g *gamescopeCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if intent == CaptureIntentLive &&
		!g.cached.CapturedAt.IsZero() &&
		time.Since(g.cached.CapturedAt) <= 100*time.Millisecond {
		if frame, ok := linuxCachedFrameForWidth(g.cached, maxWidth); ok {
			g.cacheHits++
			return frame, nil
		}
	}

	// This deliberately requires the exact Elite CLIENT XID to be the current
	// Gamescope game surface. Steam overlays, launcher windows, idle/saver UI,
	// and unresolvable Wayland surfaces all fail closed.
	if !linuxGamescopeEliteDirectlyFocused() {
		return g.focusBlankLocked(
			"Elite is not the directly presented Gamescope client; capture withheld while launcher, Steam overlay, idle/saver UI, or another surface may be active"), nil
	}

	started := time.Now()

	if err := g.ensureNativeLocked(); err != nil {
		g.failures++
		return GameFrame{}, err
	}

	tmp, err := os.CreateTemp("", "jacob-gamescope-base-*.png")
	if err != nil {
		g.failures++
		return GameFrame{}, fmt.Errorf("create temporary Gamescope capture path: %w", err)
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(path)
		g.failures++
		return GameFrame{}, fmt.Errorf("close temporary Gamescope capture path: %w", err)
	}
	// Gamescope writes the file itself. Remove the placeholder first, and always
	// remove the screenshot immediately after decoding it.
	_ = os.Remove(path)
	defer os.Remove(path)

	cp := C.CString(path)
	rc := C.jacob_gamescope_take_base_plane_png(g.native, cp, C.int(1200))
	C.free(unsafe.Pointer(cp))

	if rc <= 0 {
		g.dropNativeLocked()
		g.failures++
		if rc < 0 {
			g.timeouts++
			return GameFrame{}, fmt.Errorf("Gamescope base-plane screenshot timed out")
		}
		return GameFrame{}, fmt.Errorf("Gamescope base-plane screenshot request failed")
	}

	// Close the privacy race: if focus changed while Gamescope was producing the
	// frame, discard it without decoding or returning pixels.
	if !linuxGamescopeEliteDirectlyFocused() {
		return g.focusBlankLocked(
			"Elite focus changed during Gamescope capture; frame discarded"), nil
	}

	f, err := os.Open(path)
	if err != nil {
		g.failures++
		return GameFrame{}, fmt.Errorf("open Gamescope base-plane screenshot: %w", err)
	}
	img, err := png.Decode(f)
	closeErr := f.Close()
	if err != nil {
		g.failures++
		return GameFrame{}, fmt.Errorf("decode Gamescope base-plane screenshot: %w", err)
	}
	if closeErr != nil {
		g.failures++
		return GameFrame{}, fmt.Errorf("close Gamescope base-plane screenshot: %w", closeErr)
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		g.failures++
		return GameFrame{}, fmt.Errorf("Gamescope base-plane screenshot returned invalid dimensions %dx%d", w, h)
	}

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)

	pix := rgba.Pix
	if linuxFrameLooksBlank(pix, w, h) {
		g.blankFrames++
		frame := blankGameFrame(
			g.Name(), w, h,
			"Gamescope base-plane screenshot was effectively black; Vision results withheld")
		g.captures++
		g.lastLatency = time.Since(started)
		g.cached = frame
		return frame, nil
	}

	full := GameFrame{
		Pix:          pix,
		Width:        w,
		Height:       h,
		Stride:       rgba.Stride,
		Format:       PixelFormatRGBA8,
		CapturedAt:   time.Now().UTC(),
		Driver:       g.Name(),
		SourceWidth:  w,
		SourceHeight: h,
		Verified:     true,
		Foreground:   true,
		Blanked:      false,
	}

	g.captures++
	g.lastLatency = time.Since(started)
	g.cached = full

	frame, _ := linuxCachedFrameForWidth(full, maxWidth)
	return frame, nil
}

func (g *gamescopeCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	frame, err := g.CaptureFrame(maxWidth, CaptureIntentLive)
	if err != nil {
		return nil, CaptureInfo{
			Driver:  g.Name(),
			Blanked: true,
			Reason:  err.Error(),
		}, err
	}
	return EncodeGameFrameJPEG(frame, quality)
}

func (g *gamescopeCapture) CaptureStats() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()

	age := int64(-1)
	if !g.cached.CapturedAt.IsZero() {
		age = time.Since(g.cached.CapturedAt).Milliseconds()
	}

	return map[string]any{
		"driver":             g.Name(),
		"protocol":           "gamescope_control.take_screenshot",
		"screenshotType":     "base_plane_only",
		"captures":           g.captures,
		"cacheHits":          g.cacheHits,
		"focusRejects":       g.focusReject,
		"failures":           g.failures,
		"timeouts":           g.timeouts,
		"blankFrames":        g.blankFrames,
		"lastLatencyMs":      g.lastLatency.Milliseconds(),
		"cachedFrameAgeMs":   age,
		"cacheWindowMs":      100,
		"socket":             g.socketName(),
		"temporaryFilesOnly": true,
		"eliteFocusGuard":    "exact-client-before-and-after",
	}
}
