package control

import (
	"errors"
	"testing"

	"jacob/internal/bindings"
)

type fakeInput struct {
	focusErr error
	down     int
	up       int
}

func (f *fakeInput) Name() string                          { return "fake" }
func (f *fakeInput) Available() bool                       { return true }
func (f *fakeInput) FocusGame() error                      { return f.focusErr }
func (f *fakeInput) TapKey(string) error                   { return nil }
func (f *fakeInput) TapChord(string, []string) error       { return nil }
func (f *fakeInput) HoldChord(string, []string, int) error { return nil }
func (f *fakeInput) ChordDown(string, []string) error      { f.down++; return nil }
func (f *fakeInput) ChordUp(string, []string) error        { f.up++; return nil }

func TestTrackedReleaseIgnoresLaterFocusLoss(t *testing.T) {
	store := bindings.New("")
	// Store internals are intentionally not reached here; ReleaseAll behavior is
	// covered by seeding the held map because binding parsing has its own tests.
	in := &fakeInput{}
	b := New(store, in)
	b.held["UI_Select"] = heldChord{Action: "UI_Select", Key: "ENTER", Slot: "secondary"}
	in.focusErr = errors.New("steam overlay focused")
	if _, err := b.UpBinding("UI_Select"); err != nil {
		t.Fatalf("tracked key-up should not require focus: %v", err)
	}
	if in.up != 1 || b.HeldCount() != 0 {
		t.Fatalf("release failed: up=%d held=%d", in.up, b.HeldCount())
	}
}

func TestReleaseAllReleasesTrackedChords(t *testing.T) {
	in := &fakeInput{focusErr: errors.New("not focused")}
	b := New(bindings.New(""), in)
	b.held["A"] = heldChord{Action: "A", Key: "A"}
	b.held["B"] = heldChord{Action: "B", Key: "B"}
	n, err := b.ReleaseAll()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || in.up != 2 || b.HeldCount() != 0 {
		t.Fatalf("release all failed: released=%d up=%d held=%d", n, in.up, b.HeldCount())
	}
}
