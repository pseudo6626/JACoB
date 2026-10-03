package appearance

import "testing"

func TestThemePersists(t *testing.T) {
	dir := t.TempDir()
	a, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	html := `<style>:root{--accent:#123}</style><template data-jacob-slot="brand">Test</template>`
	if err := a.Save(html); err != nil {
		t.Fatal(err)
	}
	b, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.Get() != html {
		t.Fatalf("theme did not persist: %q", b.Get())
	}
	if err := b.Reset(); err != nil {
		t.Fatal(err)
	}
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Get() != "" {
		t.Fatal("reset theme reappeared")
	}
}
