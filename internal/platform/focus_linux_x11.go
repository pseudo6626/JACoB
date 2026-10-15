//go:build linux && cgo

package platform

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/Xatom.h>
#include <stdlib.h>
#include <string.h>

static int jacob_focus_last_x_error = 0;
static int jacob_focus_x_error_handler(Display *d, XErrorEvent *e) {
    (void)d; (void)e;
    jacob_focus_last_x_error = 1;
    return 0;
}

static int jacob_focus_read_u32(Display *d, Window w, const char *name, unsigned long *out) {
    if (!d || !name || !out) return 0;
    Atom prop = XInternAtom(d, name, True);
    if (prop == None) return 0;
    Atom actual = None;
    int format = 0;
    unsigned long nitems = 0, bytes_after = 0;
    unsigned char *data = NULL;
    int rc = XGetWindowProperty(d, w, prop, 0, 1, False, AnyPropertyType,
        &actual, &format, &nitems, &bytes_after, &data);
    if (rc != Success || !data || nitems < 1 || format != 32) {
        if (data) XFree(data);
        return 0;
    }
    *out = *((unsigned long*)data);
    XFree(data);
    return 1;
}

static void jacob_focus_classify(Display *d, Window win, int *class_elite, int *class_jacob) {
    if (!class_elite || !class_jacob) return;
    *class_elite = 0;
    *class_jacob = 0;
    XClassHint hint;
    memset(&hint, 0, sizeof(hint));
    if (!XGetClassHint(d, win, &hint)) return;
    const char *elite = "steam_app_359320";
    if ((hint.res_name && strstr(hint.res_name, elite)) ||
        (hint.res_class && strstr(hint.res_class, elite))) {
        *class_elite = 1;
    }
    if ((hint.res_name && strstr(hint.res_name, "jacob-hud")) ||
        (hint.res_class && strstr(hint.res_class, "JACoB"))) {
        *class_jacob = 1;
    }
    if (hint.res_name) XFree(hint.res_name);
    if (hint.res_class) XFree(hint.res_class);
}

static int jacob_focus_probe_display(const char *display_name,
    unsigned long *focused_app,
    unsigned long *focused_gfx,
    unsigned long *window_id) {
    if (!display_name || !focused_app || !focused_gfx || !window_id) return 0;
    *focused_app = 0;
    *focused_gfx = 0;
    *window_id = 0;

    Display *d = XOpenDisplay(display_name);
    if (!d) return 0;
    Window root = DefaultRootWindow(d);
    unsigned long value = 0;

    jacob_focus_read_u32(d, root, "GAMESCOPE_FOCUSED_APP", focused_app);
    jacob_focus_read_u32(d, root, "GAMESCOPE_FOCUSED_APP_GFX", focused_gfx);

    if (jacob_focus_read_u32(d, root, "GAMESCOPE_FOCUSED_WINDOW", &value)) {
        *window_id = value;
    }
    if (!*window_id) {
        value = 0;
        if (jacob_focus_read_u32(d, root, "_NET_ACTIVE_WINDOW", &value)) {
            *window_id = value;
        }
    }
    if (!*window_id) {
        Window focus = None;
        int revert = RevertToNone;
        XGetInputFocus(d, &focus, &revert);
        if (focus != None && focus != PointerRoot) *window_id = (unsigned long)focus;
    }

    XCloseDisplay(d);
    return (*focused_app || *focused_gfx || *window_id) ? 1 : 0;
}

static int jacob_focus_inspect_window(const char *display_name,
    unsigned long window_id,
    unsigned long *steam_game,
    unsigned long *pid,
    int *class_elite,
    int *class_jacob,
    int *viewable) {
    if (!display_name || !window_id || !steam_game || !pid || !class_elite || !class_jacob || !viewable) return 0;
    *steam_game = 0;
    *pid = 0;
    *class_elite = 0;
    *class_jacob = 0;
    *viewable = 0;

    Display *d = XOpenDisplay(display_name);
    if (!d) return 0;
    Window win = (Window)window_id;
    XWindowAttributes attr;
    jacob_focus_last_x_error = 0;
    int (*old_handler)(Display*, XErrorEvent*) = XSetErrorHandler(jacob_focus_x_error_handler);
    int got_attr = XGetWindowAttributes(d, win, &attr);
    XSync(d, False);
    XSetErrorHandler(old_handler);
    if (!got_attr || jacob_focus_last_x_error) {
        XCloseDisplay(d);
        return 0;
    }

    *viewable = attr.map_state == IsViewable ? 1 : 0;
    jacob_focus_read_u32(d, win, "STEAM_GAME", steam_game);
    jacob_focus_read_u32(d, win, "_NET_WM_PID", pid);
    jacob_focus_classify(d, win, class_elite, class_jacob);

    XCloseDisplay(d);
    return 1;
}

