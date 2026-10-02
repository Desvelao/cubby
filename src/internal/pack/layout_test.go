package pack

import (
	"math"
	"testing"

	"github.com/Desvelao/cubby/internal/manifest"
)

func boolPtr(b bool) *bool { return &b }

func jointTypePtr(j manifest.JointType) *manifest.JointType { return &j }

func TestLayoutBoxGroupsAndAutoPack(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name:     "Test Box",
			Units:    manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 100, Height: 50},
		},
		Material: manifest.Material{Thickness: 3, Kerf: 0},
		Components: []manifest.Component{
			{ID: "cards", Name: "Cards", Width: 40, Depth: 30, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "tokens", Name: "Tokens", Width: 10, Depth: 10, Height: 5, Qty: 4, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "card-compartment", Name: "Cards", Components: []string{"cards"}},
		},
	}

	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}

	if len(res.Compartments) != 2 {
		t.Fatalf("expected 2 compartments (1 group + 1 auto), got %d: %+v", len(res.Compartments), res.Compartments)
	}

	var haveGroup, haveAuto bool
	for _, c := range res.Compartments {
		switch c.Kind {
		case "group":
			haveGroup = true
			if c.ID != "card-compartment" {
				t.Errorf("unexpected group compartment id %q", c.ID)
			}
		case "auto":
			haveAuto = true
		}
	}
	if !haveGroup || !haveAuto {
		t.Fatalf("expected both a group and an auto compartment, got %+v", res.Compartments)
	}
	if len(res.TotalMissing) != 0 {
		t.Fatalf("expected everything to fit, got missing: %+v", res.TotalMissing)
	}
}

func TestLayoutBoxUnknownGroupReference(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Test", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 100, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Groups: []manifest.Group{
			{ID: "g1", Components: []string{"missing"}},
		},
	}
	if _, err := LayoutBox(m); err == nil {
		t.Fatal("expected error for unknown component reference")
	}
}

func TestLayoutBoxFillRemainingGrowsLastCompartment(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Roomy Box", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 80, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Components: []manifest.Component{
			{ID: "widget", Name: "Widget", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "only-group", Components: []string{"widget"}},
		},
		Defaults: manifest.Defaults{FillRemaining: true},
	}

	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}
	if len(res.Compartments) != 1 {
		t.Fatalf("expected 1 compartment, got %d", len(res.Compartments))
	}
	c := res.Compartments[0]
	if c.Bounds.W != 100 || c.Bounds.D != 80 {
		t.Fatalf("expected the sole compartment to grow to fill the 100x80 box, got %vx%v", c.Bounds.W, c.Bounds.D)
	}
	// The grown slack is inside Bounds now, and a group's Remaining is always empty.
	if c.Remaining != (Space{}) {
		t.Fatalf("expected Remaining to be empty after growth filled the box, got %+v", c.Remaining)
	}
}

func TestLayoutBoxFillRemainingDisabledByDefault(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Roomy Box", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 80, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Components: []manifest.Component{
			{ID: "widget", Name: "Widget", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "only-group", Components: []string{"widget"}},
		},
	}

	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}
	c := res.Compartments[0]
	if c.Bounds.W == 100 || c.Bounds.D == 80 {
		t.Fatalf("expected the compartment to stay tight-fit without fillRemaining, got %vx%v", c.Bounds.W, c.Bounds.D)
	}
}

func TestLayoutBoxFullWallsResolution(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Test", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 200, Depth: 100, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{FullWalls: true},
		Components: []manifest.Component{
			{ID: "a", Name: "A", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "b", Name: "B", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "c", Name: "C", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "inherits-default", Components: []string{"a"}},
			{ID: "opts-out", Components: []string{"b"}, FullWalls: boolPtr(false)},
		},
	}

	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}

	byID := map[string]bool{}
	for _, c := range res.Compartments {
		byID[c.ID] = c.FullWalls
	}
	if !byID["inherits-default"] {
		t.Error("expected 'inherits-default' group to inherit Defaults.FullWalls=true")
	}
	if byID["opts-out"] {
		t.Error("expected 'opts-out' group's explicit fullWalls:false to override the default")
	}
	if !byID["auto-1"] {
		t.Error("expected the auto compartment (component 'c') to use Defaults.FullWalls=true")
	}
}

func TestLayoutBoxJointTypeResolution(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Test", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 200, Depth: 100, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{JointType: manifest.JointTypePlain},
		Components: []manifest.Component{
			{ID: "a", Name: "A", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "b", Name: "B", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "c", Name: "C", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "inherits-default", Components: []string{"a"}},
			{ID: "opts-out", Components: []string{"b"}, JointType: jointTypePtr(manifest.JointTypeNotch)},
		},
	}

	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}

	byID := map[string]manifest.JointType{}
	for _, c := range res.Compartments {
		byID[c.ID] = c.JointType
	}
	if byID["inherits-default"] != manifest.JointTypePlain {
		t.Errorf("expected 'inherits-default' group to inherit Defaults.JointType=plain, got %q", byID["inherits-default"])
	}
	if byID["opts-out"] != manifest.JointTypeNotch {
		t.Errorf("expected 'opts-out' group's explicit jointType:notch to override the default, got %q", byID["opts-out"])
	}
	if byID["auto-1"] != manifest.JointTypePlain {
		t.Errorf("expected the auto compartment (component 'c') to use Defaults.JointType=plain, got %q", byID["auto-1"])
	}
}

func TestLayoutBoxMissingWhenOverfull(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Tiny", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 10, Depth: 10, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Components: []manifest.Component{
			{ID: "big", Name: "Big", Width: 50, Depth: 50, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}
	if len(res.TotalMissing) != 1 || res.TotalMissing[0].ComponentID != "big" {
		t.Fatalf("expected 'big' reported missing, got %+v", res.TotalMissing)
	}
}

