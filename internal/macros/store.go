package macros

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	schemaVersion       = 1
	policyVersion       = 1
	maxCustomSteps      = 12
	maxBuiltInSteps     = 32
	maxDelayMs          = 15000
	maxStaticTimelineMs = 60000
)

var (
	idPattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	paramPattern       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
	placeholderPattern = regexp.MustCompile(`\{\{([A-Za-z][A-Za-z0-9_]{0,31})\}\}`)
)

type Step struct {
	Type       string   `json:"type"`
	Action     string   `json:"action,omitempty"`
	Key        string   `json:"key,omitempty"`
	Modifiers  []string `json:"modifiers,omitempty"`
	Text       string   `json:"text,omitempty"`
	DelayMs    int      `json:"delayMs,omitempty"`
	HoldMs     int      `json:"holdMs,omitempty"`
	IntervalMs int      `json:"intervalMs,omitempty"`
}

type Definition struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	BuiltIn     bool     `json:"builtIn"`
	Parameters  []string `json:"parameters,omitempty"`
	Steps       []Step   `json:"steps"`
}

type PolicyAgreement struct {
	Version    int    `json:"version"`
	AcceptedAt string `json:"acceptedAt,omitempty"`
}

type PolicyState struct {
	Version        int    `json:"version"`
	Accepted       bool   `json:"accepted"`
	AcceptedAt     string `json:"acceptedAt,omitempty"`
	MaxCustomSteps int    `json:"maxCustomSteps"`
}

type diskFile struct {
	SchemaVersion int             `json:"schemaVersion"`
	Policy        PolicyAgreement `json:"policy,omitempty"`
	Macros        []Definition    `json:"macros"`
}

type Store struct {
	mu       sync.RWMutex
	path     string
	values   map[string]Definition
	defaults map[string]Definition
	policy   PolicyAgreement
}

func New(dataDir string) (*Store, error) {
	defaults := defaultDefinitions()
	s := &Store{values: map[string]Definition{}, defaults: map[string]Definition{}}
	for _, def := range defaults {
		normalized, err := normalize(def, true)
		if err != nil {
			return nil, fmt.Errorf("invalid built-in macro %s: %w", def.ID, err)
		}
		normalized.BuiltIn = true
		s.defaults[normalized.ID] = normalized
		s.values[normalized.ID] = normalized
	}
	if strings.TrimSpace(dataDir) == "" {
		return s, nil
	}
	s.path = filepath.Join(dataDir, "macros.json")
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var file diskFile
	if err := json.Unmarshal(b, &file); err != nil {
		return nil, fmt.Errorf("read %s: %w", s.path, err)
	}
	if file.SchemaVersion != 0 && file.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("unsupported macro schema %d", file.SchemaVersion)
	}
	if file.Policy.Version == policyVersion && strings.TrimSpace(file.Policy.AcceptedAt) != "" {
		s.policy = file.Policy
	}
	for _, raw := range file.Macros {
		_, builtIn := s.defaults[strings.TrimSpace(raw.ID)]
		def, err := normalize(raw, builtIn)
		if err != nil {
			return nil, fmt.Errorf("macro %q: %w", raw.ID, err)
		}
		def.BuiltIn = builtIn
		s.values[def.ID] = def
	}
	return s, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Policy() PolicyState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policyStateLocked()
}

func (s *Store) PolicyAccepted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policyAcceptedLocked()
}

