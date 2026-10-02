package isometric

import (
	"bytes"
	"encoding/xml"
	"errors"
	"image/png"
	"io"
	"math"
	"testing"

	"golang.org/x/image/font"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func fixture() (pack.BoxResult, []geometry.Panel, manifest.Material) {
	box := pack.BoxResult{
		BoxName:   "Test Box",
		InteriorW: 300,
		InteriorD: 200,
		InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "c1", Name: "Chess", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 150, D: 200}},
		},
		TotalMissing: []pack.MissingItem{{ComponentID: "die", Requested: 5, Placed: 3, Rejected: 2, Reason: "no-space"}},
	}
	panels := []geometry.Panel{
		{
			ID: "h-1", Axis: geometry.AxisWidthRun,
			Length: 300, Height: 60, Thickness: 3,
			Outline:  geometry.BuildOutline(300, 60, nil),
			Position: geometry.Placement3D{OriginX: 0, OriginY: 0, OriginZ: 0},
		},
		{
			ID: "v-1", Axis: geometry.AxisDepthRun,
			Length: 200, Height: 60, Thickness: 3,
			Notches:  []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}},
			Outline:  geometry.BuildOutline(200, 60, []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}}),
			Position: geometry.Placement3D{OriginX: 150, OriginY: 0, OriginZ: 0},
		},
	}
	mat := manifest.Material{Name: "Foamboard", Thickness: 3, Kerf: 0.1}
	return box, panels, mat
}

func TestBuildSceneProducesGeometryAndAnnotations(t *testing.T) {
	box, panels, mat := fixture()

	for _, exploded := range []bool{false, true} {
		s := BuildScene(box, panels, mat, exploded)

		if len(s.Polys) == 0 {
			t.Fatalf("exploded=%v: expected panel face polygons, got none", exploded)
		}
		if len(s.Lines) == 0 {
			t.Fatalf("exploded=%v: expected dimension/compartment lines, got none", exploded)
		}
		if len(s.Labels) < len(panels) {
			t.Fatalf("exploded=%v: expected at least one label per panel, got %d labels for %d panels", exploded, len(s.Labels), len(panels))
		}
		if s.MaxX <= s.MinX || s.MaxY <= s.MinY {
			t.Fatalf("exploded=%v: expected a non-degenerate bounding box, got (%g,%g)-(%g,%g)", exploded, s.MinX, s.MinY, s.MaxX, s.MaxY)
		}
	}
}

// TestRectBox3DSpansTheFullRectangle guards against a regression where the
// 3D extent of a decomposed panel rectangle was derived from a single
// corner's thickness offset only, collapsing every face to a sliver the
// size of the material thickness instead of the rectangle's real width and
// height.
func TestRectBox3DSpansTheFullRectangle(t *testing.T) {
	p := geometry.Panel{
		Axis: geometry.AxisWidthRun, Thickness: 3,
		Position: geometry.Placement3D{},
	}
	r := geometry.PanelRect{X: 0, Y: 0, W: 300, H: 60}

	_, _, _, w, d, h := p.RectBox(r)
	if w != 300 {
		t.Fatalf("expected width extent 300 (r.W), got %g", w)
	}
	if d != 3 {
		t.Fatalf("expected depth extent 3 (material thickness), got %g", d)
	}
	if h != 60 {
		t.Fatalf("expected height extent 60 (r.H), got %g", h)
	}
}

func TestExplodedPositionPushesPanelsAwayFromCenter(t *testing.T) {
	box := pack.BoxResult{InteriorW: 300, InteriorD: 200}

	widthRun := geometry.Panel{Axis: geometry.AxisWidthRun, Position: geometry.Placement3D{OriginY: 20}}
	pos := explodedPosition(widthRun, box)
	if got, orig, center := pos.OriginY, widthRun.Position.OriginY, box.InteriorD/2; !(absF(got-center) > absF(orig-center)) {
		t.Fatalf("width-run panel should move further from the depth-center %g: orig=%g exploded=%g", center, orig, got)
	}

	depthRun := geometry.Panel{Axis: geometry.AxisDepthRun, Position: geometry.Placement3D{OriginX: 40}}
	pos = explodedPosition(depthRun, box)
	if got, orig, center := pos.OriginX, depthRun.Position.OriginX, box.InteriorW/2; !(absF(got-center) > absF(orig-center)) {
		t.Fatalf("depth-run panel should move further from the width-center %g: orig=%g exploded=%g", center, orig, got)
	}
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestSVGExportersProduceWellFormedXML(t *testing.T) {
	box, panels, mat := fixture()
	for _, exp := range []SVGExporter{{Exploded: false}, {Exploded: true}} {
		var buf bytes.Buffer
		if err := exp.Export(&buf, box, panels, mat); err != nil {
			t.Fatalf("%s: %v", exp.Format(), err)
		}

		dec := xml.NewDecoder(&buf)
		for {
			_, err := dec.Token()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatalf("%s: output is not well-formed XML: %v", exp.Format(), err)
			}
		}
	}
}