static int jacob_focus_window_is_elite_client_dpy(Display *d, Window win) {
    if (!d || !win) return 0;
    char *name = NULL;
    int ok = 0;
    if (XFetchName(d, win, &name) && name) {
        if (strstr(name, "Elite - Dangerous (CLIENT)") != NULL) ok = 1;
        XFree(name);
    }
    return ok;
}

static int jacob_focus_window_is_elite_client(const char *display_name, unsigned long window_id) {
    if (!display_name || !window_id) return 0;
    Display *d = XOpenDisplay(display_name);
    if (!d) return 0;
    int ok = jacob_focus_window_is_elite_client_dpy(d, (Window)window_id);
    XCloseDisplay(d);
    return ok;
}

static int jacob_focus_find_elite_class_window(const char *display_name,
    unsigned long self_pid,
    unsigned long *window_id,
    unsigned long *pid) {
    if (!display_name || !window_id || !pid) return 0;
    *window_id = 0;
    *pid = 0;

    Display *d = XOpenDisplay(display_name);
    if (!d) return 0;
    Window root = DefaultRootWindow(d), rr = 0, parent = 0, *children = NULL;
    unsigned int nchildren = 0;
    if (!XQueryTree(d, root, &rr, &parent, &children, &nchildren)) {
        XCloseDisplay(d);
        return 0;
    }

    for (int i = (int)nchildren - 1; i >= 0; i--) {
        Window win = children[i];
        XWindowAttributes attr;
        jacob_focus_last_x_error = 0;
        int (*old_handler)(Display*, XErrorEvent*) = XSetErrorHandler(jacob_focus_x_error_handler);
        int got_attr = XGetWindowAttributes(d, win, &attr);
        XSync(d, False);
        XSetErrorHandler(old_handler);
        if (!got_attr || jacob_focus_last_x_error || attr.map_state != IsViewable) continue;

        unsigned long wp = 0;
        jacob_focus_read_u32(d, win, "_NET_WM_PID", &wp);
        if (self_pid && wp == self_pid) continue;

        int class_elite = 0, class_jacob = 0;
        jacob_focus_classify(d, win, &class_elite, &class_jacob);
        if (class_elite && !class_jacob && jacob_focus_window_is_elite_client_dpy(d, win)) {
            *window_id = (unsigned long)win;
            *pid = wp;
            if (children) XFree(children);
            XCloseDisplay(d);
            return 1;
        }
    }

    if (children) XFree(children);
    XCloseDisplay(d);
    return 0;
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unsafe"
)

type linuxFocusEvidence struct {
	display    string
	focusedApp uint64
	focusedGFX uint64
	window     uintptr
}

type linuxWindowEvidence struct {
	display    string
	window     uintptr
	steamGame  uint64
	pid        int
	classElite bool
	classJACoB bool
	viewable   bool
}

var linuxEliteTargetCache struct {
	sync.Mutex
	display string
	window  uintptr
}

func rememberLinuxEliteWindowTarget(display string, window uintptr) {
	if strings.TrimSpace(display) == "" || window == 0 {
		return
	}
	linuxEliteTargetCache.Lock()
	linuxEliteTargetCache.display = display
	linuxEliteTargetCache.window = window
	linuxEliteTargetCache.Unlock()
}

func cachedLinuxEliteWindowTarget() (string, uintptr, bool) {
	linuxEliteTargetCache.Lock()
	display := linuxEliteTargetCache.display
	window := linuxEliteTargetCache.window
	linuxEliteTargetCache.Unlock()
	if strings.TrimSpace(display) == "" || window == 0 {
		return "", 0, false
	}
	ev, ok := inspectLinuxX11WindowEvidence(display, window)
	if !ok || !linuxWindowIsElite(ev) {
		return "", 0, false
	}
	return display, window, true
}

