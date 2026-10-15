//go:build linux && cgo

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type deckOverlayBridge struct {
	state    *overlayState
	mu       sync.Mutex
	version  uint64
	changed  chan struct{}
	lastSeen time.Time
}

func newDeckOverlayBridge() *deckOverlayBridge {
	return &deckOverlayBridge{
		state:   newOverlayState(),
		changed: make(chan struct{}),
	}
}

func deckGamescopePresent() bool {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	st, err := os.Stat(filepath.Join(runtimeDir, "gamescope-0"))
	return err == nil && st.Mode()&os.ModeSocket != 0
}

func newOverlayDriver() OverlayDriver {
	if deckGamescopePresent() {
		return newDeckOverlayBridge()
	}
	return newLinuxNativeOverlayDriver()
}

func (o *deckOverlayBridge) Name() string    { return "linux-steam-cef-hud2-bridge" }
func (o *deckOverlayBridge) Available() bool { return true }

func (o *deckOverlayBridge) bump() {
	o.mu.Lock()
	o.version++
	close(o.changed)
	o.changed = make(chan struct{})
	o.mu.Unlock()
}

func (o *deckOverlayBridge) SetLayer(layer string, scene OverlayScene) error {
	if layer == "" {
		return fmt.Errorf("overlay layer is required")
	}
	if err := ValidateOverlayScene(&scene); err != nil {
		return err
	}
	o.state.set(layer, scene)
	o.bump()
	return nil
}

func (o *deckOverlayBridge) ClearLayer(layer string) error {
	o.state.clear(layer)
	o.bump()
	return nil
}

func (o *deckOverlayBridge) ClearPrefix(prefix string) error {
	o.state.clearPrefix(prefix)
	o.bump()
	return nil
}

func (o *deckOverlayBridge) ClearAll() error {
	o.state.clearAll()
	o.bump()
	return nil
}

func (o *deckOverlayBridge) SetVisible(v bool) error {
	o.state.setVisible(v)
	o.bump()
	return nil
}

func (o *deckOverlayBridge) DeckBridgeSeen() {
	o.mu.Lock()
	o.lastSeen = time.Now()
	o.mu.Unlock()
}

func (o *deckOverlayBridge) DeckBridgeSnapshot() DeckOverlaySnapshot {
	layers, visible := o.state.snapshot()
	o.mu.Lock()
	version := o.version
	lastSeen := o.lastSeen
	o.mu.Unlock()
	return DeckOverlaySnapshot{
		HUDVersion:        HUDVersion,
		Version:           version,
		Visible:           visible,
		Layers:            layers,
		RendererConnected: !lastSeen.IsZero() && time.Since(lastSeen) < 3*time.Second,
	}
}

func (o *deckOverlayBridge) DeckBridgeWait(since uint64, timeoutMs int) DeckOverlaySnapshot {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	o.mu.Lock()
	version := o.version
	ch := o.changed
	o.mu.Unlock()

	if since >= version && timeout > 0 {
		if timeout > 1500*time.Millisecond {
			timeout = 1500 * time.Millisecond
		}
		timer := time.NewTimer(timeout)
		select {
		case <-ch:
		case <-timer.C:
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	return o.DeckBridgeSnapshot()
}

func (o *deckOverlayBridge) Info() OverlayInfo {
	snap := o.DeckBridgeSnapshot()
	note := "Steam CEF HUD2 bridge ready; waiting for Decky renderer heartbeat."
	if snap.RendererConnected {
		note = "Steam CEF HUD2 renderer connected. Tabs use the same OverlayScene contract as Windows."
	}
	return HUD2Info(OverlayInfo{
		Available:           true,
		Driver:              o.Name(),
		Visible:             snap.Visible,
		Layers:              o.state.count(),
		Width:               1280,
		Height:              800,
		SupportsAlpha:       true,
		SupportsText:        true,
		SupportsShapes:      []string{"text", "line", "polyline", "polygon", "rect", "circle"},
		CoordinateSpace:     []string{"normalized", "pixels"},
		SurfaceCreated:      snap.RendererConnected,
		Mapped:              snap.RendererConnected && snap.Visible && o.state.count() > 0,
		CompositionVerified: snap.RendererConnected,
		Note:                note,
	})
}
