package slotted

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
	"github.com/Desvelao/cubby/internal/render"
)

func panelsByID(panels []geometry.Panel) map[string]geometry.Panel {
	m := map[string]geometry.Panel{}
	for _, p := range panels {
		m[p.ID] = p
	}
	return m
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// checkReduced asserts a panel's height, its recorded cut (full height minus
// height, when the panel is trimmed) and that its outline stays inside it.
func checkReduced(t *testing.T, p geometry.Panel, wantH, wantCut float64) {
	t.Helper()
	if !near(p.Height, wantH) {
		t.Errorf("%s: height %v, want %v", p.ID, p.Height, wantH)
	}
	if p.Axis != geometry.AxisFloor && !near(p.Cut, wantCut) && p.Cut != 0 {
		t.Errorf("%s: cut %v, want %v", p.ID, p.Cut, wantCut)
	}
	for _, pt := range p.Outline {
		if pt.Y > p.Height+1e-9 {
			t.Errorf("%s: outline point %+v above panel height %v", p.ID, pt, p.Height)
		}
	}
}

func assertReductionError(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", substr)
	}
	var re *pack.ReductionError
	if !errors.As(err, &re) {
		t.Errorf("error %T is not a *pack.ReductionError: %v", err, err)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err, substr)
	}
}

// A panel shared by several trays takes the largest reduction among them,
// whether it is given as an amount or a percentage.
func TestRenderSharedPanelLargestReductionWins(t *testing.T) {
	mat := manifest.Material{Thickness: 3}
	tests := []struct {
		name         string
		a, b, c      pack.HeightReduction
		wantH, wantV float64
	}{
		// h-1 spans rows 0 and 1 (a, b, c); v-1 sits between a and b only.
		{"amounts", pack.HeightReduction{AmountMM: 5}, pack.HeightReduction{AmountMM: 10}, pack.HeightReduction{AmountMM: 15}, 45, 50},
		{"percent beats smaller amount", pack.HeightReduction{Percent: 10}, pack.HeightReduction{AmountMM: 4}, pack.HeightReduction{}, 54, 54},
		{"amount beats smaller percent", pack.HeightReduction{Percent: 10}, pack.HeightReduction{AmountMM: 8}, pack.HeightReduction{}, 52, 52},
		{"unreduced neighbour does not cancel", pack.HeightReduction{}, pack.HeightReduction{AmountMM: 7}, pack.HeightReduction{}, 53, 53},
		{"panel filter limits each tray", pack.HeightReduction{AmountMM: 20, Panels: []string{"h-*"}}, pack.HeightReduction{AmountMM: 5}, pack.HeightReduction{}, 40, 55},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			box := twoRowBox()
			box.Compartments[0].DividerReduction = tc.a
			box.Compartments[1].DividerReduction = tc.b
			box.Compartments[2].DividerReduction = tc.c
			panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got := panelsByID(panels)
			checkReduced(t, got["h-1"], tc.wantH, 60-tc.wantH)
			checkReduced(t, got["v-1"], tc.wantV, 60-tc.wantV)
		})
	}
}

// DividerReduction must not touch boundary walls and ExternalReduction must
// not touch dividers.
func TestRenderReductionClassesAreIndependent(t *testing.T) {
	box := fullWallsBox(100, 50, []pack.CompartmentResult{
		fwCell("a", 0, 0, 50, true), fwCell("b", 50, 0, 50, true),
	})
	box.Compartments[0].ExternalReduction = pack.HeightReduction{AmountMM: 10}
	box.Compartments[1].DividerReduction = pack.HeightReduction{AmountMM: 4}
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range panels {
		want := 60.0
		switch {
		case p.ID == "v-1":
			want = 56 // divider reduction only
		case p.Axis == geometry.AxisWidthRun && p.Position.OriginX == 0:
			want = 50 // cell a's front/back walls
		case p.Axis == geometry.AxisDepthRun && p.Position.OriginX == 0:
			want = 50 // cell a's left wall
		}
		if !near(p.Height, want) {
			t.Errorf("%s (x=%v): height %v, want %v", p.ID, p.Position.OriginX, p.Height, want)
		}
	}
}

