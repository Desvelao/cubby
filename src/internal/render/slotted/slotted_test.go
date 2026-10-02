package slotted

import (
	"errors"
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
	"github.com/Desvelao/cubby/internal/render"
)

// Two rows: row0 has 2 side-by-side cells (1 interior boundary), row1 has 1
// cell spanning the row boundary. Expect: 1 horizontal panel (notched once,
// where row0's vertical divider meets it) + 1 vertical panel (notched once,
// at the end that meets the horizontal panel; its other end is open to the
// box wall so gets no notch).
func twoRowBox() pack.BoxResult {
	return pack.BoxResult{
		BoxName:   "Test",
		InteriorW: 300, InteriorD: 300, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "a", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 92, D: 142}},
			{ID: "b", Kind: "auto", Bounds: pack.Rect{X: 92, Y: 0, W: 92, D: 142}},
			{ID: "c", Kind: "group", Bounds: pack.Rect{X: 0, Y: 142, W: 268, D: 103}},
		},
	}
}

func TestRenderPanelAndNotchCounts(t *testing.T) {
	box := twoRowBox()
	mat := manifest.Material{Thickness: 3, Kerf: 0}

	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var horiz, vert int
	for _, p := range panels {
		switch p.Axis {
		case "width-run":
			horiz++
			if len(p.Notches) != 1 {
				t.Errorf("expected 1 notch on the horizontal panel, got %d: %+v", len(p.Notches), p.Notches)
			}
			if p.Notches[0].Pos != 92 {
				t.Errorf("expected notch at x=92, got %v", p.Notches[0].Pos)
			}
		case "depth-run":
			vert++
			if len(p.Notches) != 1 {
				t.Errorf("expected 1 notch on the vertical panel, got %d: %+v", len(p.Notches), p.Notches)
			}
			// The row0 panel has no row-1 divider of its own at x=92, so it
			// is lengthened by one thickness into the horizontal panel's
			// [142, 145] footprint and notched at that far end.
			if p.Length != 145 || p.Notches[0].Pos != 142 {
				t.Errorf("expected length 145 with the notch at pos=142, got length %v pos %v", p.Length, p.Notches[0].Pos)
			}
		}
	}
	if horiz != 1 || vert != 1 {
		t.Fatalf("expected 1 horizontal + 1 vertical panel, got %d horizontal, %d vertical", horiz, vert)
	}
}

func TestRenderPanelHeightDefaultsToBoxInterior(t *testing.T) {
	box := twoRowBox()
	mat := manifest.Material{Thickness: 3}
	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range panels {
		if p.Height != 60 {
			t.Fatalf("expected panel height 60 (box interior height), got %v", p.Height)
		}
	}
}

// Three rows, each with 2 side-by-side cells, so each row has an interior
// vertical divider at the same X. The horizontal dividers occupy
// [boundaryY, boundaryY+t], i.e. the top of the row below, so only a row's
// Y-start end overlaps one: the middle row's divider is notched there only,
// and its far end butts against the next horizontal divider.
func threeRowBox() pack.BoxResult {
	return pack.BoxResult{
		BoxName:   "ThreeRows",
		InteriorW: 100, InteriorD: 150, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "a1", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 50, D: 50}},
			{ID: "a2", Kind: "group", Bounds: pack.Rect{X: 50, Y: 0, W: 50, D: 50}},
			{ID: "b1", Kind: "group", Bounds: pack.Rect{X: 0, Y: 50, W: 50, D: 50}},
			{ID: "b2", Kind: "group", Bounds: pack.Rect{X: 50, Y: 50, W: 50, D: 50}},
			{ID: "c1", Kind: "group", Bounds: pack.Rect{X: 0, Y: 100, W: 50, D: 50}},
			{ID: "c2", Kind: "group", Bounds: pack.Rect{X: 50, Y: 100, W: 50, D: 50}},
		},
	}
}

func TestRenderMiddleRowVerticalPanelBothEndsNotchedWithinBounds(t *testing.T) {
	box := threeRowBox()
	mat := manifest.Material{Thickness: 3, Kerf: 0}

	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	// The middle row's vertical panel sits at OriginY=50 (row1's Y).
	var middle *geometry.Panel
	for i := range panels {
		if panels[i].Axis == geometry.AxisDepthRun && panels[i].Position.OriginY == 50 {
			middle = &panels[i]
		}
	}
	if middle == nil {
		t.Fatal("expected to find the middle row's vertical panel")
	}
	if len(middle.Notches) != 1 || middle.Notches[0].Pos != 0 {
		t.Fatalf("expected 1 notch at pos 0 on the middle row's vertical panel, got %d: %+v", len(middle.Notches), middle.Notches)
	}
	for _, n := range middle.Notches {
		if n.Pos < 0 || n.Pos+n.Width > middle.Length {
			t.Errorf("notch %+v falls outside the panel's own length %v", n, middle.Length)
		}
	}
	for _, pt := range middle.Outline {
		if pt.X < -1e-9 || pt.X > middle.Length+1e-9 {
			t.Errorf("outline point %+v falls outside the panel's own length %v", pt, middle.Length)
		}
	}
}

