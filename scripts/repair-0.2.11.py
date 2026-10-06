from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def read(rel):
    return (ROOT / rel).read_text(encoding="utf-8")


def write(rel, text):
    p = ROOT / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(text, encoding="utf-8", newline="\n")
    print("wrote", rel)


def replace_exact(rel, old, new, count=1):
    text = read(rel)
    found = text.count(old)
    if found != count:
        raise SystemExit(f"{rel}: expected {count} match(es), found {found} for {old!r}")
    write(rel, text.replace(old, new, count))


def git_show(spec):
    return subprocess.check_output(["git", "show", spec], cwd=ROOT, text=True, encoding="utf-8")


# Restore the known-good schema-3 store from the 0.2.8 hotfix tag, then merge
# the SDK 9 hide-any-tab behavior and a recovery path for orphaned split files.
store = git_show("v0.2.8-alpha-hotfix:internal/customtabs/store.go")
store = store.replace(
    'var hideableDefaultNavIDs = map[string]bool{"dashboard": true, "tutorial": true, "settings": true}\n',
    '',
    1,
)
old_load_head = '''func (s *Store) load() error {
\tb, err := os.ReadFile(s.path)
\tif errors.Is(err, os.ErrNotExist) {
\t\treturn nil
\t}
\tif err != nil {
\t\treturn err
\t}
\tvar f fileData
\tif err := json.Unmarshal(b, &f); err != nil {
\t\treturn fmt.Errorf("parse custom tabs store: %w", err)
\t}
\tmigrated := f.SchemaVersion < storeSchema'''
new_load_head = '''func (s *Store) load() error {
\tvar f fileData
\tmanifestExists := true
\tb, err := os.ReadFile(s.path)
\tif errors.Is(err, os.ErrNotExist) {
\t\tmanifestExists = false
\t} else if err != nil {
\t\treturn err
\t} else if err := json.Unmarshal(b, &f); err != nil {
\t\treturn fmt.Errorf("parse custom tabs store: %w", err)
\t}
\tmigrated := manifestExists && f.SchemaVersion < storeSchema'''
if old_load_head not in store:
    raise SystemExit("schema-3 store: load head baseline not found")
store = store.replace(old_load_head, new_load_head, 1)
store = store.replace(
    'if hideableDefaultNavIDs[id] && !hiddenSeen[id] {',
    'if valid[id] && id != "tabmanager" && !hiddenSeen[id] {',
    1,
)
old_load_tail = '''\t\ts.tabs[t.ID] = t
\t}
\ts.layout = f.Layout
\ts.layout = s.normalizedLayoutLocked()
\tif migrated {'''
new_load_tail = '''\t\ts.tabs[t.ID] = t
\t}
\trecovered, err := s.recoverOrphanedBodies()
\tif err != nil {
\t\treturn err
\t}
\tif recovered > 0 {
\t\tmigrated = true
\t}
\ts.layout = f.Layout
\ts.layout = s.normalizedLayoutLocked()
\tif migrated {'''
if old_load_tail not in store:
    raise SystemExit("schema-3 store: load tail baseline not found")
