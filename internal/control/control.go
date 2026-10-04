package control

import (
	"fmt"
	"time"

	"jacob/internal/bindings"
	"jacob/internal/platform"
)

type Bridge struct {
	bindings *bindings.Store
	input    platform.InputDriver
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
	return &Bridge{bindings: b, input: i}
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
	if err := b.input.ChordDown(key, mods); err != nil {
		return Result{}, err
	}
	return Result{Name: "binding.down", Action: action, BindingSlot: slot, Key: key, Modifiers: mods, DurationMs: time.Since(started).Milliseconds()}, nil
}

func (b *Bridge) UpBinding(action string) (Result, error) {
	started := time.Now()
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
