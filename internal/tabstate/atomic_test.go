package tabstate

import (
	"path/filepath"
	"testing"
)

func TestBatchIsAtomic(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("tab1", "a", 1); err != nil {
		t.Fatal(err)
	}
	_, err = s.Batch("tab1", []BatchOperation{
		{Op: "set", Key: "a", Value: 2},
		{Op: "nope", Key: "b", Value: 3},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	got, ok := s.Get("tab1", "a")
	if !ok || got.(float64) != 1 {
		t.Fatalf("batch partially applied: %#v", got)
	}
}

func TestCompareSet(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.CompareSet("tab1", "counter", false, nil, 1)
	if err != nil || !r.Swapped {
		t.Fatalf("first CAS: %+v %v", r, err)
	}
	r, err = s.CompareSet("tab1", "counter", true, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if r.Swapped {
		t.Fatal("stale CAS unexpectedly swapped")
	}
	r, err = s.CompareSet("tab1", "counter", true, 1.0, 2)
	if err != nil || !r.Swapped {
		t.Fatalf("second CAS: %+v %v", r, err)
	}
}

func TestBatchSetDelete(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Batch("tab1", []BatchOperation{
		{Op: "set", Key: "a", Value: map[string]any{"v": 1}},
		{Op: "set", Key: "b", Value: "x"},
		{Op: "delete", Key: "b"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("tab1", "a"); !ok {
		t.Fatal("a missing")
	}
	if _, ok := s.Get("tab1", "b"); ok {
		t.Fatal("b should be deleted")
	}
}