func TestRenderFullWallsGeneratesBoundaryPanels(t *testing.T) {
	box := pack.BoxResult{
		BoxName:   "FullWallsTest",
		InteriorW: 100, InteriorD: 100, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "full", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 100, D: 40}, FullWalls: true},
			{ID: "other", Kind: "group", Bounds: pack.Rect{X: 0, Y: 40, W: 100, D: 60}, FullWalls: false},
		},
	}
	mat := manifest.Material{Thickness: 3, Kerf: 0}

	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var horizCount, vertCount int
	var zeroNotchHoriz int
	for _, p := range panels {
		switch p.Axis {
		case "width-run":
			horizCount++
			if len(p.Notches) == 0 {
				zeroNotchHoriz++
			} else if len(p.Notches) != 2 {
				t.Errorf("expected a boundary top-wall panel to have 2 notches, got %d: %+v", len(p.Notches), p)
			}
		case "depth-run":
			vertCount++
			// Only its front end meets another panel (the front wall); its
			// far end butts against the next row's horizontal panel.
			if len(p.Notches) != 1 || p.Notches[0].Pos != 0 {
				t.Errorf("expected a boundary side-wall panel to have 1 notch (front corner), got %d: %+v", len(p.Notches), p)
			}
		}
	}
	// h-1 (interior, 0 notches, unaffected by FullWalls) + top wall (2 notches) = 2 width-run panels.
	if horizCount != 2 {
		t.Fatalf("expected 2 width-run panels (1 interior + 1 top boundary wall), got %d", horizCount)
	}
	if zeroNotchHoriz != 1 {
		t.Fatalf("expected exactly 1 zero-notch (interior) horizontal panel, got %d", zeroNotchHoriz)
	}
	// left wall + right wall = 2 depth-run panels (no interior vertical panels exist, single cell per row).
	if vertCount != 2 {
		t.Fatalf("expected 2 depth-run boundary wall panels (left+right), got %d", vertCount)
	}

	// Boundary walls must sit inside the interior: [0, W] x [0, D].
	for _, p := range panels {
		switch p.Axis {
		case "width-run":
			if p.Position.OriginY < -1e-9 || p.Position.OriginY+p.Thickness > box.InteriorD+1e-9 {
				t.Errorf("width-run panel %s at OriginY=%v extends outside interior depth %v", p.ID, p.Position.OriginY, box.InteriorD)
			}
		case "depth-run":
			if p.Position.OriginX < -1e-9 || p.Position.OriginX+p.Thickness > box.InteriorW+1e-9 {
				t.Errorf("depth-run panel %s at OriginX=%v extends outside interior width %v", p.ID, p.Position.OriginX, box.InteriorW)
			}
		}
	}
	var sawLeft, sawRight bool
	for _, p := range panels {
		if p.Axis == "depth-run" {
			sawLeft = sawLeft || p.Position.OriginX == 0
			sawRight = sawRight || p.Position.OriginX == box.InteriorW-mat.Thickness
		}
	}
	if !sawLeft || !sawRight {
		t.Errorf("expected left wall at X=0 and right wall at X=W-t, sawLeft=%v sawRight=%v", sawLeft, sawRight)
	}
}

func TestRenderNoFullWallsMatchesLegacyBehavior(t *testing.T) {
	box := twoRowBox()
	mat := manifest.Material{Thickness: 3, Kerf: 0}
	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(panels) != 2 {
		t.Fatalf("expected exactly the 2 interior panels (no boundary walls) when FullWalls is unset, got %d", len(panels))
	}
}

func TestRenderPlainJointOmitsAllNotches(t *testing.T) {
	box := twoRowBox()
	for i := range box.Compartments {
		box.Compartments[i].JointType = manifest.JointTypePlain
	}
	mat := manifest.Material{Thickness: 3, Kerf: 0}

	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(panels) == 0 {
		t.Fatal("expected some panels")
	}
	for _, p := range panels {
		if len(p.Notches) != 0 {
			t.Errorf("panel %s: expected 0 notches with JointTypePlain everywhere, got %d: %+v", p.ID, len(p.Notches), p.Notches)
		}
		if len(p.Outline) != 4 {
			t.Errorf("panel %s: expected a plain 4-point rectangle outline, got %d points: %+v", p.ID, len(p.Outline), p.Outline)
		}
	}
}

func TestRenderMixedJointTypeOnlyOmitsNotchAtThePlainBoundary(t *testing.T) {
	box := pack.BoxResult{
		BoxName:   "Mixed",
		InteriorW: 300, InteriorD: 300, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "a", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 92, D: 142}, JointType: manifest.JointTypeNotch},
			{ID: "b", Kind: "auto", Bounds: pack.Rect{X: 92, Y: 0, W: 92, D: 142}, JointType: manifest.JointTypePlain},
			{ID: "c", Kind: "group", Bounds: pack.Rect{X: 0, Y: 142, W: 268, D: 103}, FullWalls: true, JointType: manifest.JointTypeNotch},
		},
	}
	mat := manifest.Material{Thickness: 3, Kerf: 0}

	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var interiorDivider *geometry.Panel
	var horizontal *geometry.Panel
	twoNotchBoundaryWalls := 0
	for i, p := range panels {
		switch {
		case p.Axis == geometry.AxisDepthRun && p.Length == 142:
			interiorDivider = &panels[i]
		case p.Axis == geometry.AxisWidthRun && p.Length == 300:
			horizontal = &panels[i]
		case len(p.Notches) == 2:
			twoNotchBoundaryWalls++
		}
	}

	if interiorDivider == nil {
		t.Fatal("expected to find the a/b interior divider panel")
	}
	if len(interiorDivider.Notches) != 0 {
		t.Errorf("expected the a/b divider to have 0 notches since 'b' is JointTypePlain, got %d: %+v", len(interiorDivider.Notches), interiorDivider.Notches)
	}

	if horizontal == nil {
		t.Fatal("expected to find the interior horizontal panel between row0 and row1")
	}
	if len(horizontal.Notches) != 0 {
		t.Errorf("expected the horizontal panel's only crossing (a/b) to be suppressed too, got %d notches: %+v", len(horizontal.Notches), horizontal.Notches)
	}

	if twoNotchBoundaryWalls == 0 {
		t.Error("expected 'c' (JointTypeNotch, FullWalls) to still get its normal 2-notch boundary wall panels, unaffected by 'b' being plain")
	}
}

