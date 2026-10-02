package export

import (
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/geometry"
)

func testPanel(id string, l, h float64) geometry.Panel {
	return geometry.Panel{ID: id, Length: l, Height: h}
}

func TestNestPanelsWithinSheetNoOverlapAndGap(t *testing.T) {
	panels := []geometry.Panel{
		testPanel("a", 100, 50), testPanel("b", 120, 40), testPanel("c", 90, 60),
		testPanel("d", 80, 30), testPanel("e", 100, 100),
	}
	n, err := NestPanels("svg", panels, 250, -1)
	if err != nil {
		t.Fatal(err)
	}
	if n.SheetWidth != 250 {
		t.Fatalf("SheetWidth = %g, want 250", n.SheetWidth)
	}
	if len(n.Placement) != len(panels) {
		t.Fatalf("placed %d, want %d", len(n.Placement), len(panels))
	}
	const eps = 1e-9
	for _, p := range panels {
		r := n.Placement[p.ID]
		if r.X < -eps || r.Y < -eps || r.X+p.Length > n.SheetWidth+eps || r.Y+p.Height > n.UsedHeight+eps {
			t.Errorf("%s out of sheet: %+v (used height %g)", p.ID, r, n.UsedHeight)
		}
	}
	for i, a := range panels {
		for _, b := range panels[i+1:] {
			ra, rb := n.Placement[a.ID], n.Placement[b.ID]
			// Separated on at least one axis by the full gap (NestGap).
			sepX := ra.X+a.Length+NestGap <= rb.X+eps || rb.X+b.Length+NestGap <= ra.X+eps
			sepY := ra.Y+a.Height+NestGap <= rb.Y+eps || rb.Y+b.Height+NestGap <= ra.Y+eps
			if !sepX && !sepY {
				t.Errorf("%s and %s overlap or violate the %gmm gap: %+v %+v", a.ID, b.ID, NestGap, ra, rb)
			}
		}
	}
}

func TestNestPanelsDefaultWidth(t *testing.T) {
	n, err := NestPanels("dxf", []geometry.Panel{testPanel("a", 10, 10)}, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if n.SheetWidth != DefaultSheetWidth {
		t.Fatalf("SheetWidth = %g, want %g", n.SheetWidth, DefaultSheetWidth)
	}
}

func TestNestPanelsOversize(t *testing.T) {
	_, err := NestPanels("dxf", []geometry.Panel{testPanel("big", 700, 650), testPanel("ok", 10, 10)}, 600, -1)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"dxf export:", "1 panel(s)", "600mm", "big (700x650mm", "--sheet-width to at least 655mm"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "ok (") {
		t.Errorf("fitting panel reported: %q", err)
	}
}

func TestNestPanelsEmpty(t *testing.T) {
	n, err := NestPanels("svg", nil, 100, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Placement) != 0 || n.UsedHeight != 0 {
		t.Fatalf("unexpected result for empty list: %+v", n)
	}
}

func TestNestPanelsRotatesOnlyTooWidePanels(t *testing.T) {
	panels := []geometry.Panel{testPanel("long", 700, 60), testPanel("a", 100, 50), testPanel("b", 200, 40)}
	n, err := NestPanels("svg", panels, 600, -1)
	if err != nil {
		t.Fatal(err)
	}
	if !n.Placement["long"].Rotated || n.Placement["a"].Rotated || n.Placement["b"].Rotated {
		t.Fatalf("rotation flags wrong: %+v", n.Placement)
	}
	type rc struct{ x0, y0, x1, y1 float64 }
	var rects []rc
	for _, p := range panels {
		pl := n.Placement[p.ID]
		w, h := p.Length, p.Height
		if pl.Rotated {
			w, h = h, w
		}
		if pl.W != w || pl.D != h {
			t.Errorf("%s footprint %gx%g, want %gx%g", p.ID, pl.W, pl.D, w, h)
		}
		if pl.X < 0 || pl.Y < 0 || pl.X+w > 600 || pl.Y+h > n.UsedHeight {
			t.Errorf("%s out of sheet: %+v", p.ID, pl)
		}
		rects = append(rects, rc{pl.X, pl.Y, pl.X + w, pl.Y + h})
	}
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			a, b := rects[i], rects[j]
			if a.x0 < b.x1 && b.x0 < a.x1 && a.y0 < b.y1 && b.y0 < a.y1 {
				t.Errorf("panels %d and %d overlap", i, j)
			}
		}
	}
}