func TestLayoutBoxDividersDefaultsToTrueWhenUnset(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Test", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 200, Depth: 100, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Components: []manifest.Component{
			{ID: "a", Name: "A", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "c", Name: "C", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "inherits-default", Components: []string{"a"}},
		},
	}

	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}

	byID := map[string]bool{}
	for _, c := range res.Compartments {
		byID[c.ID] = c.Dividers
	}
	if !byID["inherits-default"] {
		t.Error("expected 'inherits-default' group to default Dividers=true when defaults.dividers is unset")
	}
	if !byID["auto-1"] {
		t.Error("expected the auto compartment (component 'c') to default Dividers=true when defaults.dividers is unset")
	}
}

func TestLayoutBoxDividersResolution(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Test", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 200, Depth: 100, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{Dividers: boolPtr(false)},
		Components: []manifest.Component{
			{ID: "a", Name: "A", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "b", Name: "B", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "c", Name: "C", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "inherits-default", Components: []string{"a"}},
			{ID: "opts-in", Components: []string{"b"}, Dividers: boolPtr(true)},
		},
	}

	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}

	byID := map[string]bool{}
	for _, c := range res.Compartments {
		byID[c.ID] = c.Dividers
	}
	if byID["inherits-default"] {
		t.Error("expected 'inherits-default' group to inherit Defaults.Dividers=false")
	}
	if !byID["opts-in"] {
		t.Error("expected 'opts-in' group's explicit dividers:true to override the default")
	}
	if byID["auto-1"] {
		t.Error("expected the auto compartment (component 'c') to use Defaults.Dividers=false")
	}
}

func TestLayoutBoxContentOffsetForGroupVsAuto(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Test", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 200, Depth: 100, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{Margin: 2},
		Components: []manifest.Component{
			{ID: "a", Name: "A", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "c", Name: "C", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "group", Components: []string{"a"}},
		},
	}

	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}

	byID := map[string]CompartmentResult{}
	for _, c := range res.Compartments {
		byID[c.ID] = c
	}
	g := byID["group"]
	if g.ContentOffsetX != 2 || g.ContentOffsetY != 2 {
		t.Errorf("expected group compartment ContentOffsetX/Y to equal its margin (2), got %v/%v", g.ContentOffsetX, g.ContentOffsetY)
	}
	auto := byID["auto-1"]
	if auto.ContentOffsetX != 0 || auto.ContentOffsetY != 0 {
		t.Errorf("expected auto compartment ContentOffsetX/Y to be 0, got %v/%v", auto.ContentOffsetX, auto.ContentOffsetY)
	}
}

func TestLayoutBoxFillRemainingDoesNotShiftContentOffset(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Roomy Box", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 80, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},

		Components: []manifest.Component{
			{ID: "widget", Name: "Widget", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "only-group", Components: []string{"widget"}},
		},
		Defaults: manifest.Defaults{Margin: 2, FillRemaining: true},
	}

	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}
	c := res.Compartments[0]
	if c.Bounds.W != 100 || c.Bounds.D != 80 {
		t.Fatalf("expected the sole compartment to grow to fill the 100x80 box, got %vx%v", c.Bounds.W, c.Bounds.D)
	}
	if c.ContentOffsetX != 2 || c.ContentOffsetY != 2 {
		t.Errorf("expected FillRemaining growth to leave ContentOffsetX/Y at the original margin (2), got %v/%v", c.ContentOffsetX, c.ContentOffsetY)
	}
}

func TestLayoutBoxExpandGrowsAllTrays(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 260, Depth: 260, Height: 50}},
		Material: manifest.Material{Thickness: 5},
		Defaults: manifest.Defaults{Margin: 1.5, Padding: 1, Expand: true},
		Components: []manifest.Component{
			{ID: "a", Width: 70, Depth: 92, Height: 40, Qty: 2, AllowRotate: boolPtr(false)},
			{ID: "b", Width: 70, Depth: 92, Height: 40, Qty: 2, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "t1", Components: []string{"a"}},
			{ID: "t2", Components: []string{"b"}},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Compartments) != 2 {
		t.Fatalf("want 2 compartments, got %d", len(res.Compartments))
	}
	for _, c := range res.Compartments {
		if c.Bounds.W != 260 || c.Bounds.D != 97 {
			t.Errorf("%s: want 260x97, got %vx%v", c.ID, c.Bounds.W, c.Bounds.D)
		}
		if c.CoreW != 147 {
			t.Errorf("%s: want core width 147, got %v", c.ID, c.CoreW)
		}
	}
	if res.Compartments[1].Bounds.Y != 97 {
		t.Errorf("second tray Y = %v, want 97", res.Compartments[1].Bounds.Y)
	}

	// Opt one tray out: it keeps its size.
	m.Groups[1].Expand = boolPtr(false)
	res, _ = LayoutBox(m)
	if c := res.Compartments[1]; c.Bounds.W != 147 || c.Bounds.D != 97 {
		t.Errorf("non-expanding tray resized: %vx%v", c.Bounds.W, c.Bounds.D)
	}
}

func TestLayoutBoxExpandSharesRowWidth(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 200, Depth: 200, Height: 50}},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{Expand: true},
		Components: []manifest.Component{
			{ID: "a", Width: 50, Depth: 40, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "b", Width: 50, Depth: 60, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "t1", Components: []string{"a"}},
			{ID: "t2", Components: []string{"b"}},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	a, b := res.Compartments[0], res.Compartments[1]
	if a.Bounds.Y != b.Bounds.Y || a.Bounds.W != 100 || b.Bounds.W != 100 || b.Bounds.X != 100 {
		t.Errorf("bad widths/positions: %+v %+v", a.Bounds, b.Bounds)
	}
	if a.Bounds.D == b.Bounds.D {
		t.Errorf("depths should stay independent, both %v", a.Bounds.D)
	}
}