// FullWalls boundary walls (boundaryPanel) use the cell's ExternalReduction.
func TestRenderFullWallsBoundaryExternalReduction(t *testing.T) {
	mat := manifest.Material{Thickness: 3}
	tests := []struct {
		name string
		red  pack.HeightReduction
		want map[string]float64 // axis-kind -> height; "w" width-run, "wv" depth-run
	}{
		{"amount all", pack.HeightReduction{AmountMM: 10}, map[string]float64{"w": 50, "wv": 50}},
		{"percent all", pack.HeightReduction{Percent: 25}, map[string]float64{"w": 45, "wv": 45}},
		{"only side walls", pack.HeightReduction{AmountMM: 10, Panels: []string{"wv-*"}}, map[string]float64{"w": 60, "wv": 50}},
		{"only front/back walls", pack.HeightReduction{AmountMM: 10, Panels: []string{"w-*"}}, map[string]float64{"w": 50, "wv": 60}},
		{"none", pack.HeightReduction{}, map[string]float64{"w": 60, "wv": 60}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cell := fwCell("a", 0, 0, 100, true)
			cell.ExternalReduction = tc.red
			box := fullWallsBox(100, 50, []pack.CompartmentResult{cell})
			panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(panels) != 4 {
				t.Fatalf("want 4 boundary walls, got %d", len(panels))
			}
			for _, p := range panels {
				kind := "w"
				if strings.HasPrefix(p.ID, "wv-") {
					kind = "wv"
				}
				want := tc.want[kind]
				checkReduced(t, p, want, 60-want)
				if len(p.Notches) == 0 {
					t.Errorf("%s: expected notches to be kept", p.ID)
				}
			}
		})
	}
}

// Each FullWalls cell reduces only its own boundary walls.
func TestRenderFullWallsBoundaryPerCellReduction(t *testing.T) {
	a := fwCell("a", 0, 0, 50, true)
	a.ExternalReduction = pack.HeightReduction{AmountMM: 5}
	b := fwCell("b", 50, 0, 50, true)
	b.ExternalReduction = pack.HeightReduction{AmountMM: 12}
	c := fwCell("c", 100, 0, 50, false) // not FullWalls: reduction has no walls to act on
	c.ExternalReduction = pack.HeightReduction{AmountMM: 20}
	panels, err := Backend{}.Render(fullWallsBox(150, 50, []pack.CompartmentResult{a, b, c}), manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	seenA, seenB := 0, 0
	for _, p := range panels {
		switch {
		case strings.HasPrefix(p.ID, "v-"):
			checkReduced(t, p, 60, 0)
		case p.Axis == geometry.AxisWidthRun && p.Position.OriginX == 0, p.Axis == geometry.AxisDepthRun && p.Position.OriginX == 0:
			checkReduced(t, p, 55, 5)
			seenA++
		case p.Axis == geometry.AxisWidthRun && p.Position.OriginX == 50:
			checkReduced(t, p, 48, 12)
			seenB++
		default:
			t.Errorf("unexpected panel %s at x=%v", p.ID, p.Position.OriginX)
		}
	}
	if seenA != 3 || seenB != 2 {
		t.Errorf("cell a walls %d (want 3), cell b walls %d (want 2)", seenA, seenB)
	}
}

// Boundary walls too short for their reduction: cut >= height errors, and a
// notched wall errors from half the height on.
func TestRenderFullWallsBoundaryReductionErrors(t *testing.T) {
	mat := manifest.Material{Thickness: 3}
	tests := []struct {
		name   string
		red    pack.HeightReduction
		joint  manifest.JointType
		substr string // "" means no error
	}{
		{"notched at half height", pack.HeightReduction{AmountMM: 30}, "", "half"},
		{"notched above half height", pack.HeightReduction{AmountMM: 45}, "", "half"},
		{"notched percent at half", pack.HeightReduction{Percent: 50}, "", "half"},
		{"notched just under half", pack.HeightReduction{AmountMM: 29}, "", ""},
		{"whole height", pack.HeightReduction{AmountMM: 60}, "", "leaves nothing"},
		{"over whole height plain joint", pack.HeightReduction{AmountMM: 70}, manifest.JointTypePlain, "leaves nothing"},
		{"plain joint skips half rule", pack.HeightReduction{AmountMM: 40}, manifest.JointTypePlain, ""},
		{"filter excluding all walls", pack.HeightReduction{AmountMM: 40, Panels: []string{"nope-*"}}, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cell := fwCell("a", 0, 0, 100, true)
			cell.ExternalReduction = tc.red
			cell.JointType = tc.joint
			_, err := Backend{}.Render(fullWallsBox(100, 50, []pack.CompartmentResult{cell}), mat, render.RenderOptions{})
			if tc.substr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertReductionError(t, err, tc.substr)
		})
	}
}

