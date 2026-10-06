package platform

import "sync"

type RecordedInputEvent struct {
	PressID       uint64   `json:"pressId"`
	Type          string   `json:"type"`
	Key           string   `json:"key"`
	Physical      string   `json:"physical,omitempty"`
	LocalizedName string   `json:"localizedName,omitempty"`
	ScanCode      uint32   `json:"scanCode,omitempty"`
	VirtualKey    uint32   `json:"virtualKey,omitempty"`
	Extended      bool     `json:"extended,omitempty"`
	Modifiers     []string `json:"modifiers,omitempty"`
	AtMs          int64    `json:"atMs"`
	DeltaMs       int64    `json:"deltaMs"`
	DurationMs    int64    `json:"durationMs,omitempty"`
	IsModifier    bool     `json:"isModifier,omitempty"`
}

type RecorderStatus struct {
	Available  bool   `json:"available"`
	Driver     string `json:"driver"`
	Recording  bool   `json:"recording"`
	EventCount int    `json:"eventCount"`
	Scope      string `json:"scope"`
}

type InputRecorder interface {
	Name() string
	Available() bool
	Start(func(RecordedInputEvent)) error
	Stop() ([]RecordedInputEvent, error)
	Status() RecorderStatus
}

func NewInputRecorder() InputRecorder { return newInputRecorder() }

type recorderBase struct {
	mu sync.Mutex
}