// twoRowCompartment is a group compartment whose own components shelf-packed
// into 2 rows, offset from its Bounds by a non-zero margin (ContentOffsetX/Y),
// used to test internal row-divider generation independent of any
// compartment-boundary geometry.
func twoRowCompartment() pack.CompartmentResult {
	return pack.CompartmentResult{
		ID: "tray", Kind: "group",
		Bounds: pack.Rect{X: 10, Y: 5, W: 50, D: 40},
		UsedW:  46,
		Rows: []pack.ShelfRow{
			{Y: 0, Height: 15, UsedW: 46},
			{Y: 15, Height: 12, UsedW: 30},
		},
		Dividers:       true,
		ContentOffsetX: 2,
		ContentOffsetY: 2,
	}
}

func TestRenderInternalRowDividersOneBoundaryPerRowGap(t *testing.T) {
	box := pack.BoxResult{
		BoxName:   "Internal",
		InteriorW: 100, InteriorD: 100, InteriorH: 60,
		Compartments: []pack.CompartmentResult{twoRowCompartment()},
	}
	mat := manifest.Material{Thickness: 3, Kerf: 0}

	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var found *geometry.Panel
	for i := range panels {
		if panels[i].ID == "ih-tray-1" {
			found = &panels[i]
		}
	}
	if found == nil {
		t.Fatalf("expected exactly one internal row-divider panel 'ih-tray-1', got panels: %+v", panels)
	}
	if len(found.Notches) != 0 {
		t.Errorf("expected 0 notches on the internal row divider, got %d: %+v", len(found.Notches), found.Notches)
	}
	if found.Axis != geometry.AxisWidthRun {
		t.Errorf("expected AxisWidthRun, got %v", found.Axis)
	}
	if found.Length != 46 {
		t.Errorf("expected Length == compartment UsedW (46), got %v", found.Length)
	}
	wantOriginX := 10.0 + 2.0     // Bounds.X + ContentOffsetX
	wantOriginY := 5.0 + 2.0 + 15 // Bounds.Y + ContentOffsetY + row0.Y + row0.Height
	if found.Position.OriginX != wantOriginX || found.Position.OriginY != wantOriginY {
		t.Errorf("expected Position {%v, %v}, got %+v", wantOriginX, wantOriginY, found.Position)
	}
}

func TestRenderInternalRowDividersDisabledWhenFlagFalse(t *testing.T) {
	cr := twoRowCompartment()
	cr.Dividers = false
	box := pack.BoxResult{
		BoxName:   "Internal",
		InteriorW: 100, InteriorD: 100, InteriorH: 60,
		Compartments: []pack.CompartmentResult{cr},
	}
	mat := manifest.Material{Thickness: 3, Kerf: 0}

	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, p := range panels {
		if p.ID == "ih-tray-1" {
			t.Fatalf("expected no internal row-divider panel when Dividers is false, got %+v", p)
		}
	}
}

func TestRenderInternalRowDividersNoneForSingleRow(t *testing.T) {
	cr := twoRowCompartment()
	cr.Rows = cr.Rows[:1]
	box := pack.BoxResult{
		BoxName:   "Internal",
		InteriorW: 100, InteriorD: 100, InteriorH: 60,
		Compartments: []pack.CompartmentResult{cr},
	}
	mat := manifest.Material{Thickness: 3, Kerf: 0}

	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, p := range panels {
		if p.ID == "ih-tray-1" {
			t.Fatalf("expected no internal row-divider panel with only 1 row, got %+v", p)
		}
	}
}

func TestRenderInternalColumnDividerBetweenSideBySideItems(t *testing.T) {
	cr := twoRowCompartment()
	cr.Rows = []pack.ShelfRow{{
		Y: 0, Height: 15, UsedW: 46,
		Items: []pack.PlacedItem{
			{ID: "a", Rect: pack.Rect{X: 1, Y: 1, W: 20, D: 13}},
			{ID: "b", Rect: pack.Rect{X: 25, Y: 1, W: 20, D: 13}},
		},
	}}
	box := pack.BoxResult{
		BoxName:   "Internal",
		InteriorW: 100, InteriorD: 100, InteriorH: 60,
		Compartments: []pack.CompartmentResult{cr},
	}
	mat := manifest.Material{Thickness: 3, Kerf: 0}

	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var found *geometry.Panel
	for i := range panels {
		if panels[i].ID == "iv-tray-1-1" {
			found = &panels[i]
		}
	}
	if found == nil {
		t.Fatalf("expected column divider 'iv-tray-1-1', got panels: %+v", panels)
	}
	if found.Axis != geometry.AxisDepthRun || found.Length != 15 {
		t.Errorf("expected AxisDepthRun length 15, got %v %v", found.Axis, found.Length)
	}
	// Bounds.X + ContentOffsetX + midpoint of gap (21..25) = 10+2+23
	if found.Position.OriginX != 35 || found.Position.OriginY != 7 {
		t.Errorf("expected Position {35, 7}, got %+v", found.Position)
	}

	cr.Dividers = false
	box.Compartments = []pack.CompartmentResult{cr}
	panels, _ = Backend{}.Render(box, mat, render.RenderOptions{})
	for _, p := range panels {
		if p.ID == "iv-tray-1-1" {
			t.Fatalf("dividers disabled but got %s", p.ID)
		}
	}
}

