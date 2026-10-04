package customtabs

import (
	"os"
	"path/filepath"
	"strings"
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

func TestListReturnsMetadataOnlyAndStoresHTMLSeparately(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	body := "<h1>large tab</h1>"
	tab, err := s.Save("", "Large", body)
	if err != nil {
		t.Fatal(err)
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("len(list)=%d", len(list))
	}
	if list[0].HTML != "" {
		t.Fatal("tabs.list metadata unexpectedly included HTML")
	}
	if list[0].SizeBytes != int64(len(body)) {
		t.Fatalf("sizeBytes=%d want=%d", list[0].SizeBytes, len(body))
	}
	contentPath := filepath.Join(dir, "custom-tabs", tab.ID+".html")
	got, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("content file=%q", string(got))
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "custom-tabs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), body) {
		t.Fatal("manifest still contains inline tab HTML")
	}
}

func TestStoreMigratesSchema2InlineHTML(t *testing.T) {
	dir := t.TempDir()
	legacy := `{
  "schemaVersion": 2,
  "tabs": [{"id":"tab-legacy","name":"Legacy","html":"<p>old</p>","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}],
  "layout": {"order":["dashboard","custom-tab-legacy","tabmanager","tutorial","settings"]}
}`
	if err := os.WriteFile(filepath.Join(dir, "custom-tabs.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	tab, ok := s.Get("tab-legacy")
	if !ok || tab.HTML != "<p>old</p>" {
		t.Fatalf("migrated tab=%#v ok=%v", tab, ok)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "custom-tabs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "<p>old</p>") {
		t.Fatal("legacy inline HTML remained in migrated manifest")
	}
	if !strings.Contains(string(manifest), `"schemaVersion": 3`) {
		t.Fatalf("manifest not upgraded: %s", manifest)
	}
}

func TestStoreAcceptsMultiMegabyteTab(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body := "<html><body>" + strings.Repeat("x", 2*1024*1024) + "</body></html>"
	tab, err := s.Save("", "Two MB", body)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get(tab.ID)
	if !ok || got.HTML != body {
		t.Fatal("multi-megabyte tab did not round-trip")
	}
	tooLarge := strings.Repeat("x", maxHTMLBytes+1)
	if _, err := s.Save("", "Too large", tooLarge); err == nil {
		t.Fatal("expected over-limit tab to be rejected")
	}
}
