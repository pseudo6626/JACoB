package buildinfo

import "testing"

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.2.8-alpha", "0.2.3-alpha", 1},
		{"0.2.10-alpha", "0.2.9-alpha", 1},
		{"0.2.8", "0.2.8-alpha", 1},
		{"0.2.8-alpha", "0.2.8-alpha", 0},
		{"0.2.3-alpha", "0.2.8-alpha", -1},
	}
	for _, c := range cases {
		if got := CompareVersion(c.a, c.b); got != c.want {
			t.Fatalf("CompareVersion(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