func TestLayoutBoxRemovableAndFloorResolution(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 200, Depth: 200, Height: 50}},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{Margin: 2, Removable: true},
		Components: []manifest.Component{
			{ID: "a", Width: 50, Depth: 40, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "b", Width: 50, Depth: 40, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "t1", Components: []string{"a"}, Floor: boolPtr(true)},
			{ID: "t2", Components: []string{"b"}, Removable: boolPtr(false)},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]CompartmentResult{}
	for _, c := range res.Compartments {
		byID[c.ID] = c
	}
	t1, t2 := byID["t1"], byID["t2"]
	if !t1.Removable || !t1.Floor || t1.WallT != 3 || t1.ContentOffsetX != 5 || t1.Bounds.W != 60 || t1.Bounds.D != 50 {
		t.Errorf("t1 wrong: %+v", t1)
	}
	if t2.Removable || t2.WallT != 0 || t2.ContentOffsetX != 2 || t2.Bounds.W != 54 {
		t.Errorf("t2 wrong: %+v", t2)
	}
}

func TestLayoutBoxMissingReasonAndCountsForAutoPacked(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Box", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 20, Depth: 10, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Components: []manifest.Component{
			{ID: "tall", Name: "Tall", Width: 5, Depth: 5, Height: 100, Qty: 2, AllowRotate: boolPtr(false)},
			{ID: "chip", Name: "Chip", Width: 10, Depth: 10, Height: 5, Qty: 3, AllowRotate: boolPtr(false)},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}
	got := map[string]MissingItem{}
	for _, mi := range res.TotalMissing {
		got[mi.ComponentID] = mi
	}
	if mi := got["tall"]; mi.Reason != "too-tall" || mi.Requested != 2 || mi.Rejected != 2 {
		t.Errorf("tall: %+v", mi)
	}
	if mi := got["chip"]; mi.Requested != 3 || mi.Placed != 2 || mi.Rejected != 1 || mi.Reason != "no-space" {
		t.Errorf("chip: %+v", mi)
	}
}

func TestLayoutBoxUnplacedGroupExpandsToComponents(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Box", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 20, Depth: 20, Height: 50},
		},
		Material: manifest.Material{Thickness: 3},
		Components: []manifest.Component{
			{ID: "a", Name: "A", Width: 10, Depth: 10, Height: 5, Qty: 2, AllowRotate: boolPtr(false)},
			{ID: "b", Name: "B", Width: 5, Depth: 5, Height: 5, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "g1", Name: "G1", Components: []string{"a", "b"}},
		},
	}
	m.Box.Interior.Height = 3 // every component is too tall for the group to fit
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}
	got := map[string]MissingItem{}
	for _, mi := range res.TotalMissing {
		got[mi.ComponentID] = mi
	}
	if _, ok := got["g1"]; ok {
		t.Errorf("group ID must not appear as a component: %+v", res.TotalMissing)
	}
	if got["a"].Requested != 2 || got["a"].Rejected != 2 || got["b"].Rejected != 1 {
		t.Errorf("expected group components reported missing, got %+v", res.TotalMissing)
	}
}

func TestLayoutBoxFloorReducesMaxComponentHeight(t *testing.T) {
	// Height 49 fits a 50mm interior but not once a 3mm floor is taken out.
	build := func(floor bool, removable bool) BoxResult {
		m := &manifest.Manifest{
			Version: 1,
			Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
				Interior: manifest.Dimensions{Width: 200, Depth: 200, Height: 50}},
			Material: manifest.Material{Thickness: 3},
			Defaults: manifest.Defaults{Floor: floor, Removable: removable},
			Components: []manifest.Component{
				{ID: "g", Width: 20, Depth: 20, Height: 49, Qty: 1, AllowRotate: boolPtr(false)},
				{ID: "a", Width: 20, Depth: 20, Height: 49, Qty: 1, AllowRotate: boolPtr(false)},
			},
			Groups: []manifest.Group{{ID: "grp", Components: []string{"g"}}},
		}
		res, err := LayoutBox(m)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	for _, removable := range []bool{false, true} {
		if res := build(false, removable); len(res.TotalMissing) != 0 {
			t.Errorf("removable=%v no floor: unexpected missing %+v", removable, res.TotalMissing)
		}
		res := build(true, removable)
		if len(res.TotalMissing) != 2 {
			t.Errorf("removable=%v floor: expected 2 missing, got %+v", removable, res.TotalMissing)
		}
	}
}

func TestExpandCompartmentsBandsAndEpsilon(t *testing.T) {
	const boxW = 100.0
	cs := []CompartmentResult{
		// Band y=0: one expandable, one fixed to its right.
		{ID: "a", Expand: true, Bounds: Rect{X: 0, Y: 0, W: 40, D: 10}, Remaining: Space{Width: 1, Area: 10}},
		{ID: "b", Bounds: Rect{X: 40, Y: 0, W: 40, D: 10}},
		// Band y=50: two expandable, the second's Y differs by less than epsilon.
		{ID: "c", Expand: true, Bounds: Rect{X: 0, Y: 50, W: 30, D: 20}},
		{ID: "d", Expand: true, Bounds: Rect{X: 30, Y: 50 + epsilon/2, W: 20, D: 20}},
		// Band y=100: nothing expandable, must not move.
		{ID: "e", Bounds: Rect{X: 0, Y: 100, W: 10, D: 5}},
	}
	expandCompartments(cs, boxW)
	by := map[string]CompartmentResult{}
	for _, c := range cs {
		by[c.ID] = c
	}

	// Band 1: 20 leftover goes to "a"; "b" shifts right and keeps its width.
	if a := by["a"]; a.Bounds.X != 0 || a.Bounds.W != 60 || a.CoreW != 40 || a.Bounds.D != 10 {
		t.Errorf("a: %+v core %v", a.Bounds, a.CoreW)
	}
	if a := by["a"]; a.Remaining != (Space{}) {
		t.Errorf("a remaining should shrink to 0 (clamped), not grow: %+v", a.Remaining)
	}
	if b := by["b"]; b.Bounds.X != 60 || b.Bounds.W != 40 || b.CoreW != 0 {
		t.Errorf("b: %+v core %v", b.Bounds, b.CoreW)
	}
	// Band 2 (epsilon-grouped): 50 leftover split 25/25, d shifted by c's share.
	if c := by["c"]; c.Bounds.X != 0 || c.Bounds.W != 55 || c.CoreW != 30 {
		t.Errorf("c: %+v core %v", c.Bounds, c.CoreW)
	}
	if d := by["d"]; d.Bounds.X != 55 || d.Bounds.W != 45 || d.CoreW != 20 {
		t.Errorf("d: %+v core %v", d.Bounds, d.CoreW)
	}
	// Band 3 untouched.
	if e := by["e"]; e.Bounds.X != 0 || e.Bounds.W != 10 {
		t.Errorf("e moved: %+v", e.Bounds)
	}
}

