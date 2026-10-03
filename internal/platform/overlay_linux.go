//go:build linux && cgo

package platform

/*
#cgo LDFLAGS: -lX11 -l:libXfixes.so.3
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/Xatom.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>

typedef XID XserverRegion;
extern XserverRegion XFixesCreateRegion(Display *dpy, XRectangle *rectangles, int nrectangles);
extern void XFixesSetWindowShapeRegion(Display *dpy, Window win, int shape_kind, int x_off, int y_off, XserverRegion region);
extern void XFixesDestroyRegion(Display *dpy, XserverRegion region);
#ifndef ShapeInput
#define ShapeInput 2
#endif

typedef struct {
    Display *dpy;
    int screen;
    Window win;
    Visual *visual;
    Colormap cmap;
    GC gc;
    int width;
    int height;
    int mapped;
} JacobOverlay;

static JacobOverlay* jacob_overlay_create() {
    Display *d = XOpenDisplay(NULL);
    if (!d) return NULL;
    int screen = DefaultScreen(d);
    XVisualInfo tpl;
    memset(&tpl, 0, sizeof(tpl));
    tpl.screen = screen;
    tpl.depth = 32;
    tpl.class = TrueColor;
    int n = 0;
    XVisualInfo *vis = XGetVisualInfo(d, VisualScreenMask | VisualDepthMask | VisualClassMask, &tpl, &n);
    if (!vis || n <= 0) {
        if (vis) XFree(vis);
        XCloseDisplay(d);
        return NULL;
    }
    XVisualInfo vi = vis[0];
    XFree(vis);
    Window root = RootWindow(d, screen);
    Colormap cmap = XCreateColormap(d, root, vi.visual, AllocNone);
    XSetWindowAttributes attr;
    memset(&attr, 0, sizeof(attr));
    attr.colormap = cmap;
    attr.border_pixel = 0;
    attr.background_pixel = 0;
    attr.override_redirect = True;
    int w = DisplayWidth(d, screen);
    int h = DisplayHeight(d, screen);
    Window win = XCreateWindow(d, root, 0, 0, w, h, 0, 32, InputOutput, vi.visual,
        CWColormap | CWBorderPixel | CWBackPixel | CWOverrideRedirect, &attr);
    if (!win) {
        XFreeColormap(d, cmap);
        XCloseDisplay(d);
        return NULL;
    }
    XStoreName(d, win, "JACoB HUD");

    Atom ext = XInternAtom(d, "GAMESCOPE_EXTERNAL_OVERLAY", False);
    unsigned long one = 1;
    XChangeProperty(d, win, ext, XA_CARDINAL, 32, PropModeReplace, (unsigned char*)&one, 1);
    Atom opacity = XInternAtom(d, "_NET_WM_WINDOW_OPACITY", False);
    unsigned long opaque = 0xffffffffUL;
    XChangeProperty(d, win, opacity, XA_CARDINAL, 32, PropModeReplace, (unsigned char*)&opaque, 1);

    XserverRegion empty = XFixesCreateRegion(d, NULL, 0);
    XFixesSetWindowShapeRegion(d, win, ShapeInput, 0, 0, empty);
    XFixesDestroyRegion(d, empty);

    GC gc = XCreateGC(d, win, 0, NULL);
    JacobOverlay *o = (JacobOverlay*)calloc(1, sizeof(JacobOverlay));
    o->dpy=d; o->screen=screen; o->win=win; o->visual=vi.visual; o->cmap=cmap; o->gc=gc; o->width=w; o->height=h;
    XFlush(d);
    return o;
}
static void jacob_overlay_destroy(JacobOverlay *o) {
    if (!o) return;
    if (o->gc) XFreeGC(o->dpy,o->gc);
    if (o->win) XDestroyWindow(o->dpy,o->win);
    if (o->cmap) XFreeColormap(o->dpy,o->cmap);
    if (o->dpy) XCloseDisplay(o->dpy);
    free(o);
}
static int jacob_overlay_width(JacobOverlay *o){ return o?o->width:0; }
static int jacob_overlay_height(JacobOverlay *o){ return o?o->height:0; }
static void jacob_overlay_refresh_size(JacobOverlay *o){ if(!o)return; int w=DisplayWidth(o->dpy,o->screen),h=DisplayHeight(o->dpy,o->screen); if(w!=o->width||h!=o->height){o->width=w;o->height=h;XMoveResizeWindow(o->dpy,o->win,0,0,w,h);} }
static void jacob_overlay_show(JacobOverlay *o, int show) {
    if(!o) return;
    if(show && !o->mapped){ XMapRaised(o->dpy,o->win); o->mapped=1; }
    if(!show && o->mapped){ XUnmapWindow(o->dpy,o->win); o->mapped=0; }
    XFlush(o->dpy);
}
static void jacob_overlay_begin(JacobOverlay *o) {
    if(!o) return;
    XSetForeground(o->dpy,o->gc,0x00000000UL);
    XFillRectangle(o->dpy,o->win,o->gc,0,0,o->width,o->height);
}
static void jacob_overlay_end(JacobOverlay *o) { if(o){ XFlush(o->dpy); } }
static unsigned long jacob_argb(unsigned char r,unsigned char g,unsigned char b,unsigned char a){ return ((unsigned long)a<<24)|((unsigned long)r<<16)|((unsigned long)g<<8)|b; }
static void jacob_set_color(JacobOverlay *o, unsigned char r,unsigned char g,unsigned char b,unsigned char a, int width) {
    XSetForeground(o->dpy,o->gc,jacob_argb(r,g,b,a));
    XSetLineAttributes(o->dpy,o->gc,width<1?1:width,LineSolid,CapRound,JoinRound);
}
static void jacob_line(JacobOverlay *o,int x1,int y1,int x2,int y2,unsigned char r,unsigned char g,unsigned char b,unsigned char a,int lw){ if(!o||a==0)return; jacob_set_color(o,r,g,b,a,lw); XDrawLine(o->dpy,o->win,o->gc,x1,y1,x2,y2); }
static void jacob_poly(JacobOverlay *o, XPoint *pts, int n, int closed, unsigned char sr,unsigned char sg,unsigned char sb,unsigned char sa, unsigned char fr,unsigned char fg,unsigned char fb,unsigned char fa,int lw){
    if(!o||!pts||n<2)return;
    if(fa>0 && closed){ jacob_set_color(o,fr,fg,fb,fa,lw); XFillPolygon(o->dpy,o->win,o->gc,pts,n,Complex,CoordModeOrigin); }
    if(sa>0){ jacob_set_color(o,sr,sg,sb,sa,lw); XDrawLines(o->dpy,o->win,o->gc,pts,n,CoordModeOrigin); if(closed) XDrawLine(o->dpy,o->win,o->gc,pts[n-1].x,pts[n-1].y,pts[0].x,pts[0].y); }
}
static void jacob_rect(JacobOverlay *o,int x,int y,int w,int h,unsigned char sr,unsigned char sg,unsigned char sb,unsigned char sa,unsigned char fr,unsigned char fg,unsigned char fb,unsigned char fa,int lw){ if(!o)return; if(fa>0){jacob_set_color(o,fr,fg,fb,fa,lw);XFillRectangle(o->dpy,o->win,o->gc,x,y,w,h);} if(sa>0){jacob_set_color(o,sr,sg,sb,sa,lw);XDrawRectangle(o->dpy,o->win,o->gc,x,y,w,h);} }
static void jacob_circle(JacobOverlay *o,int cx,int cy,int rad,unsigned char sr,unsigned char sg,unsigned char sb,unsigned char sa,unsigned char fr,unsigned char fg,unsigned char fb,unsigned char fa,int lw){ if(!o)return; int d=rad*2; if(fa>0){jacob_set_color(o,fr,fg,fb,fa,lw);XFillArc(o->dpy,o->win,o->gc,cx-rad,cy-rad,d,d,0,360*64);} if(sa>0){jacob_set_color(o,sr,sg,sb,sa,lw);XDrawArc(o->dpy,o->win,o->gc,cx-rad,cy-rad,d,d,0,360*64);} }
static const char* jacob_font_name(int px){ if(px>=19)return "10x20"; if(px>=14)return "9x15"; return "6x13"; }
static void jacob_text(JacobOverlay *o,int x,int y,const char* text,int px,int align,unsigned char r,unsigned char g,unsigned char b,unsigned char a){
    if(!o||!text||a==0)return; XFontStruct *font=XLoadQueryFont(o->dpy,jacob_font_name(px)); if(!font)font=XLoadQueryFont(o->dpy,"fixed"); if(!font)return;
    XSetFont(o->dpy,o->gc,font->fid); jacob_set_color(o,r,g,b,a,1); int len=strlen(text); int tw=XTextWidth(font,text,len); int tx=x; if(align==1)tx-=tw/2; else if(align==2)tx-=tw; XDrawString(o->dpy,o->win,o->gc,tx,y+font->ascent,text,len); XFreeFont(o->dpy,font);
}
*/
import "C"

