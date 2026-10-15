//go:build linux && !nocapture && cgo

package platform

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <stdlib.h>
#include <stdint.h>

static unsigned char jacob_component(unsigned long pixel, unsigned long mask) {
    if (mask == 0) return 0;
    int shift = 0;
    while (((mask >> shift) & 1UL) == 0UL && shift < (int)(sizeof(unsigned long) * 8 - 1)) shift++;
    unsigned long value = (pixel & mask) >> shift;
    unsigned long maxv = mask >> shift;
    if (maxv == 0) return 0;
    return (unsigned char)((value * 255UL + maxv / 2UL) / maxv);
}

static int jacob_get_xid(Display *d, Window w, const char *name, unsigned long *out) {
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

static int jacob_x11_available(void) {
    Display *d = XOpenDisplay(NULL);
    if (!d) return 0;
    XCloseDisplay(d);
    return 1;
}

// Resolves only the focused Gamescope/X11 window. app_id is populated when
// Gamescope exposes GAMESCOPE_FOCUSED_APP; pid is populated when the X11
// client publishes _NET_WM_PID. The Go side performs the final Elite check.
static int jacob_resolve_focused_window(unsigned long *out_window, unsigned long *out_pid, unsigned long *out_app_id) {
    if (!out_window || !out_pid || !out_app_id) return 0;
    *out_window = 0;
    *out_pid = 0;
    *out_app_id = 0;

    Display *d = XOpenDisplay(NULL);
    if (!d) return 0;
    Window root = DefaultRootWindow(d);

    unsigned long app_id = 0;
    unsigned long window_id = 0;
    jacob_get_xid(d, root, "GAMESCOPE_FOCUSED_APP", &app_id);
    if (app_id != 0) {
        jacob_get_xid(d, root, "GAMESCOPE_FOCUSED_WINDOW", &window_id);
    }
    if (window_id == 0) {
        jacob_get_xid(d, root, "_NET_ACTIVE_WINDOW", &window_id);
    }
    if (window_id == 0) {
        XCloseDisplay(d);
        return 0;
    }

    unsigned long pid = 0;
    jacob_get_xid(d, (Window)window_id, "_NET_WM_PID", &pid);
    XCloseDisplay(d);

    *out_window = window_id;
    *out_pid = pid;
    *out_app_id = app_id;
    return 1;
}

static int jacob_capture_window(unsigned long window_id, unsigned char **out_rgba, int *out_w, int *out_h) {
    if (!out_rgba || !out_w || !out_h || window_id == 0) return 0;
    *out_rgba = NULL;
    *out_w = 0;
    *out_h = 0;

    Display *d = XOpenDisplay(NULL);
    if (!d) return 0;

    Window win = (Window)window_id;
    XWindowAttributes attr;
    if (!XGetWindowAttributes(d, win, &attr) || attr.width <= 0 || attr.height <= 0 || attr.map_state != IsViewable) {
        XCloseDisplay(d);
        return 0;
    }

    int w = attr.width;
    int h = attr.height;
    XImage *img = XGetImage(d, win, 0, 0, (unsigned int)w, (unsigned int)h, AllPlanes, ZPixmap);
    if (!img) {
        XCloseDisplay(d);
        return 0;
    }

    size_t bytes = (size_t)w * (size_t)h * 4u;
    unsigned char *rgba = (unsigned char*)malloc(bytes);
    if (!rgba) {
        XDestroyImage(img);
        XCloseDisplay(d);
        return 0;
    }

    for (int y = 0; y < h; y++) {
        for (int x = 0; x < w; x++) {
            unsigned long p = XGetPixel(img, x, y);
            size_t i = ((size_t)y * (size_t)w + (size_t)x) * 4u;
            rgba[i + 0] = jacob_component(p, img->red_mask);
            rgba[i + 1] = jacob_component(p, img->green_mask);
            rgba[i + 2] = jacob_component(p, img->blue_mask);
            rgba[i + 3] = 255;
        }
    }

    XDestroyImage(img);
    XCloseDisplay(d);

    *out_rgba = rgba;
    *out_w = w;
    *out_h = h;
    return 1;
}

static void jacob_capture_free(void *p) { free(p); }
*/
import "C"

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"
)