func TestExpandCompartmentsBeyondEpsilonAreSeparateBands(t *testing.T) {
	cs := []CompartmentResult{
		{ID: "a", Expand: true, Bounds: Rect{X: 0, Y: 0, W: 40, D: 10}},
		{ID: "b", Expand: true, Bounds: Rect{X: 40, Y: 1e-3, W: 40, D: 10}},
	}
	expandCompartments(cs, 100)
	// Each is alone in its band and grows to the box edge independently.
	if cs[0].Bounds.W != 100 || cs[1].Bounds.W != 60 || cs[1].Bounds.X != 40 {
		t.Errorf("unexpected bands: %+v %+v", cs[0].Bounds, cs[1].Bounds)
	}
}

func TestExpandCompartmentsNoLeftoverIsNoop(t *testing.T) {
	cs := []CompartmentResult{
		{ID: "a", Expand: true, Bounds: Rect{X: 0, Y: 0, W: 60, D: 10}},
		{ID: "b", Expand: true, Bounds: Rect{X: 60, Y: 0, W: 50, D: 10}}, // overshoots a 100 box
	}
	expandCompartments(cs, 100)
	if cs[0].Bounds.W != 60 || cs[1].Bounds.X != 60 || cs[1].Bounds.W != 50 {
		t.Errorf("negative leftover must not shrink or shift: %+v %+v", cs[0].Bounds, cs[1].Bounds)
	}
	if cs[0].CoreW != 60 {
		t.Errorf("CoreW should still record pre-growth width, got %v", cs[0].CoreW)
	}
}

func TestLayoutBoxExpandMultipleBands(t *testing.T) {
	// Each 60-wide tray is 60x40 after margin; a 100-wide box forces one per
	// row, so every tray forms its own band and grows to the full width.
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 200, Height: 50}},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{Expand: true},
		Components: []manifest.Component{
			{ID: "a", Width: 60, Depth: 40, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "b", Width: 60, Depth: 30, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "c", Width: 60, Depth: 20, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "t1", Components: []string{"a"}},
			{ID: "t2", Components: []string{"b"}},
			{ID: "t3", Components: []string{"c"}, Expand: boolPtr(false)},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Compartments) != 3 {
		t.Fatalf("want 3 compartments, got %d", len(res.Compartments))
	}
	ys := map[float64]bool{}
	for _, c := range res.Compartments {
		ys[c.Bounds.Y] = true
		if c.ID == "t3" {
			if c.Bounds.W != 60 || c.CoreW != 0 {
				t.Errorf("t3 opted out but changed: %+v core %v", c.Bounds, c.CoreW)
			}
			continue
		}
		if c.Bounds.X != 0 || c.Bounds.W != 100 || c.CoreW != 60 {
			t.Errorf("%s: want full-width tray, got %+v core %v", c.ID, c.Bounds, c.CoreW)
		}
	}
	if len(ys) != 3 {
		t.Errorf("expected 3 separate bands, got Y set %v", ys)
	}
}

func TestLayoutBoxExpandThenFillRemaining(t *testing.T) {
	// Expand stretches width to the box edge; FillRemaining then only has
	// depth left to add to the last compartment.
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 200, Depth: 150, Height: 50}},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{Expand: true, FillRemaining: true},
		Components: []manifest.Component{
			{ID: "a", Width: 50, Depth: 40, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "b", Width: 50, Depth: 60, Height: 20, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "t1", Components: []string{"a"}},
			{ID: "t2", Components: []string{"b"}},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]CompartmentResult{}
	for _, c := range res.Compartments {
		by[c.ID] = c
	}
	// The shelf packer places the deeper t2 first (X=0); t1 is the last one.
	a, b := by["t2"], by["t1"]
	if a.Bounds.X != 0 || a.Bounds.W != 100 || b.Bounds.X != 100 || b.Bounds.W != 100 {
		t.Errorf("expand widths wrong: %+v %+v", a.Bounds, b.Bounds)
	}
	// Only the spatially-last tray (t1) gains the leftover depth.
	if a.Bounds.D != 60 || b.Bounds.D != 150 {
		t.Errorf("fillRemaining depth: first=%v last=%v, want 60 and 150", a.Bounds.D, b.Bounds.D)
	}
	if b.CoreW != 50 {
		t.Errorf("CoreW should stay pre-expand, got %v", b.CoreW)
	}
	if res.UsedW != 200 || res.UsedD != 150 || res.Remaining.Area != 0 {
		t.Errorf("box should be fully used: %+v", res)
	}

	// With Expand off for the last tray, FillRemaining grows it in both axes.
	m.Groups[0].Expand = boolPtr(false)
	res, _ = LayoutBox(m)
	last := res.Compartments[len(res.Compartments)-1]
	if last.ID != "t1" || last.Bounds.X+last.Bounds.W != 200 || last.Bounds.D != 150 {
		t.Errorf("fillRemaining should reach both edges: %+v", last)
	}
}