func TestSVGExportersRequireDistinctFormatNames(t *testing.T) {
	if (SVGExporter{}).Format() == (SVGExporter{Exploded: true}).Format() {
		t.Fatal("assembled and exploded SVG exporters must report distinct format names")
	}
	if (PNGExporter{}).Format() == (PNGExporter{Exploded: true}).Format() {
		t.Fatal("assembled and exploded PNG exporters must report distinct format names")
	}
}

func TestPNGExportersProduceDecodablyNonBlankImages(t *testing.T) {
	box, panels, mat := fixture()
	for _, exp := range []PNGExporter{{Exploded: false}, {Exploded: true}} {
		var buf bytes.Buffer
		if err := exp.Export(&buf, box, panels, mat); err != nil {
			t.Fatalf("%s: %v", exp.Format(), err)
		}

		img, err := png.Decode(&buf)
		if err != nil {
			t.Fatalf("%s: output is not a decodable PNG: %v", exp.Format(), err)
		}

		bounds := img.Bounds()
		if bounds.Dx() < 10 || bounds.Dy() < 10 {
			t.Fatalf("%s: expected a reasonably sized image, got %dx%d", exp.Format(), bounds.Dx(), bounds.Dy())
		}

		nonWhite := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r != 0xffff || g != 0xffff || b != 0xffff {
					nonWhite++
				}
			}
		}
		if nonWhite == 0 {
			t.Fatalf("%s: expected some non-white (drawn) pixels", exp.Format())
		}
	}
}

func TestExplodedDepthRunPanelMovesOnlyAlongX(t *testing.T) {
	box := pack.BoxResult{BoxName: "B", InteriorW: 300, InteriorD: 200, InteriorH: 60}
	mat := manifest.Material{Name: "M", Thickness: 3}
	mk := func(originX float64) []geometry.Panel {
		return []geometry.Panel{{
			ID: "v-1", Axis: geometry.AxisDepthRun,
			Length: 200, Height: 60, Thickness: 3,
			Outline:  geometry.BuildOutline(200, 60, nil),
			Position: geometry.Placement3D{OriginX: originX, OriginY: 7, OriginZ: 2},
		}}
	}

	// center 150, origin 40 -> 150 + (40-150)*1.6 = -26
	exploded := BuildScene(box, mk(40), mat, true)
	manual := BuildScene(box, mk(-26), mat, false)

	if len(exploded.Polys) == 0 {
		t.Fatal("expected exploded scene to contain faces")
	}
	if len(exploded.Polys) != len(manual.Polys) {
		t.Fatalf("expected %d faces, got %d", len(manual.Polys), len(exploded.Polys))
	}
	for i := range exploded.Polys {
		for j, p := range exploded.Polys[i].Points {
			q := manual.Polys[i].Points[j]
			if absF(p.X-q.X) > 1e-6 || absF(p.Y-q.Y) > 1e-6 {
				t.Fatalf("poly %d point %d: exploded %v differs from panel manually moved along X only %v", i, j, p, q)
			}
		}
	}
}

func TestExplodedSceneKeepsFaceCountAndSpreadsBounds(t *testing.T) {
	box, panels, mat := fixture()
	// Add a second depth-run panel so spreading along X is measurable.
	extra := panels[1]
	extra.ID = "v-2"
	extra.Position.OriginX = 250
	panels = append(panels, extra)

	assembled := BuildScene(box, panels, mat, false)
	exploded := BuildScene(box, panels, mat, true)
	if len(assembled.Polys) != len(exploded.Polys) {
		t.Fatalf("exploding must not add or drop faces: assembled=%d exploded=%d", len(assembled.Polys), len(exploded.Polys))
	}
	if !(exploded.MaxX-exploded.MinX > assembled.MaxX-assembled.MinX ||
		exploded.MaxY-exploded.MinY > assembled.MaxY-assembled.MinY) {
		t.Fatal("expected exploded scene to have larger projected bounds than assembled")
	}
}