func TestRenderExpansionDividers(t *testing.T) {
	cr := twoRowCompartment()
	cr.Expand = true
	cr.CoreW = 30
	box := pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 60, Compartments: []pack.CompartmentResult{cr}}
	mat := manifest.Material{Thickness: 3}
	panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]geometry.Panel{}
	for _, p := range panels {
		got[p.ID] = p
	}
	v := got["ie-v-tray"]
	if _, ok := got["ie-h-tray"]; ok {
		t.Errorf("unexpected ie-h panel")
	}
	if v.Length != 40 || v.Position.OriginX != 40 || v.Position.OriginY != 5 {
		t.Errorf("bad ie-v: %+v", v)
	}
	cr.Dividers = false
	box.Compartments = []pack.CompartmentResult{cr}
	panels, _ = Backend{}.Render(box, mat, render.RenderOptions{})
	for _, p := range panels {
		if p.ID == "ie-v-tray" || p.ID == "ie-h-tray" {
			t.Errorf("unexpected %s with dividers off", p.ID)
		}
	}
}

func removableCompartment(id string, x float64, floor bool) pack.CompartmentResult {
	return pack.CompartmentResult{
		ID: id, Kind: "group",
		Bounds: pack.Rect{X: x, Y: 0, W: 50, D: 40},
		UsedW:  40, UsedD: 30,
		Rows:      []pack.ShelfRow{{Y: 0, Height: 30, UsedW: 40}},
		Removable: true, Floor: floor, WallT: 3,
		ContentOffsetX: 5, ContentOffsetY: 5,
	}
}

func TestRenderRemovableTrayWallsAndFloor(t *testing.T) {
	box := pack.BoxResult{InteriorW: 120, InteriorD: 60, InteriorH: 50,
		Compartments: []pack.CompartmentResult{removableCompartment("a", 0, true), removableCompartment("b", 60, false)}}
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]geometry.Panel{}
	for _, p := range panels {
		byID[p.ID] = p
	}
	if len(panels) != 5+4 {
		t.Fatalf("want 9 panels (a: floor+4 walls, b: 4 walls), got %d", len(panels))
	}
	if f := byID["fl-a"]; f.Axis != geometry.AxisFloor || f.Length != 50 || f.Height != 40 || f.Thickness != 3 {
		t.Errorf("bad floor: %+v", f)
	}
	if _, ok := byID["fl-b"]; ok {
		t.Errorf("tray b has no floor")
	}
	a := byID["rw-a-front"]
	if a.Height != 47 || a.Position.OriginZ != 3 {
		t.Errorf("floored wall should stand on floor at z=3 with height 47: %+v", a)
	}
	b := byID["rw-b-front"]
	if b.Height != 50 || b.Position.OriginZ != 0 {
		t.Errorf("unfloored wall should be full height at z=0: %+v", b)
	}
	if l := byID["rv-a-right"]; l.Length != 34 || l.Position.OriginX != 47 || l.Position.OriginY != 3 {
		t.Errorf("bad right wall: %+v", l)
	}
	for id := range byID {
		if id[0] == 'h' || id[0] == 'v' {
			t.Errorf("shared-grid panel %s should not exist", id)
		}
	}
}

func TestRenderMixedRemovableErrors(t *testing.T) {
	c := removableCompartment("b", 60, false)
	c.Removable = false
	box := pack.BoxResult{InteriorW: 120, InteriorD: 60, InteriorH: 50,
		Compartments: []pack.CompartmentResult{removableCompartment("a", 0, false), c}}
	if _, err := (Backend{}).Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{}); err == nil {
		t.Fatal("expected error mixing removable and non-removable trays")
	}
}

func TestRenderGridFloorRaisesPanels(t *testing.T) {
	cr := twoRowCompartment()
	cr.Floor = true
	box := pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 60, Compartments: []pack.CompartmentResult{cr}}
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var floors int
	for _, p := range panels {
		if p.Axis == geometry.AxisFloor {
			floors++
			continue
		}
		if p.Position.OriginZ != 3 || p.Height != 57 {
			t.Errorf("%s: want z=3 h=57, got z=%v h=%v", p.ID, p.Position.OriginZ, p.Height)
		}
	}
	if floors != 1 {
		t.Errorf("want 1 floor, got %d", floors)
	}
}

// A floor on one tray raises and shortens every grid panel, including v-1,
// which borders only the floorless trays a and b, and only the floored tray
// gets a floor plate. A percent reduction is taken from the lowered base
// (height - thickness), not from the full interior height.
func TestRenderGridMixedFloorRaisesAllPanels(t *testing.T) {
	box := twoRowBox()
	box.Compartments[2].Floor = true // only tray c
	box.Compartments[0].DividerReduction = pack.HeightReduction{Percent: 10}
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var floors []string
	for _, p := range panels {
		if p.Axis == geometry.AxisFloor {
			floors = append(floors, p.ID)
			continue
		}
		if p.Position.OriginZ != 3 {
			t.Errorf("%s: OriginZ %v, want 3", p.ID, p.Position.OriginZ)
		}
	}
	if len(floors) != 1 {
		t.Fatalf("want exactly one floor plate, got %v", floors)
	}
	if !strings.HasPrefix(floors[0], "fl-") {
		t.Errorf("floor plate id %q, want fl-* prefix", floors[0])
	}
	got := panelsByID(panels)
	// 10% of (60 - 3) = 5.7, not 10% of 60 = 6. Both shared panels border
	// tray a, so both take the reduction.
	for _, id := range []string{"h-1", "v-1"} {
		p, ok := got[id]
		if !ok {
			t.Fatalf("missing panel %s", id)
		}
		if !near(p.Height, 57-5.7) {
			t.Errorf("%s: height %v, want %v", id, p.Height, 57-5.7)
		}
	}
	// Without the reduction every non-floor panel is height - thickness.
	box.Compartments[0].DividerReduction = pack.HeightReduction{}
	panels, err = Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range panels {
		if p.Axis != geometry.AxisFloor && (p.Position.OriginZ != 3 || !near(p.Height, 57)) {
			t.Errorf("%s: want z=3 h=57, got z=%v h=%v", p.ID, p.Position.OriginZ, p.Height)
		}
	}
}

