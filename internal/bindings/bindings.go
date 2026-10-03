package bindings

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type KeyRef struct {
	Device string `json:"device"`
	Key    string `json:"key"`
}

type Slot struct {
	Device    string   `json:"device"`
	Key       string   `json:"key"`
	Modifiers []KeyRef `json:"modifiers,omitempty"`
}

type Action struct {
	Name      string `json:"name"`
	Primary   Slot   `json:"primary"`
	Secondary Slot   `json:"secondary"`
}

type FileInfo struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	PresetName  string    `json:"presetName,omitempty"`
	Source      string    `json:"source,omitempty"`
	Modified    time.Time `json:"modified"`
	ActionCount int       `json:"actionCount"`
}

type Assignment struct {
	Action    string   `json:"action"`
	Key       string   `json:"key"`
	Modifiers []string `json:"modifiers,omitempty"`
}

type CloneReport struct {
	SourceFile   string `json:"sourceFile"`
	SourcePreset string `json:"sourcePreset"`
	TargetFile   string `json:"targetFile"`
	TargetPreset string `json:"targetPreset"`
	Assigned     int    `json:"assigned"`
}

type AutoBindReport struct {
	ActiveFile       string        `json:"activeFile,omitempty"`
	ScannedActions   int           `json:"scannedActions"`
	AlreadyBound     int           `json:"alreadyBound"`
	ExistingFallback int           `json:"existingFallback"`
	Assigned         int           `json:"assigned"`
	BackupPath       string        `json:"backupPath,omitempty"`
	Assignments      []Assignment  `json:"assignments,omitempty"`
	SourceFiles      []string      `json:"sourceFiles,omitempty"`
	CreatedFiles     []string      `json:"createdFiles,omitempty"`
	SelectorBackups  []string      `json:"selectorBackups,omitempty"`
	Clones           []CloneReport `json:"clones,omitempty"`
}

type LoaderDiagnostics struct {
	Path              string    `json:"path,omitempty"`
	Modified          time.Time `json:"modified,omitempty"`
	Relevant          bool      `json:"relevant"`
	HasErrors         bool      `json:"hasErrors"`
	PresetFiles       []string  `json:"presetFiles,omitempty"`
	MissingDevices    []string  `json:"missingDevices,omitempty"`
	DuplicateBindings []string  `json:"duplicateBindings,omitempty"`
	Lines             []string  `json:"lines,omitempty"`
}

type Diagnostics struct {
	Directory         string            `json:"directory"`
	ActiveFile        string            `json:"activeFile,omitempty"`
	ActiveFiles       []string          `json:"activeFiles,omitempty"`
	ActiveSource      string            `json:"activeSource,omitempty"`
	SelectorLegacy    []string          `json:"selectorLegacy,omitempty"`
	SelectorOdyssey   []string          `json:"selectorOdyssey,omitempty"`
	RequiredDevices   []string          `json:"requiredDevices,omitempty"`
	DuplicateElements []string          `json:"duplicateElements,omitempty"`
	ParseError        string            `json:"parseError,omitempty"`
	ReadOnly          bool              `json:"readOnly"`
	Loader            LoaderDiagnostics `json:"loader"`
}

func IsKnownNonFatalDuplicateBinding(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "MouseGUI")
}

func FilterBlockingDuplicateBindings(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if !IsKnownNonFatalDuplicateBinding(name) {
			out = append(out, name)
		}
	}
	return out
}

func FilterNonFatalDuplicateBindings(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if IsKnownNonFatalDuplicateBinding(name) {
			out = append(out, name)
		}
	}
	return out
}

type Store struct {
	mu           sync.RWMutex
	dir          string
	active       string
	files        []FileInfo
	actions      map[string]Action
	autoBind     AutoBindReport
	activeFiles  []string
	activeSource string
	controlDirs  []string
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func DetectDirectory() (string, []string) {
	var candidates []string
	if v := firstEnv("JACOB_BINDINGS_DIR", "EDBRIDGE_BINDINGS_DIR"); v != "" {
		candidates = append(candidates, v)
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = append(candidates, filepath.Join(local, "Frontier Developments", "Elite Dangerous", "Options", "Bindings"))
		}
		if home != "" {
			candidates = append(candidates, filepath.Join(home, "AppData", "Local", "Frontier Developments", "Elite Dangerous", "Options", "Bindings"))
		}
	} else if runtime.GOOS == "linux" && home != "" {
		roots := []string{
			filepath.Join(home, ".local", "share", "Steam"),
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".steam", "debian-installation"),
		}
		for _, root := range roots {
			candidates = append(candidates, filepath.Join(root, "steamapps", "compatdata", "359320", "pfx", "drive_c", "users", "steamuser", "AppData", "Local", "Frontier Developments", "Elite Dangerous", "Options", "Bindings"))
		}
	}
	for _, c := range unique(candidates) {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c, unique(candidates)
		}
	}
	return "", unique(candidates)
}

func New(dir string) *Store {
	s := &Store{dir: dir, actions: map[string]Action{}, controlDirs: DetectControlSchemeDirectories()}
	_ = s.Reload()
	return s
}

