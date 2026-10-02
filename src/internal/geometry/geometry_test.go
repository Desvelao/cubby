package geometry

import (
	"math"
	"testing"

	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func rect(x, y, w, d float64) pack.Rect {
	return pack.Rect{X: x, Y: y, W: w, D: d}
}

func TestBuildOutlineNoNotches(t *testing.T) {
	pts := BuildOutline(100, 40, nil)
	if len(pts) != 4 {
		t.Fatalf("expected 4 points for a plain rectangle, got %d", len(pts))
	}
}

func TestBuildOutlineBottomNotch(t *testing.T) {
	pts := BuildOutline(100, 40, []Notch{{Pos: 40, Width: 3, Edge: "bottom"}})
	// plain rectangle (4) + one notch (4 extra points from the detour)
	if len(pts) != 8 {
		t.Fatalf("expected 8 points, got %d: %+v", len(pts), pts)
	}
	// the detour must reach exactly half the panel height
	found := false
	for _, p := range pts {
		if p.Y == 20 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a point at half-height (20), got %+v", pts)
	}
}

func TestBuildOutlineTopNotch(t *testing.T) {
	pts := BuildOutline(100, 40, []Notch{{Pos: 40, Width: 3, Edge: "top"}})
	if len(pts) != 8 {
		t.Fatalf("expected 8 points, got %d: %+v", len(pts), pts)
	}
}

func TestCellBoundariesAndRowBoundaryNotches(t *testing.T) {
	a := RowNode{Cells: []Cell{
		{Bounds: rect(0, 0, 30, 10)},
		{Bounds: rect(30, 0, 20, 10)},
	}}
	b := RowNode{Cells: []Cell{
		{Bounds: rect(0, 10, 50, 10)},
	}}
	xs := RowBoundaryPositions(a, b)
	if len(xs) != 1 || xs[0].X != 30 || !xs[0].Notch {
		t.Fatalf("expected a single notchable boundary at x=30, got %+v", xs)
	}
}

func TestRowBoundaryPositionsPlainNeighborSuppressesNotch(t *testing.T) {
	a := RowNode{Cells: []Cell{
		{Bounds: rect(0, 0, 30, 10), Compartment: pack.CompartmentResult{JointType: manifest.JointTypeNotch}},
		{Bounds: rect(30, 0, 20, 10), Compartment: pack.CompartmentResult{JointType: manifest.JointTypePlain}},
	}}
	b := RowNode{Cells: []Cell{
		{Bounds: rect(0, 10, 50, 10), Compartment: pack.CompartmentResult{JointType: manifest.JointTypeNotch}},
	}}
	xs := RowBoundaryPositions(a, b)
	if len(xs) != 1 || xs[0].X != 30 || xs[0].Notch {
		t.Fatalf("expected the x=30 boundary to be left plain since one neighbor is JointTypePlain, got %+v", xs)
	}
}

func TestDecomposeFaceAreaConservation(t *testing.T) {
	length, height := 100.0, 40.0
	notches := []Notch{
		{Pos: 20, Width: 3, Edge: "bottom"},
		{Pos: 60, Width: 3, Edge: "bottom"},
	}
	rects := DecomposeFace(length, height, notches)
	total := 0.0
	for _, r := range rects {
		total += r.W * r.H
	}
	expected := length*height - float64(len(notches))*3*(height/2)
	if math.Abs(total-expected) > 1e-9 {
		t.Fatalf("expected area %v, got %v (rects=%+v)", expected, total, rects)
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// shoelace returns the signed polygon area (positive when counter-clockwise).
func shoelace(pts []Point2D) float64 {
	s := 0.0
	for i, p := range pts {
		q := pts[(i+1)%len(pts)]
		s += p.X*q.Y - q.X*p.Y
	}
	return s / 2
}

func TestBuildRegionGroupsRowsAndOrdersCells(t *testing.T) {
	box := pack.BoxResult{
		InteriorW: 100, InteriorD: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "c", Bounds: rect(0, 30, 100, 30)},
			{ID: "b", Bounds: rect(40, 0, 60, 20)},
			{ID: "a", Bounds: rect(0, 0, 40, 25)},
		},
	}
	r := BuildRegion(box)
	if r.Bounds != rect(0, 0, 100, 60) {
		t.Fatalf("bad region bounds: %+v", r.Bounds)
	}
	if len(r.Rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(r.Rows), r.Rows)
	}
	if r.Rows[0].Y != 0 || r.Rows[1].Y != 30 {
		t.Fatalf("rows must be ordered by Y: %+v", r.Rows)
	}
	first := r.Rows[0]
	if len(first.Cells) != 2 || first.Cells[0].Compartment.ID != "a" || first.Cells[1].Compartment.ID != "b" {
		t.Fatalf("cells must be ordered left to right: %+v", first.Cells)
	}
	if first.Height != 25 {
		t.Fatalf("row height must be the tallest cell (25), got %v", first.Height)
	}
	if len(r.Rows[1].Cells) != 1 || r.Rows[1].Height != 30 {
		t.Fatalf("bad second row: %+v", r.Rows[1])
	}
}

