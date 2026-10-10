package buildinfo

import (
	"strconv"
	"strings"
)

const (
	Version    = "0.2.14-alpha"
	Display    = "Alpha 0.2.14 SDK11"
	Repository = "pseudo6626/JACoB"
)

// CompareVersion compares dotted semantic-ish versions used by JACoB.
// Numeric core components are compared left to right, so future recovery
// versions such as 0.2.10.1 remain meaningful. A release without a prerelease
// suffix sorts after the same numeric version with a suffix.
func CompareVersion(a, b string) int {
	acore, apre := parseVersion(a)
	bcore, bpre := parseVersion(b)
	n := len(acore)
	if len(bcore) > n {
		n = len(bcore)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(acore) {
			av = acore[i]
		}
		if i < len(bcore) {
			bv = bcore[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
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
	return -1
}

func parseVersion(v string) ([]int, string) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V"))
	core, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		n, _ := strconv.Atoi(part)
		values = append(values, n)
	}
	for len(values) > 1 && values[len(values)-1] == 0 {
		values = values[:len(values)-1]
	}
	return values, strings.ToLower(pre)
}