func (s *Store) Directory() string  { s.mu.RLock(); defer s.mu.RUnlock(); return s.dir }
func (s *Store) ActiveFile() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.active }
func (s *Store) ActiveFiles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.activeFiles))
	copy(out, s.activeFiles)
	return out
}
func (s *Store) ActiveSource() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.activeSource }
func (s *Store) LastAutoBindReport() AutoBindReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.autoBind
}

func selectorNames(dir, filename string) []string {
	if dir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return nil
	}
	out := []string{}
	for _, line := range strings.Split(string(b), "\n") {
		v := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func activePresetNames(dir string) []string {
	odyssey := selectorNames(dir, "StartPreset.4.start")
	if len(odyssey) > 0 {
		return unique(odyssey)
	}
	return unique(selectorNames(dir, "StartPreset.start"))
}

func steamLibraryRoots() []string {
	roots := []string{}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
			roots = append(roots, filepath.Join(pf, "Steam"))
		}
	} else if home != "" {
		roots = append(roots,
			filepath.Join(home, ".local", "share", "Steam"),
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".steam", "debian-installation"),
		)
	}
	pathRE := regexp.MustCompile(`(?m)"path"\s*"([^"]+)"`)
	for _, root := range append([]string(nil), roots...) {
		b, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		for _, m := range pathRE.FindAllStringSubmatch(string(b), -1) {
			if len(m) < 2 {
				continue
			}
			v := strings.ReplaceAll(m[1], "\\\\", "\\")
			roots = append(roots, v)
		}
	}
	return unique(roots)
}

func DetectControlSchemeDirectories() []string {
	candidates := []string{}
	if v := firstEnv("JACOB_CONTROL_SCHEMES_DIR", "EDBRIDGE_CONTROL_SCHEMES_DIR"); v != "" {
		candidates = append(candidates, v)
	}
	for _, root := range steamLibraryRoots() {
		candidates = append(candidates,
			filepath.Join(root, "steamapps", "common", "Elite Dangerous", "Products", "elite-dangerous-odyssey-64", "ControlSchemes"),
			filepath.Join(root, "steamapps", "common", "Elite Dangerous", "Products", "elite-dangerous-64", "ControlSchemes"),
		)
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = append(candidates,
				filepath.Join(local, "Frontier_Developments", "Products", "elite-dangerous-odyssey-64", "ControlSchemes"),
				filepath.Join(local, "Frontier_Developments", "Products", "elite-dangerous-64", "ControlSchemes"),
			)
		}
	}
	out := []string{}
	for _, c := range unique(candidates) {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

type parsedPreset struct {
	info    FileInfo
	actions map[string]Action
}

func (s *Store) Reload() error {
	if s.dir == "" {
		return nil
	}
	presets := []parsedPreset{}
	loadDir := func(dir, source string) {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.binds"))
		sort.Slice(matches, func(i, j int) bool {
			leftInfo, _ := os.Stat(matches[i])
			rightInfo, _ := os.Stat(matches[j])
			if leftInfo == nil {
				return false
			}
			if rightInfo == nil {
				return true
			}
			return leftInfo.ModTime().After(rightInfo.ModTime())
		})
		for _, p := range matches {
			a, preset, err := parseFile(p)
			if err != nil {
				continue
			}
			st, _ := os.Stat(p)
			info := FileInfo{Path: p, Name: filepath.Base(p), PresetName: preset, Source: source, ActionCount: len(a)}
			if st != nil {
				info.Modified = st.ModTime()
			}
			presets = append(presets, parsedPreset{info: info, actions: a})
		}
	}
	loadDir(s.dir, "user")
	for _, d := range s.controlDirs {
		loadDir(d, "builtin")
	}

	activeNames := activePresetNames(s.dir)
	selected := []parsedPreset{}
	seenPaths := map[string]bool{}
	for _, name := range activeNames {
		var best *parsedPreset
		for i := range presets {
			p := &presets[i]
			if p.info.PresetName != name {
				continue
			}
			if best == nil || (p.info.Source == "user" && best.info.Source != "user") {
				best = p
			}
		}
		if best != nil && !seenPaths[best.info.Path] {
			selected = append(selected, *best)
			seenPaths[best.info.Path] = true
		}
	}
	if len(selected) == 0 {
		for _, p := range presets {
			if p.info.Source == "user" {
				selected = append(selected, p)
				break
			}
		}
		if len(selected) == 0 {
			for _, p := range presets {
				if p.info.PresetName == "KeyboardMouseOnly" {
					selected = append(selected, p)
					break
				}
			}
		}
	}

	actions := map[string]Action{}
	activeFiles := []string{}
	activeSource := ""
	active := ""
	for _, p := range selected {
		if active == "" {
			active = p.info.Path
			activeSource = p.info.Source
		}
		activeFiles = append(activeFiles, p.info.Path)
		for name, a := range p.actions {
			if _, exists := actions[name]; !exists {
				actions[name] = a
			}
		}
	}
	infos := make([]FileInfo, 0, len(presets))
	for _, p := range presets {
		infos = append(infos, p.info)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = infos
	s.active = active
	s.activeFiles = activeFiles
	s.activeSource = activeSource
	s.actions = actions
	return nil
}

func (s *Store) ListFiles() []FileInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FileInfo, len(s.files))
	copy(out, s.files)
	return out
}
func (s *Store) ListActions() []Action {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Action, 0, len(s.actions))
	for _, a := range s.actions {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (s *Store) Get(name string) (Action, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.actions[name]
	return a, ok
}

func slotBound(s Slot) bool {
	d := strings.TrimSpace(s.Device)
	return strings.TrimSpace(s.Key) != "" && d != "" && !strings.EqualFold(d, "{NoDevice}") && !strings.EqualFold(d, "NoDevice")
}

func parseFile(path string) (map[string]Action, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	actions := map[string]Action{}
	preset := ""
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "Root" {
			for _, a := range se.Attr {
				if a.Name.Local == "PresetName" {
					preset = a.Value
				}
			}
			continue
		}
		type rawSlot struct {
			Device    string `xml:"Device,attr"`
			Key       string `xml:"Key,attr"`
			Modifiers []struct {
				Device string `xml:"Device,attr"`
				Key    string `xml:"Key,attr"`
			} `xml:"Modifier"`
		}
		var node struct {
			Primary   *rawSlot `xml:"Primary"`
			Secondary *rawSlot `xml:"Secondary"`
		}
		if err := dec.DecodeElement(&node, &se); err != nil {
			return nil, "", err
		}
		if node.Primary == nil || node.Secondary == nil {
			continue
		}
		cvt := func(in *rawSlot) Slot {
			if in == nil {
				return Slot{}
			}
			out := Slot{Device: in.Device, Key: in.Key}
			for _, m := range in.Modifiers {
				out.Modifiers = append(out.Modifiers, KeyRef{Device: m.Device, Key: m.Key})
			}
			return out
		}
		actions[se.Name.Local] = Action{Name: se.Name.Local, Primary: cvt(node.Primary), Secondary: cvt(node.Secondary)}
	}
	if len(actions) == 0 {
		return nil, preset, fmt.Errorf("no discrete bindings parsed from %s", path)
	}
	return actions, preset, nil
}