func TestBuildRegionSnapTolerance(t *testing.T) {
	box := pack.BoxResult{
		InteriorW: 100, InteriorD: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "a", Bounds: rect(0, 10, 50, 20)},
			{ID: "b", Bounds: rect(50, 10+1e-9, 50, 20)}, // float noise: same row
			{ID: "c", Bounds: rect(0, 10.001, 50, 20)},   // real offset: own row
		},
	}
	r := BuildRegion(box)
	if len(r.Rows) != 2 {
		t.Fatalf("want noise merged and real offset split (2 rows), got %d: %+v", len(r.Rows), r.Rows)
	}
	if len(r.Rows[0].Cells) != 2 || len(r.Rows[1].Cells) != 1 {
		t.Fatalf("bad grouping: %+v", r.Rows)
	}
}

func TestBuildRegionEmpty(t *testing.T) {
	r := BuildRegion(pack.BoxResult{InteriorW: 10, InteriorD: 20})
	if len(r.Rows) != 0 || r.Bounds != rect(0, 0, 10, 20) {
		t.Fatalf("unexpected region for empty box: %+v", r)
	}
}

func TestSlotWidthIncludesKerf(t *testing.T) {
	if got := SlotWidth(manifest.Material{Thickness: 3}); got != 3 {
		t.Fatalf("no kerf: want 3, got %v", got)
	}
	if got := SlotWidth(manifest.Material{Thickness: 3, Kerf: 0.2}); !approx(got, 3.2) {
		t.Fatalf("with kerf: want 3.2, got %v", got)
	}
}

func TestPanelTo3DAxes(t *testing.T) {
	pos := Placement3D{OriginX: 10, OriginY: 20, OriginZ: 30}
	cases := []struct {
		axis    Axis
		x, y, z float64
	}{
		// local (length=1, height=2), thickness offset 3
		{AxisWidthRun, 11, 23, 32}, // length->X, thickness->Y, height->Z
		{AxisDepthRun, 13, 21, 32}, // length->Y, thickness->X, height->Z
		{AxisFloor, 11, 22, 33},    // length->X, height->Y, thickness->Z
	}
	for _, c := range cases {
		p := Panel{Axis: c.axis, Position: pos}
		x, y, z := p.To3D(1, 2, 3)
		if x != c.x || y != c.y || z != c.z {
			t.Errorf("%s: want (%v,%v,%v), got (%v,%v,%v)", c.axis, c.x, c.y, c.z, x, y, z)
		}
	}
}

