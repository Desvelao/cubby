package geometry

import (
	"reflect"
	"testing"
)

// cutNotchArea is the area removed from a panel of reduced height h (full
// height h+cut) by the given same-edge notches: they keep the full-height depth
// (h+cut)/2, measured from the bottom for bottom notches and, after the top is
// lowered, from the notch floor up to h for top notches.
func cutNotchArea(edge Edge, h, cut float64, notches []Notch) float64 {
	depth := (h + cut) / 2
	if edge == EdgeTop {
		depth = h - depth
	}
	total := 0.0
	for _, n := range notches {
		total += n.Width * depth
	}
	return total
}

func TestDecomposeFaceCutMatchesOutlineCut(t *testing.T) {
	const length, w = 100.0, 3.0
	cases := map[string][]Notch{
		"middle":    {{Pos: 40, Width: w}},
		"two":       {{Pos: 20, Width: w}, {Pos: 60, Width: w}},
		"unsorted":  {{Pos: 60, Width: w}, {Pos: 20, Width: w}},
		"at start":  {{Pos: 0, Width: w}},
		"at end":    {{Pos: length - w, Width: w}},
		"both ends": {{Pos: 0, Width: w}, {Pos: length - w, Width: w}},
		"adjacent":  {{Pos: 10, Width: w}, {Pos: 10 + w, Width: w}},
	}
	for _, cut := range []float64{5, 10} {
		const full = 40.0
		h := full - cut
		for _, edge := range []Edge{EdgeTop, EdgeBottom} {
			for name, base := range cases {
				notches := append([]Notch(nil), base...)
				for i := range notches {
					notches[i].Edge = edge
				}
				label := string(edge) + "/" + name
				want := length*h - cutNotchArea(edge, h, cut, notches)

				total := 0.0
				for _, r := range DecomposeFaceCut(length, h, cut, notches) {
					if r.W <= 0 || r.H <= 0 {
						t.Errorf("%s cut=%v: degenerate rect %+v", label, cut, r)
					}
					if r.X < -1e-9 || r.Y < -1e-9 || r.X+r.W > length+1e-9 || r.Y+r.H > h+1e-9 {
						t.Errorf("%s cut=%v: rect outside face: %+v", label, cut, r)
					}
					total += r.W * r.H
				}
				if !approx(total, want) {
					t.Errorf("%s cut=%v: rect area %v, want %v", label, cut, total, want)
				}

				pts := BuildOutlineCut(length, h+cut, cut, notches)
				area := shoelace(pts)
				if !approx(area, want) {
					t.Errorf("%s cut=%v: outline area %v, want %v (pts=%+v)", label, cut, area, want, pts)
				}
				for i, p := range pts {
					if p.Y > h+1e-9 {
						t.Errorf("%s cut=%v: point %d above lowered top: %+v", label, cut, i, p)
					}
					if p == pts[(i+1)%len(pts)] {
						t.Errorf("%s cut=%v: zero-length edge at %d: %+v", label, cut, i, pts)
					}
				}
			}
		}
	}
}