type fileInspection struct {
	RequiredDevices   []string
	DuplicateElements []string
}

func inspectBindingFile(path string) (fileInspection, error) {
	f, err := os.Open(path)
	if err != nil {
		return fileInspection{}, err
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	depth := 0
	counts := map[string]int{}
	devices := map[string]bool{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fileInspection{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				counts[t.Name.Local]++
			}
			for _, a := range t.Attr {
				if a.Name.Local != "Device" {
					continue
				}
				v := strings.TrimSpace(a.Value)
				if v == "" || strings.EqualFold(v, "{NoDevice}") || strings.EqualFold(v, "NoDevice") {
					continue
				}
				devices[v] = true
			}
			depth++
		case xml.EndElement:
			if depth > 0 {
				depth--
			}
		}
	}
	out := fileInspection{}
	for d := range devices {
		out.RequiredDevices = append(out.RequiredDevices, d)
	}
	for name, n := range counts {
		if n > 1 {
			out.DuplicateElements = append(out.DuplicateElements, name)
		}
	}
	sort.Strings(out.RequiredDevices)
	sort.Strings(out.DuplicateElements)
	return out, nil
}

func loadLoaderDiagnostics(dir, active string) LoaderDiagnostics {
	out := LoaderDiagnostics{}
	if dir == "" {
		return out
	}
	path := filepath.Join(dir, "BindingLoadingErrors.log")
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	out.Path = path
	if st, err := os.Stat(path); err == nil {
		out.Modified = st.ModTime()
	}
	presetRE := regexp.MustCompile(`(?i)preset file:\s*([^\r\n]+)`)
	missingRE := regexp.MustCompile(`(?i)Missing devices:\s*([^\r\n]+)`)
	dupRE := regexp.MustCompile(`(?i)multiple entries of binding\s+"([^"]+)"`)
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		out.Lines = append(out.Lines, line)
		if m := presetRE.FindStringSubmatch(line); len(m) > 1 {
			out.PresetFiles = append(out.PresetFiles, strings.TrimSpace(m[1]))
		}
		if m := missingRE.FindStringSubmatch(line); len(m) > 1 {
			for _, d := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ';' }) {
				if v := strings.TrimSpace(d); v != "" {
					out.MissingDevices = append(out.MissingDevices, v)
				}
			}
		}
		if m := dupRE.FindStringSubmatch(line); len(m) > 1 {
			out.DuplicateBindings = append(out.DuplicateBindings, strings.TrimSpace(m[1]))
		}
	}
	out.PresetFiles = unique(out.PresetFiles)
	out.MissingDevices = unique(out.MissingDevices)
	out.DuplicateBindings = unique(out.DuplicateBindings)
	out.HasErrors = len(out.MissingDevices) > 0 || len(out.DuplicateBindings) > 0 || len(out.PresetFiles) > 0
	if active == "" {
		out.Relevant = false
		return out
	}
	activeBase := filepath.Base(active)
	nameMatch := len(out.PresetFiles) == 0
	for _, p := range out.PresetFiles {
		if strings.EqualFold(filepath.Base(p), activeBase) {
			nameMatch = true
			break
		}
	}
	fresh := true
	if st, err := os.Stat(active); err == nil && !out.Modified.IsZero() {
		fresh = !out.Modified.Before(st.ModTime().Add(-2 * time.Second))
	}
	out.Relevant = nameMatch && fresh
	return out
}