func TestIsometricExportersHandleEmptyPanelsAndZeroBox(t *testing.T) {
	mat := manifest.Material{Name: "M", Thickness: 3}
	boxes := map[string]pack.BoxResult{
		"empty-panels": {BoxName: "E", InteriorW: 100, InteriorD: 100, InteriorH: 30},
		"zero-box":     {BoxName: "Z"},
	}
	for name, box := range boxes {
		for _, exploded := range []bool{false, true} {
			var svgBuf bytes.Buffer
			if err := (SVGExporter{Exploded: exploded}).Export(&svgBuf, box, nil, mat); err != nil {
				t.Fatalf("%s svg exploded=%v: %v", name, exploded, err)
			}
			dec := xml.NewDecoder(&svgBuf)
			for {
				_, err := dec.Token()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("%s svg exploded=%v: malformed XML: %v", name, exploded, err)
				}
			}

			var pngBuf bytes.Buffer
			if err := (PNGExporter{Exploded: exploded}).Export(&pngBuf, box, nil, mat); err != nil {
				t.Fatalf("%s png exploded=%v: %v", name, exploded, err)
			}
			img, err := png.Decode(&pngBuf)
			if err != nil {
				t.Fatalf("%s png exploded=%v: not decodable: %v", name, exploded, err)
			}
			if b := img.Bounds(); b.Dx() < 1 || b.Dy() < 1 {
				t.Fatalf("%s png exploded=%v: empty image %v", name, exploded, b)
			}
		}
	}
}