type linuxCapture struct{}

func newCaptureDriver() CaptureDriver { return &linuxCapture{} }
func (l *linuxCapture) Name() string  { return "linux-x11-gamescope-elite-window" }
func (l *linuxCapture) Available() bool {
	return strings.TrimSpace(os.Getenv("DISPLAY")) != "" && C.jacob_x11_available() != 0
}

func (l *linuxCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	_ = intent
	if !l.Available() {
		return GameFrame{}, fmt.Errorf("X11/XWayland display is unavailable")
	}

	windowID, err := verifiedLinuxEliteWindowX11()
	if err != nil {
		return GameFrame{}, err
	}

	var raw *C.uchar
	var cw, ch C.int
	if C.jacob_capture_window(C.ulong(windowID), &raw, &cw, &ch) == 0 || raw == nil {
		return GameFrame{}, fmt.Errorf("verified Elite window could not be captured through X11/XWayland")
	}
	defer C.jacob_capture_free(unsafe.Pointer(raw))

	w := int(cw)
	h := int(ch)
	if w <= 0 || h <= 0 {
		return GameFrame{}, fmt.Errorf("verified Elite window returned invalid dimensions %dx%d", w, h)
	}

	pix := C.GoBytes(unsafe.Pointer(raw), C.int(w*h*4))
	sourceW, sourceH := w, h
	if maxWidth > 0 && w > maxWidth {
		pix, w, h = scaleRGBANearest(pix, w, h, maxWidth)
	}

	return GameFrame{
		Pix:          pix,
		Width:        w,
		Height:       h,
		Stride:       w * 4,
		Format:       PixelFormatRGBA8,
		CapturedAt:   time.Now().UTC(),
		Driver:       l.Name(),
		SourceWidth:  sourceW,
		SourceHeight: sourceH,
		Verified:     true,
		Foreground:   true,
		Blanked:      false,
	}, nil
}

func (l *linuxCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	frame, err := l.CaptureFrame(maxWidth, CaptureIntentLive)
	if err != nil {
		return nil, CaptureInfo{Driver: l.Name(), Blanked: true, Reason: err.Error()}, err
	}
	return EncodeGameFrameJPEG(frame, quality)
}

func verifiedLinuxEliteWindowX11() (uintptr, error) {
	if win, ok := linuxNativeEliteWindow(); ok {
		return win, nil
	}

	var win, pid, appID C.ulong
	if C.jacob_resolve_focused_window(&win, &pid, &appID) == 0 || win == 0 {
		return 0, fmt.Errorf("focused X11/Gamescope window is unavailable")
	}

	if uint64(appID) == 359320 {
		return uintptr(win), nil
	}
	if pid > 0 && linuxPIDTreeIsElite(int(pid)) {
		return uintptr(win), nil
	}
	return 0, fmt.Errorf("focused X11/Gamescope window was not verified as Elite Dangerous")
}

func scaleRGBANearest(src []byte, sw, sh, maxWidth int) ([]byte, int, int) {
	if maxWidth <= 0 || sw <= maxWidth || sw <= 0 || sh <= 0 {
		return src, sw, sh
	}
	dw := maxWidth
	dh := sh * dw / sw
	if dh < 1 {
		dh = 1
	}
	dst := make([]byte, dw*dh*4)
	for y := 0; y < dh; y++ {
		sy := y * sh / dh
		for x := 0; x < dw; x++ {
			sx := x * sw / dw
			si := (sy*sw + sx) * 4
			di := (y*dw + x) * 4
			copy(dst[di:di+4], src[si:si+4])
		}
	}
	return dst, dw, dh
}