// Notches on either edge, including flush with the panel ends (Pos 0 and
// Pos+Width == length), must give a rectangle set and an outline that agree
// on area, with no degenerate rectangles and a counter-clockwise outline.
func TestDecomposeFaceMatchesOutline(t *testing.T) {
	const length, height, w = 100.0, 40.0, 3.0
	cases := map[string][]Notch{
		"middle":     {{Pos: 40, Width: w}},
		"two":        {{Pos: 20, Width: w}, {Pos: 60, Width: w}},
		"unsorted":   {{Pos: 60, Width: w}, {Pos: 20, Width: w}},
		"at start":   {{Pos: 0, Width: w}},
		"at end":     {{Pos: length - w, Width: w}},
		"both ends":  {{Pos: 0, Width: w}, {Pos: length - w, Width: w}},
		"adjacent":   {{Pos: 10, Width: w}, {Pos: 10 + w, Width: w}},
		"all ends+m": {{Pos: 0, Width: w}, {Pos: 50, Width: w}, {Pos: length - w, Width: w}},
	}
	for _, edge := range []Edge{EdgeTop, EdgeBottom} {
		for name, base := range cases {
			notches := append([]Notch(nil), base...)
			for i := range notches {
				notches[i].Edge = edge
			}
			label := string(edge) + "/" + name

			want := length*height - float64(len(notches))*w*(height/2)
			rects := DecomposeFace(length, height, notches)
			total := 0.0
			for _, r := range rects {
				if r.W <= 0 || r.H <= 0 {
					t.Errorf("%s: degenerate rect %+v", label, r)
				}
				if r.X < -1e-9 || r.Y < -1e-9 || r.X+r.W > length+1e-9 || r.Y+r.H > height+1e-9 {
					t.Errorf("%s: rect outside face: %+v", label, r)
				}
				total += r.W * r.H
			}
			if !approx(total, want) {
				t.Errorf("%s: DecomposeFace area %v, want %v", label, total, want)
			}

			pts := BuildOutline(length, height, notches)
			area := shoelace(pts)
			if !approx(area, want) {
				t.Errorf("%s: outline area %v, want %v (pts=%+v)", label, area, want, pts)
			}
			if area <= 0 {
				t.Errorf("%s: outline is not counter-clockwise (signed area %v)", label, area)
			}
			for i, p := range pts {
				if p == pts[(i+1)%len(pts)] {
					t.Errorf("%s: zero-length outline edge at %d: %+v", label, i, pts)
				}
			}
		}
	}
}

func TestDecomposeFaceTopEdgeLayout(t *testing.T) {
	rects := DecomposeFace(100, 40, []Notch{{Pos: 40, Width: 3, Edge: EdgeTop}})
	want := []PanelRect{
		{X: 0, Y: 0, W: 100, H: 20},  // untouched bottom half
		{X: 0, Y: 20, W: 40, H: 20},  // tooth left of the notch
		{X: 43, Y: 20, W: 57, H: 20}, // tooth right of the notch
	}
	if len(rects) != len(want) {
		t.Fatalf("want %+v, got %+v", want, rects)
	}
	for i := range want {
		if rects[i] != want[i] {
			t.Errorf("rect %d: want %+v, got %+v", i, want[i], rects[i])
		}
	}
}

// Cells of one row may have different depths (group compartments are sized
// tightly). The row takes the deepest cell's depth; each Cell keeps its own
// Bounds, so the shorter cell does not reach the row's far edge.
func TestBuildRegionUnequalDepthRow(t *testing.T) {
	box := pack.BoxResult{
		InteriorW: 100, InteriorD: 100,
		Compartments: []pack.CompartmentResult{
			{ID: "tall", Bounds: rect(40, 0, 40, 60)},
			{ID: "short", Bounds: rect(0, 0, 40, 40)},
		},
	}
	r := BuildRegion(box)
	if len(r.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(r.Rows))
	}
	row := r.Rows[0]
	if row.Height != 60 {
		t.Errorf("row height = %v, want 60 (max cell depth)", row.Height)
	}
	if len(row.Cells) != 2 || row.Cells[0].Compartment.ID != "short" || row.Cells[0].Bounds.D != 40 {
		t.Errorf("cells should be ordered by X and keep their own depth, got %+v", row.Cells)
	}
	xs := cellBoundaries(row)
	if len(xs) != 1 || xs[0].X != 40 {
		t.Errorf("expected one boundary at x=40 regardless of depth, got %+v", xs)
	}
}