func narrowMiddleRowBox(midH float64) pack.BoxResult {
	return pack.BoxResult{
		InteriorW: 100, InteriorD: 100 + midH, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "a", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 100, D: 50}},
			{ID: "b1", Kind: "group", Bounds: pack.Rect{X: 0, Y: 50, W: 50, D: midH}},
			{ID: "b2", Kind: "group", Bounds: pack.Rect{X: 50, Y: 50, W: 50, D: midH}},
			{ID: "c", Kind: "group", Bounds: pack.Rect{X: 0, Y: 50 + midH, W: 100, D: 50}},
		},
	}
}

func TestRenderNarrowRowNotchesErrors(t *testing.T) {
	mat := manifest.Material{Thickness: 3}
	// The middle panel is lengthened by one thickness to reach into the next
	// divider, so its two end notches overlap once the row is < slotWidth.
	for _, h := range []float64{2, 1} {
		_, err := Backend{}.Render(narrowMiddleRowBox(h), mat, render.RenderOptions{})
		if err == nil {
			t.Fatalf("row height %v: expected error", h)
		}
		if !strings.Contains(err.Error(), "v-1") {
			t.Errorf("row height %v: error should name the panel: %v", h, err)
		}
	}
	// Exactly slotWidth is fine.
	if _, err := (Backend{}).Render(narrowMiddleRowBox(3), mat, render.RenderOptions{}); err != nil {
		t.Fatalf("row height 3: %v", err)
	}
}

func TestRenderAdjacentBoundariesOverlappingNotchesErrors(t *testing.T) {
	mat := manifest.Material{Thickness: 3}
	build := func(gap float64) pack.BoxResult {
		return pack.BoxResult{
			InteriorW: 100, InteriorD: 100, InteriorH: 60,
			Compartments: []pack.CompartmentResult{
				{ID: "a", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 40, D: 50}},
				{ID: "b", Kind: "group", Bounds: pack.Rect{X: 40, Y: 0, W: gap, D: 50}},
				{ID: "c", Kind: "group", Bounds: pack.Rect{X: 40 + gap, Y: 0, W: 60 - gap, D: 50}},
				{ID: "d", Kind: "group", Bounds: pack.Rect{X: 0, Y: 50, W: 100, D: 50}},
			},
		}
	}
	_, err := Backend{}.Render(build(2), mat, render.RenderOptions{})
	if err == nil || !strings.Contains(err.Error(), "h-1") || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected overlap error naming h-1, got %v", err)
	}
	if _, err := (Backend{}).Render(build(3), mat, render.RenderOptions{}); err != nil {
		t.Fatalf("boundaries exactly one slot apart should render: %v", err)
	}
}

func TestRenderNarrowFullWallsBoundaryErrors(t *testing.T) {
	box := pack.BoxResult{
		InteriorW: 100, InteriorD: 5, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "a", Kind: "group", FullWalls: true, Bounds: pack.Rect{X: 0, Y: 0, W: 100, D: 5}},
		},
	}
	if _, err := (Backend{}).Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{}); err == nil {
		t.Fatal("expected error for boundary wall shorter than two slots")
	}
}

// notchRect returns a notch's world-space XY footprint (x0, y0, x1, y1): the
// slot's extent along the panel's own length crossed with the panel's
// thickness across it.
func notchRect(p geometry.Panel, n geometry.Notch) (x0, y0, x1, y1 float64) {
	if p.Axis == geometry.AxisDepthRun {
		return p.Position.OriginX, p.Position.OriginY + n.Pos,
			p.Position.OriginX + p.Thickness, p.Position.OriginY + n.Pos + n.Width
	}
	return p.Position.OriginX + n.Pos, p.Position.OriginY,
		p.Position.OriginX + n.Pos + n.Width, p.Position.OriginY + p.Thickness
}

// overlapsCrossing reports whether the notch's slot region overlaps, by at
// least a full thickness x thickness square, a notch of a panel running along
// the other axis: i.e. the two half-laps really meet at the crossing.
func overlapsCrossing(panels []geometry.Panel, p geometry.Panel, n geometry.Notch) bool {
	ax0, ay0, ax1, ay1 := notchRect(p, n)
	for _, q := range panels {
		if q.Axis == p.Axis || q.Axis == geometry.AxisFloor {
			continue
		}
		for _, m := range q.Notches {
			bx0, by0, bx1, by1 := notchRect(q, m)
			w := min(ax1, bx1) - max(ax0, bx0)
			h := min(ay1, by1) - max(ay0, by0)
			if w >= p.Thickness-1e-9 && h >= p.Thickness-1e-9 {
				return true
			}
		}
	}
	return false
}

