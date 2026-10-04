package localization

import "testing"

func TestLocalePersistsAndNormalizes(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got != "en" {
		t.Fatalf("default=%q", got)
	}
	if err := s.Set("es-ES"); err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got != "es" {
		t.Fatalf("set=%q", got)
	}
	again, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Get(); got != "es" {
		t.Fatalf("persisted=%q", got)
	}
	if err := s.Set("xx"); err == nil {
		t.Fatal("expected unsupported language error")
	}
}

func TestSupportedSixLanguages(t *testing.T) {
	got := Supported()
	if len(got) != 6 {
		t.Fatalf("supported=%d", len(got))
	}
	want := []string{"en", "ru", "de", "fr", "zh-CN", "es"}
	for i, code := range want {
		if got[i].Code != code {
			t.Fatalf("supported[%d]=%q want %q", i, got[i].Code, code)
		}
	}
}
