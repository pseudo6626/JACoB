//go:build !windows && !linux

package platform

import "fmt"

type unavailableRecorder struct{}

func newInputRecorder() InputRecorder          { return &unavailableRecorder{} }
func (r *unavailableRecorder) Name() string    { return "unavailable" }
func (r *unavailableRecorder) Available() bool { return false }
func (r *unavailableRecorder) Start(func(RecordedInputEvent)) error {
	return fmt.Errorf("input recording is unavailable on this platform")
}
func (r *unavailableRecorder) Stop() ([]RecordedInputEvent, error) {
	return nil, fmt.Errorf("input recording is unavailable on this platform")
}
func (r *unavailableRecorder) Status() RecorderStatus {
	return RecorderStatus{Available: false, Driver: r.Name(), Scope: "Elite foreground keyboard only"}
}