func (s *Store) Diagnostics() Diagnostics {
	s.mu.RLock()
	dir := s.dir
	active := s.active
	activeFiles := append([]string(nil), s.activeFiles...)
	source := s.activeSource
	s.mu.RUnlock()

	d := Diagnostics{
		Directory:       dir,
		ActiveFile:      active,
		ActiveFiles:     activeFiles,
		ActiveSource:    source,
		SelectorLegacy:  selectorNames(dir, "StartPreset.start"),
		SelectorOdyssey: selectorNames(dir, "StartPreset.4.start"),
		ReadOnly:        source == "builtin",
		Loader:          loadLoaderDiagnostics(dir, active),
	}
	if active != "" {
		inspection, err := inspectBindingFile(active)
		if err != nil {
			d.ParseError = err.Error()
		} else {
			d.RequiredDevices = inspection.RequiredDevices
			d.DuplicateElements = inspection.DuplicateElements
		}
	}
	return d
}

func (s *Store) autofillSafetyCheck() error {
	d := s.Diagnostics()
	if len(d.ActiveFiles) == 0 {
		return fmt.Errorf("no active Elite bindings preset is available")
	}
	for _, path := range d.ActiveFiles {
		if _, _, err := parseFile(path); err != nil {
			return fmt.Errorf("active preset %s does not parse cleanly: %w", filepath.Base(path), err)
		}
		inspection, err := inspectBindingFile(path)
		if err != nil {
			return fmt.Errorf("inspect active preset %s: %w", filepath.Base(path), err)
		}
		blockingDuplicates := FilterBlockingDuplicateBindings(inspection.DuplicateElements)
		if len(blockingDuplicates) > 0 {
			return fmt.Errorf("active preset %s contains duplicate binding elements (%s); refusing to clone malformed bindings", filepath.Base(path), strings.Join(blockingDuplicates, ", "))
		}
	}
	if d.Loader.Relevant {
		if len(d.Loader.MissingDevices) > 0 {
			return fmt.Errorf("Elite reported missing input devices for the active preset (%s); resolve them before creating a JACoB clone", strings.Join(d.Loader.MissingDevices, ", "))
		}
		blockingLoaderDuplicates := FilterBlockingDuplicateBindings(d.Loader.DuplicateBindings)
		if len(blockingLoaderDuplicates) > 0 {
			return fmt.Errorf("Elite reported duplicate bindings for the active preset (%s); resolve them before creating a JACoB clone", strings.Join(blockingLoaderDuplicates, ", "))
		}
	}
	return nil
}

type presetClonePlan struct {
	sourcePath   string
	sourcePreset string
	targetPath   string
	targetPreset string
	major        string
	minor        string
	actions      map[string]Action
	assignments  map[string]Assignment
	original     []byte
	updated      []byte
}

func readRootMeta(data []byte) (preset, major, minor string, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, e := dec.Token()
		if e == io.EOF {
			return "", "", "", fmt.Errorf("Root element not found")
		}
		if e != nil {
			return "", "", "", e
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "Root" {
			continue
		}
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "PresetName":
				preset = a.Value
			case "MajorVersion":
				major = a.Value
			case "MinorVersion":
				minor = a.Value
			}
		}
		return preset, major, minor, nil
	}
}

func inferredPresetVersion(path, major, minor string, odyssey bool) (string, string) {
	if major != "" && minor != "" {
		return major, minor
	}
	re := regexp.MustCompile(`(?i)\.([0-9]+)\.([0-9]+)\.binds$`)
	if m := re.FindStringSubmatch(filepath.Base(path)); len(m) == 3 {
		return m[1], m[2]
	}
	if odyssey {
		return "4", "2"
	}
	return "4", "0"
}

func escapeXMLAttr(v string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(v))
	return strings.ReplaceAll(b.String(), `"`, "&#34;")
}

func setRootAttribute(tag, name, value string) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `="[^"]*"`)
	repl := name + `="` + escapeXMLAttr(value) + `"`
	if re.MatchString(tag) {
		return re.ReplaceAllString(tag, repl)
	}
	idx := strings.LastIndex(tag, ">")
	if idx < 0 {
		return tag
	}
	return tag[:idx] + " " + repl + tag[idx:]
}

func renamePresetBytes(src []byte, preset, major, minor string) ([]byte, error) {
	re := regexp.MustCompile(`(?s)<Root\b[^>]*>`)
	loc := re.FindIndex(src)
	if loc == nil {
		return nil, fmt.Errorf("Root opening element not found")
	}
	tag := string(src[loc[0]:loc[1]])
	tag = setRootAttribute(tag, "PresetName", preset)
	tag = setRootAttribute(tag, "MajorVersion", major)
	tag = setRootAttribute(tag, "MinorVersion", minor)
	out := make([]byte, 0, len(src)+len(tag)-(loc[1]-loc[0]))
	out = append(out, src[:loc[0]]...)
	out = append(out, []byte(tag)...)
	out = append(out, src[loc[1]:]...)
	return out, nil
}

