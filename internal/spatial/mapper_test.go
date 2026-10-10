package spatial

import (
	"math"
	"testing"
)

func TestTriangulate(t *testing.T) {
	c1 := Vec3{X: 0, Y: 0, Z: 0}
	c2 := Vec3{X: 10, Y: 0, Z: 0}
	target := Vec3{X: 4, Y: 3, Z: 40}
	d1 := normalize(sub(target, c1))
	d2 := normalize(sub(target, c2))
	got, angle, miss, ok := triangulate(c1, d1, c2, d2)
	if !ok {
		t.Fatal("triangulation failed")
	}
	if angle <= 0 || miss > 1e-6 {
		t.Fatalf("angle=%v miss=%v", angle, miss)
	}
	if norm(sub(got, target)) > 1e-5 {
		t.Fatalf("got %+v want %+v", got, target)
	}
}

func TestSolvePositionFromBearings(t *testing.T) {
	camera := Vec3{X: 12, Y: -4, Z: 7}
	points := []Vec3{
		{X: -20, Y: 8, Z: 90},
		{X: 35, Y: 18, Z: 120},
		{X: 5, Y: -30, Z: 75},
		{X: -45, Y: -15, Z: 130},
	}
	obs := make([]lineObservation, 0, len(points))
	for _, p := range points {
		obs = append(obs, lineObservation{point: p, ray: normalize(sub(p, camera)), weight: 1})
	}
	got, ok := solvePosition(obs)
	if !ok {
		t.Fatal("position solve failed")
	}
	if norm(sub(got, camera)) > 1e-6 {
		t.Fatalf("got %+v want %+v", got, camera)
	}
}

func TestScreenRayProjectionRoundTrip(t *testing.T) {
	hfov, vfov := fovRadians(80, 1920, 1080)
	inputs := []Vec2{{X: 0.5, Y: 0.5}, {X: 0.2, Y: 0.7}, {X: 0.83, Y: 0.15}}
	for _, p := range inputs {
		r := screenRay(p, hfov, vfov)
		x := 0.5 + (r.X/r.Z)/(2*math.Tan(hfov/2))
		y := 0.5 - (r.Y/r.Z)/(2*math.Tan(vfov/2))
		if math.Abs(x-p.X) > 1e-9 || math.Abs(y-p.Y) > 1e-9 {
			t.Fatalf("round trip got (%v,%v) want (%v,%v)", x, y, p.X, p.Y)
		}
	}
}

func TestRotateInverseRotate(t *testing.T) {
	v := Vec3{X: 3, Y: -2, Z: 11}
	w := rotate(v, 0.7, -0.25, 0.13)
	got := inverseRotate(w, 0.7, -0.25, 0.13)
	if norm(sub(got, v)) > 1e-9 {
		t.Fatalf("got %+v want %+v", got, v)
	}
}
