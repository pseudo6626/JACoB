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

func TestLayoutPersistsAndOrdersCustomTabs(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	one, err := s.Save("", "One", "<p>1</p>")
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.Save("", "Two", "<p>2</p>")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := s.SaveLayout([]string{"settings", "custom-" + two.ID, "dashboard", "custom-" + one.ID, "tabmanager", "tutorial"}, []string{"dashboard", "settings", "tabmanager"})
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.HiddenDefaults) != 2 || layout.HiddenDefaults[0] != "dashboard" || layout.HiddenDefaults[1] != "settings" {
		t.Fatalf("unexpected hidden defaults: %#v", layout.HiddenDefaults)
	}
	list := s.List()
	if len(list) != 2 || list[0].ID != two.ID || list[1].ID != one.ID {
		t.Fatalf("unexpected custom tab order: %#v", list)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Layout()
	if len(got.Order) < 6 || got.Order[0] != "settings" || got.Order[1] != "custom-"+two.ID {
		t.Fatalf("layout did not persist: %#v", got)
	}
}

func TestLayoutDropsDeletedCustomTab(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tab, err := s.Save("", "Gone", "<p>x</p>")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveLayout([]string{"dashboard", "custom-" + tab.ID, "tabmanager", "tutorial", "settings"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(tab.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range s.Layout().Order {
		if id == "custom-"+tab.ID {
			t.Fatalf("deleted tab remained in layout: %#v", s.Layout())
		}
	}
}

func TestLayoutCanHideCustomTabs(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	tab, err := s.Save("", "Hidden Tool", "<p>x</p>")
	if err != nil {
		t.Fatal(err)
	}
	navID := "custom-" + tab.ID
	layout, err := s.SaveLayout([]string{"dashboard", navID, "tabmanager", "tutorial", "settings"}, []string{navID, "settings", "tabmanager"})
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.HiddenDefaults) != 2 || layout.HiddenDefaults[0] != navID || layout.HiddenDefaults[1] != "settings" {
		t.Fatalf("unexpected hidden navigation: %#v", layout.HiddenDefaults)
	}
	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Layout()
	found := false
	for _, id := range got.HiddenDefaults {
		if id == navID {
			found = true
		}
		if id == "tabmanager" {
			t.Fatalf("Tab Manager must never be hideable: %#v", got.HiddenDefaults)
		}
	}
	if !found {
		t.Fatalf("hidden custom tab did not persist: %#v", got.HiddenDefaults)
	}
	if err := s2.Delete(tab.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range s2.Layout().HiddenDefaults {
		if id == navID {
			t.Fatalf("deleted custom tab remained hidden in layout: %#v", s2.Layout())
		}
	}
}