store = store.replace(old_load_tail, new_load_tail, 1)
helper = r'''
// recoverOrphanedBodies repairs the 0.2.10 regression where schema-3 body
// files survived in custom-tabs/ but the in-memory/manifest view lost them.
// Manifest metadata always wins when it still exists.
func (s *Store) recoverOrphanedBodies() (int, error) {
	entries, err := os.ReadDir(s.contentDir)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".html") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !validTabID(id) {
			continue
		}
		if _, exists := s.tabs[id]; exists {
			continue
		}
		path := s.tabPath(id)
		info, err := os.Stat(path)
		if err != nil || info.Size() <= 0 || info.Size() > maxHTMLBytes {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(body)) == "" {
			continue
		}
		stamp := info.ModTime().UTC()
		if stamp.IsZero() {
			stamp = time.Now().UTC()
		}
		ts := stamp.Format(time.RFC3339Nano)
		s.tabs[id] = Tab{
			ID:        id,
			Name:      recoveredTabName(id, body),
			SizeBytes: info.Size(),
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		recovered++
	}
	return recovered, nil
}

func recoveredTabName(id string, body []byte) string {
	const maxProbe = 64 * 1024
	probe := body
	if len(probe) > maxProbe {
		probe = probe[:maxProbe]
	}
	text := string(probe)
	lower := strings.ToLower(text)
	if start := strings.Index(lower, "<title"); start >= 0 {
		if gt := strings.Index(lower[start:], ">"); gt >= 0 {
			bodyStart := start + gt + 1
			if end := strings.Index(lower[bodyStart:], "</title>"); end >= 0 {
				name := strings.Join(strings.Fields(strings.TrimSpace(text[bodyStart:bodyStart+end])), " ")
				if len(name) > 80 {
					name = name[:80]
				}
				if name != "" {
					return name
				}
			}
		}
	}
	name := "Recovered " + id
	if len(name) > 80 {
		name = name[:80]
	}
	return name
}

'''
marker = 'func (s *Store) normalizedLayoutLocked() Layout {'
if marker not in store:
    raise SystemExit("schema-3 store: normalized layout marker not found")
store = store.replace(marker, helper + marker, 1)
write("internal/customtabs/store.go", store)

# Restore schema-3 tests lost during 0.2.10 and merge SDK 9 visibility/recovery tests.
tests = git_show("v0.2.8-alpha-hotfix:internal/customtabs/store_test.go")
tests += r'''

func TestLayoutCanHideCustomTabs(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil { t.Fatal(err) }
	tab, err := s.Save("", "Hidden Tool", "<p>x</p>")
	if err != nil { t.Fatal(err) }
	navID := "custom-" + tab.ID
	layout, err := s.SaveLayout([]string{"dashboard", navID, "tabmanager", "tutorial", "settings"}, []string{navID, "settings", "tabmanager"})
	if err != nil { t.Fatal(err) }
	if len(layout.HiddenDefaults) != 2 || layout.HiddenDefaults[0] != navID || layout.HiddenDefaults[1] != "settings" {
		t.Fatalf("unexpected hidden navigation: %#v", layout.HiddenDefaults)
	}
	s2, err := New(dir)
	if err != nil { t.Fatal(err) }
	found := false
	for _, id := range s2.Layout().HiddenDefaults {
		if id == navID { found = true }
		if id == "tabmanager" { t.Fatal("Tab Manager must never be hideable") }
	}
	if !found { t.Fatalf("hidden custom tab did not persist: %#v", s2.Layout().HiddenDefaults) }
}

func TestStoreRecoversOrphanedSchema3Bodies(t *testing.T) {
	dir := t.TempDir()
	content := filepath.Join(dir, "custom-tabs")
	if err := os.MkdirAll(content, 0o700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(dir, "custom-tabs.json"), []byte(`{"schemaVersion":2,"tabs":[],"layout":{}}`), 0o600); err != nil { t.Fatal(err) }
	body := "<!doctype html><html><head><title>My Mining Tab</title></head><body>x</body></html>"
	if err := os.WriteFile(filepath.Join(content, "tab-orphan.html"), []byte(body), 0o600); err != nil { t.Fatal(err) }
	s, err := New(dir)
	if err != nil { t.Fatal(err) }
	list := s.List()
	if len(list) != 1 || list[0].ID != "tab-orphan" || list[0].Name != "My Mining Tab" {
		t.Fatalf("orphan not recovered: %#v", list)
	}
	got, ok := s.Get("tab-orphan")
	if !ok || got.HTML != body { t.Fatalf("orphan body not readable: %#v ok=%v", got, ok) }
	manifest, err := os.ReadFile(filepath.Join(dir, "custom-tabs.json"))
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(manifest), `"schemaVersion": 3`) || !strings.Contains(string(manifest), `"tab-orphan"`) {
		t.Fatalf("recovered manifest not persisted: %s", manifest)
	}
}
'''
write("internal/customtabs/store_test.go", tests)

