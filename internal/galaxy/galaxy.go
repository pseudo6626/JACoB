package galaxy

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	galaxyOriginX = -49985.0
	galaxyOriginY = -40985.0
	galaxyOriginZ = -24105.0
)

var proceduralNamePattern = regexp.MustCompile(`^(.+?)\s+([A-Za-z]{2})-([A-Za-z])\s+([a-hA-H])(?:(\d+)-(\d+)|(\d+))$`)

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type Bounds struct {
	Min Position `json:"min"`
	Max Position `json:"max"`
}

type ParsedSystemName struct {
	Raw        string `json:"raw"`
	RegionName string `json:"regionName"`
	Letters    string `json:"letters"`
	L1         int    `json:"l1"`
	L2         int    `json:"l2"`
	L3         int    `json:"l3"`
	MassCode   string `json:"massCode"`
	MassIndex  int    `json:"massIndex"`
	N1         uint64 `json:"n1"`
	N2         uint64 `json:"n2"`
	Sequence   uint64 `json:"sequence"`
	BoxelLabel string `json:"boxelLabel"`
}

type DecodedAddress struct {
	ID64      string    `json:"id64"`
	MassCode  string    `json:"massCode"`
	MassIndex int       `json:"massIndex"`
	EdgeLy    float64   `json:"edgeLy"`
	Sequence  uint64    `json:"sequence"`
	BodyID    uint64    `json:"bodyId"`
	BoxelID   string    `json:"boxelId"`
	Corner    Position  `json:"corner"`
	Center    Position  `json:"center"`
	Bounds    Bounds    `json:"bounds"`
	Grid10Ly  [3]uint64 `json:"grid10Ly"`
	RawGrid   [3]uint64 `json:"rawGrid"`
}

type EncodeRequest struct {
	MassCode  string    `json:"massCode,omitempty"`
	MassIndex *int      `json:"massIndex,omitempty"`
	Corner    *Position `json:"corner,omitempty"`
	Sequence  uint64    `json:"sequence,omitempty"`
	BodyID    uint64    `json:"bodyId,omitempty"`
}

type Cell struct {
	MassCode      string    `json:"massCode"`
	MassIndex     int       `json:"massIndex"`
	EdgeLy        float64   `json:"edgeLy"`
	Known         bool      `json:"known"`
	Exact         bool      `json:"exact"`
	Relation      string    `json:"relation"`
	Corner        *Position `json:"corner,omitempty"`
	Center        *Position `json:"center,omitempty"`
	Bounds        *Bounds   `json:"bounds,omitempty"`
	PossibleCells uint64    `json:"possibleCells,omitempty"`
	Reason        string    `json:"reason,omitempty"`
}

type BoxelInfo struct {
	Address DecodedAddress `json:"address"`
	Base    Cell           `json:"base"`
	Parents []Cell         `json:"parents"`
}

type HierarchyRequest struct {
	ID64     string    `json:"id64"`
	Position *Position `json:"position,omitempty"`
}

type HierarchyResult struct {
	ID64         string `json:"id64"`
	BaseMass     string `json:"baseMass"`
	BaseIndex    int    `json:"baseIndex"`
	PositionUsed bool   `json:"positionUsed"`
	Cells        []Cell `json:"cells"`
}

func ParseSystemName(name string) (ParsedSystemName, error) {
	raw := strings.TrimSpace(name)
	m := proceduralNamePattern.FindStringSubmatch(raw)
	if m == nil {
		return ParsedSystemName{}, errors.New("system name is not a supported procedural name")
	}
	mass := strings.ToLower(m[4])
	idx := int(mass[0] - 'a')
	if idx < 0 || idx > 7 {
		return ParsedSystemName{}, errors.New("invalid mass code")
	}
	var n1, n2 uint64
	var err error
	if m[5] != "" {
		n1, err = strconv.ParseUint(m[5], 10, 64)
		if err != nil {
			return ParsedSystemName{}, fmt.Errorf("invalid N1: %w", err)
		}
		n2, err = strconv.ParseUint(m[6], 10, 64)
	} else {
		n2, err = strconv.ParseUint(m[7], 10, 64)
	}
	if err != nil {
		return ParsedSystemName{}, fmt.Errorf("invalid sequence: %w", err)
	}
	letters := strings.ToUpper(m[2] + "-" + m[3])
	l1 := int(strings.ToUpper(m[2])[0] - 'A')
	l2 := int(strings.ToUpper(m[2])[1] - 'A')
	l3 := int(strings.ToUpper(m[3])[0] - 'A')
	label := fmt.Sprintf("%s %s %s", strings.TrimSpace(m[1]), letters, mass)
	if n1 != 0 {
		label += strconv.FormatUint(n1, 10)
	}
	return ParsedSystemName{
		Raw: raw, RegionName: strings.TrimSpace(m[1]), Letters: letters,
		L1: l1, L2: l2, L3: l3, MassCode: mass, MassIndex: idx,
		N1: n1, N2: n2, Sequence: n2, BoxelLabel: label,
	}, nil
}