func TestNestPanelsUnrotatedWhenFits(t *testing.T) {
	n, err := NestPanels("svg", []geometry.Panel{testPanel("a", 590, 20), testPanel("b", 20, 590)}, 600, -1)
	if err != nil {
		t.Fatal(err)
	}
	for id, pl := range n.Placement {
		if pl.Rotated {
			t.Errorf("%s rotated although it fits", id)
		}
	}
}

func TestPlacementOutlineRotation(t *testing.T) {
	p := geometry.Panel{ID: "p", Length: 10, Height: 4, Outline: []geometry.Point2D{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 4}, {X: 0, Y: 4}}}
	got := Placement{Rotated: true}.Outline(p)
	want := []geometry.Point2D{{X: 4, Y: 0}, {X: 4, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pt %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestNestPanelsGap(t *testing.T) {
	panels := []geometry.Panel{testPanel("a", 100, 60), testPanel("b", 100, 60)}
	for _, tc := range []struct{ gap, wantX float64 }{{-1, 107.5}, {5, 107.5}, {0, 100}, {20, 130}} {
		n, err := NestPanels("svg", panels, 0, tc.gap)
		if err != nil {
			t.Fatal(err)
		}
		if got := n.Placement["b"].X; got != tc.wantX {
			t.Errorf("gap %g: b.X = %g, want %g", tc.gap, got, tc.wantX)
		}
	}
	if _, err := NestPanels("svg", []geometry.Panel{testPanel("w", 100, 100)}, 110, 20); err == nil || !strings.Contains(err.Error(), "at least 120mm") {
		t.Errorf("error should reflect the gap, got %v", err)
	}
	if GapOrDefault(nil) != NestGap {
		t.Error("nil gap should default")
	}
}

func TestNestPanelsDuplicateIDErrors(t *testing.T) {
	panels := []geometry.Panel{testPanel("a", 10, 10), testPanel("b", 20, 10), testPanel("a", 30, 10)}
	_, err := NestPanels("svg", panels, 0, -1)
	if err == nil {
		t.Fatal("expected duplicate ID error")
	}
	if msg := err.Error(); !strings.HasPrefix(msg, "svg export:") || !strings.Contains(msg, `"a"`) {
		t.Errorf("error should be format-prefixed and name the ID: %v", err)
	}
}

func shoelaceArea(pts []geometry.Point2D) float64 {
	var s float64
	for i, a := range pts {
		b := pts[(i+1)%len(pts)]
		s += a.X*b.Y - b.X*a.Y
	}
	return s / 2
}

// TestNestPanelsHeightReducedNotched nests a panel whose top was trimmed by
// Cut (Height is already reduced, notches keep the full-height depth) with
// notches on its top or bottom edge. The sheet footprint uses the reduced
// height, and the rotated outline must stay inside it, keep its area and its
// counter-clockwise orientation.
func TestNestPanelsHeightReducedNotched(t *testing.T) {
	// Full height 60, cut 20: reduced height 40, notch depth 30. A top notch
	// opens in the trimmed edge so it only removes 3x10; a bottom one 3x30.
	for _, tc := range []struct {
		edge      geometry.Edge
		notchArea float64
	}{
		{geometry.EdgeTop, 3 * 10},
		{geometry.EdgeBottom, 3 * 30},
	} {
		for _, sc := range []struct {
			name    string
			length  float64
			rotated bool
		}{
			{"fits sheet", 500, false},
			{"rotated to fit sheet", 700, true},
		} {
			t.Run(string(tc.edge)+"/"+sc.name, func(t *testing.T) {
				notches := []geometry.Notch{{Pos: 100, Width: 3, Edge: tc.edge}, {Pos: 400, Width: 3, Edge: tc.edge}}
				p := geometry.Panel{ID: "cut", Length: sc.length, Height: 40, Cut: 20, Thickness: 3, Notches: notches}
				want := sc.length*40 - 2*tc.notchArea
				n, err := NestPanels("svg", []geometry.Panel{p}, 600, -1)
				if err != nil {
					t.Fatal(err)
				}
				pl := n.Placement["cut"]
				if pl.Rotated != sc.rotated {
					t.Fatalf("Rotated = %v, want %v", pl.Rotated, sc.rotated)
				}
				w, d := p.Length, p.Height
				if sc.rotated {
					w, d = d, w
				}
				if pl.W != w || pl.D != d {
					t.Fatalf("footprint %gx%g, want %gx%g", pl.W, pl.D, w, d)
				}
				pts := pl.Outline(p)
				minX, minY, maxX, maxY := 1e9, 1e9, -1e9, -1e9
				for _, pt := range pts {
					minX, maxX = min(minX, pt.X), max(maxX, pt.X)
					minY, maxY = min(minY, pt.Y), max(maxY, pt.Y)
				}
				if minX != 0 || minY != 0 || maxX != w || maxY != d {
					t.Errorf("outline bbox (%g,%g)-(%g,%g), want (0,0)-(%g,%g)", minX, minY, maxX, maxY, w, d)
				}
				if got := shoelaceArea(pts); got != want {
					t.Errorf("outline signed area = %g, want %g (counter-clockwise, area preserved)", got, want)
				}
			})
		}
	}
}

func TestNestPanelsGapBoundaries(t *testing.T) {
	const sheet, gap = 600.0, 5.0
	tests := []struct {
		name        string
		l, h        float64
		wantErr     bool
		wantRotated bool
	}{
		{"length == sheet-gap", 595, 20, false, false},
		{"length == sheet, thin rotates", 600, 20, false, true},
		{"length == sheet, height >= length", 600, 600, true, false},
		{"length between, thin rotates", 597, 20, false, true},
		{"length between, height >= length", 597, 700, true, false},
		{"square-ish between", 597, 598, true, false},
		{"square-ish fits padded", 595, 595, false, false},
		{"long thin rotates", 700, 20, false, true},
		{"tall stays unrotated", 20, 700, false, false},
		{"tall exactly rotated sheet-gap", 20, 595, false, false},
		{"wide rotates to padded width", 700, 595, false, true},
		{"wide rotated only fits unpadded", 700, 600, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n, err := NestPanels("svg", []geometry.Panel{testPanel("p", tc.l, tc.h)}, sheet, gap)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), "at least") {
					t.Errorf("error %q lacks sheet-width hint", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			pl := n.Placement["p"]
			if pl.Rotated != tc.wantRotated {
				t.Errorf("Rotated = %v, want %v", pl.Rotated, tc.wantRotated)
			}
			if pl.X < 0 || pl.X+pl.W > sheet {
				t.Errorf("out of sheet: %+v", pl)
			}
		})
	}
}

func TestNestPanelsGapOnlyErrorExplainsGap(t *testing.T) {
	_, err := NestPanels("svg", []geometry.Panel{testPanel("p", 600, 700)}, 600, 5)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"at least 605mm", "fit without the 5mm gap", "--nest-gap to at most 0mm"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	// Zero gap: the same panel fits exactly.
	if _, err := NestPanels("svg", []geometry.Panel{testPanel("p", 600, 700)}, 600, 0); err != nil {
		t.Errorf("gap 0: %v", err)
	}
}