// Every notch must sit where a crossing panel's notch actually overlaps it
// (the crossing panel's real footprint: a divider at boundaryY occupies
// [boundaryY, boundaryY+t], not the row above it). Otherwise material is cut
// with nothing to lap into.
func TestRenderNotchesInterlockWithCrossingPanelFootprint(t *testing.T) {
	// Row boundaries do not line up between rows, to also cover a boundary
	// present in only the upper or only the lower row.
	mixed := pack.BoxResult{
		BoxName:   "Mixed",
		InteriorW: 100, InteriorD: 150, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "a1", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 40, D: 50}},
			{ID: "a2", Kind: "group", Bounds: pack.Rect{X: 40, Y: 0, W: 60, D: 50}},
			{ID: "b1", Kind: "group", Bounds: pack.Rect{X: 0, Y: 50, W: 100, D: 50}},
			{ID: "c1", Kind: "group", Bounds: pack.Rect{X: 0, Y: 100, W: 70, D: 50}},
			{ID: "c2", Kind: "group", Bounds: pack.Rect{X: 70, Y: 100, W: 30, D: 50}},
		},
	}
	cases := map[string]pack.BoxResult{"two-row": twoRowBox(), "three-row": threeRowBox(), "mixed": mixed}
	for name, box := range cases {
		t.Run(name, func(t *testing.T) {
			mat := manifest.Material{Thickness: 3, Kerf: 0.2}
			panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range panels {
				for _, n := range p.Notches {
					if !overlapsCrossing(panels, p, n) {
						t.Errorf("panel %s notch %+v (origin %+v) does not overlap any crossing panel's notch", p.ID, n, p.Position)
					}
				}
			}
		})
	}
}

// Floor + FullWalls: the floor plate sits at z=0 with thickness t, and every
// grid panel (interior dividers and boundary walls alike) stands on it at
// z=t with height H-t, so the tops are flush at H.
func TestRenderFloorWithFullWallsPositions(t *testing.T) {
	box := pack.BoxResult{
		InteriorW: 100, InteriorD: 100, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "full", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 100, D: 40}, FullWalls: true, Floor: true},
			{ID: "other", Kind: "group", Bounds: pack.Rect{X: 0, Y: 40, W: 100, D: 60}},
		},
	}
	const th = 3.0
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: th}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var floors, standing int
	for _, p := range panels {
		if p.Axis == geometry.AxisFloor {
			floors++
			if p.ID != "fl-full" || p.Position.OriginZ != 0 || p.Thickness != th ||
				p.Position.OriginX != 0 || p.Position.OriginY != 0 || p.Length != 100 || p.Height != 40 {
				t.Errorf("bad floor plate: %+v", p)
			}
			continue
		}
		standing++
		if p.Position.OriginZ != th {
			t.Errorf("%s: want OriginZ=%v, got %v", p.ID, th, p.Position.OriginZ)
		}
		if top := p.Position.OriginZ + p.Height; top != box.InteriorH {
			t.Errorf("%s: top at %v, want %v", p.ID, top, box.InteriorH)
		}
	}
	if floors != 1 {
		t.Errorf("want 1 floor (only the floored tray), got %d", floors)
	}
	// 1 interior row divider + top wall + left + right walls (see FullWalls test).
	if standing != 4 {
		t.Errorf("want 4 standing panels, got %d", standing)
	}
}

// Removable tray with Expand and a floor: the expansion divider is a
// depth-run panel just inside the right wall, between the side walls, and
// stands on the floor like the walls do.
func TestRenderRemovableExpandPositions(t *testing.T) {
	cr := removableCompartment("a", 10, true)
	cr.Dividers = true
	cr.Expand = true
	cr.CoreW = 30 // tray grew from 30 to Bounds.W=50
	box := pack.BoxResult{InteriorW: 120, InteriorD: 60, InteriorH: 50, Compartments: []pack.CompartmentResult{cr}}
	const th = 3.0
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: th}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]geometry.Panel{}
	for _, p := range panels {
		byID[p.ID] = p
	}
	ie, ok := byID["ie-v-a"]
	if !ok {
		t.Fatalf("missing expansion divider; panels: %d", len(panels))
	}
	b := cr.Bounds
	if ie.Axis != geometry.AxisDepthRun {
		t.Errorf("want depth-run, got %s", ie.Axis)
	}
	// x = old right-wall position (Bounds.X + CoreW) shifted in by the wall
	// so it sits inside it; y/length span between the front and back walls.
	if ie.Position.OriginX != b.X+cr.CoreW-cr.WallT || ie.Position.OriginY != b.Y+cr.WallT || ie.Length != b.D-2*cr.WallT {
		t.Errorf("bad expansion divider placement: %+v", ie)
	}
	if ie.Position.OriginZ != th || ie.Height != box.InteriorH-th {
		t.Errorf("divider must stand on the floor: want z=%v h=%v, got z=%v h=%v", th, box.InteriorH-th, ie.Position.OriginZ, ie.Height)
	}
	// It must sit between the side walls (left wall inner face .. right wall inner face).
	if ie.Position.OriginX < b.X+cr.WallT || ie.Position.OriginX+ie.Thickness > b.X+b.W-cr.WallT {
		t.Errorf("divider outside the tray's side walls: %+v", ie)
	}
	// Floor at z=0 and the walls at z=t agree with the divider.
	if fl := byID["fl-a"]; fl.Position.OriginZ != 0 {
		t.Errorf("floor z: %+v", fl)
	}
	if w := byID["rw-a-front"]; w.Position.OriginZ != ie.Position.OriginZ || w.Height != ie.Height {
		t.Errorf("wall and divider disagree on z/height: wall %+v", w)
	}
}

// footprint returns a panel's world-space XY rectangle.
func footprint(p geometry.Panel) (x0, y0, x1, y1 float64) {
	if p.Axis == geometry.AxisDepthRun {
		return p.Position.OriginX, p.Position.OriginY,
			p.Position.OriginX + p.Thickness, p.Position.OriginY + p.Length
	}
	return p.Position.OriginX, p.Position.OriginY,
		p.Position.OriginX + p.Length, p.Position.OriginY + p.Thickness
}