func TestPNGLargeBoxIsClampedToMaxSide(t *testing.T) {
	box := pack.BoxResult{BoxName: "Big", InteriorW: 5000, InteriorD: 4000, InteriorH: 1000}
	panels := []geometry.Panel{{
		ID: "h-1", Axis: geometry.AxisWidthRun,
		Length: 5000, Height: 1000, Thickness: 3,
		Outline: geometry.BuildOutline(5000, 1000, nil),
	}}
	var buf bytes.Buffer
	if err := (PNGExporter{}).Export(&buf, box, panels, manifest.Material{Name: "M", Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() > int(pngMaxSide)+1 || b.Dy() > int(pngMaxSide)+1 {
		t.Fatalf("image %dx%d exceeds clamp %g", b.Dx(), b.Dy(), pngMaxSide)
	}
	if b.Dx() < int(pngMaxSide)-2 && b.Dy() < int(pngMaxSide)-2 {
		t.Fatalf("expected the larger side to sit at the clamp, got %dx%d", b.Dx(), b.Dy())
	}
}

// posIn returns the position of the box with the given id in order.
func posIn(order []int, id int) int {
	for i, v := range order {
		if v == id {
			return i
		}
	}
	return -1
}

func TestOrderBoxesEggCrateBackToFront(t *testing.T) {
	// A crossing egg-crate ring seen from +X+Y+Z: the camera is nearer to
	// higher X, Y and Z, so a box must be drawn after any box that is
	// separated from it on the low side of an axis.
	//   A: low-Y wall   B: high-Y wall   C: low-X wall   D: high-X wall
	const (
		A = iota
		B
		C
		D
	)
	box := func(x0, x1, y0, y1 float64) sceneBox {
		return sceneBox{min: vec3{x0, y0, 0}, max: vec3{x1, y1, 50}}
	}
	boxes := []sceneBox{
		A: box(0, 100, 0, 3),
		B: box(0, 100, 97, 100),
		C: box(0, 3, 3, 97),
		D: box(97, 100, 3, 97),
	}
	want := [][2]int{{A, B}, {A, C}, {A, D}, {C, B}, {D, B}, {C, D}}
	// Try every rotation of the input order: the result must not depend on it.
	for shift := 0; shift < len(boxes); shift++ {
		rot := make([]sceneBox, len(boxes))
		ids := make([]int, len(boxes))
		for i := range boxes {
			rot[i] = boxes[(i+shift)%len(boxes)]
			ids[i] = (i + shift) % len(boxes)
		}
		order := orderBoxes(rot)
		final := make([]int, len(order))
		for i, o := range order {
			final[i] = ids[o]
		}
		for _, w := range want {
			if posIn(final, w[0]) > posIn(final, w[1]) {
				t.Fatalf("shift %d: box %d must be drawn before box %d, got order %v", shift, w[0], w[1], final)
			}
		}
	}
}

func TestOrderBoxesLongThinPanelBeatsAverageDepth(t *testing.T) {
	// A long thin wall along X at low Y, and a small block at low X just
	// in front of it (higher Y). The block is nearer to the camera, yet its
	// average depth is far smaller than the long wall's, so a per-face
	// average-depth sort would draw it first and let the wall cover it.
	wall := sceneBox{min: vec3{0, 0, 0}, max: vec3{1000, 3, 10}}
	block := sceneBox{min: vec3{0, 3, 0}, max: vec3{3, 20, 10}}
	if wall.centerDepth() < block.centerDepth() {
		t.Fatal("test setup: expected the wall's center to be deeper toward the camera than the block's")
	}
	order := orderBoxes([]sceneBox{block, wall})
	if order[0] != 1 || order[1] != 0 {
		t.Fatalf("wall (index 1) must be drawn before the block in front of it, got %v", order)
	}
}

func TestOrderBoxesToleratesCyclesAndKeepsAllBoxes(t *testing.T) {
	// Three boxes chasing each other around a pinwheel have no valid order;
	// every box must still be emitted exactly once.
	boxes := []sceneBox{
		{min: vec3{0, 0, 0}, max: vec3{10, 3, 3}},
		{min: vec3{8, 0, 0}, max: vec3{11, 10, 3}},
		{min: vec3{0, 8, 0}, max: vec3{11, 11, 3}},
		{min: vec3{0, 0, 0}, max: vec3{3, 11, 3}},
	}
	order := orderBoxes(boxes)
	if len(order) != len(boxes) {
		t.Fatalf("order %v lost boxes", order)
	}
	seen := map[int]bool{}
	for _, o := range order {
		if seen[o] {
			t.Fatalf("duplicate box in order %v", order)
		}
		seen[o] = true
	}
}

func hiddenFixture() (pack.BoxResult, []geometry.Panel, manifest.Material) {
	box := pack.BoxResult{BoxName: "Hidden", InteriorW: 300, InteriorD: 100, InteriorH: 100}
	panels := []geometry.Panel{
		{ // large wall near the camera side (high Y)
			ID: "front-wall", Axis: geometry.AxisWidthRun,
			Length: 300, Height: 100, Thickness: 3,
			Outline:  geometry.BuildOutline(300, 100, nil),
			Position: geometry.Placement3D{OriginY: 50},
		},
		{ // small panel entirely in the big wall's shadow
			ID: "tucked", Axis: geometry.AxisWidthRun,
			Length: 50, Height: 20, Thickness: 3,
			Outline:  geometry.BuildOutline(50, 20, nil),
			Position: geometry.Placement3D{OriginX: 100},
		},
	}
	return box, panels, manifest.Material{Name: "M", Thickness: 3}
}

func labelTexts(s *Scene) map[string]bool {
	out := map[string]bool{}
	for _, l := range s.Labels {
		out[l.Text] = true
	}
	return out
}

func TestHiddenPanelIsNotLabelledButKeepsItsFaces(t *testing.T) {
	box, panels, mat := hiddenFixture()
	s := BuildScene(box, panels, mat, false)
	texts := labelTexts(s)
	if !texts["front-wall"] {
		t.Fatalf("visible panel must be labelled: %v", texts)
	}
	if texts["tucked"] {
		t.Fatalf("fully hidden panel should not be labelled: %v", texts)
	}
	// Dropping a label must not change the drawn faces: 3 visible faces per box.
	if len(s.Polys) != 6 {
		t.Fatalf("polys = %d, want 6", len(s.Polys))
	}
}

func TestPartiallyVisiblePanelKeepsLabel(t *testing.T) {
	box, panels, mat := fixture() // v-1 crosses h-1 and is only partly covered
	texts := labelTexts(BuildScene(box, panels, mat, false))
	for _, id := range []string{"h-1", "v-1"} {
		if !texts[id] {
			t.Fatalf("panel %q lost its label: %v", id, texts)
		}
	}
}

func TestPNGLegendFitsMeasuredTextWidth(t *testing.T) {
	long := "A very long box name that would overflow a per-character width estimate by a wide margin — isometric preview"
	box := pack.BoxResult{BoxName: long, InteriorW: 40, InteriorD: 40, InteriorH: 20}
	panels := []geometry.Panel{{
		ID: "h-1", Axis: geometry.AxisWidthRun,
		Length: 40, Height: 20, Thickness: 3,
		Outline: geometry.BuildOutline(40, 20, nil),
	}}
	var buf bytes.Buffer
	if err := (PNGExporter{}).Export(&buf, box, panels, manifest.Material{Name: "M", Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	title := long + " — isometric preview"
	need := font.MeasureString(labelFace, pngText(title)).Ceil()
	if got := img.Bounds().Dx(); got < need {
		t.Fatalf("image width %d does not cover the measured legend width %d", got, need)
	}
	// The scene alone (in mm at 4 px/mm) is far narrower than the text, so
	// the width must come from the text extent.
	if got := img.Bounds().Dx(); got > need+int(pngMargin*pngScale)*4 {
		t.Fatalf("image width %d unreasonably larger than text width %d", got, need)
	}
}

func TestPNGTextSanitizes(t *testing.T) {
	cases := map[string]string{
		"plain 123":            "plain 123",
		"A — B":                "A - B",
		"Café naïve":           "Cafe naive",
		"“q” it’s…":            "\"q\" it's...",
		"smile \U0001F600 end": "smile ? end",
		"日本":                   "??",
		"tab\there":            "tab?here",
	}
	for in, want := range cases {
		if got := pngText(in); got != want {
			t.Errorf("pngText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPNGNonASCIINamesDecodeAndFit(t *testing.T) {
	name := "Café — naïve \U0001F600 wide name to force text extent beyond the scene"
	box := pack.BoxResult{BoxName: name, InteriorW: 40, InteriorD: 40, InteriorH: 20}
	panels := []geometry.Panel{{
		ID: "Café-1", Axis: geometry.AxisWidthRun,
		Length: 40, Height: 20, Thickness: 3,
		Outline: geometry.BuildOutline(40, 20, nil),
	}}
	var buf bytes.Buffer
	if err := (PNGExporter{}).Export(&buf, box, panels, manifest.Material{Name: "M", Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	need := font.MeasureString(labelFace, pngText(name+" — isometric preview")).Ceil()
	if got := img.Bounds().Dx(); got < need {
		t.Fatalf("image width %d does not cover sanitized legend width %d", got, need)
	}
}

// TestHeightReducedNotchedPanelKeepsFullHeightNotchDepth guards the isometric
// decomposition of a panel whose top was trimmed by Cut: notches must stay at
// the full-height depth (50) so the boxes match the panel's true outline.
func TestHeightReducedNotchedPanelKeepsFullHeightNotchDepth(t *testing.T) {
	notches := []geometry.Notch{{Pos: 40, Width: 3, Edge: geometry.EdgeBottom}}
	p := geometry.Panel{
		ID: "v-1", Axis: geometry.AxisDepthRun,
		Length: 100, Height: 80, Cut: 20, Thickness: 3,
		Notches: notches,
		Outline: geometry.BuildOutlineCut(100, 100, 20, notches),
	}

	var area float64
	for _, r := range geometry.DecomposeFaceCut(p.Length, p.Height, p.Cut, p.Notches) {
		area += r.W * r.H
	}
	poly := p.OutlinePolygon()
	var shoelace float64
	for i, a := range poly {
		b := poly[(i+1)%len(poly)]
		shoelace += a.X*b.Y - b.X*a.Y
	}
	if want := shoelace / 2; area != want {
		t.Fatalf("decomposed area = %g, want outline area %g", area, want)
	}

	_, max := BoxBounds([]geometry.Panel{p})
	if max[2] != 80 {
		t.Fatalf("max Z = %g, want reduced height 80", max[2])
	}
	for _, r := range geometry.DecomposeFaceCut(p.Length, p.Height, p.Cut, p.Notches) {
		if r.Y == 0 && r.H != 50 {
			t.Fatalf("tooth height = %g, want full-height notch depth 50", r.H)
		}
	}

	_, panels, mat := fixture()
	s := BuildScene(pack.BoxResult{InteriorW: 300, InteriorD: 200, InteriorH: 100}, append(panels, p), mat, false)
	if len(s.Polys) == 0 {
		t.Fatal("expected polygons for scene with a height-reduced panel")
	}
}

// TestHeightReducedPanelTopAndBottomNotches covers both notch edges on a
// panel trimmed by Cut (full height 60, cut 20, reduced height 40, notch depth
// 30): the decomposed boxes must tile the true outline, stay below the reduced
// top, and the scene must build and render in assembled and exploded views.
func TestHeightReducedPanelTopAndBottomNotches(t *testing.T) {
	for _, tc := range []struct {
		edge      geometry.Edge
		notchArea float64
		// toothH is the height of a column under a notch (y from 0).
		toothH float64
	}{
		{geometry.EdgeTop, 3 * 10, 30},
		{geometry.EdgeBottom, 3 * 30, 40},
	} {
		for _, axis := range []geometry.Axis{geometry.AxisWidthRun, geometry.AxisDepthRun} {
			t.Run(string(tc.edge)+"/"+string(axis), func(t *testing.T) {
				notches := []geometry.Notch{{Pos: 20, Width: 3, Edge: tc.edge}, {Pos: 70, Width: 3, Edge: tc.edge}}
				p := geometry.Panel{
					ID: "r-1", Axis: axis, Length: 100, Height: 40, Cut: 20, Thickness: 3,
					Notches: notches, Position: geometry.Placement3D{OriginX: 10, OriginY: 20},
				}
				rects := geometry.DecomposeFaceCut(p.Length, p.Height, p.Cut, p.Notches)
				var area float64
				for _, r := range rects {
					area += r.W * r.H
					if r.Y+r.H > 40 {
						t.Errorf("rect %+v rises above the reduced top (40)", r)
					}
					inNotch := (r.X >= 20 && r.X+r.W <= 23) || (r.X >= 70 && r.X+r.W <= 73)
					if inNotch && tc.edge == geometry.EdgeTop && (r.Y != 0 || r.H != tc.toothH) {
						t.Errorf("notch column rect %+v, want y=0 h=%g", r, tc.toothH)
					}
					if inNotch && tc.edge == geometry.EdgeBottom && (r.Y != 30 || r.H != 10) {
						t.Errorf("notch column rect %+v, want y=30 h=10", r)
					}
				}
				if want := 100*40 - 2*tc.notchArea; area != want {
					t.Errorf("decomposed area = %g, want %g", area, want)
				}
				if got := math.Abs(shoelace(p.OutlinePolygon())); got != area {
					t.Errorf("outline polygon area = %g, want decomposed area %g", got, area)
				}

				_, max := BoxBounds([]geometry.Panel{p})
				if max[2] != 40 {
					t.Errorf("max Z = %g, want reduced height 40", max[2])
				}

				box, panels, mat := fixture()
				panels = append(panels, p)
				for _, exploded := range []bool{false, true} {
					if s := BuildScene(box, panels, mat, exploded); len(s.Polys) == 0 {
						t.Fatalf("exploded=%v: expected polygons", exploded)
					}
					var buf bytes.Buffer
					if err := (SVGExporter{Exploded: exploded}).Export(&buf, box, panels, mat); err != nil {
						t.Fatalf("exploded=%v: %v", exploded, err)
					}
					dec := xml.NewDecoder(&buf)
					for {
						if _, err := dec.Token(); err != nil {
							if errors.Is(err, io.EOF) {
								break
							}
							t.Fatalf("exploded=%v: not well-formed XML: %v", exploded, err)
						}
					}
				}
			})
		}
	}
}

func shoelace(pts []geometry.Point2D) float64 {
	var s float64
	for i, a := range pts {
		b := pts[(i+1)%len(pts)]
		s += a.X*b.Y - b.X*a.Y
	}
	return s / 2
}

func TestSVGExportersStripInvalidXMLCharacters(t *testing.T) {
	bad := "a\x00b\x01c\x1bd\x0be\x7ff\xffg"
	box, panels, mat := fixture()
	box.BoxName += bad
	box.Compartments[0].Name += bad
	panels[0].ID += bad
	panels[1].ID += bad
	for _, exp := range []SVGExporter{{Exploded: false}, {Exploded: true}} {
		var buf bytes.Buffer
		if err := exp.Export(&buf, box, panels, mat); err != nil {
			t.Fatalf("%s: %v", exp.Format(), err)
		}
		dec := xml.NewDecoder(bytes.NewReader(buf.Bytes()))
		for {
			_, err := dec.Token()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatalf("%s: output is not well-formed XML: %v", exp.Format(), err)
			}
		}
	}
}