func linuxX11Displays() []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}

	add(os.Getenv("DISPLAY"))
	matches, _ := filepath.Glob("/tmp/.X11-unix/X*")
	sort.Strings(matches)
	for _, path := range matches {
		name := filepath.Base(path)
		if len(name) > 1 && name[0] == 'X' {
			add(":" + name[1:])
		}
	}
	return out
}

func probeLinuxX11Display(display string) (linuxFocusEvidence, bool) {
	cd := C.CString(display)
	defer C.free(unsafe.Pointer(cd))
	var app, gfx, win C.ulong
	if C.jacob_focus_probe_display(cd, &app, &gfx, &win) == 0 {
		return linuxFocusEvidence{}, false
	}
	return linuxFocusEvidence{
		display:    display,
		focusedApp: uint64(app),
		focusedGFX: uint64(gfx),
		window:     uintptr(win),
	}, true
}

func inspectLinuxX11WindowEvidence(display string, window uintptr) (linuxWindowEvidence, bool) {
	if window == 0 {
		return linuxWindowEvidence{}, false
	}
	cd := C.CString(display)
	defer C.free(unsafe.Pointer(cd))
	var steamGame, pid C.ulong
	var classElite, classJACoB, viewable C.int
	if C.jacob_focus_inspect_window(cd, C.ulong(window), &steamGame, &pid, &classElite, &classJACoB, &viewable) == 0 {
		return linuxWindowEvidence{}, false
	}
	return linuxWindowEvidence{
		display: display, window: window, steamGame: uint64(steamGame), pid: int(pid),
		classElite: classElite != 0, classJACoB: classJACoB != 0, viewable: viewable != 0,
	}, true
}

func linuxWindowHasEliteClientTitle(ev linuxWindowEvidence) bool {
	if strings.TrimSpace(ev.display) == "" || ev.window == 0 {
		return false
	}
	cd := C.CString(ev.display)
	defer C.free(unsafe.Pointer(cd))
	return C.jacob_focus_window_is_elite_client(cd, C.ulong(ev.window)) != 0
}

func linuxWindowIsElite(ev linuxWindowEvidence) bool {
	if !ev.viewable || ev.window == 0 {
		return false
	}
	if ev.pid == os.Getpid() || ev.classJACoB {
		return false
	}
	if !linuxWindowHasEliteClientTitle(ev) {
		return false
	}
	if ev.classElite {
		return true
	}
	return ev.steamGame == 359320 && ev.pid > 0 && linuxPIDTreeIsElite(ev.pid)
}

func inspectLinuxX11Window(display string, window uintptr) (exists bool, elite bool) {
	ev, ok := inspectLinuxX11WindowEvidence(display, window)
	return ok, ok && linuxWindowIsElite(ev)
}

func inspectFocusedWindowKind(window uintptr, displays []string) (known bool, jacob bool) {
	if window == 0 {
		return false, false
	}
	for _, display := range displays {
		if ev, ok := inspectLinuxX11WindowEvidence(display, window); ok {
			return true, ev.classJACoB || ev.pid == os.Getpid()
		}
	}
	return false, false
}

func findEliteFocusedWindowTarget(window uintptr, displays []string) (string, uintptr, bool) {
	if window == 0 {
		return "", 0, false
	}
	for _, display := range displays {
		if ev, ok := inspectLinuxX11WindowEvidence(display, window); ok && linuxWindowIsElite(ev) {
			return display, window, true
		}
	}
	return "", 0, false
}

func findEliteClassWindowTarget(displays []string) (string, uintptr, bool) {
	for _, display := range displays {
		cd := C.CString(display)
		var win, pid C.ulong
		ok := C.jacob_focus_find_elite_class_window(cd, C.ulong(os.Getpid()), &win, &pid) != 0
		C.free(unsafe.Pointer(cd))
		if !ok || win == 0 {
			continue
		}
		ev, exists := inspectLinuxX11WindowEvidence(display, uintptr(win))
		if exists && linuxWindowIsElite(ev) {
			return display, uintptr(win), true
		}
	}
	return "", 0, false
}