func TestLayoutBoxAutoCompartmentInheritsDefaultsOnly(t *testing.T) {
	// A group overrides every setting; the auto compartment for ungrouped
	// components must use the defaults, not the group's overrides.
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 200, Depth: 200, Height: 50}},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{
			Padding: 1, Margin: 2, Removable: true, Floor: true, Expand: true,
			FullWalls: true, JointType: manifest.JointTypePlain, Dividers: boolPtr(false),
		},
		Components: []manifest.Component{
			{ID: "g", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "loose", Width: 20, Depth: 20, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{{
			ID: "grp", Components: []string{"g"},
			Removable: boolPtr(false), Floor: boolPtr(false), Expand: boolPtr(false),
			FullWalls: boolPtr(false), JointType: jointTypePtr(manifest.JointTypeNotch),
			Dividers: boolPtr(true), Padding: new(float64), Margin: new(float64),
		}},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]CompartmentResult{}
	for _, c := range res.Compartments {
		by[c.Kind] = c
	}
	g, a := by["group"], by["auto"]
	if g.Removable || g.Floor || g.Expand || g.FullWalls || !g.Dividers || g.JointType != manifest.JointTypeNotch || g.WallT != 0 {
		t.Errorf("group overrides not applied: %+v", g)
	}
	if !a.Removable || !a.Floor || !a.Expand || !a.FullWalls || a.Dividers || a.JointType != manifest.JointTypePlain || a.WallT != 3 {
		t.Errorf("auto compartment should inherit defaults: %+v", a)
	}
	// Content 22 (20 + padding) plus 2*3 wall = 28 core, then expanded to the box edge.
	// The 172 of width slack now lies inside Bounds, so no width remains; the
	// free rect below the tray (200 x 180, 28 deep tray) keeps 152 of depth and
	// 200*180 - 200*28 = 30400 of area.
	if a.Remaining != (Space{Width: 0, Depth: 152, Area: 30400}) {
		t.Errorf("expanded auto compartment should have no Remaining, got %+v", a.Remaining)
	}
	if a.ContentOffsetX != 3 || a.ContentOffsetY != 3 || a.CoreW != 28 || a.Bounds.W != 200 {
		t.Errorf("auto offsets/size wrong: %+v core %v", a, a.CoreW)
	}
}

func TestLayoutBoxGroupThatDoesNotFitBox(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 100, Height: 50}},
		Material: manifest.Material{Thickness: 3},
		Components: []manifest.Component{
			{ID: "big1", Width: 80, Depth: 80, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "big2", Width: 80, Depth: 80, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "g1", Components: []string{"big1"}},
			{ID: "g2", Components: []string{"big2"}},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Compartments) != 1 || res.Compartments[0].ID != "g1" {
		t.Fatalf("expected only g1 placed, got %+v", res.Compartments)
	}
	if len(res.TotalMissing) != 1 {
		t.Fatalf("expected big2 missing, got %+v", res.TotalMissing)
	}
	if mi := res.TotalMissing[0]; mi.ComponentID != "big2" || mi.Rejected != 1 || mi.Placed != 0 || mi.Reason == "" {
		t.Errorf("big2 missing item wrong: %+v", mi)
	}
}

func TestLayoutBoxDroppedGroupKeepsInnerReasons(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 100, Height: 50}},
		Material: manifest.Material{Thickness: 3},
		Components: []manifest.Component{
			{ID: "big1", Width: 80, Depth: 80, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "big2", Width: 80, Depth: 80, Height: 10, Qty: 2, AllowRotate: boolPtr(false)},
			{ID: "tall", Width: 10, Depth: 10, Height: 90, Qty: 3, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "g1", Components: []string{"big1"}},
			{ID: "g2", Components: []string{"big2", "tall"}},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Compartments) != 1 || res.Compartments[0].ID != "g1" {
		t.Fatalf("expected only g1 placed, got %+v", res.Compartments)
	}
	got := map[string]MissingItem{}
	for _, mi := range res.TotalMissing {
		got[mi.ComponentID] = mi
	}
	if len(res.TotalMissing) != 2 {
		t.Fatalf("expected big2 and tall missing once each, got %+v", res.TotalMissing)
	}
	// big2 was placeable inside its group, so it gets the box-level reason.
	if mi := got["big2"]; mi.Requested != 2 || mi.Placed != 0 || mi.Rejected != 2 || mi.Reason == "" || mi.Reason == "too-tall" {
		t.Errorf("big2: %+v", mi)
	}
	// tall keeps the reason from the group's own packing.
	if mi := got["tall"]; mi.Requested != 3 || mi.Placed != 0 || mi.Rejected != 3 || mi.Reason != "too-tall" {
		t.Errorf("tall: %+v", mi)
	}
}

func TestLayoutBoxConvertsThicknessAndKerfFromBoxUnits(t *testing.T) {
	// material.thickness and kerf are authored in box.units and normalized
	// to mm once in LayoutBox (BoxResult.Material). With units "cm" a
	// thickness of 0.3 is 3 mm.
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsCM,
			Interior: manifest.Dimensions{Width: 20, Depth: 20, Height: 5}}, // 200x200x50 mm
		Material: manifest.Material{Name: "ply", Thickness: 0.3, Kerf: 0.01},
		Defaults: manifest.Defaults{Margin: 0.2, Removable: true}, // margin 2 mm
		Components: []manifest.Component{
			{ID: "g", Width: 5, Depth: 4, Height: 2, Qty: 1, AllowRotate: boolPtr(false)},     // 50x40x20 mm
			{ID: "loose", Width: 3, Depth: 3, Height: 2, Qty: 1, AllowRotate: boolPtr(false)}, // 30x30x20 mm
		},
		Groups: []manifest.Group{{ID: "grp", Components: []string{"g"}}},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.InteriorW != 200 || res.InteriorH != 50 {
		t.Fatalf("interior not converted to mm: %vx%v", res.InteriorW, res.InteriorH)
	}
	if math.Abs(res.Material.Thickness-3) > 1e-9 || math.Abs(res.Material.Kerf-0.1) > 1e-9 || res.Material.Name != "ply" {
		t.Fatalf("BoxResult.Material not normalized to mm: %+v", res.Material)
	}
	if m.Material.Thickness != 0.3 {
		t.Errorf("input manifest must not be mutated, got %v", m.Material.Thickness)
	}
	by := map[string]CompartmentResult{}
	for _, c := range res.Compartments {
		by[c.Kind] = c
	}
	g, a := by["group"], by["auto"]
	near := func(x, y float64) bool { return math.Abs(x-y) < 1e-9 }
	if !near(g.WallT, 3) || g.ContentOffsetX != 5 || !near(g.Bounds.W, 50+2*5) || !near(g.Bounds.D, 40+2*5) {
		t.Errorf("group: thickness must be converted to mm: %+v", g)
	}
	if !near(a.WallT, 3) || a.ContentOffsetX != 3 || !near(a.Bounds.W, 30+2*3) {
		t.Errorf("auto: thickness must be converted to mm: %+v", a)
	}
}

