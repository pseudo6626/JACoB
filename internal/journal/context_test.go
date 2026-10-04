package journal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSeedContextFindsLatestLocationAndLoadout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Journal.test.log")
	data := "" +
		`{"timestamp":"2026-10-03T20:00:00Z","event":"Location","StarSystem":"Sol"}` + "\n" +
		`{"timestamp":"2026-10-03T20:00:01Z","event":"Loadout","Ship":"krait_light","ShipName":"Wayfarer","ShipIdent":"NH-01","MaxJumpRange":65.4321}` + "\n" +
		`{"timestamp":"2026-10-03T20:00:02Z","event":"FSDJump","StarSystem":"Jackson's Lighthouse"}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	w := New(dir, false, nil)
	w.seedContext(path)
	snap := w.Snapshot()
	ctx, _ := snap["journalContext"].(map[string]any)
	if got := ctx["currentSystem"]; got != "Jackson's Lighthouse" {
		t.Fatalf("currentSystem = %v", got)
	}
	if got := ctx["maxJumpRange"]; got != 65.4321 {
		t.Fatalf("maxJumpRange = %v", got)
	}
	if got := ctx["shipName"]; got != "Wayfarer" {
		t.Fatalf("shipName = %v", got)
	}
}

func TestSeedContextFindsCarrierStats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Journal.carrier.log")
	data := `{"timestamp":"2026-10-03T20:00:00Z","event":"CarrierStats","CarrierID":3700005632,"Callsign":"ABC-123","Name":"Test Carrier","SpaceUsage":{"TotalCapacity":25000,"Cargo":5000,"CargoSpaceReserved":1200,"FreeSpace":8800},"Finance":{"AvailableBalance":123456789}}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	w := New(dir, false, nil)
	w.seedContext(path)
	ctx, _ := w.Snapshot()["journalContext"].(map[string]any)
	if ctx["carrierCallsign"] != "ABC-123" {
		t.Fatalf("carrierCallsign = %v", ctx["carrierCallsign"])
	}
	if ctx["carrierFreeSpace"] != float64(8800) {
		t.Fatalf("carrierFreeSpace = %v", ctx["carrierFreeSpace"])
	}
	if ctx["carrierAvailableBalance"] != float64(123456789) {
		t.Fatalf("carrierAvailableBalance = %v", ctx["carrierAvailableBalance"])
	}
}

func TestReadMarketSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Market.json")
	data := `{"event":"Market","MarketID":42,"StationName":"Carrier","StarSystem":"Sol","Items":[{"Name":"$tritium_name;","Name_Localised":"Tritium","Category":"$MARKET_category_chemicals;","Category_Localised":"Chemicals","MeanPrice":50000}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	w := New(dir, false, nil)
	mods := map[string]time.Time{}
	if err := w.readEliteFiles(mods); err != nil {
		t.Fatal(err)
	}
	market, _ := w.Snapshot()["market"].(map[string]any)
	if market["StationName"] != "Carrier" {
		t.Fatalf("station = %v", market["StationName"])
	}
	items, ok := market["Items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v", market["Items"])
	}
}

func TestReadEliteFilesExposesKnownAndFutureSnapshots(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"Status.json":      `{"event":"Status","Heading":42}`,
		"Market.json":      `{"event":"Market","StationName":"Carrier"}`,
		"Cargo.json":       `{"event":"Cargo","Inventory":[{"Name":"tritium","Count":12}]}`,
		"NavRoute.json":    `{"event":"Route","Route":[{"StarSystem":"Sol"}]}`,
		"ModulesInfo.json": `{"event":"ModuleInfo","Modules":[{"Slot":"FrameShiftDrive"}]}`,
		"Outfitting.json":  `{"event":"Outfitting","Items":[]}`,
		"Shipyard.json":    `{"event":"Shipyard","PriceList":[]}`,
		"Backpack.json":    `{"event":"Backpack","Items":[]}`,
		"ShipLocker.json":  `{"event":"ShipLocker","Items":[]}`,
		"FCMaterials.json": `{"event":"FCMaterials","Items":[]}`,
		"Future_File.json": `{"event":"FutureSnapshot","Value":7}`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w := New(dir, false, nil)
	mods := map[string]time.Time{}
	if err := w.readEliteFiles(mods); err != nil {
		t.Fatal(err)
	}
	snap := w.Snapshot()
	all, ok := snap["eliteFiles"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("eliteFiles type = %T", snap["eliteFiles"])
	}
	for _, key := range []string{"status", "market", "cargo", "navRoute", "modulesInfo", "outfitting", "shipyard", "backpack", "shipLocker", "fcMaterials", "futureFile"} {
		if _, ok := all[key]; !ok {
			t.Fatalf("missing eliteFiles[%q] from %#v", key, all)
		}
	}
	if snap["status"].(map[string]any)["Heading"] != float64(42) {
		t.Fatalf("status not mirrored: %#v", snap["status"])
	}
	if snap["market"].(map[string]any)["StationName"] != "Carrier" {
		t.Fatalf("market not mirrored: %#v", snap["market"])
	}
	future, fileName, _, found := w.EliteFile("Future_File.json")
	if !found || fileName != "Future_File.json" || future["Value"] != float64(7) {
		t.Fatalf("future lookup = found %v file %q data %#v", found, fileName, future)
	}
}

func TestEliteFileKeyKnownNames(t *testing.T) {
	cases := map[string]string{
		"Status.json":      "status",
		"ModulesInfo.json": "modulesInfo",
		"NavRoute.json":    "navRoute",
		"ShipLocker.json":  "shipLocker",
		"FCMaterials.json": "fcMaterials",
		"Future_File.json": "futureFile",
	}
	for in, want := range cases {
		if got := eliteFileKey(in); got != want {
			t.Fatalf("eliteFileKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJournalHistoryIsBoundedAndFilterable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Journal.2026-10-04T100000.01.log")
	data := "" +
		`{"timestamp":"2026-10-04T10:00:00Z","event":"Location","StarSystem":"Sol"}` + "\n" +
		`{"timestamp":"2026-10-04T10:00:01Z","event":"FSDJump","StarSystem":"Alpha Centauri"}` + "\n" +
		`{"timestamp":"2026-10-04T10:00:02Z","event":"FSDJump","StarSystem":"Barnard's Star"}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	w := New(dir, false, nil)
	files := w.JournalFiles()
	if len(files) != 1 || files[0]["file"] != filepath.Base(path) {
		t.Fatalf("files = %#v", files)
	}
	events, info, err := w.ReadJournal(filepath.Base(path), "FSDJump", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0]["StarSystem"] != "Alpha Centauri" {
		t.Fatalf("events = %#v", events)
	}
	if info["truncated"] != true {
		t.Fatalf("info = %#v", info)
	}
	if _, _, err := w.ReadJournal("../secrets.log", "", 0, 100); err == nil {
		t.Fatal("expected path traversal journal name to be rejected")
	}
}