func linuxFocusSnapshot() ([]string, []linuxFocusEvidence) {
	displays := linuxX11Displays()
	probes := make([]linuxFocusEvidence, 0, len(displays))
	for _, display := range displays {
		if probe, ok := probeLinuxX11Display(display); ok {
			probes = append(probes, probe)
		}
	}
	return displays, probes
}

// linuxNativeEliteWindowTarget returns only a real, viewable Elite client X11
// window. JACoB's own HUD deliberately carries STEAM_GAME=359320 so Gamescope
// composites it with Elite; that property alone is never enough to identify
// the game window here.
func linuxNativeEliteWindowTarget() (display string, window uintptr, ok bool) {
	displays, probes := linuxFocusSnapshot()
	if len(probes) == 0 {
		return "", 0, false
	}

	resolveEliteAppProbe := func(probe linuxFocusEvidence) (string, uintptr, bool) {
		if d, w, found := findEliteFocusedWindowTarget(probe.window, displays); found {
			rememberLinuxEliteWindowTarget(d, w)
			return d, w, true
		}

		if known, jacob := inspectFocusedWindowKind(probe.window, displays); known {
			if jacob {
				if d, w, found := cachedLinuxEliteWindowTarget(); found {
					return d, w, true
				}
				if d, w, found := findEliteClassWindowTarget(displays); found {
					rememberLinuxEliteWindowTarget(d, w)
					return d, w, true
				}
			}
			return "", 0, false
		}

		if d, w, found := findEliteClassWindowTarget(displays); found {
			rememberLinuxEliteWindowTarget(d, w)
			return d, w, true
		}
		return "", 0, false
	}

	for _, probe := range probes {
		if probe.focusedGFX == 359320 {
			return resolveEliteAppProbe(probe)
		}
	}
	for _, probe := range probes {
		if probe.focusedGFX != 0 && probe.focusedGFX != 359320 {
			return "", 0, false
		}
	}

	for _, probe := range probes {
		if probe.focusedApp == 359320 {
			return resolveEliteAppProbe(probe)
		}
	}

	for _, probe := range probes {
		if d, w, found := findEliteFocusedWindowTarget(probe.window, displays); found {
			rememberLinuxEliteWindowTarget(d, w)
			return d, w, true
		}
	}
	return "", 0, false
}

func linuxNativeFocusAvailable() bool {
	for _, display := range linuxX11Displays() {
		if _, ok := probeLinuxX11Display(display); ok {
			return true
		}
	}
	return false
}

// linuxNativeFocusProbe answers the input-safety question. Steam app id 359320
// is shared by the Frontier launcher and the game, so input is allowed only when
// linuxNativeEliteWindowTarget resolves the actual Elite - Dangerous (CLIENT)
// window (or safely recovers it while JACoB's own HUD is the focused surface).
func linuxNativeFocusProbe() (resolved bool, elite bool, window uintptr) {
	displays, probes := linuxFocusSnapshot()
	if len(probes) == 0 {
		return false, false, 0
	}

	for _, probe := range probes {
		if probe.focusedGFX == 359320 {
			if _, w, ok := linuxNativeEliteWindowTarget(); ok {
				return true, true, w
			}
			return true, false, probe.window
		}
	}
	for _, probe := range probes {
		if probe.focusedGFX != 0 && probe.focusedGFX != 359320 {
			return true, false, probe.window
		}
	}
	for _, probe := range probes {
		if probe.focusedApp == 359320 {
			if _, w, ok := linuxNativeEliteWindowTarget(); ok {
				return true, true, w
			}
			return true, false, probe.window
		}
	}
	for _, probe := range probes {
		if _, w, found := findEliteFocusedWindowTarget(probe.window, displays); found {
			return true, true, w
		}
	}
	return false, false, 0
}

func linuxNativeEliteFocus() (bool, bool) {
	resolved, elite, _ := linuxNativeFocusProbe()
	return resolved, elite
}

func linuxNativeEliteWindow() (uintptr, bool) {
	_, window, ok := linuxNativeEliteWindowTarget()
	return window, ok
}
