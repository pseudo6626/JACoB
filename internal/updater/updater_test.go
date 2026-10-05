package updater

import (
	"runtime"
	"testing"
)

func TestSelectAssetForCurrentPlatform(t *testing.T) {
	assets := []Asset{
		{Name: "JACoB-Alpha-0.2.8-No-Recorder.exe"},
		{Name: "JACoB-Alpha-0.2.8-Portable.exe"},
		{Name: "JACoB-Alpha-0.2.8-Setup.exe"},
		{Name: "JACoB-Alpha-0.2.8-SteamDeck-linux-amd64"},
	}
	a := selectAsset(assets)
	if runtime.GOOS == "windows" && (a == nil || a.Name != assets[2].Name) {
		t.Fatalf("wrong windows asset: %#v", a)
	}
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" && (a == nil || a.Name != assets[3].Name) {
		t.Fatalf("wrong linux asset: %#v", a)
	}
}

func TestWindowsLegacyInstallerNameIsRecognized(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("selector follows current runtime")
	}
	a := selectAsset([]Asset{{Name: "JACoB-Alpha-0.2.8.exe"}, {Name: "JACoB-Alpha-0.2.8-Portable.exe"}})
	if a == nil || a.Name != "JACoB-Alpha-0.2.8.exe" {
		t.Fatalf("legacy installer name not selected: %#v", a)
	}
}
