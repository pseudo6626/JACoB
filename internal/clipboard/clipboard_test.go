package clipboard

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var got *Error
	if !errors.As(err, &got) || got.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

func TestWriteValidationAndAcknowledgement(t *testing.T) {
	var writes []string
	s := &Service{detect: func() backend {
		return backend{name: "test", write: func(_ context.Context, text string) error {
			writes = append(writes, text)
			return nil
		}}
	}}
	for _, text := range []string{"", "Phroea Phio UK-W c18-0", "Зоря 🌟\r\nsecond line", strings.Repeat("x", MaxTextBytes)} {
		result, err := s.WriteText(text)
		if err != nil || !result.Written || result.Bytes != len(text) || writes[len(writes)-1] != text {
			t.Fatalf("write mismatch: result=%+v error=%v", result, err)
		}
	}
	before := len(writes)
	for _, text := range []string{"a\x00b", string([]byte{0xff}), strings.Repeat("x", MaxTextBytes+1), strings.Repeat("🌟", MaxTextBytes/4+1)} {
		_, err := s.WriteText(text)
		wantCode(t, err, "BAD_PARAMS")
	}
	if len(writes) != before {
		t.Fatal("invalid text reached backend")
	}
}

func TestUnavailableAndFailedWriteNeverAcknowledgeSuccess(t *testing.T) {
	for _, tc := range []struct {
		b    backend
		code string
	}{
		{backend{name: "missing", reason: "no display"}, "CLIPBOARD_UNAVAILABLE"},
		{backend{name: "bad", write: func(context.Context, string) error { return errors.New("backend failed") }}, "CLIPBOARD_FAILED"},
		{backend{name: "busy", write: func(context.Context, string) error { return &Error{"CLIPBOARD_BUSY", "locked"} }}, "CLIPBOARD_BUSY"},
	} {
		s := &Service{detect: func() backend { return tc.b }}
		result, err := s.WriteText("target")
		wantCode(t, err, tc.code)
		if result.Written {
			t.Fatal("failed write claimed success")
		}
	}
}

func TestConcurrentWriteDoesNotQueueStaleTarget(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	s := &Service{detect: func() backend {
		return backend{name: "test", write: func(context.Context, string) error {
			close(entered)
			<-release
			return nil
		}}
	}}
	go func() { _, err := s.WriteText("first"); done <- err }()
	<-entered
	_, err := s.WriteText("stale second")
	wantCode(t, err, "CLIPBOARD_BUSY")
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestInfoDoesNotWriteAndCanBeRechecked(t *testing.T) {
	available := false
	s := &Service{detect: func() backend {
		if !available {
			return backend{name: "missing", reason: "no display"}
		}
		return backend{name: "test", write: func(context.Context, string) error { t.Fatal("info must not write"); return nil }}
	}}
	if s.Info().Available {
		t.Fatal("unexpected availability")
	}
	available = true
	i := s.Info()
	if !i.Available || !i.WriteOnly || !i.HostOnly || i.MaxBytes != MaxTextBytes {
		t.Fatalf("bad info: %+v", i)
	}
}

func TestServiceTimeoutNeverAcknowledgesSuccess(t *testing.T) {
	s := &Service{detect: func() backend {
		return backend{name: "blocked", write: func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return ctx.Err()
		}}
	}}
	result, err := s.WriteText("target")
	wantCode(t, err, "CLIPBOARD_TIMEOUT")
	if result.Written {
		t.Fatal("timed-out write claimed success")
	}
}

func TestLinuxBackendSelection(t *testing.T) {
	for _, tc := range []struct {
		name, wayland, x11 string
		tools              []string
		want               string
	}{
		{"Wayland", "wayland-0", ":0", []string{"wl-copy", "xclip"}, "wl-copy"},
		{"X11 fallback when utility missing", "wayland-0", ":0", []string{"xclip"}, "xclip"},
		{"X11", "", ":0", []string{"wl-copy", "xclip", "xsel"}, "xclip"},
		{"xsel", "", ":0", []string{"xsel"}, "xsel"},
		{"headless", "", "", []string{"wl-copy", "xclip", "xsel"}, ""},
		{"no tool", "wayland-0", ":0", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := linuxCommand(func(k string) string {
				if k == "WAYLAND_DISPLAY" {
					return tc.wayland
				}
				return tc.x11
			}, func(name string) (string, error) {
				for _, v := range tc.tools {
					if v == name {
						return "/usr/bin/" + name, nil
					}
				}
				return "", os.ErrNotExist
			})
			if ok != (tc.want != "") || spec.name != tc.want {
				t.Fatalf("got %+v, %v", spec, ok)
			}
		})
	}
}

func TestCommandUsesStdinAndPreservesUnicode(t *testing.T) {
	// Run this test binary as a fake clipboard executable on either OS.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JACOB_CLIPBOARD_TEST_CHILD", "1")
	text := "Зоря 🌟\n$(do not execute) `literal` --flag"
	t.Setenv("JACOB_CLIPBOARD_TEST_EXPECT", text)
	err = runCommand(context.Background(), commandSpec{name: "test", path: exe, args: []string{"-test.run=^TestClipboardCommandChild$"}}, text)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JACOB_CLIPBOARD_TEST_EXPECT", "different")
	if runCommand(context.Background(), commandSpec{name: "test", path: exe, args: []string{"-test.run=^TestClipboardCommandChild$"}}, text) == nil {
		t.Fatal("nonzero backend exit was ignored")
	}
}