func TestMaterialToMM(t *testing.T) {
	for _, tc := range []struct {
		u    manifest.Units
		want float64
	}{{"", 3.175}, {manifest.UnitsMM, 3.175}, {manifest.UnitsCM, 31.75}, {manifest.UnitsIN, 3.175 * 25.4}} {
		got, err := manifest.Material{Name: "x", Thickness: 3.175, Kerf: 0.5}.ToMM(tc.u)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got.Thickness-tc.want) > 1e-9 || math.Abs(got.Kerf-0.5*tc.want/3.175) > 1e-9 || got.Name != "x" {
			t.Errorf("units %q: got %+v, want thickness %v", tc.u, got, tc.want)
		}
	}
	if _, err := (manifest.Material{}).ToMM("furlong"); err == nil {
		t.Error("expected error for unknown units")
	}
}

func TestExpandShrinksRemainingByShare(t *testing.T) {
	cs := []CompartmentResult{
		{ID: "a", Expand: true, Bounds: Rect{W: 40, D: 10}, Remaining: Space{Width: 50, Depth: 3, Area: 700}},
		{ID: "g", Expand: true, Kind: "group", Bounds: Rect{Y: 50, W: 40, D: 10}},
	}
	expandCompartments(cs, 100)
	// a: 60 slack absorbed; Remaining.Width 50 clamps to 0, depth untouched, area 700-600.
	if want := (Space{Width: 0, Depth: 3, Area: 100}); cs[0].Remaining != want {
		t.Errorf("a remaining = %+v, want %+v", cs[0].Remaining, want)
	}
	if cs[1].Remaining != (Space{}) {
		t.Errorf("group remaining must stay empty, got %+v", cs[1].Remaining)
	}
	for _, c := range cs {
		if c.Remaining.Width < 0 || c.Remaining.Depth < 0 || c.Remaining.Area < 0 {
			t.Errorf("%s: negative remaining %+v", c.ID, c.Remaining)
		}
	}
}

func allRejectedManifest(removable bool, margin float64) *manifest.Manifest {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{
			Name: "Box", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 100, Height: 20},
		},
		Material: manifest.Material{Thickness: 3},
		Components: []manifest.Component{
			{ID: "tall", Name: "Tall", Width: 10, Depth: 10, Height: 40, Qty: 2, AllowRotate: boolPtr(false)},
			{ID: "wide", Name: "Wide", Width: 500, Depth: 10, Height: 5, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "ok", Name: "Ok", Width: 10, Depth: 10, Height: 5, Qty: 1, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{
			{ID: "dead", Name: "Dead", Components: []string{"tall", "wide"}, Removable: boolPtr(removable)},
			{ID: "live", Name: "Live", Components: []string{"ok"}},
		},
	}
	m.Defaults.Margin = margin
	return m
}

func checkAllRejected(t *testing.T, m *manifest.Manifest) {
	t.Helper()
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}
	for _, c := range res.Compartments {
		if c.ID == "dead" {
			t.Fatalf("all-rejected group must not produce a compartment: %+v", c)
		}
		if c.Bounds.W <= 0 || c.Bounds.D <= 0 {
			t.Errorf("degenerate compartment %+v", c)
		}
	}
	if len(res.Compartments) != 1 || res.Compartments[0].ID != "live" {
		t.Fatalf("expected only the live compartment, got %+v", res.Compartments)
	}
	got := map[string]MissingItem{}
	for _, mi := range res.TotalMissing {
		got[mi.ComponentID] = mi
	}
	if mi := got["tall"]; mi.Requested != 2 || mi.Rejected != 2 || mi.Placed != 0 || mi.Reason != "too-tall" {
		t.Errorf("tall: %+v", mi)
	}
	if mi := got["wide"]; mi.Requested != 1 || mi.Rejected != 1 || mi.Reason != "too-wide" {
		t.Errorf("wide: %+v", mi)
	}
	if _, ok := got["ok"]; ok || len(res.TotalMissing) != 2 {
		t.Errorf("unexpected missing list: %+v", res.TotalMissing)
	}
}

func TestLayoutBoxAllRejectedGroupNonRemovableZeroMargin(t *testing.T) {
	checkAllRejected(t, allRejectedManifest(false, 0))
}

func TestLayoutBoxAllRejectedGroupRemovable(t *testing.T) {
	checkAllRejected(t, allRejectedManifest(true, 0))
}

func TestLayoutBoxMixedGroupKeepsCompartment(t *testing.T) {
	m := allRejectedManifest(false, 0)
	m.Groups = []manifest.Group{{ID: "mixed", Name: "Mixed", Components: []string{"tall", "ok"}}}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}
	if len(res.Compartments) != 1 || res.Compartments[0].ID != "mixed" {
		t.Fatalf("expected the mixed compartment, got %+v", res.Compartments)
	}
	if c := res.Compartments[0]; c.Bounds.W <= 0 || len(c.Rows) == 0 {
		t.Errorf("mixed compartment should have content: %+v", c)
	}
	var tall *MissingItem
	for i := range res.TotalMissing {
		if res.TotalMissing[i].ComponentID == "tall" {
			tall = &res.TotalMissing[i]
		}
	}
	if tall == nil || tall.Reason != "too-tall" || tall.Rejected != 2 {
		t.Errorf("tall should be reported missing, got %+v", res.TotalMissing)
	}
}

