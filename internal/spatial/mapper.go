package spatial

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"jacob/internal/vision"
)

const (
	defaultHFOVDeg = 80.0
	defaultWidth   = 480
	maxScenes      = 8
	maxMapPoints   = 2500
	sceneTTL       = 45 * time.Minute
)

type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type Config struct {
	HorizontalFOVDeg float64 `json:"horizontalFovDeg"`
	MaxWidth         int     `json:"maxWidth"`
}

type BeginRequest struct {
	HorizontalFOVDeg float64 `json:"horizontalFovDeg"`
	MaxWidth         int     `json:"maxWidth"`
}

type Pose struct {
	At              string  `json:"at"`
	Position        Vec3    `json:"position"`
	YawDeg          float64 `json:"yawDeg"`
	PitchDeg        float64 `json:"pitchDeg"`
	RollDeg         float64 `json:"rollDeg"`
	Metric          bool    `json:"metric"`
	Confidence      float64 `json:"confidence"`
	TrackingQuality float64 `json:"trackingQuality"`
}

type Landmark struct {
	ID           string  `json:"id"`
	Label        string  `json:"label,omitempty"`
	Kind         string  `json:"kind,omitempty"`
	Position     Vec3    `json:"position"`
	Positioned   bool    `json:"positioned"`
	Confidence   float64 `json:"confidence"`
	Observations int     `json:"observations"`
	LastSeen     string  `json:"lastSeen,omitempty"`
}

type Status struct {
	Scene            string `json:"scene"`
	State            string `json:"state"`
	At               string `json:"at"`
	Available        bool   `json:"available"`
	Blanked          bool   `json:"blanked"`
	Reason           string `json:"reason,omitempty"`
	Pose             Pose   `json:"pose"`
	TrackedFeatures  int    `json:"trackedFeatures"`
	MappedFeatures   int    `json:"mappedFeatures"`
	LandmarkCount    int    `json:"landmarkCount"`
	MetricLandmarks  int    `json:"metricLandmarks"`
	MatchedMapPoints int    `json:"matchedMapPoints"`
	FrameWidth       int    `json:"frameWidth"`
	FrameHeight      int    `json:"frameHeight"`
}

