//go:build linux

package platform

import "fmt"

type linuxRecorder struct{}

func newInputRecorder() InputRecorder    { return &linuxRecorder{} }
func (r *linuxRecorder) Name() string    { return "linux-evdev-pending" }
func (r *linuxRecorder) Available() bool { return false }
func (r *linuxRecorder) Start(func(RecordedInputEvent)) error {
	return fmt.Errorf("Elite-scoped input recording is not yet enabled on the Linux/Steam Deck adapter")
}
func (r *linuxRecorder) Stop() ([]RecordedInputEvent, error) {
	return nil, fmt.Errorf("input recorder is not running")
}
func (r *linuxRecorder) Status() RecorderStatus {
	return RecorderStatus{Available: false, Driver: r.Name(), Scope: "Elite foreground keyboard only"}
}