import (
	"fmt"
	"math"
	"os"
	"sync"
	"unsafe"
)

type linuxOverlay struct {
	state  *overlayState
	mu     sync.Mutex
	native *C.JacobOverlay
	err    error
}

func newOverlayDriver() OverlayDriver {
	o := &linuxOverlay{state: newOverlayState()}
	if os.Getenv("DISPLAY") == "" {
		o.err = fmt.Errorf("DISPLAY is not set; Gamescope/XWayland overlay is unavailable")
		return o
	}
	o.native = C.jacob_overlay_create()
	if o.native == nil {
		o.err = fmt.Errorf("could not create a 32-bit X11 overlay window")
	}
	return o
}
func (o *linuxOverlay) Name() string    { return "linux-x11-gamescope-external-overlay" }
func (o *linuxOverlay) Available() bool { return o.native != nil && o.err == nil }
func (o *linuxOverlay) SetLayer(layer string, scene OverlayScene) error {
	if layer == "" {
		return fmt.Errorf("overlay layer is required")
	}
	if err := ValidateOverlayScene(&scene); err != nil {
		return err
	}
	if !o.Available() {
		return o.err
	}
	o.state.set(layer, scene)
	o.render()
	return nil
}
func (o *linuxOverlay) ClearLayer(layer string) error { o.state.clear(layer); o.render(); return nil }
func (o *linuxOverlay) ClearAll() error               { o.state.clearAll(); o.render(); return nil }
func (o *linuxOverlay) SetVisible(v bool) error       { o.state.setVisible(v); o.render(); return nil }
func (o *linuxOverlay) Info() OverlayInfo {
	_, v := o.state.snapshot()
	w, h := 0, 0
	if o.native != nil {
		w = int(C.jacob_overlay_width(o.native))
		h = int(C.jacob_overlay_height(o.native))
	}
	note := "Designed for Steam Deck Gaming Mode/Gamescope via GAMESCOPE_EXTERNAL_OVERLAY; also works on many composited X11 desktops."
	if o.err != nil {
		note = o.err.Error()
	}
	return OverlayInfo{Available: o.Available(), Driver: o.Name(), Visible: v, Layers: o.state.count(), Width: w, Height: h, SupportsAlpha: true, SupportsText: true, SupportsShapes: []string{"text", "line", "polyline", "polygon", "rect", "circle"}, CoordinateSpace: []string{"normalized", "pixels"}, Note: note}
}

