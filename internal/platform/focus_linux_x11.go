//go:build linux && cgo

package platform

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <stdlib.h>

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

static int jacob_focus_x11_available(void) {
    Display *d = XOpenDisplay(NULL);
    if (!d) return 0;
    XCloseDisplay(d);
    return 1;
}

static int jacob_focus_probe(unsigned long *app_id, unsigned long *steam_game, unsigned long *pid, unsigned long *window_id) {
    if (!app_id || !steam_game || !pid || !window_id) return 0;
    *app_id = 0;
    *steam_game = 0;
    *pid = 0;
    *window_id = 0;

    Display *d = XOpenDisplay(NULL);
    if (!d) return 0;
    Window root = DefaultRootWindow(d);
    Window candidate = 0;
    unsigned long value = 0;

    if (jacob_focus_read_u32(d, root, "GAMESCOPE_FOCUSED_APP", &value)) {
        *app_id = value;
    }

    value = 0;
    if (jacob_focus_read_u32(d, root, "GAMESCOPE_FOCUSED_WINDOW", &value)) {
        candidate = (Window)value;
    }
    if (!candidate) {
        value = 0;
        if (jacob_focus_read_u32(d, root, "_NET_ACTIVE_WINDOW", &value)) {
            candidate = (Window)value;
        }
    }
    if (!candidate) {
        Window focus = None;
        int revert = RevertToNone;
        XGetInputFocus(d, &focus, &revert);
        if (focus != None && focus != PointerRoot) candidate = focus;
    }

    // Wine/Proton can put focus on a child. Walk toward the root until we find
    // the Steam app id or process id published by the top-level client.
    Window cur = candidate;
    for (int depth = 0; cur && cur != root && depth < 12; depth++) {
        if (!*steam_game) {
            value = 0;
            if (jacob_focus_read_u32(d, cur, "STEAM_GAME", &value)) *steam_game = value;
        }
        if (!*pid) {
            value = 0;
            if (jacob_focus_read_u32(d, cur, "_NET_WM_PID", &value)) *pid = value;
        }
        if (*steam_game && *pid) break;

        Window rr = 0, parent = 0, *children = NULL;
        unsigned int nchildren = 0;
        if (!XQueryTree(d, cur, &rr, &parent, &children, &nchildren)) break;
        if (children) XFree(children);
        if (!parent || parent == cur) break;
        cur = parent;
    }

    *window_id = (unsigned long)candidate;
    XCloseDisplay(d);
    return (*app_id || *steam_game || *pid || *window_id) ? 1 : 0;
}
*/
import "C"

func linuxNativeFocusAvailable() bool {
	return C.jacob_focus_x11_available() != 0
}

// linuxNativeEliteFocus returns (resolved, elite). resolved=true means the
// compositor/X11 focus could be identified strongly enough that callers must
// honor the result, including a negative result when another app has focus.
func linuxNativeFocusProbe() (resolved bool, elite bool, window uintptr) {
	var appID, steamGame, pid, win C.ulong
	if C.jacob_focus_probe(&appID, &steamGame, &pid, &win) == 0 {
		return false, false, 0
	}
	window = uintptr(win)
	if appID != 0 {
		return true, uint64(appID) == 359320, window
	}
	if steamGame != 0 {
		return true, uint64(steamGame) == 359320, window
	}
	if pid != 0 {
		return true, linuxPIDTreeIsElite(int(pid)), window
	}
	return false, false, window
}

func linuxNativeEliteFocus() (bool, bool) {
	resolved, elite, _ := linuxNativeFocusProbe()
	return resolved, elite
}

func linuxNativeEliteWindow() (uintptr, bool) {
	resolved, elite, window := linuxNativeFocusProbe()
	return window, resolved && elite && window != 0
}