# Bump the build so 0.2.10 actually sees a newer release.
replace_exact("internal/buildinfo/version.go", 'Version    = "0.2.10-alpha"', 'Version    = "0.2.11-alpha"')
replace_exact("internal/buildinfo/version.go", 'Display    = "Alpha 0.2.10 SDK9 Recovery"', 'Display    = "Alpha 0.2.11 SDK9"')
p = ROOT / "internal/buildinfo/version_test.go"
if p.exists():
    p.write_text(p.read_text(encoding="utf-8").replace("0.2.10-alpha", "0.2.11-alpha"), encoding="utf-8", newline="\n")

# Move the default/documented service port to 6626 throughout source/docs.
text_exts = {".go", ".js", ".md", ".html", ".json", ".txt", ".ps1", ".sh"}
for p in ROOT.rglob("*"):
    if not p.is_file() or ".git" in p.parts or p.suffix.lower() not in text_exts:
        continue
    try:
        text = p.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        continue
    if "4510" in text:
        p.write_text(text.replace("4510", "6626"), encoding="utf-8", newline="\n")
        print("port migrated", p.relative_to(ROOT))

# New launcher uses 6626 but recognizes an already-running legacy 4510 instance.
main_rel = "cmd/jacob/main.go"
main = read(main_rel)
old = 'const localURL = "http://127.0.0.1:6626/"'
new = 'const (\n\tlocalURL       = "http://127.0.0.1:6626/"\n\tlegacyLocalURL = "http://127.0.0.1:4510/"\n)'
if old not in main: raise SystemExit("cmd/jacob/main.go: localURL baseline did not match")
main = main.replace(old, new, 1)
old = 'if runningJACoB() {\n\t\topenBrowser(localURL)\n\t\treturn\n\t}'
new = 'if runningURL := runningJACoB(); runningURL != "" {\n\t\topenBrowser(runningURL)\n\t\treturn\n\t}'
if old not in main: raise SystemExit("cmd/jacob/main.go: startup probe baseline did not match")
main = main.replace(old, new, 1)
old_fn = '''func runningJACoB() bool {
\tclient := http.Client{Timeout: 250 * time.Millisecond}
\tr, err := client.Get(localURL + "api/health")
\tif err != nil {
\t\treturn false
\t}
\tdefer r.Body.Close()
\tif r.StatusCode != 200 {
\t\treturn false
\t}
\tvar v map[string]any
\tif json.NewDecoder(r.Body).Decode(&v) != nil {
\t\treturn false
\t}
\treturn strings.EqualFold(asString(v["product"]), "JACoB")
}'''
new_fn = '''func runningJACoB() string {
\tfor _, url := range []string{localURL, legacyLocalURL} {
\t\tif probeJACoB(url) {
\t\t\treturn url
\t\t}
\t}
\treturn ""
}

func probeJACoB(url string) bool {
\tclient := http.Client{Timeout: 250 * time.Millisecond}
\tr, err := client.Get(url + "api/health")
\tif err != nil {
\t\treturn false
\t}
\tdefer r.Body.Close()
\tif r.StatusCode != 200 {
\t\treturn false
\t}
\tvar v map[string]any
\tif json.NewDecoder(r.Body).Decode(&v) != nil {
\t\treturn false
\t}
\treturn strings.EqualFold(asString(v["product"]), "JACoB")
}'''
if old_fn not in main: raise SystemExit("cmd/jacob/main.go: runningJACoB baseline did not match")
write(main_rel, main.replace(old_fn, new_fn, 1))

# Prefer the canonical Windows platform filename, with old names retained as fallback.
up_rel = "internal/updater/updater.go"
up = read(up_rel)
old = '''\t\t\tif strings.Contains(n, "portable") || strings.Contains(n, "no-recorder") {
\t\t\t\treturn false
\t\t\t}
\t\t\treturn strings.Contains(n, "setup") || strings.Contains(n, "alpha-")'''
new = '''\t\t\tif strings.Contains(n, "portable") || strings.Contains(n, "no-recorder") {
\t\t\t\treturn false
\t\t\t}
\t\t\tif strings.Contains(n, "win-x64") {
\t\t\t\treturn runtime.GOARCH == "amd64"
\t\t\t}
\t\t\treturn strings.Contains(n, "setup") || strings.Contains(n, "alpha-")'''
if old not in up: raise SystemExit("internal/updater/updater.go: matcher baseline did not match")
write(up_rel, up.replace(old, new, 1))

