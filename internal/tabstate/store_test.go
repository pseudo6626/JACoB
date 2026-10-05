package tabstate

import "testing"

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("abc-123", "orders", map[string]any{"count": 3.0}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get("abc-123", "orders")
	if !ok {
		t.Fatal("missing stored value")
	}
	m := got.(map[string]any)
	if m["count"].(float64) != 3 {
		t.Fatalf("unexpected value: %#v", got)
	}
	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Get("abc-123", "orders"); !ok {
		t.Fatal("value did not persist")
	}
	if err := s2.Delete("abc-123", "orders"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Get("abc-123", "orders"); ok {
		t.Fatal("value remained after delete")
	}
}
