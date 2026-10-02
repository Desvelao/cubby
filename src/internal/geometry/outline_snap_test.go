package geometry

import (
	"math"
	"testing"
)

func checkOutlineClean(t *testing.T, name string, pts []Point2D, length, height float64) {
	t.Helper()
	for i, p := range pts {
		if p.X < 0 || p.X > length || p.Y < 0 || p.Y > height {
			t.Errorf("%s: point %d %v outside [0,%v]x[0,%v]", name, i, p, length, height)
		}
		q := pts[(i+1)%len(pts)]
		if math.Hypot(q.X-p.X, q.Y-p.Y) <= notchEps {
			t.Errorf("%s: near-zero edge between %v and %v", name, p, q)
		}
	}
}

func TestOutlineSnapsFlushNotchBounds(t *testing.T) {
	type tc struct{ length, pos, width float64 }
	a, b := 0.2, 0.1 // variables: constant arithmetic would be exact
	cases := []tc{
		{0.3, 0.2, 0.1}, // Pos+Width is 1 ulp above length
		{0.3, a - 0.0, b},
		{0.7, 0.7 - b*3, b * 3},
		{10, 10 - 0.1 - 1e-10, 0.1}, // ends just inside length
		{10, 1e-10, 0.1},            // starts just after 0
		{10, -5e-10, 0.1},           // starts just before 0
	}
	if a+b == 0.3 {
		t.Fatal("expected 0.2+0.1 != 0.3 in floating point")
	}
	for _, c := range cases {
		for _, edge := range []Edge{EdgeBottom, EdgeTop} {
			n := []Notch{{Pos: c.pos, Width: c.width, Edge: edge}}
			if err := ValidateNotches(c.length, 0.4, n); err != nil {
				t.Fatalf("%+v: unexpected validation error: %v", c, err)
			}
			const height = 0.4
			checkOutlineClean(t, "BuildOutline", BuildOutline(c.length, height, n), c.length, height)
			checkOutlineClean(t, "BuildOutlineCut", BuildOutlineCut(c.length, height-0.1, 0.1, n), c.length, height-0.1)

			for _, cut := range []float64{0, 0.1} {
				h := height - cut
				depth := (h + cut) / 2
				rects := DecomposeFaceCut(c.length, h, cut, n)
				area := 0.0
				for _, r := range rects {
					if r.X < 0 || r.X+r.W > c.length || r.Y < 0 || r.Y+r.H > h {
						t.Errorf("%+v cut=%v: rect %+v outside panel", c, cut, r)
					}
					if r.W <= notchEps || r.H <= notchEps {
						t.Errorf("%+v cut=%v: degenerate rect %+v", c, cut, r)
					}
					area += r.W * r.H
				}
				wantArea := c.length*h - (math.Min(math.Max(c.pos+c.width, 0), c.length)-math.Max(c.pos, 0))*(depth)
				if edge == EdgeTop {
					wantArea = c.length*h - (math.Min(math.Max(c.pos+c.width, 0), c.length)-math.Max(c.pos, 0))*(h-depth)
				}
				if math.Abs(area-wantArea) > 1e-8 {
					t.Errorf("%+v edge=%s cut=%v: area %v want %v", c, edge, cut, area, wantArea)
				}
			}
		}
	}
}
