//go:build !windows && !linux

package platform

import "fmt"

type unsupportedInput struct{}

func newInputDriver() InputDriver           { return &unsupportedInput{} }
func (u *unsupportedInput) Name() string    { return "unsupported" }
func (u *unsupportedInput) Available() bool { return false }
func (u *unsupportedInput) FocusGame() error {
	return fmt.Errorf("game focus is unsupported on this platform")
}
func (u *unsupportedInput) TapKey(key string) error { return u.TapChord(key, nil) }
func (u *unsupportedInput) TapChord(key string, modifiers []string) error {
	return fmt.Errorf("input injection unsupported on this platform")
}
func (u *unsupportedInput) HoldChord(key string, modifiers []string, durationMs int) error {
	return fmt.Errorf("input injection unsupported on this platform")
}
