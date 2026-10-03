package customtabs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	tab, err := s.Save("", "Route Queue", "<h1>hi</h1>")
	if err != nil {
		t.Fatal(err)
	}
	if tab.ID == "" {
		t.Fatal("expected id")
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get(tab.ID)
	if !ok {
		t.Fatal("saved tab did not reload")
	}
	if got.Name != "Route Queue" || got.HTML != "<h1>hi</h1>" {
		t.Fatalf("unexpected tab: %#v", got)
	}
}

func TestStoreUpdateDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	tab, err := s.Save("", "One", "<p>1</p>")
	if err != nil {
		t.Fatal(err)
	}
	tab, err = s.Save(tab.ID, "Two", "<p>2</p>")
	if err != nil {
		t.Fatal(err)
	}
	if tab.Name != "Two" {
		t.Fatalf("name=%q", tab.Name)
	}
	if err := s.Delete(tab.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(tab.ID); ok {
		t.Fatal("tab still exists")
	}
	if _, err := os.Stat(filepath.Join(dir, "custom-tabs.json")); err != nil {
		t.Fatal(err)
	}
}