func sanitizePresetFilename(v string) string {
	bad := regexp.MustCompile(`[<>:"/\\|?*]+`)
	v = strings.TrimSpace(bad.ReplaceAllString(v, "_"))
	v = strings.Trim(v, ". ")
	if v == "" {
		return "JACoB"
	}
	return v
}

func (s *Store) uniqueCloneTarget(sourcePreset, major, minor string) (string, string) {
	base := "JACoB - " + strings.TrimSpace(sourcePreset)
	if sourcePreset == "" {
		base = "JACoB Custom"
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(sourcePreset)), "jacob -") {
		base = strings.TrimSpace(sourcePreset) + " Copy"
	}

	existingNames := map[string]bool{}
	for _, f := range s.ListFiles() {
		existingNames[strings.ToLower(f.PresetName)] = true
	}
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s %d", base, n)
		}
		file := sanitizePresetFilename(name) + "." + major + "." + minor + ".binds"
		path := filepath.Join(s.dir, file)
		if existingNames[strings.ToLower(name)] {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			continue
		}
		return name, path
	}
}

func replaceSelectorNames(src []byte, mapping map[string]string) ([]byte, bool) {
	text := string(src)
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	trailing := strings.HasSuffix(text, "\n")
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(norm, "\n")
	if trailing && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	changed := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if target, ok := mapping[trim]; ok {
			prefixLen := len(line) - len(strings.TrimLeft(line, " \t"))
			suffixLen := len(line) - len(strings.TrimRight(line, " \t"))
			prefix := line[:prefixLen]
			suffix := ""
			if suffixLen > 0 {
				suffix = line[len(line)-suffixLen:]
			}
			lines[i] = prefix + target + suffix
			changed = true
		}
	}
	out := strings.Join(lines, newline)
	if trailing {
		out += newline
	}
	return []byte(out), changed
}

func validateCloneBindings(original, cloned []byte, targetPreset string, assignments map[string]Assignment) error {
	beforeActions, _, err := parseBytes(original)
	if err != nil {
		return fmt.Errorf("source bindings validation failed: %w", err)
	}
	afterActions, afterPreset, err := parseBytes(cloned)
	if err != nil {
		return fmt.Errorf("clone bindings validation failed: %w", err)
	}
	if afterPreset != targetPreset {
		return fmt.Errorf("clone preset name=%q want %q", afterPreset, targetPreset)
	}
	if len(beforeActions) != len(afterActions) {
		return fmt.Errorf("action count changed from %d to %d", len(beforeActions), len(afterActions))
	}
	for name, before := range beforeActions {
		after, ok := afterActions[name]
		if !ok {
			return fmt.Errorf("action %s disappeared from clone", name)
		}
		if !slotsEqual(before.Primary, after.Primary) {
			return fmt.Errorf("primary binding for %s changed unexpectedly", name)
		}
		if _, assigned := assignments[name]; !assigned && !slotsEqual(before.Secondary, after.Secondary) {
			return fmt.Errorf("secondary binding for %s changed unexpectedly", name)
		}
	}
	return nil
}

