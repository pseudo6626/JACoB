package customtabs

import "testing"

func TestSetCommunityPersistsMetadata(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tab, err := s.Save("", "Example", "<!doctype html><title>Example</title>")
	if err != nil {
		t.Fatal(err)
	}
	meta := &CommunityInfo{PackageID: "community.example.a1b2c3", LastSubmittedVersion: "1.0.0", Author: "CMDR Example", Tags: []string{"utility"}, LocalAuthor: true, Status: "prepared"}
	if _, err := s.SetCommunity(tab.ID, meta); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get(tab.ID)
	if !ok || got.Community == nil {
		t.Fatalf("community metadata missing: %#v", got)
	}
	if got.Community.PackageID != meta.PackageID || got.Community.LastSubmittedVersion != "1.0.0" {
		t.Fatalf("unexpected metadata: %#v", got.Community)
	}

	s2, err := New(s.Directory())
	if err != nil {
		t.Fatal(err)
	}
	got, ok = s2.Get(tab.ID)
	if !ok || got.Community == nil || got.Community.PackageID != meta.PackageID {
		t.Fatalf("metadata did not persist: %#v", got.Community)
	}
}
