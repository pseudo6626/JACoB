// Package clipboard exposes bounded, write-only access to the host clipboard.
// It never focuses a window, sends input, or exposes clipboard contents.
package clipboard

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const MaxTextBytes = 16 * 1024

type Info struct {
	Available bool   `json:"available"`
	Backend   string `json:"backend"`
	Reason    string `json:"reason,omitempty"`
	WriteOnly bool   `json:"writeOnly"`
	HostOnly  bool   `json:"hostOnly"`
	MaxBytes  int    `json:"maxTextBytes"`
}

type Result struct {
	Written bool   `json:"written"`
	Backend string `json:"backend"`
	Bytes   int    `json:"bytes"`
}

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

type backend struct {
	name   string
	reason string
	write  func(context.Context, string) error
}

type Service struct {
	mu     sync.Mutex
	detect func() backend
}

func New() *Service { return &Service{detect: nativeBackend} }

func (s *Service) Info() Info {
	b := s.detect()
	return Info{Available: b.write != nil, Backend: b.name, Reason: b.reason,
		WriteOnly: true, HostOnly: true, MaxBytes: MaxTextBytes}
}

func (s *Service) WriteText(text string) (Result, error) {
	if len(text) > MaxTextBytes || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return Result{}, &Error{"BAD_PARAMS", "text must be valid UTF-8, contain no NUL, and be at most 16384 UTF-8 bytes"}
	}
	// Reject concurrent writers rather than accumulating stale targets in a queue.
	if !s.mu.TryLock() {
		return Result{}, &Error{"CLIPBOARD_BUSY", "another clipboard write is in progress; try again"}
	}
	defer s.mu.Unlock()
	b := s.detect()
	if b.write == nil {
		return Result{}, &Error{"CLIPBOARD_UNAVAILABLE", b.reason}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.write(ctx, text); err != nil {
		var apiErr *Error
		if errors.As(err, &apiErr) {
			return Result{}, apiErr
		}
		if ctx.Err() != nil {
			return Result{}, &Error{"CLIPBOARD_TIMEOUT", "clipboard backend did not finish within two seconds"}
		}
		return Result{}, &Error{"CLIPBOARD_FAILED", err.Error()}
	}
	return Result{Written: true, Backend: b.name, Bytes: len(text)}, nil
}