type ObserveRequest struct {
	Scene      string  `json:"scene"`
	Landmark   string  `json:"landmark"`
	Label      string  `json:"label,omitempty"`
	Kind       string  `json:"kind,omitempty"`
	Screen     Vec2    `json:"screen"`
	RangeM     float64 `json:"rangeM,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type ProjectRequest struct {
	Scene    string `json:"scene"`
	Landmark string `json:"landmark,omitempty"`
	Point    *Vec3  `json:"point,omitempty"`
}

type Projection struct {
	Visible    bool    `json:"visible"`
	Behind     bool    `json:"behind"`
	Screen     Vec2    `json:"screen"`
	RangeM     float64 `json:"rangeM"`
	Confidence float64 `json:"confidence"`
}

type MapPoint struct {
	ID           int     `json:"id"`
	Position     Vec3    `json:"position"`
	Signature    string  `json:"signature"`
	Confidence   float64 `json:"confidence"`
	Observations int     `json:"observations"`
}

type SceneExport struct {
	Version   int        `json:"version"`
	Config    Config     `json:"config"`
	Landmarks []Landmark `json:"landmarks"`
	MapPoints []MapPoint `json:"mapPoints"`
}

type ImportRequest struct {
	Data SceneExport `json:"data"`
}

type Mapper struct {
	mu     sync.Mutex
	vision *vision.Tracker
	scenes map[string]*scene
	next   uint64
}

type poseState struct {
	pos        Vec3
	yaw        float64
	pitch      float64
	roll       float64
	metric     bool
	confidence float64
	quality    float64
	at         string
}

type scene struct {
	id         string
	config     Config
	pose       poseState
	tracks     map[int]*track
	points     map[int]*mapPointState
	landmarks  map[string]*Landmark
	semantic   map[string]semanticObservation
	nextTrack  int
	nextPoint  int
	depthM     float64
	frameIndex int64
	frameW     int
	frameH     int
	state      string
	updated    time.Time
	blanked    bool
	reason     string
	imported   bool
	matchedMap int
}

type track struct {
	id        int
	x         float64
	y         float64
	score     float64
	signature uint64
	age       int
	missed    int
	pointID   int
	anchor    *trackAnchor
}

type trackAnchor struct {
	pos   Vec3
	yaw   float64
	pitch float64
	roll  float64
	x     float64
	y     float64
}

type mapPointState struct {
	id           int
	position     Vec3
	signature    uint64
	confidence   float64
	observations int
}

type semanticObservation struct {
	frame    int64
	point    Vec3
	screen   Vec2
	rangeM   float64
	hasRange bool
	conf     float64
}

type featureMatch struct {
	t    *track
	f    vision.SpatialFeature
	oldX float64
	oldY float64
	cost float64
}

func New(v *vision.Tracker) *Mapper {
	return &Mapper{vision: v, scenes: map[string]*scene{}}
}

func (m *Mapper) Info() map[string]any {
	available := m != nil && m.vision != nil
	if available {
		if v, ok := m.vision.Info()["available"].(bool); ok {
			available = v
		}
	}
	return map[string]any{
		"available":        available,
		"eliteOnly":        true,
		"rawFramesExposed": false,
		"coordinateFrame":  "scene-local-rhs-x-right-y-up-z-forward",
		"units":            "metres",
		"metricAnchors":    []string{"range-bearing"},
		"capabilities":     []string{"persistent-feature-tracks", "visual-odometry", "range-anchored-scale", "metric-landmarks", "triangulated-map-points", "relocalization", "world-to-screen-projection", "scene-export-import"},
		"limits":           map[string]any{"scenes": maxScenes, "mapPointsPerScene": maxMapPoints},
	}
}

func (m *Mapper) Begin(req BeginRequest) (Status, error) {
	if m == nil || m.vision == nil {
		return Status{}, errors.New("spatial mapper is unavailable")
	}
	cfg := normalizeConfig(Config{HorizontalFOVDeg: req.HorizontalFOVDeg, MaxWidth: req.MaxWidth})
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked()
	if len(m.scenes) >= maxScenes {
		return Status{}, fmt.Errorf("spatial scene limit reached (%d)", maxScenes)
	}
	m.next++
	id := fmt.Sprintf("scene-%x-%x", time.Now().UnixMilli(), m.next)
	s := newScene(id, cfg)
	m.scenes[id] = s
	return statusLocked(s, true), nil
}

func (m *Mapper) Import(req ImportRequest) (Status, error) {
	if req.Data.Version != 1 {
		return Status{}, fmt.Errorf("unsupported spatial scene version %d", req.Data.Version)
	}
	st, err := m.Begin(BeginRequest{
		HorizontalFOVDeg: req.Data.Config.HorizontalFOVDeg,
		MaxWidth:         req.Data.Config.MaxWidth,
	})
	if err != nil {
		return Status{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.scenes[st.Scene]
	s.imported = true
	s.state = "relocalizing"
	for _, lm := range req.Data.Landmarks {
		copyLM := lm
		if copyLM.ID == "" {
			continue
		}
		s.landmarks[copyLM.ID] = &copyLM
	}
	for _, p := range req.Data.MapPoints {
		if p.ID <= 0 || len(s.points) >= maxMapPoints {
			continue
		}
		sig, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(p.Signature), "0x"), 16, 64)
		if err != nil {
			continue
		}
		s.points[p.ID] = &mapPointState{id: p.ID, position: p.Position, signature: sig, confidence: clamp01(p.Confidence), observations: maxInt(1, p.Observations)}
		if p.ID >= s.nextPoint {
			s.nextPoint = p.ID + 1
		}
	}
	s.updated = time.Now()
	return statusLocked(s, true), nil
}

func (m *Mapper) End(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.scenes[id]; !ok {
		return false
	}
	delete(m.scenes, id)
	return true
}

func (m *Mapper) Update(id string) (Status, error) {
	m.mu.Lock()
	s, ok := m.scenes[id]
	if !ok {
		m.mu.Unlock()
		return Status{}, fmt.Errorf("unknown spatial scene %q", id)
	}
	cfg := s.config
	m.mu.Unlock()

	frame, err := m.vision.SpatialFrame(cfg.MaxWidth)
	if err != nil {
		return Status{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok = m.scenes[id]
	if !ok {
		return Status{}, fmt.Errorf("unknown spatial scene %q", id)
	}
	s.updated = time.Now()
	s.frameIndex++
	s.frameW, s.frameH = frame.Width, frame.Height
	s.blanked, s.reason = frame.Blanked, frame.Reason
	s.pose.at = frame.At

	if !frame.Available {
		s.state = "unavailable"
		return statusLocked(s, false), nil
	}
	if frame.Blanked {
		s.state = "blanked"
		s.pose.quality *= 0.75
		return statusLocked(s, true), nil
	}
	if frame.Width <= 0 || frame.Height <= 0 {
		s.state = "lost"
		return statusLocked(s, true), nil
	}

	hfov, vfov := fovRadians(s.config.HorizontalFOVDeg, frame.Width, frame.Height)
	matches, unmatched := matchFeatures(s, frame.Features, frame.Motion)
	sim := similarity(matches)

	// Dominant frame translation is primarily useful as angular camera motion.
	// Scale supplies a weak forward/backward odometry cue once a metric depth
	// anchor exists. Metric landmark observations correct the accumulated drift.
	if frame.Motion.Compared && frame.Motion.Confidence > 0.02 {
		w := clamp01(frame.Motion.Confidence)
		s.pose.yaw -= frame.Motion.DX * hfov * w
		s.pose.pitch += frame.Motion.DY * vfov * w
		s.pose.pitch = clamp(s.pose.pitch, -1.45, 1.45)
	}
	if sim.valid && sim.confidence > 0.05 {
		s.pose.roll -= sim.rotation * 0.5 * sim.confidence
		if s.pose.metric && s.depthM > 1 && sim.scale > 0.92 && sim.scale < 1.09 {
			forwardM := s.depthM * (1 - 1/sim.scale)
			if math.Abs(forwardM) < s.depthM*0.15 {
				fwd := rotate(Vec3{Z: 1}, s.pose.yaw, s.pose.pitch, s.pose.roll)
				s.pose.pos = add(s.pose.pos, scale(fwd, forwardM*sim.confidence))
			}
		}
	}

	applyMatches(s, matches)
	for _, f := range unmatched {
		s.nextTrack++
		s.tracks[s.nextTrack] = &track{id: s.nextTrack, x: f.X, y: f.Y, score: f.Score, signature: f.Signature, age: 1}
	}
	for id, t := range s.tracks {
		if t.missed > 4 {
			delete(s.tracks, id)
		}
	}

	if s.imported {
		associateImportedPoints(s)
	}
	known := visibleMapObservations(s)
	if len(known) >= 3 {
		if pos, ok := solvePosition(known); ok {
			alpha := clamp(0.18+0.06*float64(len(known)), 0.18, 0.68)
			s.pose.pos = lerp(s.pose.pos, pos, alpha)
			s.pose.metric = true
			s.pose.confidence = clamp01(math.Max(s.pose.confidence, 0.35+0.04*float64(minInt(len(known), 10))))
			s.matchedMap = len(known)
			if s.imported && len(known) >= 5 {
				s.state = "tracking"
				s.imported = false
			}
		}
	} else {
		s.matchedMap = len(known)
	}

	if s.pose.metric {
		triangulateTracks(s, hfov, vfov)
	}

	active := 0
	mature := 0
	for _, t := range s.tracks {
		if t.missed == 0 {
			active++
			if t.age >= 3 {
				mature++
			}
		}
	}
	matchQuality := 0.0
	if len(frame.Features) > 0 {
		matchQuality = float64(len(matches)) / float64(len(frame.Features))
	}
	s.pose.quality = clamp01(0.65*s.pose.quality + 0.35*clamp01(matchQuality*1.8))
	if mature >= 8 {
		s.pose.confidence = clamp01(math.Max(s.pose.confidence, 0.18+s.pose.quality*0.42))
	}
	if s.state != "relocalizing" {
		switch {
		case s.pose.metric && s.pose.confidence >= 0.45:
			s.state = "tracking"
		case s.pose.metric:
			s.state = "metric"
		case active >= 8:
			s.state = "visual"
		default:
			s.state = "acquiring"
		}
	}
	return statusLocked(s, true), nil
}

func (m *Mapper) Observe(req ObserveRequest) (Landmark, error) {
	if req.Scene == "" || req.Landmark == "" {
		return Landmark{}, errors.New("scene and landmark are required")
	}
	if req.Screen.X < 0 || req.Screen.X > 1 || req.Screen.Y < 0 || req.Screen.Y > 1 {
		return Landmark{}, errors.New("screen coordinates must be normalized to [0,1]")
	}
	if req.RangeM < 0 {
		return Landmark{}, errors.New("rangeM cannot be negative")
	}
	conf := req.Confidence
	if conf <= 0 {
		conf = 1
	}
	conf = clamp01(conf)

	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.scenes[req.Scene]
	if !ok {
		return Landmark{}, fmt.Errorf("unknown spatial scene %q", req.Scene)
	}
	if s.frameW <= 0 || s.frameH <= 0 {
		return Landmark{}, errors.New("spatial.update must produce a frame before observations can be added")
	}
	hfov, vfov := fovRadians(s.config.HorizontalFOVDeg, s.frameW, s.frameH)
	rayCam := screenRay(req.Screen, hfov, vfov)
	rayWorld := rotate(rayCam, s.pose.yaw, s.pose.pitch, s.pose.roll)

	lm, exists := s.landmarks[req.Landmark]
	if !exists {
		lm = &Landmark{ID: req.Landmark}
		s.landmarks[req.Landmark] = lm
	}
	if req.Label != "" {
		lm.Label = req.Label
	}
	if req.Kind != "" {
		lm.Kind = req.Kind
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	lm.LastSeen = now
	lm.Observations++

	newlyPositioned := false
	if !lm.Positioned && req.RangeM > 0 {
		lm.Position = add(s.pose.pos, scale(rayWorld, req.RangeM))
		lm.Positioned = true
		lm.Confidence = clamp01(0.45 + 0.45*conf)
		newlyPositioned = true
		s.pose.metric = true
		s.pose.confidence = math.Max(s.pose.confidence, 0.28*conf)
	}
	if lm.Positioned && req.RangeM > 0 && !newlyPositioned {
		candidate := sub(lm.Position, scale(rayWorld, req.RangeM))
		alpha := clamp(0.20+0.55*conf, 0.20, 0.75)
		s.pose.pos = lerp(s.pose.pos, candidate, alpha)
		s.pose.metric = true
		s.pose.confidence = clamp01(math.Max(s.pose.confidence, 0.45+0.35*conf))
		lm.Confidence = clamp01(math.Max(lm.Confidence, 0.55+0.4*conf))
	}
	if req.RangeM > 0 {
		if s.depthM <= 0 {
			s.depthM = req.RangeM
		} else {
			s.depthM = 0.82*s.depthM + 0.18*req.RangeM
		}
	}
	if lm.Positioned {
		s.semantic[req.Landmark] = semanticObservation{
			frame:    s.frameIndex,
			point:    lm.Position,
			screen:   req.Screen,
			rangeM:   req.RangeM,
			hasRange: req.RangeM > 0,
			conf:     conf,
		}
		semanticLines := make([]lineObservation, 0, len(s.semantic))
		for id, ob := range s.semantic {
			if s.frameIndex-ob.frame > 2 {
				delete(s.semantic, id)
				continue
			}
			ray := rotate(screenRay(ob.screen, hfov, vfov), s.pose.yaw, s.pose.pitch, s.pose.roll)
			semanticLines = append(semanticLines, lineObservation{point: ob.point, ray: ray, weight: ob.conf})
		}
		if len(semanticLines) >= 2 {
			if pos, ok := solvePosition(semanticLines); ok {
				s.pose.pos = lerp(s.pose.pos, pos, 0.35)
				s.pose.metric = true
				s.pose.confidence = clamp01(math.Max(s.pose.confidence, 0.50))
			}
		}
	}
	s.pose.at = now
	s.updated = time.Now()
	if s.pose.metric && s.state != "relocalizing" {
		s.state = "metric"
	}
	return *lm, nil
}

func (m *Mapper) Pose(id string) (Pose, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.scenes[id]
	if !ok {
		return Pose{}, fmt.Errorf("unknown spatial scene %q", id)
	}
	return publicPose(s.pose), nil
}

func (m *Mapper) Landmarks(id string) ([]Landmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.scenes[id]
	if !ok {
		return nil, fmt.Errorf("unknown spatial scene %q", id)
	}
	out := make([]Landmark, 0, len(s.landmarks))
	for _, lm := range s.landmarks {
		out = append(out, *lm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *Mapper) Project(req ProjectRequest) (Projection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.scenes[req.Scene]
	if !ok {
		return Projection{}, fmt.Errorf("unknown spatial scene %q", req.Scene)
	}
	var p Vec3
	if req.Landmark != "" {
		lm, ok := s.landmarks[req.Landmark]
		if !ok || !lm.Positioned {
			return Projection{}, fmt.Errorf("landmark %q is not positioned", req.Landmark)
		}
		p = lm.Position
	} else if req.Point != nil {
		p = *req.Point
	} else {
		return Projection{}, errors.New("landmark or point is required")
	}
	if s.frameW <= 0 || s.frameH <= 0 {
		return Projection{}, errors.New("no spatial frame is available")
	}
	hfov, vfov := fovRadians(s.config.HorizontalFOVDeg, s.frameW, s.frameH)
	rel := inverseRotate(sub(p, s.pose.pos), s.pose.yaw, s.pose.pitch, s.pose.roll)
	rng := norm(rel)
	if rel.Z <= 0.001 {
		return Projection{Visible: false, Behind: true, RangeM: rng, Confidence: s.pose.confidence}, nil
	}
	x := 0.5 + (rel.X/rel.Z)/(2*math.Tan(hfov/2))
	y := 0.5 - (rel.Y/rel.Z)/(2*math.Tan(vfov/2))
	visible := x >= 0 && x <= 1 && y >= 0 && y <= 1
	return Projection{Visible: visible, Screen: Vec2{X: x, Y: y}, RangeM: rng, Confidence: s.pose.confidence}, nil
}

func (m *Mapper) Export(id string) (SceneExport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.scenes[id]
	if !ok {
		return SceneExport{}, fmt.Errorf("unknown spatial scene %q", id)
	}
	out := SceneExport{Version: 1, Config: s.config}
	for _, lm := range s.landmarks {
		out.Landmarks = append(out.Landmarks, *lm)
	}
	for _, p := range s.points {
		out.MapPoints = append(out.MapPoints, MapPoint{
			ID: p.id, Position: p.position, Signature: fmt.Sprintf("%016x", p.signature),
			Confidence: p.confidence, Observations: p.observations,
		})
	}
	sort.Slice(out.Landmarks, func(i, j int) bool { return out.Landmarks[i].ID < out.Landmarks[j].ID })
	sort.Slice(out.MapPoints, func(i, j int) bool { return out.MapPoints[i].ID < out.MapPoints[j].ID })
	return out, nil
}

func normalizeConfig(cfg Config) Config {
	if cfg.HorizontalFOVDeg < 35 || cfg.HorizontalFOVDeg > 140 {
		cfg.HorizontalFOVDeg = defaultHFOVDeg
	}
	if cfg.MaxWidth < 240 {
		cfg.MaxWidth = defaultWidth
	}
	if cfg.MaxWidth > 640 {
		cfg.MaxWidth = 640
	}
	return cfg
}

func newScene(id string, cfg Config) *scene {
	return &scene{
		id:        id,
		config:    cfg,
		tracks:    map[int]*track{},
		points:    map[int]*mapPointState{},
		landmarks: map[string]*Landmark{},
		semantic:  map[string]semanticObservation{},
		nextPoint: 1,
		state:     "acquiring",
		updated:   time.Now(),
		pose:      poseState{confidence: 0.05, quality: 0.05},
	}
}

func (m *Mapper) cleanupLocked() {
	cutoff := time.Now().Add(-sceneTTL)
	for id, s := range m.scenes {
		if s.updated.Before(cutoff) {
			delete(m.scenes, id)
		}
	}
}

func statusLocked(s *scene, available bool) Status {
	active := 0
	for _, t := range s.tracks {
		if t.missed == 0 {
			active++
		}
	}
	metricLM := 0
	for _, lm := range s.landmarks {
		if lm.Positioned {
			metricLM++
		}
	}
	return Status{
		Scene:            s.id,
		State:            s.state,
		At:               s.pose.at,
		Available:        available,
		Blanked:          s.blanked,
		Reason:           s.reason,
		Pose:             publicPose(s.pose),
		TrackedFeatures:  active,
		MappedFeatures:   len(s.points),
		LandmarkCount:    len(s.landmarks),
		MetricLandmarks:  metricLM,
		MatchedMapPoints: s.matchedMap,
		FrameWidth:       s.frameW,
		FrameHeight:      s.frameH,
	}
}

func publicPose(p poseState) Pose {
	return Pose{
		At: p.at, Position: p.pos,
		YawDeg: p.yaw * 180 / math.Pi, PitchDeg: p.pitch * 180 / math.Pi, RollDeg: p.roll * 180 / math.Pi,
		Metric: p.metric, Confidence: clamp01(p.confidence), TrackingQuality: clamp01(p.quality),
	}
}

func matchFeatures(s *scene, features []vision.SpatialFeature, motion vision.Motion) ([]featureMatch, []vision.SpatialFeature) {
	type candidate struct {
		ti   int
		fi   int
		cost float64
	}
	tracks := make([]*track, 0, len(s.tracks))
	for _, t := range s.tracks {
		if t.missed <= 2 {
			tracks = append(tracks, t)
		}
	}
	candidates := make([]candidate, 0, len(tracks)*2)
	dx, dy := 0.0, 0.0
	if motion.Compared {
		dx, dy = motion.DX, motion.DY
	}
	gate := 0.065
	if motion.Compared {
		gate += 0.035 * (1 - clamp01(motion.Confidence))
	}
	for ti, t := range tracks {
		px, py := t.x+dx, t.y+dy
		for fi, f := range features {
			sx, sy := f.X-px, f.Y-py
			dist := math.Hypot(sx, sy)
			if dist > gate {
				continue
			}
			ham := bits.OnesCount64(t.signature ^ f.Signature)
			if ham > 24 {
				continue
			}
			candidates = append(candidates, candidate{ti: ti, fi: fi, cost: dist*3 + float64(ham)/96})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].cost < candidates[j].cost })
	usedT := make([]bool, len(tracks))
	usedF := make([]bool, len(features))
	matches := make([]featureMatch, 0, minInt(len(tracks), len(features)))
	for _, c := range candidates {
		if usedT[c.ti] || usedF[c.fi] {
			continue
		}
		usedT[c.ti], usedF[c.fi] = true, true
		t := tracks[c.ti]
		matches = append(matches, featureMatch{t: t, f: features[c.fi], oldX: t.x, oldY: t.y, cost: c.cost})
	}
	for _, t := range s.tracks {
		t.missed++
	}
	unmatched := make([]vision.SpatialFeature, 0)
	for i, f := range features {
		if !usedF[i] {
			unmatched = append(unmatched, f)
		}
	}
	return matches, unmatched
}

func applyMatches(s *scene, matches []featureMatch) {
	for _, m := range matches {
		t := m.t
		t.x, t.y = m.f.X, m.f.Y
		t.score = m.f.Score
		if t.signature == 0 {
			t.signature = m.f.Signature
		} else {
			// Keep the stable signature. The live signature is intentionally not
			// averaged bit-by-bit because BRIEF bits are categorical.
			if bits.OnesCount64(t.signature^m.f.Signature) > 20 && t.age < 4 {
				t.signature = m.f.Signature
			}
		}
		t.age++
		t.missed = 0
		if t.pointID > 0 {
			if p := s.points[t.pointID]; p != nil {
				p.observations++
			}
		}
	}
}

type similarityEstimate struct {
	valid      bool
	scale      float64
	rotation   float64
	confidence float64
}

func similarity(matches []featureMatch) similarityEstimate {
	if len(matches) < 5 {
		return similarityEstimate{}
	}
	var ox, oy, nx, ny float64
	for _, m := range matches {
		ox += m.oldX
		oy += m.oldY
		nx += m.f.X
		ny += m.f.Y
	}
	n := float64(len(matches))
	ox, oy, nx, ny = ox/n, oy/n, nx/n, ny/n
	var aa, bb, denom float64
	for _, m := range matches {
		x1, y1 := m.oldX-ox, m.oldY-oy
		x2, y2 := m.f.X-nx, m.f.Y-ny
		aa += x1*x2 + y1*y2
		bb += x1*y2 - y1*x2
		denom += x1*x1 + y1*y1
	}
	if denom < 1e-8 {
		return similarityEstimate{}
	}
	a, b := aa/denom, bb/denom
	scaleV := math.Hypot(a, b)
	if scaleV < 0.5 || scaleV > 1.5 {
		return similarityEstimate{}
	}
	rot := math.Atan2(b, a)
	var errSum float64
	for _, m := range matches {
		x1, y1 := m.oldX-ox, m.oldY-oy
		px := nx + a*x1 - b*y1
		py := ny + b*x1 + a*y1
		errSum += math.Hypot(m.f.X-px, m.f.Y-py)
	}
	errMean := errSum / n
	conf := clamp01(float64(len(matches))/28) * clamp01(1-errMean/0.04)
	return similarityEstimate{valid: true, scale: scaleV, rotation: rot, confidence: conf}
}

func associateImportedPoints(s *scene) {
	if len(s.points) == 0 {
		return
	}
	used := map[int]bool{}
	for _, t := range s.tracks {
		if t.pointID > 0 || t.missed != 0 {
			if t.pointID > 0 {
				used[t.pointID] = true
			}
			continue
		}
		bestID, bestHam, second := 0, 65, 65
		for id, p := range s.points {
			if used[id] {
				continue
			}
			h := bits.OnesCount64(t.signature ^ p.signature)
			if h < bestHam {
				second = bestHam
				bestHam, bestID = h, id
			} else if h < second {
				second = h
			}
		}
		if bestID > 0 && bestHam <= 10 && (second-bestHam >= 3 || bestHam <= 5) {
			t.pointID = bestID
			used[bestID] = true
		}
	}
}

type lineObservation struct {
	point  Vec3
	ray    Vec3
	weight float64
}

func visibleMapObservations(s *scene) []lineObservation {
	if s.frameW <= 0 || s.frameH <= 0 {
		return nil
	}
	hfov, vfov := fovRadians(s.config.HorizontalFOVDeg, s.frameW, s.frameH)
	out := make([]lineObservation, 0, 32)
	for _, t := range s.tracks {
		if t.missed != 0 || t.pointID <= 0 {
			continue
		}
		p := s.points[t.pointID]
		if p == nil {
			continue
		}
		ray := rotate(screenRay(Vec2{X: t.x, Y: t.y}, hfov, vfov), s.pose.yaw, s.pose.pitch, s.pose.roll)
		out = append(out, lineObservation{point: p.position, ray: ray, weight: clamp(0.25+p.confidence*0.75, 0.25, 1)})
	}
	return out
}

func solvePosition(obs []lineObservation) (Vec3, bool) {
	var a [3][3]float64
	var b [3]float64
	totalWeight := 0.0
	for _, o := range obs {
		d := normalize(o.ray)
		w := o.weight
		if w <= 0 {
			w = 1
		}
		// A_i = I - d d^T. The camera position is the point minimizing
		// perpendicular distance to all landmark bearing lines.
		m := [3][3]float64{
			{1 - d.X*d.X, -d.X * d.Y, -d.X * d.Z},
			{-d.Y * d.X, 1 - d.Y*d.Y, -d.Y * d.Z},
			{-d.Z * d.X, -d.Z * d.Y, 1 - d.Z*d.Z},
		}
		p := [3]float64{o.point.X, o.point.Y, o.point.Z}
		for r := 0; r < 3; r++ {
			for c := 0; c < 3; c++ {
				a[r][c] += w * m[r][c]
				b[r] += w * m[r][c] * p[c]
			}
		}
		totalWeight += w
	}
	if totalWeight <= 0 {
		return Vec3{}, false
	}
	x, ok := solve3(a, b)
	if !ok {
		return Vec3{}, false
	}
	return Vec3{X: x[0], Y: x[1], Z: x[2]}, true
}

func triangulateTracks(s *scene, hfov, vfov float64) {
	if len(s.points) >= maxMapPoints || s.pose.confidence < 0.18 {
		return
	}
	for _, t := range s.tracks {
		if t.missed != 0 || t.pointID > 0 || t.age < 3 {
			continue
		}
		if t.anchor == nil {
			t.anchor = &trackAnchor{
				pos: s.pose.pos, yaw: s.pose.yaw, pitch: s.pose.pitch, roll: s.pose.roll,
				x: t.x, y: t.y,
			}
			continue
		}
		base := norm(sub(s.pose.pos, t.anchor.pos))
		if base < 1.0 {
			continue
		}
		r1 := rotate(screenRay(Vec2{X: t.anchor.x, Y: t.anchor.y}, hfov, vfov), t.anchor.yaw, t.anchor.pitch, t.anchor.roll)
		r2 := rotate(screenRay(Vec2{X: t.x, Y: t.y}, hfov, vfov), s.pose.yaw, s.pose.pitch, s.pose.roll)
		p, angle, miss, ok := triangulate(t.anchor.pos, r1, s.pose.pos, r2)
		if !ok || angle < 0.0087 || miss > math.Max(2.5, norm(sub(p, s.pose.pos))*0.04) {
			if base > math.Max(15, s.depthM*0.15) {
				t.anchor = &trackAnchor{pos: s.pose.pos, yaw: s.pose.yaw, pitch: s.pose.pitch, roll: s.pose.roll, x: t.x, y: t.y}
			}
			continue
		}
		if dot(sub(p, t.anchor.pos), r1) <= 0 || dot(sub(p, s.pose.pos), r2) <= 0 {
			continue
		}
		id := s.nextPoint
		s.nextPoint++
		conf := clamp01((angle/0.10)*0.55 + (1-clamp01(miss/5))*0.25 + s.pose.confidence*0.20)
		s.points[id] = &mapPointState{id: id, position: p, signature: t.signature, confidence: conf, observations: 2}
		t.pointID = id
		if len(s.points) >= maxMapPoints {
			return
		}
	}
}

func triangulate(c1, d1, c2, d2 Vec3) (Vec3, float64, float64, bool) {
	d1, d2 = normalize(d1), normalize(d2)
	b := dot(d1, d2)
	den := 1 - b*b
	if den < 1e-7 {
		return Vec3{}, 0, 0, false
	}
	w0 := sub(c1, c2)
	d := dot(d1, w0)
	e := dot(d2, w0)
	s := (b*e - d) / den
	t := (e - b*d) / den
	if s <= 0 || t <= 0 {
		return Vec3{}, 0, 0, false
	}
	p1 := add(c1, scale(d1, s))
	p2 := add(c2, scale(d2, t))
	p := scale(add(p1, p2), 0.5)
	angle := math.Acos(clamp(b, -1, 1))
	return p, angle, norm(sub(p1, p2)), true
}

func fovRadians(hDeg float64, w, h int) (float64, float64) {
	hfov := hDeg * math.Pi / 180
	aspect := float64(w) / float64(h)
	if aspect <= 0 {
		aspect = 16.0 / 9.0
	}
	vfov := 2 * math.Atan(math.Tan(hfov/2)/aspect)
	return hfov, vfov
}

func screenRay(p Vec2, hfov, vfov float64) Vec3 {
	x := (p.X - 0.5) * 2 * math.Tan(hfov/2)
	y := (0.5 - p.Y) * 2 * math.Tan(vfov/2)
	return normalize(Vec3{X: x, Y: y, Z: 1})
}

func rotate(v Vec3, yaw, pitch, roll float64) Vec3 {
	cy, sy := math.Cos(yaw), math.Sin(yaw)
	cp, sp := math.Cos(pitch), math.Sin(pitch)
	cr, sr := math.Cos(roll), math.Sin(roll)
	// R = Ry(yaw) * Rx(pitch) * Rz(roll), camera -> world.
	x1 := cr*v.X - sr*v.Y
	y1 := sr*v.X + cr*v.Y
	z1 := v.Z
	x2 := x1
	y2 := cp*y1 - sp*z1
	z2 := sp*y1 + cp*z1
	return Vec3{X: cy*x2 + sy*z2, Y: y2, Z: -sy*x2 + cy*z2}
}

func inverseRotate(v Vec3, yaw, pitch, roll float64) Vec3 {
	// Apply transposed rotation in reverse order.
	cy, sy := math.Cos(yaw), math.Sin(yaw)
	cp, sp := math.Cos(pitch), math.Sin(pitch)
	cr, sr := math.Cos(roll), math.Sin(roll)
	x1 := cy*v.X - sy*v.Z
	y1 := v.Y
	z1 := sy*v.X + cy*v.Z
	x2 := x1
	y2 := cp*y1 + sp*z1
	z2 := -sp*y1 + cp*z1
	return Vec3{X: cr*x2 + sr*y2, Y: -sr*x2 + cr*y2, Z: z2}
}

func solve3(a [3][3]float64, b [3]float64) ([3]float64, bool) {
	m := [3][4]float64{
		{a[0][0], a[0][1], a[0][2], b[0]},
		{a[1][0], a[1][1], a[1][2], b[1]},
		{a[2][0], a[2][1], a[2][2], b[2]},
	}
	for col := 0; col < 3; col++ {
		pivot := col
		for r := col + 1; r < 3; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(m[pivot][col]) < 1e-9 {
			return [3]float64{}, false
		}
		if pivot != col {
			m[pivot], m[col] = m[col], m[pivot]
		}
		div := m[col][col]
		for c := col; c < 4; c++ {
			m[col][c] /= div
		}
		for r := 0; r < 3; r++ {
			if r == col {
				continue
			}
			f := m[r][col]
			for c := col; c < 4; c++ {
				m[r][c] -= f * m[col][c]
			}
		}
	}
	return [3]float64{m[0][3], m[1][3], m[2][3]}, true
}

func add(a, b Vec3) Vec3           { return Vec3{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z} }
func sub(a, b Vec3) Vec3           { return Vec3{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z} }
func scale(v Vec3, s float64) Vec3 { return Vec3{X: v.X * s, Y: v.Y * s, Z: v.Z * s} }
func dot(a, b Vec3) float64        { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func norm(v Vec3) float64          { return math.Sqrt(dot(v, v)) }
func normalize(v Vec3) Vec3 {
	n := norm(v)
	if n <= 1e-12 {
		return Vec3{Z: 1}
	}
	return scale(v, 1/n)
}
func lerp(a, b Vec3, t float64) Vec3 {
	t = clamp01(t)
	return add(scale(a, 1-t), scale(b, t))
}
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func clamp01(v float64) float64 { return clamp(v, 0, 1) }
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
