package control

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"jacob/internal/bindings"
	"jacob/internal/platform"
)

type heldChord struct {
	Action string
	Key    string
	Mods   []string
	Slot   string
}

type Bridge struct {
	bindings *bindings.Store
	input    platform.InputDriver
	heldMu   sync.Mutex
	held     map[string]heldChord
}

type Result struct {
	Name        string   `json:"name"`
	Action      string   `json:"action,omitempty"`
	BindingSlot string   `json:"bindingSlot,omitempty"`
	Key         string   `json:"key,omitempty"`
	Modifiers   []string `json:"modifiers,omitempty"`
	DurationMs  int64    `json:"durationMs"`
}

func New(b *bindings.Store, i platform.InputDriver) *Bridge {
	return &Bridge{bindings: b, input: i, held: map[string]heldChord{}}
}

func (b *Bridge) resolveAction(action string) (key string, mods []string, slot string, err error) {
	a, ok := b.bindings.Get(action)
	if !ok {
		return "", nil, "", fmt.Errorf("Elite binding %q was not found in the active binds file", action)
	}
	key, mods, ok = bindings.KeyboardChord(a.Secondary)
	slot = "secondary"
	if !ok {
		key, mods, ok = bindings.KeyboardChord(a.Primary)
		slot = "primary"
	}
	if !ok {
		return "", nil, "", fmt.Errorf("binding %q has no keyboard primary/secondary binding that JACoB can inject", action)
	}
	return key, mods, slot, nil
}

func (b *Bridge) PressBinding(action string) (Result, error) {
	started := time.Now()
	key, mods, slot, err := b.resolveAction(action)
	if err != nil {
		return Result{}, err
	}
	if err := b.input.FocusGame(); err != nil {
		return Result{}, fmt.Errorf("could not target Elite Dangerous: %w", err)
	}
	if err := b.input.TapChord(key, mods); err != nil {
		return Result{}, err
	}
	return Result{Name: "binding.press", Action: action, BindingSlot: slot, Key: key, Modifiers: mods, DurationMs: time.Since(started).Milliseconds()}, nil
}

func (b *Bridge) DownBinding(action string) (Result, error) {
	started := time.Now()
	key, mods, slot, err := b.resolveAction(action)
	if err != nil {
		return Result{}, err
	}
	if err := b.input.FocusGame(); err != nil {
		return Result{}, fmt.Errorf("could not target Elite Dangerous: %w", err)
	}

	b.heldMu.Lock()
	defer b.heldMu.Unlock()
	if existing, ok := b.held[action]; ok {
		return Result{Name: "binding.down", Action: action, BindingSlot: existing.Slot, Key: existing.Key, Modifiers: append([]string(nil), existing.Mods...), DurationMs: time.Since(started).Milliseconds()}, nil
	}
	if err := b.input.ChordDown(key, mods); err != nil {
		return Result{}, err
	}
	b.held[action] = heldChord{Action: action, Key: key, Mods: append([]string(nil), mods...), Slot: slot}
	return Result{Name: "binding.down", Action: action, BindingSlot: slot, Key: key, Modifiers: mods, DurationMs: time.Since(started).Milliseconds()}, nil
}

func (b *Bridge) UpBinding(action string) (Result, error) {
	started := time.Now()

	// A release for a chord JACoB itself pressed is always permitted. Re-checking
	// foreground here can strand a virtual key if Gamescope/Steam steals focus
	// between DOWN and UP.
	b.heldMu.Lock()
	if held, ok := b.held[action]; ok {
		if err := b.input.ChordUp(held.Key, held.Mods); err != nil {
			b.heldMu.Unlock()
			return Result{}, err
		}
		delete(b.held, action)
		b.heldMu.Unlock()
		return Result{Name: "binding.up", Action: action, BindingSlot: held.Slot, Key: held.Key, Modifiers: append([]string(nil), held.Mods...), DurationMs: time.Since(started).Milliseconds()}, nil
	}
	b.heldMu.Unlock()

	key, mods, slot, err := b.resolveAction(action)
	if err != nil {
		return Result{}, err
	}
	if err := b.input.FocusGame(); err != nil {
		return Result{}, fmt.Errorf("could not target Elite Dangerous: %w", err)
	}
	if err := b.input.ChordUp(key, mods); err != nil {
		return Result{}, err
	}
	return Result{Name: "binding.up", Action: action, BindingSlot: slot, Key: key, Modifiers: mods, DurationMs: time.Since(started).Milliseconds()}, nil
}

func (b *Bridge) HoldBinding(action string, durationMs int) (Result, error) {
	started := time.Now()
	if durationMs < 20 {
		durationMs = 20
	}
	if durationMs > 10000 {
		durationMs = 10000
	}
	key, mods, slot, err := b.resolveAction(action)
	if err != nil {
		return Result{}, err
	}
	if err := b.input.FocusGame(); err != nil {
		return Result{}, fmt.Errorf("could not target Elite Dangerous: %w", err)
	}
	if err := b.input.HoldChord(key, mods, durationMs); err != nil {
		return Result{}, err
	}
	return Result{Name: "binding.hold", Action: action, BindingSlot: slot, Key: key, Modifiers: mods, DurationMs: time.Since(started).Milliseconds()}, nil
}

func (b *Bridge) TypeText(text string, intervalMs int) (Result, error) {
	started := time.Now()
	if intervalMs < 0 {
		intervalMs = 0
	}
	if intervalMs > 1000 {
		intervalMs = 1000
	}
	if err := b.input.FocusGame(); err != nil {
		return Result{}, fmt.Errorf("could not target Elite Dangerous: %w", err)
	}
	if err := platform.TypeText(b.input, text, time.Duration(intervalMs)*time.Millisecond); err != nil {
		return Result{}, err
	}
	return Result{Name: "input.text", DurationMs: time.Since(started).Milliseconds()}, nil
}

func (b *Bridge) HeldCount() int {
	b.heldMu.Lock()
	defer b.heldMu.Unlock()
	return len(b.held)
}

func (b *Bridge) HeldActions() []string {
	b.heldMu.Lock()
	defer b.heldMu.Unlock()
	out := make([]string, 0, len(b.held))
	for action := range b.held {
		out = append(out, action)
	}
	sort.Strings(out)
	return out
}

// ReleaseAll releases only chords that JACoB previously pressed through
// binding.down. It deliberately does not verify foreground; releases must be
// fail-safe even after Steam/Gamescope changes focus.
func (b *Bridge) ReleaseAll() (int, error) {
	b.heldMu.Lock()
	defer b.heldMu.Unlock()
	if len(b.held) == 0 {
		return 0, nil
	}
	actions := make([]string, 0, len(b.held))
	for action := range b.held {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	released := 0
	var firstErr error
	for _, action := range actions {
		h := b.held[action]
		if err := b.input.ChordUp(h.Key, h.Mods); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("release %s: %w", action, err)
			}
			continue
		}
		delete(b.held, action)
		released++
	}
	return released, firstErr
}
