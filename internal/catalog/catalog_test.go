package catalog

import (
	"encoding/json"
	"testing"
)

func TestDecodeSpanshRecordPreservesID64(t *testing.T) {
	body := `{"record":{"id64":10477373803,"name":"Test","coords":{"x":1.25,"y":2,"z":3},"body_count":4}}`
	rec, err := decodeSpanshRecord(body)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rec["id64"].(json.Number); !ok {
		t.Fatalf("id64 type=%T want json.Number", rec["id64"])
	}
	s := summaryFromRecord(rec, "")
	if s["id64"] != "10477373803" || s["name"] != "Test" {
		t.Fatalf("unexpected summary: %#v", s)
	}
}

func TestDecodeDirectRecord(t *testing.T) {
	rec, err := decodeSpanshRecord(`{"id64":3309179996515,"name":"Synuefe EN-H d11-96"}`)
	if err != nil {
		t.Fatal(err)
	}
	s := summaryFromRecord(rec, "")
	if s["id64"] != "3309179996515" {
		t.Fatalf("id64=%v", s["id64"])
	}
}
