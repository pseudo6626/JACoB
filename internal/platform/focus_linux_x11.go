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
    int *class_elite) {
    if (!display_name || !window_id || !steam_game || !pid || !class_elite) return 0;
    *steam_game = 0;
    *pid = 0;
    *class_elite = 0;

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

    jacob_focus_read_u32(d, win, "STEAM_GAME", steam_game);
    jacob_focus_read_u32(d, win, "_NET_WM_PID", pid);

    XClassHint hint;
    memset(&hint, 0, sizeof(hint));
    if (XGetClassHint(d, win, &hint)) {
        const char *needle = "steam_app_359320";
        if ((hint.res_name && strstr(hint.res_name, needle)) ||
            (hint.res_class && strstr(hint.res_class, needle))) {
            *class_elite = 1;
        }
        if (hint.res_name) XFree(hint.res_name);
        if (hint.res_class) XFree(hint.res_class);
    }

    XCloseDisplay(d);
    return 1;
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"
)

type linuxFocusEvidence struct {
	display    string
	focusedApp uint64
	focusedGFX uint64
	window     uintptr
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

	// Keep JACoB's inherited game display first, then inspect every local
	// Gamescope XWayland socket. Steam Deck Gaming Mode currently uses separate
	// control and game XWayland servers, commonly :0 and :1.
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

func inspectLinuxX11Window(display string, window uintptr) (exists bool, elite bool) {
	if window == 0 {
		return false, false
	}
	cd := C.CString(display)
	defer C.free(unsafe.Pointer(cd))
	var steamGame, pid C.ulong
	var classElite C.int
	if C.jacob_focus_inspect_window(cd, C.ulong(window), &steamGame, &pid, &classElite) == 0 {
		return false, false
	}
	if uint64(steamGame) == 359320 || classElite != 0 {
		return true, true
	}
	if pid != 0 && linuxPIDTreeIsElite(int(pid)) {
		return true, true
	}
	return true, false
}

func findEliteFocusedWindow(window uintptr, displays []string) (uintptr, bool) {
	if window == 0 {
		return 0, false
	}
	for _, display := range displays {
		if exists, elite := inspectLinuxX11Window(display, window); exists && elite {
			return window, true
		}
	}
	return 0, false
}

func linuxNativeFocusAvailable() bool {
	for _, display := range linuxX11Displays() {
		if _, ok := probeLinuxX11Display(display); ok {
			return true
		}
	}
	return false
}

// linuxNativeFocusProbe correlates Gamescope's control XWayland server with
// the game XWayland server. On Steam Deck, GAMESCOPE_FOCUSED_* is normally
// published on :0 while Elite's actual 1280x800 window lives on :1.
func linuxNativeFocusProbe() (resolved bool, elite bool, window uintptr) {
	displays := linuxX11Displays()
	if len(displays) == 0 {
		return false, false, 0
	}

	probes := make([]linuxFocusEvidence, 0, len(displays))
	for _, display := range displays {
		if probe, ok := probeLinuxX11Display(display); ok {
			probes = append(probes, probe)
		}
	}
	if len(probes) == 0 {
		return false, false, 0
	}

	// GAMESCOPE_FOCUSED_APP_GFX is the best signal for the game currently being
	// presented. Steam's own UI can temporarily occupy GAMESCOPE_FOCUSED_APP
	// while the GFX app remains Elite, so never let that UI id veto GFX=359320.
	for _, probe := range probes {
		if probe.focusedGFX == 359320 {
			if win, ok := findEliteFocusedWindow(probe.window, displays); ok {
				return true, true, win
			}
			// Gamescope itself has positively identified Elite as the presented GFX
			// app. Keep the XID so capture can resolve it on the inherited game display.
			return true, true, probe.window
		}
	}

	// If Gamescope positively identifies some other non-zero GFX app, that is
	// strong negative evidence and input must remain blocked.
	for _, probe := range probes {
		if probe.focusedGFX != 0 && probe.focusedGFX != 359320 {
			return true, false, probe.window
		}
	}

	// Older Gamescope builds may expose only GAMESCOPE_FOCUSED_APP.
	for _, probe := range probes {
		if probe.focusedApp == 359320 {
			if win, ok := findEliteFocusedWindow(probe.window, displays); ok {
				return true, true, win
			}
			return true, true, probe.window
		}
	}

	// Finally, verify any focused-window XID across all XWayland servers. A
	// Steam/Game Mode UI app id alone is not treated as a negative result.
	for _, probe := range probes {
		if win, ok := findEliteFocusedWindow(probe.window, displays); ok {
			return true, true, win
		}
	}

	return false, false, 0
}

func linuxNativeEliteFocus() (bool, bool) {
	resolved, elite, _ := linuxNativeFocusProbe()
	return resolved, elite
}

func linuxNativeEliteWindow() (uintptr, bool) {
	resolved, elite, window := linuxNativeFocusProbe()
	return window, resolved && elite && window != 0
}
