package platform

type InputDriver interface {
	Name() string
	Available() bool
	FocusGame() error
	TapKey(key string) error
	TapChord(key string, modifiers []string) error
	HoldChord(key string, modifiers []string, durationMs int) error
}

func NewInputDriver() InputDriver { return newInputDriver() }