func (s *Store) AcceptPolicy() (PolicyState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.policy
	s.policy = PolicyAgreement{Version: policyVersion, AcceptedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := s.persistLocked(); err != nil {
		s.policy = previous
		return PolicyState{}, err
	}
	return s.policyStateLocked(), nil
}

func (s *Store) policyAcceptedLocked() bool {
	return s.policy.Version == policyVersion && strings.TrimSpace(s.policy.AcceptedAt) != ""
}

func (s *Store) policyStateLocked() PolicyState {
	return PolicyState{Version: policyVersion, Accepted: s.policyAcceptedLocked(), AcceptedAt: s.policy.AcceptedAt, MaxCustomSteps: maxCustomSteps}
}

func (s *Store) requirePolicyLocked() error {
	if s.policyAcceptedLocked() {
		return nil
	}
	return fmt.Errorf("macro use agreement must be accepted on the JACoB host before custom macros or macro edits are enabled")
}

func (s *Store) List() []Definition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Definition, 0, len(s.values))
	for _, def := range s.values {
		out = append(out, clone(def))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BuiltIn != out[j].BuiltIn {
			return out[i].BuiltIn
		}
		if strings.EqualFold(out[i].Name, out[j].Name) {
			return out[i].ID < out[j].ID
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (s *Store) Resolve(idOrName string) (Definition, bool) {
	query := strings.TrimSpace(idOrName)
	if query == "" {
		return Definition{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if def, ok := s.values[query]; ok {
		return clone(def), true
	}
	for _, def := range s.values {
		if strings.EqualFold(def.Name, query) {
			return clone(def), true
		}
	}
	return Definition{}, false
}

func (s *Store) Save(raw Definition) (Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requirePolicyLocked(); err != nil {
		return Definition{}, err
	}
	if strings.TrimSpace(raw.ID) == "" {
		raw.ID = uniqueID(slug(raw.Name), s.values)
	}
	_, builtIn := s.defaults[strings.TrimSpace(raw.ID)]
	def, err := normalize(raw, builtIn)
	if err != nil {
		return Definition{}, err
	}
	def.BuiltIn = builtIn
	previous, existed := s.values[def.ID]
	s.values[def.ID] = def
	if err := s.persistLocked(); err != nil {
		if existed {
			s.values[def.ID] = previous
		} else {
			delete(s.values, def.ID)
		}
		return Definition{}, err
	}
	return clone(def), nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requirePolicyLocked(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if _, ok := s.defaults[id]; ok {
		return fmt.Errorf("built-in macro %q cannot be deleted; reset it instead", id)
	}
	if _, ok := s.values[id]; !ok {
		return fmt.Errorf("macro %q was not found", id)
	}
	previous := s.values[id]
	delete(s.values, id)
	if err := s.persistLocked(); err != nil {
		s.values[id] = previous
		return err
	}
	return nil
}

func (s *Store) Reset(id string) (Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requirePolicyLocked(); err != nil {
		return Definition{}, err
	}
	id = strings.TrimSpace(id)
	def, ok := s.defaults[id]
	if !ok {
		return Definition{}, fmt.Errorf("macro %q is not a built-in macro", id)
	}
	previous, existed := s.values[id]
	s.values[id] = def
	if err := s.persistLocked(); err != nil {
		if existed {
			s.values[id] = previous
		} else {
			delete(s.values, id)
		}
		return Definition{}, err
	}
	return clone(def), nil
}

func (s *Store) persistLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	defs := make([]Definition, 0, len(s.values))
	for _, def := range s.values {
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	b, err := json.MarshalIndent(diskFile{SchemaVersion: schemaVersion, Policy: s.policy, Macros: defs}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := s.path + ".tmp"
	backup := s.path + ".previous"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	_ = os.Remove(backup)
	hadPrevious := false
	if _, err := os.Stat(s.path); err == nil {
		if err := os.Rename(s.path, backup); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		hadPrevious = true
	} else if !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		if hadPrevious {
			_ = os.Rename(backup, s.path)
		}
		_ = os.Remove(tmp)
		return err
	}
	if hadPrevious {
		_ = os.Remove(backup)
	}
	return nil
}

func normalize(raw Definition, builtIn bool) (Definition, error) {
	def := clone(raw)
	def.ID = strings.TrimSpace(def.ID)
	def.Name = strings.TrimSpace(def.Name)
	def.Description = strings.TrimSpace(def.Description)
	if !idPattern.MatchString(def.ID) {
		return Definition{}, fmt.Errorf("id %q must be 1-64 letters, numbers, dot, underscore, or hyphen", def.ID)
	}
	if def.Name == "" || len([]rune(def.Name)) > 80 {
		return Definition{}, fmt.Errorf("name must contain 1-80 characters")
	}
	if len([]rune(def.Description)) > 500 {
		return Definition{}, fmt.Errorf("description is limited to 500 characters")
	}
	stepLimit := maxCustomSteps
	if builtIn {
		stepLimit = maxBuiltInSteps
	}
	if len(def.Steps) == 0 || len(def.Steps) > stepLimit {
		return Definition{}, fmt.Errorf("macro must contain 1-%d steps", stepLimit)
	}
	params := make([]string, 0, len(def.Parameters))
	seenParams := map[string]bool{}
	for _, rawParam := range def.Parameters {
		p := strings.TrimSpace(rawParam)
		if !paramPattern.MatchString(p) {
			return Definition{}, fmt.Errorf("invalid parameter name %q", p)
		}
		key := strings.ToLower(p)
		if seenParams[key] {
			continue
		}
		seenParams[key] = true
		params = append(params, p)
	}
	def.Parameters = params
	staticTimeline := 0
	for i := range def.Steps {
		step := &def.Steps[i]
		step.Type = strings.ToLower(strings.TrimSpace(step.Type))
		step.Action = strings.TrimSpace(step.Action)
		step.Key = strings.TrimSpace(step.Key)
		step.Modifiers = cleanStrings(step.Modifiers)
		if step.DelayMs < 0 || step.DelayMs > maxDelayMs {
			return Definition{}, fmt.Errorf("step %d delayMs must be 0-%d", i+1, maxDelayMs)
		}
		if step.HoldMs < 0 || step.HoldMs > 10000 || (step.HoldMs > 0 && step.HoldMs < 20) {
			return Definition{}, fmt.Errorf("step %d holdMs must be 0 or 20-10000", i+1)
		}
		if step.IntervalMs < 0 || step.IntervalMs > 250 {
			return Definition{}, fmt.Errorf("step %d intervalMs must be 0-250", i+1)
		}
		staticTimeline += step.DelayMs + step.HoldMs
		switch step.Type {
		case "binding":
			if step.Action == "" || len(step.Action) > 128 {
				return Definition{}, fmt.Errorf("step %d requires a valid Elite binding action", i+1)
			}
		case "raw":
			if step.Key == "" || len(step.Key) > 64 {
				return Definition{}, fmt.Errorf("step %d requires a physical/raw key", i+1)
			}
			if len(step.Modifiers) > 8 {
				return Definition{}, fmt.Errorf("step %d has too many modifiers", i+1)
			}
		case "text":
			if step.Text == "" || len([]rune(step.Text)) > 256 {
				return Definition{}, fmt.Errorf("step %d text must contain 1-256 characters", i+1)
			}
			for _, match := range placeholderPattern.FindAllStringSubmatch(step.Text, -1) {
				if len(match) < 2 || !containsParam(params, match[1]) {
					return Definition{}, fmt.Errorf("step %d uses undeclared parameter %q", i+1, match[1])
				}
			}
		default:
			return Definition{}, fmt.Errorf("step %d type %q is unsupported", i+1, step.Type)
		}
		if builtIn {
			if err := validateBuiltInStep(*step, i+1); err != nil {
				return Definition{}, err
			}
		}
	}
	if staticTimeline > maxStaticTimelineMs {
		return Definition{}, fmt.Errorf("macro static timeline exceeds %d ms", maxStaticTimelineMs)
	}
	return def, nil
}

var builtInNavigationActions = map[string]bool{
	"GalaxyMapOpen": true, "SystemMapOpen": true,
	"FocusLeftPanel": true, "FocusCommsPanel": true, "FocusRadarPanel": true, "FocusRightPanel": true,
	"QuickCommsPanel": true, "OpenCodexGoToDiscovery": true,
	"UI_Up": true, "UI_Down": true, "UI_Left": true, "UI_Right": true, "UI_Select": true, "UI_Back": true,
	"UI_TabLeft": true, "UI_TabRight": true, "MoveFreeCamRight": true,
}

func validateBuiltInStep(step Step, index int) error {
	switch step.Type {
	case "binding":
		if !builtInNavigationActions[step.Action] {
			return fmt.Errorf("step %d: built-in macros are restricted to JACoB's logistics/menu-navigation binding allowlist", index)
		}
	case "raw":
		if !strings.EqualFold(step.Key, "SC:01") || len(step.Modifiers) != 0 {
			return fmt.Errorf("step %d: built-in raw-key steps are restricted to unmodified Escape", index)
		}
	case "text":
		if strings.TrimSpace(placeholderPattern.ReplaceAllString(step.Text, "")) != "" {
			return fmt.Errorf("step %d: built-in text steps may contain declared parameters only", index)
		}
	}
	return nil
}

func ExpandText(text string, params map[string]string) (string, error) {
	missing := ""
	out := placeholderPattern.ReplaceAllStringFunc(text, func(token string) string {
		match := placeholderPattern.FindStringSubmatch(token)
		if len(match) < 2 {
			return token
		}
		key := match[1]
		value, ok := params[key]
		if !ok || strings.TrimSpace(value) == "" {
			missing = key
			return token
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("macro parameter %q is required", missing)
	}
	return out, nil
}

func containsParam(params []string, wanted string) bool {
	for _, p := range params {
		if p == wanted {
			return true
		}
	}
	return false
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		key := strings.ToUpper(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}

func clone(def Definition) Definition {
	def.Parameters = append([]string(nil), def.Parameters...)
	def.Steps = append([]Step(nil), def.Steps...)
	for i := range def.Steps {
		def.Steps[i].Modifiers = append([]string(nil), def.Steps[i].Modifiers...)
	}
	return def
}

func slug(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "macro"
	}
	if len(out) > 48 {
		out = strings.TrimRight(out[:48], "-")
	}
	return out
}

func uniqueID(base string, existing map[string]Definition) string {
	if !idPattern.MatchString(base) {
		base = "macro"
	}
	if _, ok := existing[base]; !ok {
		return base
	}
	for i := 2; i < 10000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if len(candidate) > 64 {
			candidate = fmt.Sprintf("macro-%d", i)
		}
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
	}
	return "macro-custom"
}

func defaultDefinitions() []Definition {
	backs := func(n int) []Step {
		out := make([]Step, 0, n)
		for i := 0; i < n; i++ {
			delay := 0
			if i > 0 {
				delay = 90
			}
			out = append(out, Step{Type: "raw", Key: "SC:01", DelayMs: delay, HoldMs: 60})
		}
		return out
	}
	route := backs(5)
	route = append(route,
		Step{Type: "binding", Action: "GalaxyMapOpen", DelayMs: 150},
		Step{Type: "binding", Action: "UI_Up", DelayMs: 1400},
		Step{Type: "binding", Action: "UI_Select", DelayMs: 180},
		Step{Type: "text", Text: "{{system}}", DelayMs: 300, IntervalMs: 18},
		Step{Type: "binding", Action: "UI_Down", DelayMs: 250},
		Step{Type: "binding", Action: "UI_Select", DelayMs: 180},
		Step{Type: "binding", Action: "MoveFreeCamRight", DelayMs: 850},
		Step{Type: "binding", Action: "UI_Right", DelayMs: 140},
	)
	for i := 0; i < 7; i++ {
		delay := 90
		if i == 0 {
			delay = 180
		}
		route = append(route, Step{Type: "binding", Action: "UI_Down", DelayMs: delay})
	}
	route = append(route,
		Step{Type: "binding", Action: "UI_Select", DelayMs: 90},
		Step{Type: "binding", Action: "GalaxyMapOpen", DelayMs: 250},
	)
	return []Definition{
		{ID: "exit_menus", Name: "Exit menus", Description: "Back out of nested Elite menus with five physical Escape presses.", BuiltIn: true, Steps: backs(5)},
		{ID: "open_galaxy_map", Name: "Open Galaxy Map", Description: "Open or close the Galaxy Map using the active Elite binding.", BuiltIn: true, Steps: []Step{{Type: "binding", Action: "GalaxyMapOpen"}}},
		{ID: "open_system_map", Name: "Open System Map", Description: "Open or close the System Map using the active Elite binding.", BuiltIn: true, Steps: []Step{{Type: "binding", Action: "SystemMapOpen"}}},
		{ID: "open_navigation_panel", Name: "Open navigation panel", Description: "Focus the left/navigation panel.", BuiltIn: true, Steps: []Step{{Type: "binding", Action: "FocusLeftPanel"}}},
		{ID: "open_comms_panel", Name: "Open comms panel", Description: "Focus the communications panel.", BuiltIn: true, Steps: []Step{{Type: "binding", Action: "FocusCommsPanel"}}},
		{ID: "open_role_panel", Name: "Open role panel", Description: "Focus the lower/role panel.", BuiltIn: true, Steps: []Step{{Type: "binding", Action: "FocusRadarPanel"}}},
		{ID: "open_internal_panel", Name: "Open internal panel", Description: "Focus the right/internal panel.", BuiltIn: true, Steps: []Step{{Type: "binding", Action: "FocusRightPanel"}}},
		{ID: "open_quick_comms", Name: "Open quick comms", Description: "Open the quick text communications entry using the active Elite binding.", BuiltIn: true, Steps: []Step{{Type: "binding", Action: "QuickCommsPanel"}}},
		{ID: "open_codex_discovery", Name: "Open Codex discovery", Description: "Open the Codex directly to discoveries using the active Elite binding.", BuiltIn: true, Steps: []Step{{Type: "binding", Action: "OpenCodexGoToDiscovery"}}},
		{ID: "plot_route", Name: "Plot route to system", Description: "Open Galaxy Map, search for a system, select Plot Route, and return to flight. Timing and steps are editable in Settings.", BuiltIn: true, Parameters: []string{"system"}, Steps: route},
	}
}