func DecodeAddress(raw string) (DecodedAddress, error) {
	id, err := parseID64(raw)
	if err != nil {
		return DecodedAddress{}, err
	}
	m := int(id & 7)
	mass := string(rune('a' + m))
	zRaw := (id >> 3) & lowMask(14-m)
	yRaw := (id >> uint(17-m)) & lowMask(13-m)
	xRaw := (id >> uint(30-2*m)) & lowMask(14-m)
	scale := uint64(1) << uint(m)
	z10 := zRaw * scale
	y10 := yRaw * scale
	x10 := xRaw * scale
	shift := uint(44 - 3*m)
	seq := (id >> shift) & lowMask(11+3*m)
	body := (id >> 55) & 0x1ff
	boxelID := id & lowMask(int(shift))
	edge := float64(uint64(10) * (uint64(1) << uint(m)))
	corner := Position{
		X: galaxyOriginX + float64(x10)*10,
		Y: galaxyOriginY + float64(y10)*10,
		Z: galaxyOriginZ + float64(z10)*10,
	}
	bounds, center := geometry(corner, edge)
	return DecodedAddress{
		ID64: strconv.FormatUint(id, 10), MassCode: mass, MassIndex: m, EdgeLy: edge,
		Sequence: seq, BodyID: body, BoxelID: strconv.FormatUint(boxelID, 10),
		Corner: corner, Center: center, Bounds: bounds,
		Grid10Ly: [3]uint64{x10, y10, z10},
		RawGrid:  [3]uint64{xRaw, yRaw, zRaw},
	}, nil
}

func EncodeAddress(in EncodeRequest) (string, error) {
	m, err := massIndex(in.MassCode, in.MassIndex)
	if err != nil {
		return "", err
	}
	if in.Corner == nil {
		return "", errors.New("corner is required")
	}
	if in.BodyID > 0x1ff {
		return "", errors.New("bodyId exceeds 9-bit address field")
	}
	width := 11 + 3*m
	if in.Sequence > lowMask(width) {
		return "", fmt.Errorf("sequence exceeds %d-bit address field for mass code %c", width, 'a'+m)
	}
	scale := uint64(1) << uint(m)
	x10, err := cornerIndex(in.Corner.X, galaxyOriginX, scale, "x")
	if err != nil {
		return "", err
	}
	y10, err := cornerIndex(in.Corner.Y, galaxyOriginY, scale, "y")
	if err != nil {
		return "", err
	}
	z10, err := cornerIndex(in.Corner.Z, galaxyOriginZ, scale, "z")
	if err != nil {
		return "", err
	}
	xRaw, yRaw, zRaw := x10/scale, y10/scale, z10/scale
	if xRaw > lowMask(14-m) || yRaw > lowMask(13-m) || zRaw > lowMask(14-m) {
		return "", errors.New("corner is outside the normal Elite system-address grid")
	}
	id := uint64(m)
	id |= zRaw << 3
	id |= yRaw << uint(17-m)
	id |= xRaw << uint(30-2*m)
	id |= in.Sequence << uint(44-3*m)
	id |= in.BodyID << 55
	return strconv.FormatUint(id, 10), nil
}

func AddressForSequence(raw string, sequence uint64) (string, error) {
	id, err := parseID64(raw)
	if err != nil {
		return "", err
	}
	m := int(id & 7)
	width := 11 + 3*m
	if sequence > lowMask(width) {
		return "", fmt.Errorf("sequence exceeds %d-bit address field for mass code %c", width, 'a'+m)
	}
	shift := uint(44 - 3*m)
	mask := lowMask(width) << shift
	id = (id &^ mask) | (sequence << shift)
	return strconv.FormatUint(id, 10), nil
}

func Boxel(raw string) (BoxelInfo, error) {
	d, err := DecodeAddress(raw)
	if err != nil {
		return BoxelInfo{}, err
	}
	base := cellForCorner(d.Corner, d.MassIndex, "base", true)
	parents := make([]Cell, 0, 7-d.MassIndex)
	for m := d.MassIndex + 1; m <= 7; m++ {
		parents = append(parents, cellForPosition(d.Center, m, "parent", true))
	}
	return BoxelInfo{Address: d, Base: base, Parents: parents}, nil
}