func (o *linuxOverlay) render() {
	if !o.Available() {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	layers, visible := o.state.snapshot()
	show := visible && len(layers) > 0
	C.jacob_overlay_show(o.native, boolInt(show))
	if !show {
		return
	}
	C.jacob_overlay_refresh_size(o.native)
	w := int(C.jacob_overlay_width(o.native))
	h := int(C.jacob_overlay_height(o.native))
	C.jacob_overlay_begin(o.native)
	for _, scene := range o.state.orderedScenes() {
		for _, it := range scene.Items {
			drawLinuxItem(o.native, w, h, scene, it)
		}
	}
	C.jacob_overlay_end(o.native)
}
func boolInt(v bool) C.int {
	if v {
		return 1
	}
	return 0
}
func drawLinuxItem(n *C.JacobOverlay, w, h int, scene OverlayScene, it OverlayItem) {
	x := overlayCoord(it.X, w, scene.Space)
	y := overlayCoord(it.Y, h, scene.Space)
	x2 := overlayCoord(it.X2, w, scene.Space)
	y2 := overlayCoord(it.Y2, h, scene.Space)
	iw := overlayCoord(it.W, w, scene.Space)
	ih := overlayCoord(it.H, h, scene.Space)
	stroke := it.Stroke
	if stroke == "" {
		stroke = it.Color
	}
	if stroke == "" {
		stroke = "#ffffff"
	}
	sr, sg, sb, sa, _ := ParseOverlayColor(stroke)
	fr, fg, fb, fa, _ := ParseOverlayColor(it.Fill)
	lw := C.int(math.Max(1, it.LineWidth))
	switch it.Type {
	case "line":
		C.jacob_line(n, C.int(x), C.int(y), C.int(x2), C.int(y2), C.uchar(sr), C.uchar(sg), C.uchar(sb), C.uchar(sa), lw)
	case "polyline", "polygon":
		if len(it.Points) >= 2 {
			pts := make([]C.XPoint, len(it.Points))
			for i, p := range it.Points {
				pts[i].x = C.short(overlayCoord(p.X, w, scene.Space))
				pts[i].y = C.short(overlayCoord(p.Y, h, scene.Space))
			}
			closed := 0
			if it.Type == "polygon" {
				closed = 1
			}
			C.jacob_poly(n, (*C.XPoint)(unsafe.Pointer(&pts[0])), C.int(len(pts)), C.int(closed), C.uchar(sr), C.uchar(sg), C.uchar(sb), C.uchar(sa), C.uchar(fr), C.uchar(fg), C.uchar(fb), C.uchar(fa), lw)
		}
	case "rect":
		C.jacob_rect(n, C.int(x), C.int(y), C.int(iw), C.int(ih), C.uchar(sr), C.uchar(sg), C.uchar(sb), C.uchar(sa), C.uchar(fr), C.uchar(fg), C.uchar(fb), C.uchar(fa), lw)
	case "circle":
		r := overlayRadius(it.R, w, h, scene.Space)
		C.jacob_circle(n, C.int(x), C.int(y), C.int(r), C.uchar(sr), C.uchar(sg), C.uchar(sb), C.uchar(sa), C.uchar(fr), C.uchar(fg), C.uchar(fb), C.uchar(fa), lw)
	case "text":
		col := it.Color
		if col == "" {
			col = "#ffffff"
		}
		r, g, b, a, _ := ParseOverlayColor(col)
		align := 0
		if it.Align == "center" {
			align = 1
		} else if it.Align == "right" {
			align = 2
		}
		for i, line := range splitOverlayLines(it.Text) {
			cs := C.CString(line)
			C.jacob_text(n, C.int(x), C.int(y+i*(int(it.FontSize)+3)), cs, C.int(it.FontSize), C.int(align), C.uchar(r), C.uchar(g), C.uchar(b), C.uchar(a))
			C.free(unsafe.Pointer(cs))
		}
	}
}
func splitOverlayLines(s string) []string {
	out := []string{}
	start := 0
	for i, c := range s {
		if c == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