// notchCovers reports whether one of p's notches covers the given world-space
// rectangle (a crossing's overlap region).
func notchCovers(p geometry.Panel, x0, y0, x1, y1 float64) bool {
	const eps = 1e-9
	for _, n := range p.Notches {
		nx0, ny0, nx1, ny1 := notchRect(p, n)
		if nx0 <= x0+eps && ny0 <= y0+eps && nx1 >= x1-eps && ny1 >= y1-eps {
			return true
		}
	}
	return false
}

// checkInterlock verifies, for a layout of standing panels, that every notch
// laps into a crossing panel's notch, and that every real crossing of a
// width-run and a depth-run panel is notched in both (a half-lap).
func checkInterlock(t *testing.T, panels []geometry.Panel) {
	t.Helper()
	for _, p := range panels {
		if p.Axis == geometry.AxisFloor {
			continue
		}
		for _, n := range p.Notches {
			if !overlapsCrossing(panels, p, n) {
				t.Errorf("panel %s notch %+v (origin %+v, len %v) does not overlap any crossing panel's notch", p.ID, n, p.Position, p.Length)
			}
		}
	}
	for _, p := range panels {
		for _, q := range panels {
			if p.Axis != geometry.AxisWidthRun || q.Axis != geometry.AxisDepthRun {
				continue
			}
			px0, py0, px1, py1 := footprint(p)
			qx0, qy0, qx1, qy1 := footprint(q)
			x0, y0, x1, y1 := max(px0, qx0), max(py0, qy0), min(px1, qx1), min(py1, qy1)
			if x1-x0 < p.Thickness-1e-9 || y1-y0 < p.Thickness-1e-9 {
				continue
			}
			if !notchCovers(p, x0, y0, x1, y1) || !notchCovers(q, x0, y0, x1, y1) {
				t.Errorf("crossing of %s and %s at [%v,%v]x[%v,%v] is not notched in both", p.ID, q.ID, x0, x1, y0, y1)
			}
		}
	}
}

func fullWallsBox(w, d float64, cells ...[]pack.CompartmentResult) pack.BoxResult {
	box := pack.BoxResult{InteriorW: w, InteriorD: d, InteriorH: 60}
	for _, row := range cells {
		box.Compartments = append(box.Compartments, row...)
	}
	return box
}

func fwCell(id string, x, y, w, d float64, full bool) pack.CompartmentResult {
	return pack.CompartmentResult{ID: id, Kind: "group", Bounds: pack.Rect{X: x, Y: y, W: w, D: d}, FullWalls: full}
}

// FullWalls boundary walls must only be notched where a crossing panel really
// meets them, and every joint must be notched on both sides.
func TestRenderFullWallsNotchesInterlock(t *testing.T) {
	cases := map[string]pack.BoxResult{
		"single-row-all-full": fullWallsBox(150, 50, []pack.CompartmentResult{
			fwCell("a", 0, 0, 50, 50, true), fwCell("b", 50, 0, 50, 50, true), fwCell("c", 100, 0, 50, 50, true),
		}),
		"single-row-mixed": fullWallsBox(150, 50, []pack.CompartmentResult{
			fwCell("a", 0, 0, 50, 50, true), fwCell("b", 50, 0, 50, 50, false), fwCell("c", 100, 0, 50, 50, true),
		}),
		"single-row-first-only": fullWallsBox(100, 50, []pack.CompartmentResult{
			fwCell("a", 0, 0, 50, 50, true), fwCell("b", 50, 0, 50, 50, false),
		}),
		"single-row-last-only": fullWallsBox(100, 50, []pack.CompartmentResult{
			fwCell("a", 0, 0, 50, 50, false), fwCell("b", 50, 0, 50, 50, true),
		}),
		"grid-all-full": fullWallsBox(100, 150,
			[]pack.CompartmentResult{fwCell("a1", 0, 0, 50, 50, true), fwCell("a2", 50, 0, 50, 50, true)},
			[]pack.CompartmentResult{fwCell("b1", 0, 50, 50, 50, true), fwCell("b2", 50, 50, 50, 50, true)},
			[]pack.CompartmentResult{fwCell("c1", 0, 100, 50, 50, true), fwCell("c2", 50, 100, 50, 50, true)},
		),
		"grid-staggered-mixed": fullWallsBox(100, 150,
			[]pack.CompartmentResult{fwCell("a1", 0, 0, 40, 50, true), fwCell("a2", 40, 0, 60, 50, false)},
			[]pack.CompartmentResult{fwCell("b1", 0, 50, 100, 50, true)},
			[]pack.CompartmentResult{fwCell("c1", 0, 100, 70, 50, false), fwCell("c2", 70, 100, 30, 50, true)},
		),
	}
	for name, box := range cases {
		t.Run(name, func(t *testing.T) {
			panels, err := Backend{}.Render(box, manifest.Material{Thickness: 3, Kerf: 0.2}, render.RenderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			checkInterlock(t, panels)
		})
	}
}

func TestRenderDividerHeightReduction(t *testing.T) {
	mat := manifest.Material{Thickness: 3}
	tests := []struct {
		name string
		red  pack.HeightReduction
		want map[string]float64 // panel id -> expected height; others stay 60
	}{
		{"amount all", pack.HeightReduction{AmountMM: 10}, map[string]float64{"h-1": 50, "v-1": 50}},
		{"percent all", pack.HeightReduction{Percent: 10}, map[string]float64{"h-1": 54, "v-1": 54}},
		{"only matching ids", pack.HeightReduction{AmountMM: 10, Panels: []string{"h-*"}}, map[string]float64{"h-1": 50}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			box := twoRowBox()
			for i := range box.Compartments {
				box.Compartments[i].DividerReduction = tc.red
			}
			panels, err := Backend{}.Render(box, mat, render.RenderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range panels {
				want, ok := tc.want[p.ID]
				if !ok {
					want = 60
				}
				if p.Height != want {
					t.Errorf("%s: height %v, want %v", p.ID, p.Height, want)
				}
				for _, pt := range p.Outline {
					if pt.Y > p.Height+1e-9 {
						t.Errorf("%s: outline point %+v above panel height %v", p.ID, pt, p.Height)
					}
				}
			}
		})
	}
}

func TestRenderHeightReductionKeepsNotchDepth(t *testing.T) {
	box := twoRowBox()
	for i := range box.Compartments {
		box.Compartments[i].DividerReduction = pack.HeightReduction{AmountMM: 10}
	}
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range panels {
		if p.ID != "h-1" {
			continue
		}
		low := p.Height
		for _, pt := range p.Outline {
			if pt.Y < low && pt.Y > 0 {
				low = pt.Y
			}
		}
		if low != 30 { // notch depth stays half of the full 60 height
			t.Errorf("notch floor at %v, want 30", low)
		}
	}
}

func TestRenderHeightReductionTooLargeForNotches(t *testing.T) {
	box := twoRowBox()
	for i := range box.Compartments {
		box.Compartments[i].DividerReduction = pack.HeightReduction{AmountMM: 30}
	}
	if _, err := (Backend{}).Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{}); err == nil {
		t.Fatal("expected an error when the reduction reaches the notch depth")
	}
}