// Within-tray dividers (ie-v-/iv-/ih-) are trimmed by DividerReduction, on
// both the grid and removable-tray paths.
func TestRenderInternalDividerReduction(t *testing.T) {
	build := func(red pack.HeightReduction, removable bool) pack.BoxResult {
		cr := twoRowCompartment()
		cr.Rows[0].Items = []pack.PlacedItem{
			{ID: "a", Rect: pack.Rect{X: 1, Y: 1, W: 20, D: 13}},
			{ID: "b", Rect: pack.Rect{X: 25, Y: 1, W: 20, D: 13}},
		}
		cr.Expand = true
		cr.CoreW = 30
		cr.DividerReduction = red
		if removable {
			cr.Removable = true
			cr.WallT = 2
		}
		return pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 60, Compartments: []pack.CompartmentResult{cr}}
	}
	mat := manifest.Material{Thickness: 3}
	for _, removable := range []bool{false, true} {
		name := "grid"
		if removable {
			name = "removable"
		}
		t.Run(name, func(t *testing.T) {
			tests := []struct {
				name string
				red  pack.HeightReduction
				want map[string]float64
			}{
				{"amount all", pack.HeightReduction{AmountMM: 10}, map[string]float64{"ie-v-tray": 50, "iv-tray-1-1": 50, "ih-tray-1": 50}},
				{"percent all", pack.HeightReduction{Percent: 20}, map[string]float64{"ie-v-tray": 48, "iv-tray-1-1": 48, "ih-tray-1": 48}},
				{"only column dividers", pack.HeightReduction{AmountMM: 10, Panels: []string{"iv-*"}}, map[string]float64{"iv-tray-1-1": 50}},
				{"only row divider", pack.HeightReduction{AmountMM: 10, Panels: []string{"ih-*"}}, map[string]float64{"ih-tray-1": 50}},
				{"only expansion divider", pack.HeightReduction{AmountMM: 10, Panels: []string{"ie-*"}}, map[string]float64{"ie-v-tray": 50}},
				// Plain (un-notched) dividers may be cut past half height.
				{"past half height", pack.HeightReduction{AmountMM: 40}, map[string]float64{"ie-v-tray": 20, "iv-tray-1-1": 20, "ih-tray-1": 20}},
			}
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					panels, err := Backend{}.Render(build(tc.red, removable), mat, render.RenderOptions{})
					if err != nil {
						t.Fatal(err)
					}
					got := panelsByID(panels)
					for _, id := range []string{"ie-v-tray", "iv-tray-1-1", "ih-tray-1"} {
						p, ok := got[id]
						if !ok {
							t.Fatalf("missing divider %s", id)
						}
						want, ok := tc.want[id]
						if !ok {
							want = 60
						}
						checkReduced(t, p, want, 60-want)
					}
				})
			}
		})
	}
}

// Removable-tray walls are not affected by DividerReduction, nor its
// dividers by ExternalReduction.
func TestRenderRemovableReductionClassesAreIndependent(t *testing.T) {
	cr := removableCompartment("t", 0, false)
	cr.Dividers = true
	cr.Rows = []pack.ShelfRow{{Y: 0, Height: 15, UsedW: 40}, {Y: 15, Height: 15, UsedW: 40}}
	cr.ExternalReduction = pack.HeightReduction{AmountMM: 10}
	cr.DividerReduction = pack.HeightReduction{AmountMM: 4}
	box := pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 40, Compartments: []pack.CompartmentResult{cr}}
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := panelsByID(panels)
	for _, id := range []string{"rw-t-front", "rw-t-back", "rv-t-left", "rv-t-right"} {
		checkReduced(t, got[id], 30, 10)
	}
	checkReduced(t, got["ih-t-1"], 36, 4)
}