func (s *Store) EnsureMissingPrimaryBindings() (AutoBindReport, error) {
	if err := s.Reload(); err != nil {
		return AutoBindReport{}, err
	}
	if err := s.autofillSafetyCheck(); err != nil {
		return AutoBindReport{}, err
	}

	s.mu.RLock()
	sourcePaths := append([]string(nil), s.activeFiles...)
	dir := s.dir
	s.mu.RUnlock()
	if len(sourcePaths) == 0 {
		return AutoBindReport{}, fmt.Errorf("no active bindings files to clone")
	}

	odysseySelector := selectorNames(dir, "StartPreset.4.start")
	isOdyssey := len(odysseySelector) > 0
	report := AutoBindReport{SourceFiles: append([]string(nil), sourcePaths...)}

	plans := make([]*presetClonePlan, 0, len(sourcePaths))
	used := map[string]bool{}
	allActionNames := map[string]bool{}

	for _, sourcePath := range sourcePaths {
		original, err := os.ReadFile(sourcePath)
		if err != nil {
			return report, fmt.Errorf("read source preset %s: %w", filepath.Base(sourcePath), err)
		}
		actions, parsedPreset, err := parseBytes(original)
		if err != nil {
			return report, fmt.Errorf("parse source preset %s: %w", filepath.Base(sourcePath), err)
		}
		metaPreset, major, minor, err := readRootMeta(original)
		if err != nil {
			return report, fmt.Errorf("read source metadata %s: %w", filepath.Base(sourcePath), err)
		}
		sourcePreset := parsedPreset
		if sourcePreset == "" {
			sourcePreset = metaPreset
		}
		if sourcePreset == "" {
			sourcePreset = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
		}
		major, minor = inferredPresetVersion(sourcePath, major, minor, isOdyssey)
		targetPreset, targetPath := s.uniqueCloneTarget(sourcePreset, major, minor)
		plan := &presetClonePlan{
			sourcePath: sourcePath, sourcePreset: sourcePreset,
			targetPath: targetPath, targetPreset: targetPreset,
			major: major, minor: minor, actions: actions,
			assignments: map[string]Assignment{}, original: original,
		}
		plans = append(plans, plan)
		for name, a := range actions {
			allActionNames[name] = true
			if c, ok := slotCanonical(a.Primary); ok {
				used[c] = true
			}
			if c, ok := slotCanonical(a.Secondary); ok {
				used[c] = true
			}
			if slotBound(a.Primary) {
				report.AlreadyBound++
			} else if slotBound(a.Secondary) {
				report.ExistingFallback++
			}
		}
	}
	report.ScannedActions = len(allActionNames)

	candidates := candidateChords(used)
	ci := 0
	globalAssignments := map[string]Assignment{}
	actionNames := make([]string, 0, len(allActionNames))
	for name := range allActionNames {
		actionNames = append(actionNames, name)
	}
	sort.Strings(actionNames)

	for _, name := range actionNames {
		needs := false
		for _, plan := range plans {
			if a, ok := plan.actions[name]; ok && !slotBound(a.Primary) && !slotBound(a.Secondary) {
				needs = true
				break
			}
		}
		if !needs {
			continue
		}
		if ci >= len(candidates) {
			return report, fmt.Errorf("not enough collision-free keyboard chords for unbound Elite commands")
		}
		c := candidates[ci]
		ci++
		globalAssignments[name] = Assignment{Action: name, Key: c.key, Modifiers: append([]string(nil), c.modifiers...)}
	}

	if len(globalAssignments) == 0 {
		s.mu.Lock()
		s.autoBind = report
		s.mu.Unlock()
		return report, nil
	}

	for _, plan := range plans {
		for name, a := range plan.actions {
			if slotBound(a.Primary) || slotBound(a.Secondary) {
				continue
			}
			if assignment, ok := globalAssignments[name]; ok {
				plan.assignments[name] = assignment
			}
		}
		renamed, err := renamePresetBytes(plan.original, plan.targetPreset, plan.major, plan.minor)
		if err != nil {
			return report, err
		}
		updated := renamed
		if len(plan.assignments) > 0 {
			updated, err = injectSecondaryBindings(renamed, plan.assignments)
			if err != nil {
				return report, err
			}
		}
		if err := validateCloneBindings(plan.original, updated, plan.targetPreset, plan.assignments); err != nil {
			return report, fmt.Errorf("refusing unsafe clone %s: %w", plan.targetPreset, err)
		}
		plan.updated = updated
	}

	created := []string{}
	rollbackCreated := func() {
		for _, p := range created {
			_ = os.Remove(p)
		}
	}
	for _, plan := range plans {
		if err := writeExclusiveSynced(plan.targetPath, plan.updated, 0644); err != nil {
			rollbackCreated()
			return report, fmt.Errorf("create JACoB clone %s: %w", filepath.Base(plan.targetPath), err)
		}
		created = append(created, plan.targetPath)
		report.CreatedFiles = append(report.CreatedFiles, plan.targetPath)
		report.Clones = append(report.Clones, CloneReport{
			SourceFile: plan.sourcePath, SourcePreset: plan.sourcePreset,
			TargetFile: plan.targetPath, TargetPreset: plan.targetPreset,
			Assigned: len(plan.assignments),
		})
	}

	mapping := map[string]string{}
	for _, plan := range plans {
		mapping[plan.sourcePreset] = plan.targetPreset
	}

	type selectorState struct {
		path    string
		existed bool
		before  []byte
		backup  string
	}
	selectorStates := []selectorState{}
	stamp := time.Now().Format("20060102-150405.000")
	selectorNamesToTry := []string{"StartPreset.4.start", "StartPreset.start"}
	changedAnySelector := false
	for _, name := range selectorNamesToTry {
		path := filepath.Join(dir, name)
		before, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			rollbackCreated()
			return report, fmt.Errorf("read selector %s: %w", name, err)
		}
		after, changed := replaceSelectorNames(before, mapping)
		if !changed {
			continue
		}
		backup := path + ".jacob-backup-" + stamp
		if err := writeExclusiveSynced(backup, before, 0644); err != nil {
			rollbackCreated()
			return report, fmt.Errorf("backup selector %s: %w", name, err)
		}
		if err := atomicReplace(path, after, 0644); err != nil {
			_ = os.Remove(backup)
			rollbackCreated()
			return report, fmt.Errorf("switch selector %s: %w", name, err)
		}
		selectorStates = append(selectorStates, selectorState{path: path, existed: true, before: before, backup: backup})
		report.SelectorBackups = append(report.SelectorBackups, backup)
		if report.BackupPath == "" {
			report.BackupPath = backup
		}
		changedAnySelector = true
	}

	if !changedAnySelector {
		if len(plans) != 1 {
			rollbackCreated()
			return report, fmt.Errorf("no selector referenced the active presets and multiple source presets are active; refusing to guess category ownership")
		}
		path := filepath.Join(dir, "StartPreset.4.start")
		content := []byte(strings.Repeat(plans[0].targetPreset+"\n", 4))
		if err := writeExclusiveSynced(path, content, 0644); err != nil {
			rollbackCreated()
			return report, fmt.Errorf("create Odyssey selector: %w", err)
		}
		selectorStates = append(selectorStates, selectorState{path: path, existed: false})
		changedAnySelector = true
	}

	rollbackSelectors := func() {
		for _, st := range selectorStates {
			if st.existed {
				_ = atomicReplace(st.path, st.before, 0644)
			} else {
				_ = os.Remove(st.path)
			}
		}
	}

	if err := s.Reload(); err != nil {
		rollbackSelectors()
		rollbackCreated()
		_ = s.Reload()
		return report, fmt.Errorf("JACoB clone could not be loaded; selector restored: %w", err)
	}
	activeSet := map[string]bool{}
	for _, p := range s.ActiveFiles() {
		activeSet[p] = true
	}
	for _, plan := range plans {
		if !activeSet[plan.targetPath] {
			rollbackSelectors()
			rollbackCreated()
			_ = s.Reload()
			return report, fmt.Errorf("Elite selector did not resolve JACoB clone %s; original selection restored", plan.targetPreset)
		}
	}

	for _, name := range actionNames {
		if a, ok := globalAssignments[name]; ok {
			report.Assignments = append(report.Assignments, a)
		}
	}
	report.Assigned = len(report.Assignments)
	if len(report.CreatedFiles) > 0 {
		report.ActiveFile = report.CreatedFiles[0]
	}
	s.mu.Lock()
	s.autoBind = report
	s.mu.Unlock()
	return report, nil
}