func BoxelHierarchy(req HierarchyRequest) (HierarchyResult, error) {
	d, err := DecodeAddress(req.ID64)
	if err != nil {
		return HierarchyResult{}, err
	}
	cells := make([]Cell, 0, 8)
	for m := 7; m >= 0; m-- {
		relation := "parent"
		if m == d.MassIndex {
			relation = "base"
		} else if m < d.MassIndex {
			relation = "child"
		}
		if req.Position != nil {
			cell := cellForPosition(*req.Position, m, relation, true)
			if m == d.MassIndex {
				// The encoded base boxel is authoritative. A supplied position must lie inside it.
				if !contains(d.Bounds, *req.Position) {
					return HierarchyResult{}, errors.New("position does not lie inside the ID64 base boxel")
				}
				cell = cellForCorner(d.Corner, m, relation, true)
			}
			cells = append(cells, cell)
			continue
		}
		if m >= d.MassIndex {
			if m == d.MassIndex {
				cells = append(cells, cellForCorner(d.Corner, m, relation, true))
			} else {
				cells = append(cells, cellForPosition(d.Center, m, relation, true))
			}
			continue
		}
		diff := d.MassIndex - m
		possible := uint64(1) << uint(3*diff)
		cells = append(cells, Cell{
			MassCode: string(rune('a' + m)), MassIndex: m, EdgeLy: edgeForMass(m),
			Known: false, Exact: false, Relation: relation, PossibleCells: possible,
			Reason: "ID64 identifies only the base boxel; supply position to resolve finer child cells",
		})
	}
	return HierarchyResult{
		ID64: d.ID64, BaseMass: d.MassCode, BaseIndex: d.MassIndex,
		PositionUsed: req.Position != nil, Cells: cells,
	}, nil
}

func cellForPosition(p Position, m int, relation string, exact bool) Cell {
	edge := edgeForMass(m)
	c := Position{
		X: math.Floor((p.X-galaxyOriginX)/edge)*edge + galaxyOriginX,
		Y: math.Floor((p.Y-galaxyOriginY)/edge)*edge + galaxyOriginY,
		Z: math.Floor((p.Z-galaxyOriginZ)/edge)*edge + galaxyOriginZ,
	}
	return cellForCorner(c, m, relation, exact)
}

func cellForCorner(c Position, m int, relation string, exact bool) Cell {
	edge := edgeForMass(m)
	b, center := geometry(c, edge)
	return Cell{
		MassCode: string(rune('a' + m)), MassIndex: m, EdgeLy: edge,
		Known: true, Exact: exact, Relation: relation, Corner: &c, Center: &center, Bounds: &b,
	}
}

func geometry(c Position, edge float64) (Bounds, Position) {
	max := Position{X: c.X + edge, Y: c.Y + edge, Z: c.Z + edge}
	center := Position{X: c.X + edge/2, Y: c.Y + edge/2, Z: c.Z + edge/2}
	return Bounds{Min: c, Max: max}, center
}

func contains(b Bounds, p Position) bool {
	const eps = 1e-7
	return p.X >= b.Min.X-eps && p.X < b.Max.X+eps &&
		p.Y >= b.Min.Y-eps && p.Y < b.Max.Y+eps &&
		p.Z >= b.Min.Z-eps && p.Z < b.Max.Z+eps
}

func edgeForMass(m int) float64 { return float64(uint64(10) * (uint64(1) << uint(m))) }

func massIndex(code string, index *int) (int, error) {
	if strings.TrimSpace(code) != "" {
		c := strings.ToLower(strings.TrimSpace(code))
		if len(c) != 1 || c[0] < 'a' || c[0] > 'h' {
			return 0, errors.New("massCode must be a through h")
		}
		m := int(c[0] - 'a')
		if index != nil && *index != m {
			return 0, errors.New("massCode and massIndex disagree")
		}
		return m, nil
	}
	if index == nil || *index < 0 || *index > 7 {
		return 0, errors.New("massCode or massIndex is required")
	}
	return *index, nil
}

func cornerIndex(v, origin float64, scale uint64, axis string) (uint64, error) {
	raw := (v - origin) / 10
	rounded := math.Round(raw)
	if math.Abs(raw-rounded) > 1e-7 {
		return 0, fmt.Errorf("%s corner must align to the 10 ly address grid", axis)
	}
	if rounded < 0 {
		return 0, fmt.Errorf("%s corner is below the address-grid origin", axis)
	}
	i := uint64(rounded)
	if i%scale != 0 {
		return 0, fmt.Errorf("%s corner is not aligned to this mass-code boxel size", axis)
	}
	return i, nil
}

func parseID64(raw string) (uint64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, errors.New("id64 is required")
	}
	if strings.ContainsAny(s, ".eE+-") {
		return 0, errors.New("id64 must be an unsigned decimal integer string")
	}
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id64: %w", err)
	}
	return id, nil
}

func lowMask(bits int) uint64 {
	if bits <= 0 {
		return 0
	}
	if bits >= 64 {
		return ^uint64(0)
	}
	return (uint64(1) << uint(bits)) - 1
}
