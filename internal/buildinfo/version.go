package buildinfo

import (
	"strconv"
	"strings"
)

const (
	Version    = "0.2.8-alpha"
	Display    = "Alpha 0.2.8 SDK7 Hotfix4"
	Repository = "pseudo6626/JACoB"
)

// CompareVersion compares dotted semantic-ish versions used by JACoB.
// It returns 1 when a>b, -1 when a<b, and 0 when they are equivalent.
func CompareVersion(a, b string) int {
	amaj, amin, apat, apre := parseVersion(a)
	bmaj, bmin, bpat, bpre := parseVersion(b)
	for _, pair := range [][2]int{{amaj, bmaj}, {amin, bmin}, {apat, bpat}} {
		if pair[0] > pair[1] {
			return 1
		}
		if pair[0] < pair[1] {
			return -1
		}
	}
	if apre == bpre {
		return 0
	}
	if apre == "" {
		return 1
	}
	if bpre == "" {
		return -1
	}
	if apre > bpre {
		return 1
	}
	if apre < bpre {
		return -1
	}
	return 0
}

func parseVersion(v string) (int, int, int, string) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V"))
	core, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	vals := [3]int{}
	for i := 0; i < len(parts) && i < len(vals); i++ {
		vals[i], _ = strconv.Atoi(parts[i])
	}
	return vals[0], vals[1], vals[2], strings.ToLower(pre)
}
