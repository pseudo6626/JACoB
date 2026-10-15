//go:build linux && !nocapture && cgo

package platform

/*
#cgo LDFLAGS: -lX11 -lXcomposite
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/extensions/Xcomposite.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>

static int jacob_capture_x_error = 0;
static int jacob_capture_x_error_handler(Display *d, XErrorEvent *e) {
    (void)d; (void)e;
    jacob_capture_x_error = 1;
    return 0;
}

static unsigned char jacob_component(unsigned long pixel, unsigned long mask) {
    if (mask == 0) return 0;
    int shift = 0;
    while (((mask >> shift) & 1UL) == 0UL && shift < (int)(sizeof(unsigned long) * 8 - 1)) shift++;
    unsigned long value = (pixel & mask) >> shift;
    unsigned long maxv = mask >> shift;
    if (maxv == 0) return 0;
    return (unsigned char)((value * 255UL + maxv / 2UL) / maxv);
}

static int jacob_x11_display_available(const char *display_name) {
    Display *d = XOpenDisplay(display_name && *display_name ? display_name : NULL);
    if (!d) return 0;
    XCloseDisplay(d);
    return 1;
}

static XImage* jacob_get_image_guarded(Display *d, Drawable draw, int w, int h) {
    jacob_capture_x_error = 0;
    int (*old_handler)(Display*, XErrorEvent*) = XSetErrorHandler(jacob_capture_x_error_handler);
    XImage *img = XGetImage(d, draw, 0, 0, (unsigned int)w, (unsigned int)h, AllPlanes, ZPixmap);
    XSync(d, False);
    XSetErrorHandler(old_handler);
    if (jacob_capture_x_error) {
        if (img) XDestroyImage(img);
        return NULL;
    }
    return img;
}

static int jacob_capture_window(const char *display_name, unsigned long window_id,
    unsigned char **out_rgba, int *out_w, int *out_h, int *used_composite) {
    if (!out_rgba || !out_w || !out_h || !used_composite || window_id == 0) return 0;
    *out_rgba = NULL;
    *out_w = 0;
    *out_h = 0;
    *used_composite = 0;

    Display *d = XOpenDisplay(display_name && *display_name ? display_name : NULL);
    if (!d) return 0;

    Window win = (Window)window_id;
    XWindowAttributes attr;
    jacob_capture_x_error = 0;
    int (*old_handler)(Display*, XErrorEvent*) = XSetErrorHandler(jacob_capture_x_error_handler);
    int got_attr = XGetWindowAttributes(d, win, &attr);
    XSync(d, False);
    XSetErrorHandler(old_handler);
    if (!got_attr || jacob_capture_x_error || attr.width <= 0 || attr.height <= 0 || attr.map_state != IsViewable) {
        XCloseDisplay(d);
        return 0;
    }

    int w = attr.width;
    int h = attr.height;
    XImage *img = NULL;
    Pixmap named = None;

    int event_base = 0, error_base = 0;
    if (XCompositeQueryExtension(d, &event_base, &error_base)) {
        jacob_capture_x_error = 0;
        old_handler = XSetErrorHandler(jacob_capture_x_error_handler);
        named = XCompositeNameWindowPixmap(d, win);
        XSync(d, False);
        XSetErrorHandler(old_handler);
        if (jacob_capture_x_error) named = None;
        if (named != None) {
            img = jacob_get_image_guarded(d, named, w, h);
            if (img) *used_composite = 1;
        }
    }

    if (!img) {
        img = jacob_get_image_guarded(d, win, w, h);
        *used_composite = 0;
    }
    if (named != None) XFreePixmap(d, named);
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
        const unsigned char *row = (const unsigned char*)img->data + (size_t)y * (size_t)img->bytes_per_line;
        for (int x = 0; x < w; x++) {
            unsigned long p = 0;
            if (img->bits_per_pixel == 32) {
                uint32_t v = 0;
                memcpy(&v, row + (size_t)x * 4u, 4u);
                p = (unsigned long)v;
            } else if (img->bits_per_pixel == 24) {
                const unsigned char *q = row + (size_t)x * 3u;
                if (img->byte_order == LSBFirst) p = (unsigned long)q[0] | ((unsigned long)q[1] << 8) | ((unsigned long)q[2] << 16);
                else p = ((unsigned long)q[0] << 16) | ((unsigned long)q[1] << 8) | (unsigned long)q[2];
            } else {
                p = XGetPixel(img, x, y);
            }
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
	"strings"
	"sync"
	"time"
	"unsafe"
)

type linuxCapture struct {
	mu            sync.Mutex
	cached        GameFrame
	captures      uint64
	cacheHits     uint64
	failures      uint64
	lastLatency   time.Duration
	targetDisplay string
	targetWindow  uintptr
}

func newCaptureDriver() CaptureDriver { return &linuxCapture{} }
func (l *linuxCapture) Name() string  { return "linux-x11-xcomposite-elite-window" }

func (l *linuxCapture) Available() bool {
	for _, display := range linuxX11Displays() {
		cd := C.CString(display)
		ok := C.jacob_x11_display_available(cd) != 0
		C.free(unsafe.Pointer(cd))
		if ok {
			return true
		}
	}
	return false
}

func (l *linuxCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	if !l.Available() {
		return GameFrame{}, fmt.Errorf("X11/XWayland display is unavailable")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if intent == CaptureIntentLive && !l.cached.CapturedAt.IsZero() && time.Since(l.cached.CapturedAt) <= 100*time.Millisecond {
		if frame, ok := linuxCachedFrameForWidth(l.cached, maxWidth); ok {
			l.cacheHits++
			return frame, nil
		}
	}
	started := time.Now()

	display, windowID, ok := linuxNativeEliteWindowTarget()
	if !ok || strings.TrimSpace(display) == "" || windowID == 0 {
		l.failures++
		return GameFrame{}, fmt.Errorf("foreground Gamescope window could not be resolved to the real Elite Dangerous client")
	}
	l.targetDisplay = display
	l.targetWindow = windowID

	cd := C.CString(display)
	defer C.free(unsafe.Pointer(cd))
	var raw *C.uchar
	var cw, ch, usedComposite C.int
	if C.jacob_capture_window(cd, C.ulong(windowID), &raw, &cw, &ch, &usedComposite) == 0 || raw == nil {
		l.failures++
		return GameFrame{}, fmt.Errorf("verified Elite window could not be captured through X11/XComposite")
	}
	defer C.jacob_capture_free(unsafe.Pointer(raw))

	w := int(cw)
	h := int(ch)
	if w <= 0 || h <= 0 {
		l.failures++
		return GameFrame{}, fmt.Errorf("verified Elite window returned invalid dimensions %dx%d", w, h)
	}

	pix := C.GoBytes(unsafe.Pointer(raw), C.int(w*h*4))
	sourceW, sourceH := w, h
	driver := l.Name()
	if usedComposite == 0 {
		driver = "linux-x11-direct-elite-window"
	}
	if linuxFrameLooksBlank(pix, w, h) {
		frame := blankGameFrame(driver, sourceW, sourceH,
			"verified Elite window returned an effectively black frame; capture was blanked instead of exposing false Vision results")
		l.captures++
		l.lastLatency = time.Since(started)
		l.cached = frame
		return frame, nil
	}

	full := GameFrame{
		Pix:          pix,
		Width:        w,
		Height:       h,
		Stride:       w * 4,
		Format:       PixelFormatRGBA8,
		CapturedAt:   time.Now().UTC(),
		Driver:       driver,
		SourceWidth:  sourceW,
		SourceHeight: sourceH,
		Verified:     true,
		Foreground:   true,
		Blanked:      false,
	}
	l.captures++
	l.lastLatency = time.Since(started)
	l.cached = full
	frame, _ := linuxCachedFrameForWidth(full, maxWidth)
	return frame, nil
}

func linuxCachedFrameForWidth(frame GameFrame, maxWidth int) (GameFrame, bool) {
	if frame.CapturedAt.IsZero() || frame.Width <= 0 || frame.Height <= 0 {
		return GameFrame{}, false
	}
	if frame.Blanked || maxWidth <= 0 || frame.Width <= maxWidth {
		return frame, true
	}
	pix, w, h := scaleRGBANearest(frame.Pix, frame.Width, frame.Height, maxWidth)
	out := frame
	out.Pix = pix
	out.Width = w
	out.Height = h
	out.Stride = w * 4
	return out, true
}

func (l *linuxCapture) CaptureStats() map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	age := int64(-1)
	if !l.cached.CapturedAt.IsZero() {
		age = time.Since(l.cached.CapturedAt).Milliseconds()
	}
	return map[string]any{
		"captures": l.captures, "cacheHits": l.cacheHits, "failures": l.failures,
		"lastLatencyMs": l.lastLatency.Milliseconds(), "cachedFrameAgeMs": age,
		"cacheWindowMs": 100, "targetDisplay": l.targetDisplay,
		"targetWindow": fmt.Sprintf("0x%x", l.targetWindow),
	}
}

func (l *linuxCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	frame, err := l.CaptureFrame(maxWidth, CaptureIntentLive)
	if err != nil {
		return nil, CaptureInfo{Driver: l.Name(), Blanked: true, Reason: err.Error()}, err
	}
	return EncodeGameFrameJPEG(frame, quality)
}

func linuxFrameLooksBlank(pix []byte, w, h int) bool {
	if w <= 0 || h <= 0 || len(pix) < w*h*4 {
		return true
	}
	const gridX, gridY = 64, 36
	maxChannel := byte(0)
	var sum uint64
	samples := 0
	for gy := 0; gy < gridY; gy++ {
		y := gy * (h - 1) / (gridY - 1)
		for gx := 0; gx < gridX; gx++ {
			x := gx * (w - 1) / (gridX - 1)
			i := (y*w + x) * 4
			r, g, b := pix[i], pix[i+1], pix[i+2]
			if r > maxChannel {
				maxChannel = r
			}
			if g > maxChannel {
				maxChannel = g
			}
			if b > maxChannel {
				maxChannel = b
			}
			sum += uint64(r) + uint64(g) + uint64(b)
			samples += 3
		}
	}
	if samples == 0 {
		return true
	}
	mean := float64(sum) / float64(samples)
	return maxChannel <= 8 && mean < 1.5
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
