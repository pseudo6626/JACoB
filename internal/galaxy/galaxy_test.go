package galaxy

import "testing"

func TestParseSystemName(t *testing.T) {
	p, err := ParseSystemName("Synuefe EN-H d11-96")
	if err != nil {
		t.Fatal(err)
	}
	if p.RegionName != "Synuefe" || p.L1 != 4 || p.L2 != 13 || p.L3 != 7 || p.MassCode != "d" || p.N1 != 11 || p.N2 != 96 {
		t.Fatalf("unexpected parse: %+v", p)
	}
}

func TestAddressRoundTrip(t *testing.T) {
	const id = "3309179996515"
	d, err := DecodeAddress(id)
	if err != nil {
		t.Fatal(err)
	}
	if d.MassCode != "d" || d.MassIndex != 3 || d.Sequence != 96 {
		t.Fatalf("unexpected decode: %+v", d)
	}
	if d.Corner.X != 735 || d.Corner.Y != -185 || d.Corner.Z != -105 {
		t.Fatalf("unexpected corner: %+v", d.Corner)
	}
	got, err := EncodeAddress(EncodeRequest{
		MassCode: d.MassCode, Corner: &d.Corner, Sequence: d.Sequence, BodyID: d.BodyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("round trip %s != %s", got, id)
	}
}

func TestAddressForSequence(t *testing.T) {
	next, err := AddressForSequence("3309179996515", 97)
	if err != nil {
		t.Fatal(err)
	}
	d, err := DecodeAddress(next)
	if err != nil {
		t.Fatal(err)
	}
	if d.Sequence != 97 || d.MassCode != "d" {
		t.Fatalf("unexpected sequence decode: %+v", d)
	}
	if d.Corner.X != 735 || d.Corner.Y != -185 || d.Corner.Z != -105 {
		t.Fatalf("boxel moved: %+v", d.Corner)
	}
}

func TestHierarchyNeedsPositionForChildren(t *testing.T) {
	h, err := BoxelHierarchy(HierarchyRequest{ID64: "3309179996515"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Cells) != 8 {
		t.Fatalf("cells=%d", len(h.Cells))
	}
	known := 0
	for _, c := range h.Cells {
		if c.Known {
			known++
		}
	}
	if known != 5 {
		t.Fatalf("known=%d want 5 (H..D)", known)
	}

	pos := Position{X: 760, Y: -150, Z: -80}
	h, err = BoxelHierarchy(HierarchyRequest{ID64: "3309179996515", Position: &pos})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range h.Cells {
		if !c.Known {
			t.Fatalf("expected known cell: %+v", c)
		}
	}
}