func TestRenderExternalHeightReductionRemovableTray(t *testing.T) {
	cr := pack.CompartmentResult{
		ID: "t", Kind: "group", Removable: true, WallT: 3,
		Bounds:            pack.Rect{W: 50, D: 40},
		ExternalReduction: pack.HeightReduction{Percent: 25},
	}
	panels, err := Backend{}.Render(pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: 40, Compartments: []pack.CompartmentResult{cr}}, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(panels) != 4 {
		t.Fatalf("want 4 walls, got %d", len(panels))
	}
	for _, p := range panels {
		if p.Height != 30 {
			t.Errorf("%s: height %v, want 30", p.ID, p.Height)
		}
	}
}

// Current behaviour for a row whose cells have unequal depths: the vertical
// panel between them spans the full row height (the deepest cell's depth),
// not the shorter cell's own depth, so it extends past the shorter
// compartment into the uncovered strip. No panel closes that strip.
func TestRenderUnequalDepthRowVerticalPanelSpansRowHeight(t *testing.T) {
	box := pack.BoxResult{
		BoxName:   "Unequal",
		InteriorW: 100, InteriorD: 100, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "short", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 40, D: 40}},
			{ID: "tall", Kind: "group", Bounds: pack.Rect{X: 40, Y: 0, W: 40, D: 60}},
		},
	}
	panels, err := Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var vs []geometry.Panel
	for _, p := range panels {
		if p.Axis == geometry.AxisDepthRun {
			vs = append(vs, p)
		}
	}
	if len(vs) != 1 {
		t.Fatalf("expected exactly 1 vertical panel, got %d: %+v", len(vs), vs)
	}
	v := vs[0]
	if v.Length != 60 {
		t.Errorf("vertical panel length = %v, want 60 (row height = deepest cell), not the short cell's depth 40", v.Length)
	}
	if v.Position.OriginX != 40 || v.Position.OriginY != 0 {
		t.Errorf("unexpected placement %+v", v.Position)
	}
}

func TestRenderHeightMustExceedFloor(t *testing.T) {
	mat := manifest.Material{Thickness: 3}
	for _, h := range []float64{3, 2} {
		cr := twoRowCompartment()
		cr.Floor = true
		grid := pack.BoxResult{InteriorW: 100, InteriorD: 100, InteriorH: h, Compartments: []pack.CompartmentResult{cr}}
		rem := pack.BoxResult{InteriorW: 120, InteriorD: 60, InteriorH: h,
			Compartments: []pack.CompartmentResult{removableCompartment("a", 0, true)}}
		for name, box := range map[string]pack.BoxResult{"grid": grid, "removable": rem} {
			_, err := Backend{}.Render(box, mat, render.RenderOptions{})
			var re *pack.ReductionError
			if err == nil || !errors.As(err, &re) || !strings.Contains(err.Error(), "must exceed floor thickness 3") {
				t.Errorf("%s height %g: want ReductionError about floor, got %v", name, h, err)
			}
		}
	}
	// Without a floor a short height is fine.
	cr := removableCompartment("a", 0, false)
	box := pack.BoxResult{InteriorW: 120, InteriorD: 60, InteriorH: 2, Compartments: []pack.CompartmentResult{cr}}
	if _, err := (Backend{}).Render(box, mat, render.RenderOptions{}); err != nil {
		t.Errorf("no floor: %v", err)
	}
}

func TestRenderRemovableTrayTooSmallForWalls(t *testing.T) {
	for _, dim := range []struct{ w, d float64 }{{6, 40}, {50, 6}, {4, 40}, {50, 3}} {
		cr := removableCompartment("a", 0, false)
		cr.Bounds.W, cr.Bounds.D = dim.w, dim.d
		box := pack.BoxResult{InteriorW: 120, InteriorD: 60, InteriorH: 50, Compartments: []pack.CompartmentResult{cr}}
		_, err := Backend{}.Render(box, manifest.Material{Thickness: 3}, render.RenderOptions{})
		if err == nil || !strings.Contains(err.Error(), "too small") {
			t.Errorf("%gx%g: want too-small error, got %v", dim.w, dim.d, err)
		}
	}
}