ut_rel = "internal/updater/updater_test.go"
ut = read(ut_rel)
if "TestWindowsCanonicalInstallerNameIsRecognized" not in ut:
    ut += '''
func TestWindowsCanonicalInstallerNameIsRecognized(t *testing.T) {
\tif runtime.GOOS != "windows" { t.Skip("selector follows current runtime") }
\tassets := []Asset{
\t\t{Name: "JACoB-0.2.11-alpha-win-x64-portable.exe"},
\t\t{Name: "JACoB-0.2.11-alpha-win-x64-no-recorder.exe"},
\t\t{Name: "JACoB-0.2.11-alpha-win-x64.exe"},
\t}
\ta := selectAsset(assets)
\tif a == nil || a.Name != "JACoB-0.2.11-alpha-win-x64.exe" { t.Fatalf("canonical installer name not selected: %#v", a) }
}
'''
write(ut_rel, ut)

# Update current-version labels without rewriting historical 0.2.10 release notes.
for rel in [
    "README.md",
    "internal/webui/assets/app.js",
    "docs/JACOB_CUSTOM_TAB_REFERENCE.md",
    "internal/webui/assets/docs/JACOB_CUSTOM_TAB_REFERENCE.md",
    "cmd/installer/payload/Documentation/JACoB-Custom-Tab-Developer-Reference.md",
    "internal/webui/assets/docs/developer-reference.html",
    "cmd/installer/payload/Documentation/JACoB-Custom-Tab-Developer-Reference.html",
]:
    p = ROOT / rel
    if not p.exists(): continue
    text = p.read_text(encoding="utf-8").replace("Alpha 0.2.10", "Alpha 0.2.11").replace("0.2.10-alpha", "0.2.11-alpha")
    p.write_text(text, encoding="utf-8", newline="\n")

# Restore schema-3 persistence documentation in all Markdown copies.
md_section = '''### 2.3 Persistence

Saved tabs are stored by the JACoB core, not browser local storage. On Windows the default data directory is normally:

```text
%APPDATA%\\JACoB
```

Saved-tab metadata and navigation order are stored in:

```text
custom-tabs.json
```

Each tab body is stored separately under:

```text
custom-tabs/<tab-id>.html
```

The manifest therefore stays small even when individual tabs contain large datasets or complex UI. `tabs.list` returns metadata only; JACoB loads a tab's HTML on demand with `tabs.get` when the tab is opened, edited, or loaded for a registered background action. Existing schema-2 stores with inline HTML are migrated automatically without changing tab IDs or per-tab `Elite.store` state. JACoB 0.2.11 also recovers surviving schema-3 body files if the 0.2.10 regression left them orphaned from the manifest.

A saved tab metadata record contains:

```json
{
  "id": "tab-0123456789abcdef",
  "name": "Route Queue",
  "sizeBytes": 182304,
  "createdAt": "2026-10-03T16:00:00Z",
  "updatedAt": "2026-10-03T16:10:00Z"
}
```

Maximum saved HTML size is 4 MiB per tab.

### 2.4 Overlay isolation'''
for rel in [
    "docs/JACOB_CUSTOM_TAB_REFERENCE.md",
    "internal/webui/assets/docs/JACOB_CUSTOM_TAB_REFERENCE.md",
    "cmd/installer/payload/Documentation/JACoB-Custom-Tab-Developer-Reference.md",
]:
    text = read(rel)
    text, n = re.subn(r"### 2\.3 Persistence[\s\S]*?### 2\.4 Overlay isolation", lambda _m: md_section, text, count=1)
    if n != 1: raise SystemExit(f"{rel}: persistence docs block not found")
    write(rel, text)

