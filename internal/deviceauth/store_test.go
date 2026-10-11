package deviceauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnrollmentUsesPerDeviceSecretAndHidesItFromView(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dev, credential, err := s.Enroll("Телефон Chrome")
	if err != nil {
		t.Fatal(err)
	}
	if dev.ID == "" || credential == "" || !strings.Contains(credential, ".") {
		t.Fatalf("invalid enrollment: device=%+v credential=%q", dev, credential)
	}
	got, ok := s.Authenticate(credential)
	if !ok || got.ID != dev.ID {
		t.Fatalf("credential did not authenticate: got=%+v ok=%v", got, ok)
	}
	if _, ok := s.Authenticate(credential + "x"); ok {
		t.Fatal("modified credential authenticated")
	}
	b, err := json.Marshal(dev)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), "hash") {
		t.Fatalf("public device view leaked credential material: %s", b)
	}
}

func TestRememberedAdminPersistsButTemporaryDeviceDoesNot(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	temporary, _, err := s.Enroll("Temporary")
	if err != nil {
		t.Fatal(err)
	}
	remembered, credential, err := s.Enroll("Remembered")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetAdmin(remembered.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRemember(remembered.ID, true); err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(filepath.Join(root, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(credential, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("malformed test credential %q", credential)
	}
	if strings.Contains(string(disk), credential) || strings.Contains(string(disk), parts[1]) {
		t.Fatalf("devices.json persisted plaintext credential material: %s", disk)
	}

	reloaded, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Get(temporary.ID); ok {
		t.Fatal("temporary device survived process restart")
	}
	got, ok := reloaded.Authenticate(credential)
	if !ok || !got.Remember || !got.Admin {
		t.Fatalf("remembered admin did not persist: %+v ok=%v", got, ok)
	}
}

func TestTurningRememberOffRemovesPersistence(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	dev, _, err := s.Enroll("Tablet")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRemember(dev.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRemember(dev.ID, false); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Get(dev.ID); ok {
		t.Fatal("forgotten device persisted after restart")
	}
}
