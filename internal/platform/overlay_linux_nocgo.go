//go:build linux && !cgo

package platform

import "fmt"

type linuxNoCGOOverlay struct{ state *overlayState }

func newOverlayDriver() OverlayDriver        { return &linuxNoCGOOverlay{state: newOverlayState()} }
func (o *linuxNoCGOOverlay) Name() string    { return "linux-overlay-requires-cgo-x11" }
func (o *linuxNoCGOOverlay) Available() bool { return false }
func (o *linuxNoCGOOverlay) SetLayer(layer string, scene OverlayScene) error {
	return fmt.Errorf("Steam Deck overlay requires the CGO/X11 build")
}
func (o *linuxNoCGOOverlay) ClearLayer(layer string) error   { return nil }
func (o *linuxNoCGOOverlay) ClearPrefix(prefix string) error { return nil }
func (o *linuxNoCGOOverlay) ClearAll() error                 { return nil }
func (o *linuxNoCGOOverlay) SetVisible(v bool) error         { o.state.setVisible(v); return nil }
func (o *linuxNoCGOOverlay) Info() OverlayInfo {
	return OverlayInfo{Available: false, Driver: o.Name(), SupportsText: true, SupportsShapes: []string{"text", "line", "polyline", "polygon", "rect", "circle"}, CoordinateSpace: []string{"normalized", "pixels"}, Note: "Rebuild with CGO_ENABLED=1 and X11/XFixes development libraries."}
}
