//go:build windows

package platform

import "testing"

func TestResolveWindowsPhysicalKey(t *testing.T) {
	got, err := resolveWindowsKey("SC:E0:38")
	if err != nil {
		t.Fatal(err)
	}
	if got.scan != 0x38 || !got.extended {
		t.Fatalf("resolved=%+v", got)
	}
}