// With a tray floor, the reduction is taken from the height above the floor
// (height - zBase), not from the full interior height.
func TestRenderReductionWithTrayFloor(t *testing.T) {
	mat := manifest.Material{Thickness: 3}
	t.Run("grid dividers", func(t *testing.T) {
		tests := []struct {
			name string
			red  pack.HeightReduction
			want float64
		}{
			{"amount", pack.HeightReduction{AmountMM: 10}, 47},
			{"percent of height above floor", pack.HeightReduction{Percent: 10}, 57 - 5.7},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				box := twoRowBox()
				box.Compartments[0].Floor = true
				for i := range box.Compartments {
					box.Compartments[i].DividerReduction = tc.red
				}
				panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
				if err != nil {
					t.Fatal(err)
				}
				n := 0
				for _, p := range panels {
					if p.Axis == geometry.AxisFloor {
						continue
					}
					n++
					checkReduced(t, p, tc.want, 57-tc.want)
					if p.Position.OriginZ != 3 {
						t.Errorf("%s: z %v, want 3", p.ID, p.Position.OriginZ)
					}
				}
				if n != 2 {
					t.Errorf("want 2 divider panels, got %d", n)
				}
			})
		}
	})
	t.Run("grid full walls external", func(t *testing.T) {
		cell := fwCell("a", 0, 0, 100, true)
		cell.Floor = true
		cell.ExternalReduction = pack.HeightReduction{Percent: 10}
		panels, err := Backend{}.Render(fullWallsBox(100, 50, []pack.CompartmentResult{cell}), mat, render.RenderOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range panels {
			if p.Axis == geometry.AxisFloor {
				continue
			}
			checkReduced(t, p, 57-5.7, 5.7)
		}
	})
	t.Run("removable external and divider", func(t *testing.T) {
		cr := removableCompartment("t", 0, true)
		cr.Dividers = true
		cr.Rows = []pack.ShelfRow{{Y: 0, Height: 15, UsedW: 40}, {Y: 15, Height: 15, UsedW: 40}}
		cr.ExternalReduction = pack.HeightReduction{Percent: 50} // of 40-3=37, not of 40
		cr.DividerReduction = pack.HeightReduction{Percent: 10}
		box := pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 40, Compartments: []pack.CompartmentResult{cr}}
		panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
		if err != nil {
			t.Fatal(err)
		}
		got := panelsByID(panels)
		for _, id := range []string{"rw-t-front", "rw-t-back", "rv-t-left", "rv-t-right"} {
			checkReduced(t, got[id], 18.5, 18.5)
		}
		checkReduced(t, got["ih-t-1"], 37-3.7, 3.7)
		if got["ih-t-1"].Position.OriginZ != 3 {
			t.Errorf("ih-t-1 z %v, want 3", got["ih-t-1"].Position.OriginZ)
		}
	})
	t.Run("amount that fits full height but not above the floor", func(t *testing.T) {
		box := twoRowBox()
		box.Compartments[0].Floor = true
		box.Compartments[0].DividerReduction = pack.HeightReduction{AmountMM: 58}
		box.Compartments[0].Dividers = true
		cr := removableCompartment("t", 0, true)
		cr.ExternalReduction = pack.HeightReduction{AmountMM: 38}
		_, err := Backend{}.Render(pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 40, Compartments: []pack.CompartmentResult{cr}}, mat, render.RenderOptions{})
		assertReductionError(t, err, "leaves nothing")
		// 58 < 60 passes without a floor but not 57 above one.
		if _, err := (Backend{}).Render(box, mat, render.RenderOptions{}); err == nil {
			t.Error("expected error: reduction exceeds the height above the floor")
		}
	})
}

// A reduction that removes the whole panel is rejected as a ReductionError on
// every path that builds panels.
func TestRenderReductionConsumesWholePanel(t *testing.T) {
	mat := manifest.Material{Thickness: 3}
	full := pack.HeightReduction{AmountMM: 60}
	tests := []struct {
		name string
		box  func() pack.BoxResult
	}{
		{"grid h/v dividers", func() pack.BoxResult {
			b := twoRowBox()
			for i := range b.Compartments {
				b.Compartments[i].DividerReduction = full
			}
			return b
		}},
		{"grid vertical divider only", func() pack.BoxResult {
			b := twoRowBox()
			b.Compartments[0].DividerReduction = pack.HeightReduction{AmountMM: 60, Panels: []string{"v-*"}}
			return b
		}},
		{"grid percent 100", func() pack.BoxResult {
			b := twoRowBox()
			b.Compartments[0].DividerReduction = pack.HeightReduction{Percent: 100}
			return b
		}},
		{"internal divider", func() pack.BoxResult {
			cr := twoRowCompartment()
			cr.DividerReduction = full
			return pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 60, Compartments: []pack.CompartmentResult{cr}}
		}},
		{"removable wall", func() pack.BoxResult {
			cr := removableCompartment("t", 0, false)
			cr.ExternalReduction = pack.HeightReduction{AmountMM: 60}
			return pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 60, Compartments: []pack.CompartmentResult{cr}}
		}},
		{"removable internal divider", func() pack.BoxResult {
			cr := removableCompartment("t", 0, false)
			cr.Dividers = true
			cr.Rows = []pack.ShelfRow{{Y: 0, Height: 15, UsedW: 40}, {Y: 15, Height: 15, UsedW: 40}}
			cr.DividerReduction = full
			return pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 60, Compartments: []pack.CompartmentResult{cr}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Backend{}.Render(tc.box(), mat, render.RenderOptions{})
			assertReductionError(t, err, "leaves nothing")
		})
	}
}

// A percent reduction applies to the box interior height.
func TestRenderReductionRelativeToInteriorHeight(t *testing.T) {
	box := twoRowBox()
	box.InteriorH = 40
	for i := range box.Compartments {
		box.Compartments[i].DividerReduction = pack.HeightReduction{Percent: 10}
	}
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range panels {
		checkReduced(t, p, 36, 4)
	}
}
