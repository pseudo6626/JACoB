package tabstorage

import (
	"bytes"
	"errors"
	"testing"
)

func TestPutReadListAndDelete(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"system":"Sol"}`)
	item, err := s.PutBytes("tab-one", "systems/1.json", "application/json", want)
	if err != nil {
		t.Fatal(err)
	}
	if item.SizeBytes != int64(len(want)) || item.Type != "application/json" {
		t.Fatalf("bad item: %+v", item)
	}
	read, err := s.Read("tab-one", "systems/1.json", 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(read.Data) != string(want[:4]) || read.EOF {
		t.Fatalf("bad first chunk: %+v", read)
	}
	read2, err := s.Read("tab-one", "systems/1.json", 4, ChunkBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(append(read.Data, read2.Data...), want) || !read2.EOF {
		t.Fatal("chunked read mismatch")
	}
	list, err := s.List("tab-one", "systems/", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "systems/1.json" {
		t.Fatalf("bad list: %+v", list)
	}
	deleted, err := s.Delete("tab-one", "systems/1.json")
	if err != nil || !deleted {
		t.Fatalf("delete: %v %v", deleted, err)
	}
	if _, err := s.Read("tab-one", "systems/1.json", 0, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wanted not found, got %v", err)
	}
}

func TestIsolationAndTraversal(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutBytes("tab-a", "data/x.bin", "application/octet-stream", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read("tab-b", "data/x.bin", 0, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tab read should miss: %v", err)
	}
	for _, bad := range []string{"../escape", "/absolute", "a/../../b", `a\\b`, ".jacob-storage.json", "data/.jacob-secret"} {
		if _, err := s.PutBytes("tab-a", bad, "text/plain", []byte("x")); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("%q should be invalid: %v", bad, err)
		}
	}
}

func TestQuotaAndClear(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Info("tab-q")
	if err != nil {
		t.Fatal(err)
	}
	if info.QuotaBytes != DefaultQuotaBytes {
		t.Fatalf("default quota %d", info.QuotaBytes)
	}
	info, err = s.SetQuota("tab-q", 256*1024*1024)
	if err != nil || info.QuotaBytes != 256*1024*1024 {
		t.Fatalf("quota: %+v %v", info, err)
	}
	if _, err := s.SetQuota("tab-q", 300*1024*1024); err == nil {
		t.Fatal("arbitrary quota should fail")
	}
	if _, err := s.PutBytes("tab-q", "one.txt", "text/plain", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear("tab-q"); err != nil {
		t.Fatal(err)
	}
	info, err = s.Info("tab-q")
	if err != nil || info.UsedBytes != 0 || info.Items != 0 || info.QuotaBytes != DefaultQuotaBytes {
		t.Fatalf("clear: %+v %v", info, err)
	}
}

func TestUploadOffsets(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.Begin("tab-u", "big.bin", "application/octet-stream", 6)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteChunk("tab-u", u.UploadID, 1, []byte("abc")); !errors.Is(err, ErrBadOffset) {
		t.Fatalf("expected bad offset, got %v", err)
	}
	if err := s.WriteChunk("tab-u", u.UploadID, 0, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteChunk("tab-u", u.UploadID, 3, []byte("def")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit("tab-u", u.UploadID); err != nil {
		t.Fatal(err)
	}
}