html_section = '''<h3 id="persistence">2.3 Persistence</h3>
<p>Saved tabs are stored by the JACoB core, not browser local storage. On Windows the default data directory is normally:</p>
<pre class="text"><code>%APPDATA%\\JACoB</code></pre>
<p>Saved-tab metadata and navigation order are stored in <code>custom-tabs.json</code>.</p>
<p>Each tab body is stored separately under <code>custom-tabs/&lt;tab-id&gt;.html</code>. The manifest stays small, <code>tabs.list</code> returns metadata only, and JACoB loads HTML on demand with <code>tabs.get</code>. Schema-2 inline stores migrate automatically. JACoB 0.2.11 also recovers surviving schema-3 body files orphaned by the 0.2.10 regression.</p>
<p>A metadata record contains <code>id</code>, <code>name</code>, <code>sizeBytes</code>, <code>createdAt</code>, and <code>updatedAt</code>. Maximum saved HTML size is 4 MiB per tab.</p>
<h3 id="overlay-isolation">2.4 Overlay isolation</h3>'''
for rel in [
    "internal/webui/assets/docs/developer-reference.html",
    "cmd/installer/payload/Documentation/JACoB-Custom-Tab-Developer-Reference.html",
]:
    text = read(rel)
    # Pandoc-generated heading IDs have changed across documentation builds
    # (for example overlay-isolation vs 24-overlay-isolation). Match the
    # visible section headings rather than one generated ID spelling.
    pattern = r'<h3 id="[^"]*persistence[^"]*">2\.3 Persistence</h3>[\s\S]*?<h3 id="[^"]*overlay-isolation[^"]*">2\.4 Overlay isolation</h3>'
    text, n = re.subn(pattern, lambda _m: html_section, text, count=1)
    if n != 1:
        raise SystemExit(f"{rel}: generated persistence docs block not found")

    # Some checked-in generated references predate the Markdown source and
    # still show Elite.api.version === 6. Keep that displayed example aligned
    # with the SDK 9 source while we are repairing the generated copy.
    sdk_pattern = r'(<p>The current SDK version is:</p>[\s\S]*?<span class="at">version</span>[\s\S]*?<span class="op">===</span>\s*<span class="dv">)\d+(</span>)'
    text, sdk_n = re.subn(sdk_pattern, lambda m: m.group(1) + "9" + m.group(2), text, count=1)
    if sdk_n != 1:
        raise SystemExit(f"{rel}: generated SDK version example not found")
    write(rel, text)

for rel in ["docs/USER_GUIDE.md", "cmd/installer/payload/Documentation/JACoB-User-Guide.md"]:
    p = ROOT / rel
    if not p.exists(): continue
    text = p.read_text(encoding="utf-8")
    text = text.replace("Home, Tutorial and Settings may be hidden from the navigation bar.", "Any default or custom tab may be hidden from the navigation bar.")
    text = text.replace("Hidden default pages keep their settings and may be restored at any time.", "Hidden tabs remain installed; hidden custom tabs may keep running in the background and can be restored at any time.")
    p.write_text(text, encoding="utf-8", newline="\n")

write("RELEASE-NOTES-0.2.11-alpha.md", '''# JACoB 0.2.11 Alpha — Tab Recovery + Port 6626

- Restores the schema-3 custom-tab store used by 0.2.8: metadata in `custom-tabs.json`, HTML bodies in `custom-tabs/<id>.html`, lazy `tabs.get`, 4 MiB per-tab limit, atomic writes, and ID/path validation.
- Recovers surviving split HTML files if the 0.2.10 regression left them orphaned from the manifest. Existing metadata is preserved whenever it is still present; otherwise a title is recovered from the HTML when possible.
- Retains SDK 9 cross-tab actions and the ability to hide any custom/default navigation tab except Tab Manager.
- Moves the default JACoB service port from 4510 to 6626. The launcher can still detect an already-running legacy 4510 instance, but new instances bind 6626.
- Updates the Windows updater matcher for canonical `*-win-x64.exe` installer names while preserving compatibility with older alpha/setup names.
- Restores schema-3 regression tests and corrects the SDK documentation.

Before installing this build after 0.2.10, back up `%APPDATA%\\JACoB` if possible. The repair is designed to recover the existing split tab bodies in place.
''')

print("JACoB 0.2.11 repair patch prepared successfully.")