func writeExclusiveSynced(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func atomicReplace(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".jacob-bindings-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		_ = os.Remove(tmp)
	}()
	if err := f.Chmod(mode); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	closed = true
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

type chordCandidate struct {
	key       string
	modifiers []string
	canonical string
}

func candidateChords(used map[string]bool) []chordCandidate {
	keys := []string{}
	for c := 'A'; c <= 'Z'; c++ {
		keys = append(keys, "Key_"+string(c))
	}
	for c := '0'; c <= '9'; c++ {
		keys = append(keys, "Key_"+string(c))
	}
	for i := 1; i <= 12; i++ {
		keys = append(keys, fmt.Sprintf("Key_F%d", i))
	}
	keys = append(keys, "Key_UpArrow", "Key_DownArrow", "Key_LeftArrow", "Key_RightArrow", "Key_Insert", "Key_Delete", "Key_Home", "Key_End", "Key_PageUp", "Key_PageDown", "Key_Space", "Key_Tab", "Key_Backspace")
	pools := [][]string{
		{"Key_LeftControl", "Key_LeftAlt"}, {"Key_LeftControl", "Key_LeftShift"}, {"Key_LeftAlt", "Key_LeftShift"},
		{"Key_LeftControl"}, {"Key_LeftAlt"}, {"Key_LeftShift"},
	}
	out := []chordCandidate{}
	for _, mods := range pools {
		for _, key := range keys {
			slot := Slot{Device: "Keyboard", Key: key}
			for _, m := range mods {
				slot.Modifiers = append(slot.Modifiers, KeyRef{Device: "Keyboard", Key: m})
			}
			canon, ok := slotCanonical(slot)
			if !ok || used[canon] {
				continue
			}
			used[canon] = true
			out = append(out, chordCandidate{key: key, modifiers: append([]string(nil), mods...), canonical: canon})
		}
	}
	return out
}

func slotCanonical(slot Slot) (string, bool) {
	key, mods, ok := KeyboardChord(slot)
	if !ok {
		return "", false
	}
	sort.Strings(mods)
	return strings.Join(append(mods, key), "+"), true
}

func buildSecondaryTag(a Assignment) string {
	if len(a.Modifiers) == 0 {
		return fmt.Sprintf(`<Secondary Device="Keyboard" Key="%s" />`, a.Key)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<Secondary Device="Keyboard" Key="%s">`, a.Key))
	for _, m := range a.Modifiers {
		b.WriteString(fmt.Sprintf(`<Modifier Device="Keyboard" Key="%s" />`, m))
	}
	b.WriteString(`</Secondary>`)
	return b.String()
}

func injectSecondaryBindings(src []byte, assignments map[string]Assignment) ([]byte, error) {
	out := append([]byte(nil), src...)
	names := make([]string, 0, len(assignments))
	for name := range assignments {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		a := assignments[name]
		open := []byte("<" + name + ">")
		close := []byte("</" + name + ">")
		start := bytes.Index(out, open)
		if start < 0 {
			return nil, fmt.Errorf("action %s not found while patching bindings", name)
		}
		endRel := bytes.Index(out[start+len(open):], close)
		if endRel < 0 {
			return nil, fmt.Errorf("closing action %s not found while patching bindings", name)
		}
		end := start + len(open) + endRel
		block := out[start:end]
		secRel := bytes.Index(block, []byte("<Secondary"))
		if secRel < 0 {
			return nil, fmt.Errorf("action %s has no Secondary element", name)
		}
		secStart := start + secRel
		gtRel := bytes.IndexByte(out[secStart:], '>')
		if gtRel < 0 {
			return nil, fmt.Errorf("action %s has malformed Secondary element", name)
		}
		secEnd := secStart + gtRel + 1
		tag := string(out[secStart:secEnd])
		compact := strings.ReplaceAll(strings.ReplaceAll(tag, " ", ""), "\t", "")
		if !(strings.Contains(compact, `Device="{NoDevice}"`) || strings.Contains(compact, `Device="NoDevice"`)) || !strings.Contains(compact, `Key=""`) {
			return nil, fmt.Errorf("action %s Secondary is no longer empty; refusing to overwrite it", name)
		}
		if !strings.HasSuffix(strings.TrimSpace(tag), "/>") {
			return nil, fmt.Errorf("action %s empty Secondary is not self-closing; refusing unsafe edit", name)
		}
		repl := []byte(buildSecondaryTag(a))
		next := make([]byte, 0, len(out)-(secEnd-secStart)+len(repl))
		next = append(next, out[:secStart]...)
		next = append(next, repl...)
		next = append(next, out[secEnd:]...)
		out = next
	}
	return out, nil
}

func parseBytes(data []byte) (map[string]Action, string, error) {
	tmp, err := os.CreateTemp("", "jacob-binds-*.binds")
	if err != nil {
		return nil, "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return nil, "", err
	}
	if err := tmp.Close(); err != nil {
		return nil, "", err
	}
	return parseFile(name)
}

func slotsEqual(a, b Slot) bool {
	if a.Device != b.Device || a.Key != b.Key || len(a.Modifiers) != len(b.Modifiers) {
		return false
	}
	for i := range a.Modifiers {
		if a.Modifiers[i] != b.Modifiers[i] {
			return false
		}
	}
	return true
}

func validatePatchedBindings(original []byte, updated []byte, assignments map[string]Assignment) error {
	beforeActions, beforePreset, err := parseBytes(original)
	if err != nil {
		return fmt.Errorf("original bindings validation failed: %w", err)
	}
	afterActions, afterPreset, err := parseBytes(updated)
	if err != nil {
		return fmt.Errorf("patched bindings validation failed: %w", err)
	}
	if beforePreset != afterPreset {
		return fmt.Errorf("preset name changed from %q to %q", beforePreset, afterPreset)
	}
	if len(beforeActions) != len(afterActions) {
		return fmt.Errorf("action count changed from %d to %d", len(beforeActions), len(afterActions))
	}
	for name, before := range beforeActions {
		after, ok := afterActions[name]
		if !ok {
			return fmt.Errorf("action %s disappeared after patch", name)
		}
		if !slotsEqual(before.Primary, after.Primary) {
			return fmt.Errorf("primary binding for %s changed unexpectedly", name)
		}
		if _, assigned := assignments[name]; !assigned && !slotsEqual(before.Secondary, after.Secondary) {
			return fmt.Errorf("secondary binding for %s changed unexpectedly", name)
		}
	}
	return nil
}

func NormalizeKeyboardKey(raw string) (string, bool) {
	k := strings.TrimPrefix(strings.TrimSpace(raw), "Key_")
	if k == "" {
		return "", false
	}
	aliases := map[string]string{
		"Return": "ENTER", "Enter": "ENTER", "Escape": "ESC", "Esc": "ESC", "Space": "SPACE", "Tab": "TAB", "Backspace": "BACKSPACE",
		"UpArrow": "UP", "DownArrow": "DOWN", "LeftArrow": "LEFT", "RightArrow": "RIGHT",
		"LeftControl": "CTRL", "RightControl": "RIGHTCONTROL", "LeftShift": "SHIFT", "RightShift": "RIGHTSHIFT", "LeftAlt": "ALT", "RightAlt": "RIGHTALT",
		"PageUp": "PAGEUP", "PageDown": "PAGEDOWN", "Insert": "INSERT", "Delete": "DELETE", "Home": "HOME", "End": "END",
	}
	if v, ok := aliases[k]; ok {
		return v, true
	}
	u := strings.ToUpper(k)
	if strings.HasPrefix(u, "NUMPAD_") {
		u = strings.Replace(u, "NUMPAD_", "NUMPAD", 1)
	}
	return u, true
}

func KeyboardChord(slot Slot) (key string, modifiers []string, ok bool) {
	if !strings.EqualFold(slot.Device, "Keyboard") || slot.Key == "" {
		return "", nil, false
	}
	key, ok = NormalizeKeyboardKey(slot.Key)
	if !ok {
		return "", nil, false
	}
	for _, m := range slot.Modifiers {
		if !strings.EqualFold(m.Device, "Keyboard") {
			return "", nil, false
		}
		mk, mok := NormalizeKeyboardKey(m.Key)
		if !mok {
			return "", nil, false
		}
		modifiers = append(modifiers, mk)
	}
	return key, modifiers, true
}

func unique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
