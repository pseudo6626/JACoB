//go:build !windows && !linux

package platform

type unavailableOverlay struct{ state *overlayState }

func newOverlayDriver() OverlayDriver                                         { return &unavailableOverlay{state: newOverlayState()} }
func (o *unavailableOverlay) Name() string                                    { return "overlay-unavailable" }
func (o *unavailableOverlay) Available() bool                                 { return false }
func (o *unavailableOverlay) SetLayer(layer string, scene OverlayScene) error { return nil }
func (o *unavailableOverlay) ClearLayer(layer string) error                   { return nil }
func (o *unavailableOverlay) ClearPrefix(prefix string) error                 { return nil }
func (o *unavailableOverlay) ClearAll() error                                 { return nil }
func (o *unavailableOverlay) SetVisible(v bool) error                         { return nil }
func (o *unavailableOverlay) Info() OverlayInfo {
	return OverlayInfo{Available: false, Driver: o.Name(), Visible: false, SupportsShapes: []string{"text", "line", "polyline", "polygon", "rect", "circle"}, CoordinateSpace: []string{"normalized", "pixels"}}
}
