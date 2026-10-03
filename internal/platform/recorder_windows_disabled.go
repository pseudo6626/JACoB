//go:build windows && norecorder

package platform

import "fmt"

type disabledRecorder struct{}

func newInputRecorder() InputRecorder       { return &disabledRecorder{} }
func (r *disabledRecorder) Name() string    { return "not-installed" }
func (r *disabledRecorder) Available() bool { return false }
func (r *disabledRecorder) Start(func(RecordedInputEvent)) error {
	return fmt.Errorf("input recorder was not installed")
}
func (r *disabledRecorder) Stop() ([]RecordedInputEvent, error) {
	return nil, fmt.Errorf("input recorder was not installed")
}
func (r *disabledRecorder) Status() RecorderStatus {
	return RecorderStatus{Available: false, Driver: r.Name(), Scope: "optional component not installed"}
}