func TestCutNotchDepthIsFullHeightDepth(t *testing.T) {
	const length, h, cut, w = 100.0, 30.0, 10.0, 4.0
	depth := (h + cut) / 2 // 20, not h/2

	// Bottom notch: floor of the slot sits at the full-height depth.
	pts := BuildOutlineCut(length, h+cut, cut, []Notch{{Pos: 40, Width: w, Edge: EdgeBottom}})
	found := false
	for _, p := range pts {
		if p.X > 40 && p.X < 40+w {
			t.Errorf("unexpected vertex inside notch span: %+v", p)
		}
		if (p == Point2D{40, depth}) || (p == Point2D{40 + w, depth}) {
			found = true
		}
	}
	if !found {
		t.Errorf("bottom notch floor not at y=%v: %+v", depth, pts)
	}
	rects := DecomposeFaceCut(length, h, cut, []Notch{{Pos: 40, Width: w, Edge: EdgeBottom}})
	if rects[0] != (PanelRect{X: 0, Y: depth, W: length, H: h - depth}) {
		t.Errorf("bottom base rect = %+v, want y=%v h=%v", rects[0], depth, h-depth)
	}

	// Top notch: floor at the same height (depth from the full-height top).
	pts = BuildOutlineCut(length, h+cut, cut, []Notch{{Pos: 40, Width: w, Edge: EdgeTop}})
	found = false
	for _, p := range pts {
		if p == (Point2D{40, depth}) || p == (Point2D{40 + w, depth}) {
			found = true
		}
	}
	if !found {
		t.Errorf("top notch floor not at y=%v: %+v", depth, pts)
	}
	rects = DecomposeFaceCut(length, h, cut, []Notch{{Pos: 40, Width: w, Edge: EdgeTop}})
	if rects[0] != (PanelRect{X: 0, Y: 0, W: length, H: depth}) {
		t.Errorf("top base rect = %+v, want h=%v", rects[0], depth)
	}
}

func TestOutlinePolygonFlushNotchesOnlyTrueCorners(t *testing.T) {
	const length, w = 100.0, 3.0
	for _, cut := range []float64{0, 6} {
		const full = 40.0
		h := full - cut
		for _, edge := range []Edge{EdgeTop, EdgeBottom} {
			p := Panel{Length: length, Height: h, Cut: cut, Notches: []Notch{
				{Pos: 0, Width: w, Edge: edge},
				{Pos: length - w, Width: w, Edge: edge},
			}}
			poly := p.OutlinePolygon()
			label := string(edge)
			// A flush notch at each end adds a step (two corners) per end to
			// the four rectangle corners.
			if len(poly) != 8 {
				t.Errorf("%s cut=%v: want 8 corners, got %d: %+v", label, cut, len(poly), poly)
			}
			for i, pt := range poly {
				a, c := poly[(i+len(poly)-1)%len(poly)], poly[(i+1)%len(poly)]
				if pt == c {
					t.Errorf("%s cut=%v: duplicate point at %d: %+v", label, cut, i, poly)
				}
				if (pt.X-a.X)*(c.Y-a.Y)-(pt.Y-a.Y)*(c.X-a.X) == 0 {
					t.Errorf("%s cut=%v: collinear point %d: %+v", label, cut, i, poly)
				}
			}
			want := length*h - cutNotchArea(edge, h, cut, p.Notches)
			if got := shoelace(poly); !approx(got, want) {
				t.Errorf("%s cut=%v: polygon area %v, want %v", label, cut, got, want)
			}
		}
	}

	// Plain rectangle collapses to its four corners.
	rect := Panel{Length: 10, Height: 5}
	if got := rect.OutlinePolygon(); len(got) != 4 {
		t.Errorf("rectangle: want 4 corners, got %+v", got)
	}
}

func TestOutlinePointsFallback(t *testing.T) {
	notches := []Notch{{Pos: 20, Width: 3, Edge: EdgeTop}, {Pos: 60, Width: 3, Edge: EdgeTop}}
	base := Panel{Length: 100, Height: 30, Cut: 10, Notches: notches}
	want := BuildOutlineCut(100, 40, 10, notches)

	for name, outline := range map[string][]Point2D{
		"nil":       nil,
		"empty":     {},
		"one point": {{1, 1}},
		"two":       {{0, 0}, {1, 1}},
	} {
		p := base
		p.Outline = outline
		if got := p.OutlinePoints(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", name, got, want)
		}
	}

	custom := []Point2D{{0, 0}, {7, 0}, {7, 7}}
	p := base
	p.Outline = custom
	got := p.OutlinePoints()
	if !reflect.DeepEqual(got, custom) {
		t.Errorf("populated outline: got %+v, want %+v", got, custom)
	}
	if len(got) > 0 && &got[0] != &custom[0] {
		t.Errorf("populated outline should be returned as-is, not copied")
	}
}
