//go:build linux

package vision

import "testing"

func TestParseTesseractTSVAcceptsDecimalConfidence(t *testing.T) {
	data := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t10\t20\t60\t18\t96.437218\tTEST\n",
	)
	got := parseTesseractTSV(data, 200, 100)
	if got.Text != "TEST" {
		t.Fatalf("expected TEST, got %q", got.Text)
	}
	if len(got.Lines) != 1 || len(got.Lines[0].Words) != 1 {
		t.Fatalf("expected one recognized word, got %#v", got.Lines)
	}
}