func TestLayoutBoxEveryGroupEmpty(t *testing.T) {
	m := allRejectedManifest(false, 0)
	m.Groups = m.Groups[:1]
	m.Components = m.Components[:2]
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatalf("LayoutBox: %v", err)
	}
	if len(res.Compartments) != 0 || len(res.TotalMissing) != 2 {
		t.Fatalf("expected no compartments and 2 missing, got %+v", res)
	}
}

func TestLayoutBoxProject(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 100, Height: 50}},
		Material:   manifest.Material{Thickness: 3},
		Components: []manifest.Component{{ID: "a", Width: 10, Depth: 10, Height: 5, Qty: 1}},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project != nil {
		t.Fatalf("Project = %+v, want nil when the manifest has none", res.Project)
	}

	m.Project = manifest.Project{Name: "P", Tags: []string{"x"}}
	res, err = LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project == nil || res.Project.Name != "P" || len(res.Project.Tags) != 1 {
		t.Fatalf("Project = %+v", res.Project)
	}
	res.Project.Tags[0] = "changed"
	if m.Project.Tags[0] != "x" {
		t.Fatal("BoxResult.Project shares its tags slice with the manifest")
	}
}

func TestLayoutBoxHeightReductionInheritance(t *testing.T) {
	m := &manifest.Manifest{
		Version:  1,
		Box:      manifest.Box{Name: "b", Units: manifest.UnitsCM, Interior: manifest.Dimensions{Width: 20, Depth: 20, Height: 5}},
		Material: manifest.Material{Thickness: 0.3},
		Defaults: manifest.Defaults{
			ExternalHeightReduction: manifest.HeightReduction{Amount: 1},
			DividerHeightReduction:  manifest.HeightReduction{Percent: 20},
		},
		Components: []manifest.Component{
			{ID: "a", Width: 2, Depth: 2, Height: 1, Qty: 1},
			{ID: "b", Width: 2, Depth: 2, Height: 1, Qty: 1},
		},
		Groups: []manifest.Group{
			{ID: "g1", Components: []string{"a"}},
			{ID: "g2", Components: []string{"b"}, DividerHeightReduction: &manifest.HeightReduction{Amount: 0.5, Panels: []string{"ih-*"}}},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, cr := range res.Compartments {
		if cr.ExternalReduction.AmountMM != 10 {
			t.Errorf("%s: external %+v, want 10 mm (1 cm)", cr.ID, cr.ExternalReduction)
		}
		switch cr.ID {
		case "g1":
			if cr.DividerReduction.Percent != 20 {
				t.Errorf("g1 should inherit 20%%, got %+v", cr.DividerReduction)
			}
		case "g2":
			if cr.DividerReduction.AmountMM != 5 || len(cr.DividerReduction.Panels) != 1 {
				t.Errorf("g2 should override to 5 mm on ih-*, got %+v", cr.DividerReduction)
			}
		}
	}
}

func TestLayoutBoxHeightReductionGroupReplacesDefault(t *testing.T) {
	m := &manifest.Manifest{
		Version:  1,
		Box:      manifest.Box{Name: "b", Units: manifest.UnitsMM, Interior: manifest.Dimensions{Width: 200, Depth: 200, Height: 50}},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{
			ExternalHeightReduction: manifest.HeightReduction{Percent: 20, Panels: []string{"w-*"}},
			DividerHeightReduction:  manifest.HeightReduction{Amount: 4},
		},
		Components: []manifest.Component{
			{ID: "a", Width: 20, Depth: 20, Height: 10, Qty: 1},
			{ID: "b", Width: 20, Depth: 20, Height: 10, Qty: 1},
		},
		Groups: []manifest.Group{
			{ID: "g1", Components: []string{"a"}},
			{ID: "g2", Components: []string{"b"},
				ExternalHeightReduction: &manifest.HeightReduction{Amount: 2},
				DividerHeightReduction:  &manifest.HeightReduction{Percent: 10}},
		},
	}
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, cr := range res.Compartments {
		switch cr.ID {
		case "g1":
			if cr.ExternalReduction.Percent != 20 || len(cr.ExternalReduction.Panels) != 1 || cr.DividerReduction.AmountMM != 4 {
				t.Errorf("g1 should inherit defaults, got %+v / %+v", cr.ExternalReduction, cr.DividerReduction)
			}
		case "g2":
			// Replacement, not a merge: the default's percent and panel
			// filter must not leak into the group's setting.
			if e := cr.ExternalReduction; e.AmountMM != 2 || e.Percent != 0 || len(e.Panels) != 0 {
				t.Errorf("g2 external = %+v, want 2 mm only", e)
			}
			if d := cr.DividerReduction; d.Percent != 10 || d.AmountMM != 0 {
				t.Errorf("g2 divider = %+v, want 10%% only", d)
			}
		}
	}
}

// autoSpillManifest builds a 100x80 box with one 60x40 group tray in the
// corner (always non-removable, so its geometry is stable), leaving two free
// rects: the bottom gap (100x40) and the trailing gap of the group row
// (40x40). Ungrouped component "s" is packed into them.
func autoSpillManifest(removableDefault bool, w, d float64, qty int) *manifest.Manifest {
	return &manifest.Manifest{
		Version: 1,
		Box: manifest.Box{Name: "B", Units: manifest.UnitsMM,
			Interior: manifest.Dimensions{Width: 100, Depth: 80, Height: 50}},
		Material: manifest.Material{Thickness: 3},
		Defaults: manifest.Defaults{Removable: removableDefault},
		Components: []manifest.Component{
			{ID: "g", Width: 60, Depth: 40, Height: 10, Qty: 1, AllowRotate: boolPtr(false)},
			{ID: "s", Width: w, Depth: d, Height: 10, Qty: qty, AllowRotate: boolPtr(false)},
		},
		Groups: []manifest.Group{{ID: "grp", Components: []string{"g"}, Removable: boolPtr(false)}},
	}
}

func autoCompartments(res BoxResult) []CompartmentResult {
	var out []CompartmentResult
	for _, c := range res.Compartments {
		if c.Kind == "auto" {
			out = append(out, c)
		}
	}
	return out
}

func TestLayoutBoxAutoPackSpillsAcrossFreeRects(t *testing.T) {
	// Free rects: bottom 100x40 (area 4000, tried first) and trailing 40x40.
	// 35x30 fits two per row in the bottom rect (a third would need 105 > 100);
	// the third spills into the trailing rect as a second auto compartment.
	res, err := LayoutBox(autoSpillManifest(false, 35, 30, 3))
	if err != nil {
		t.Fatal(err)
	}
	autos := autoCompartments(res)
	if len(autos) != 2 {
		t.Fatalf("want 2 auto compartments, got %d: %+v", len(autos), res.Compartments)
	}
	if len(res.TotalMissing) != 0 {
		t.Errorf("nothing should be missing: %+v", res.TotalMissing)
	}
	by := map[string]CompartmentResult{}
	for _, c := range autos {
		by[c.ID] = c
	}
	a1, a2 := by["auto-1"], by["auto-2"]
	if a1.Bounds.X != 0 || a1.Bounds.Y != 40 || a1.Bounds.W != 70 || a1.Bounds.D != 30 {
		t.Errorf("auto-1 should sit in the bottom rect: %+v", a1.Bounds)
	}
	if a2.Bounds.X != 60 || a2.Bounds.Y != 0 || a2.Bounds.W != 35 || a2.Bounds.D != 30 {
		t.Errorf("auto-2 should sit in the trailing rect: %+v", a2.Bounds)
	}
	count := func(c CompartmentResult) int {
		n := 0
		for _, r := range c.Rows {
			for range r.Items {
				n++
			}
		}
		return n
	}
	if count(a1) != 2 || count(a2) != 1 {
		t.Errorf("want 2 items then 1, got %d and %d", count(a1), count(a2))
	}
}

func TestLayoutBoxAutoPackRemovableShrinksRectsByTwoWall(t *testing.T) {
	// Same layout with removable defaults: each free rect shrinks by
	// 2*thickness (6) per axis before packing, so the trailing 40x40 rect
	// offers only 34x34 and the 35-wide third item no longer fits anywhere.
	// The first reason (bottom rect: no-space) wins over the later one
	// (trailing rect: too-wide).
	res, err := LayoutBox(autoSpillManifest(true, 35, 30, 3))
	if err != nil {
		t.Fatal(err)
	}
	autos := autoCompartments(res)
	if len(autos) != 1 {
		t.Fatalf("want 1 auto compartment, got %+v", res.Compartments)
	}
	a := autos[0]
	// 70x30 of content plus a wall on every side.
	if a.WallT != 3 || a.Bounds.W != 76 || a.Bounds.D != 36 || a.ContentOffsetX != 3 {
		t.Errorf("removable auto tray wrong: %+v", a)
	}
	if len(res.TotalMissing) != 1 {
		t.Fatalf("want 1 missing, got %+v", res.TotalMissing)
	}
	mi := res.TotalMissing[0]
	if mi.ComponentID != "s" || mi.Requested != 3 || mi.Placed != 2 || mi.Rejected != 1 || mi.Reason != "no-space" {
		t.Errorf("first failure reason (no-space) should be kept: %+v", mi)
	}
}

func TestLayoutBoxAutoPackKeepsFirstFailureReason(t *testing.T) {
	// 45x30 x4: two fit in the bottom rect (90 wide); the other two fail there
	// with no-space, then the trailing 40x40 rect rejects them as too-wide.
	// The reported reason is the one from the largest rect (the first tried).
	res, err := LayoutBox(autoSpillManifest(false, 45, 30, 4))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TotalMissing) != 1 {
		t.Fatalf("want 1 missing, got %+v", res.TotalMissing)
	}
	mi := res.TotalMissing[0]
	if mi.Reason != "no-space" || mi.Placed != 2 || mi.Rejected != 2 {
		t.Errorf("want no-space with 2 placed / 2 rejected, got %+v", mi)
	}
}

func TestLayoutBoxAutoPackIgnoresPerGroupSettings(t *testing.T) {
	// The group sets a large padding and margin; auto-packed components use
	// defaults.padding (0), so 35-wide items sit flush (two = 70 wide).
	// The one group setting that still reaches auto compartments is floor
	// when the defaults are non-removable: any floored tray lowers the shared
	// grid, so the auto height limit drops by one thickness (50 - 3 = 47).
	m := autoSpillManifest(false, 35, 30, 2)
	pad := 5.0
	m.Groups[0].Padding = &pad
	m.Groups[0].Margin = &pad
	m.Groups[0].Floor = boolPtr(true)
	m.Components[1].Height = 48 // taller than 47 -> rejected only via the group floor
	res, err := LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TotalMissing) != 1 || res.TotalMissing[0].ComponentID != "s" || res.TotalMissing[0].Reason != "too-tall" {
		t.Errorf("expected s too-tall because of group floor, got %+v", res.TotalMissing)
	}

	// Group padding 5 grows the group tray to 80x60, leaving a 100x20 bottom
	// rect. Three 15x15 items sit flush there (45 wide), not 5 apart (60).
	m = autoSpillManifest(false, 15, 15, 3)
	m.Groups[0].Padding = &pad
	res, err = LayoutBox(m)
	if err != nil {
		t.Fatal(err)
	}
	autos := autoCompartments(res)
	if len(autos) != 1 || len(res.TotalMissing) != 0 {
		t.Fatalf("expected one auto compartment, got %+v missing %+v", res.Compartments, res.TotalMissing)
	}
	if autos[0].UsedW != 45 || autos[0].UsedD != 15 || autos[0].ContentOffsetX != 0 {
		t.Errorf("auto packing should use default padding 0: %+v", autos[0])
	}
}
