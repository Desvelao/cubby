package isometric

import (
	"fmt"
	"math"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// rgb is a flat sRGB color used by both the SVG and PNG renderers.
type rgb struct{ R, G, B uint8 }

func (c rgb) scaled(factor float64) rgb {
	return rgb{clampByte(float64(c.R) * factor), clampByte(float64(c.G) * factor), clampByte(float64(c.B) * factor)}
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func (c rgb) hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

var (
	widthRunColor   = rgb{214, 176, 122} // warm tan, width-run panels
	depthRunColor   = rgb{176, 198, 214} // cool blue-gray, depth-run panels
	floorColor      = rgb{190, 190, 168} // neutral khaki, floor plates
	strokeColor     = rgb{45, 45, 45}
	compartmentLine = rgb{110, 110, 110}
	dimensionColor  = rgb{30, 90, 200}
	labelColor      = rgb{20, 20, 20}
	legendColor     = rgb{20, 20, 20}
	caseColor       = rgb{150, 80, 80}
)

// Poly is a filled, stroked, depth-sorted face. Points are already 2D
// projected drawing-space coordinates.
type Poly struct {
	Points []vec2
	Fill   rgb
	Stroke rgb
}

// Line is a stroked segment: a dimension witness line, tick mark, or
// compartment boundary edge.
type Line struct {
	A, B   vec2
	Stroke rgb
	Dashed bool
}

// Label is a text annotation. Pos is its left-baseline anchor in
// drawing-space coordinates; Size is an approximate cap-height, in the same
// units as the projected geometry (millimeters).
type Label struct {
	Pos   vec2
	Text  string
	Size  float64
	Color rgb
}

// Scene is a depth-sorted, format-agnostic isometric drawing: fill it once
// from a packed box and render it as SVG or PNG.
type Scene struct {
	Polys  []Poly // back-to-front draw order
	Lines  []Line
	Labels []Label

	MinX, MinY, MaxX, MaxY float64

	// CoreMaxX is MaxX without the legend's estimated text width, for
	// renderers that measure text exactly (PNG) instead of estimating.
	CoreMaxX float64
}

func newScene() *Scene {
	return &Scene{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
}

func (s *Scene) extend(p vec2) {
	if p.X < s.MinX {
		s.MinX = p.X
	}
	if p.X > s.MaxX {
		s.MaxX = p.X
	}
	if p.Y < s.MinY {
		s.MinY = p.Y
	}
	if p.Y > s.MaxY {
		s.MaxY = p.Y
	}
}

// BuildScene projects a packed box's panels into an annotated isometric
// scene. When exploded is true, panels are pushed outward along their own
// thin axis so the egg-crate grid separates into individually legible
// sheets.
func BuildScene(box pack.BoxResult, panels []geometry.Panel, mat manifest.Material, exploded bool) *Scene {
	return BuildSceneCase(box, panels, mat, exploded, false)
}

// BuildSceneCase is BuildScene with an optional box case: when boxCase is
// true the box interior is outlined as a wireframe (it stays in the assembled
// position even when the panels are exploded).
func BuildSceneCase(box pack.BoxResult, panels []geometry.Panel, mat manifest.Material, exploded, boxCase bool) *Scene {
	s := newScene()

	// Every decomposed rectangle is an axis-aligned box; faces are grouped
	// per box so the painter's algorithm can order whole boxes rather than
	// individual faces (see orderBoxes).
	var boxes []sceneBox
	var placedPanels []geometry.Panel

	gap := 0.08 * math.Max(box.InteriorW, math.Max(box.InteriorD, box.InteriorH))
	if gap < 15 {
		gap = 15
	}

	for pi, p := range panels {
		ep := p
		if exploded {
			ep.Position = explodedPosition(p, box)
		}
		placedPanels = append(placedPanels, ep)

		base := widthRunColor
		switch p.Axis {
		case geometry.AxisDepthRun:
			base = depthRunColor
		case geometry.AxisFloor:
			base = floorColor
		}

		for _, r := range geometry.DecomposeFaceCut(p.Length, p.Height, p.Cut, p.Notches) {
			ox, oy, oz, w, d, h := ep.RectBox(r)
			sb := sceneBox{
				panel: pi,
				min:   vec3{ox, oy, oz},
				max:   vec3{ox + w, oy + d, oz + h},
			}
			for _, f := range boxFaces(ox, oy, oz, w, d, h) {
				if !facesCamera(f.normal) {
					continue
				}
				pts := make([]vec2, 4)
				for i, c := range f.corners {
					pts[i] = project(c)
					s.extend(pts[i])
				}
				sb.polys = append(sb.polys, Poly{Points: pts, Fill: base.scaled(shade(f.normal)), Stroke: strokeColor})
			}
			boxes = append(boxes, sb)
		}
	}

	var owner []int // owner[i] is the panel index of s.Polys[i]
	for _, i := range orderBoxes(boxes) {
		for _, poly := range boxes[i].polys {
			s.Polys = append(s.Polys, poly)
			owner = append(owner, boxes[i].panel)
		}
	}

	// Labels are added after (and independently of) face ordering, and only
	// for panels that are not provably hidden behind other faces.
	hidden := hiddenPanels(s.Polys, owner, len(panels))
	for pi, p := range panels {
		if !hidden[pi] {
			addPanelLabel(s, placedPanels[pi], p)
		}
	}

	if boxCase {
		addBoxCase(s, box)
	}
	addCompartments(s, box)
	addBoxDimensions(s, box, gap)
	addLegend(s, box, mat, panels, exploded)

	return s
}

// sceneBox is one axis-aligned box (a decomposed panel rectangle) with its
// camera-facing faces.
type sceneBox struct {
	panel    int
	min, max vec3
	polys    []Poly
}

func (b sceneBox) centerDepth() float64 {
	return depth(vec3{(b.min.X + b.max.X) / 2, (b.min.Y + b.max.Y) / 2, (b.min.Z + b.max.Z) / 2})
}

// behind reports whether box a may be occluded by box b: b lies entirely
// on the camera side of a along some axis (a.max <= b.min), and the boxes
// are not also separated the other way round (in which case a ray toward the
// camera can never pass from one into the other, so no order is required).
func behind(a, b sceneBox) bool {
	const eps = 1e-9
	ab := a.max.X <= b.min.X+eps || a.max.Y <= b.min.Y+eps || a.max.Z <= b.min.Z+eps
	ba := b.max.X <= a.min.X+eps || b.max.Y <= a.min.Y+eps || b.max.Z <= a.min.Z+eps
	return ab && !ba
}

// orderBoxes returns box indices in back-to-front painter's order. For
// axis-aligned boxes under the isometric view, a box must be drawn before
// any box it is separated from along an axis on the camera side; the boxes
// are topologically sorted by that relation, ties broken by center depth
// (then input order). A cycle, which no consistent order can satisfy, falls
// back to the farthest remaining box.
func orderBoxes(boxes []sceneBox) []int {
	n := len(boxes)
	after := make([][]int, n) // after[i]: boxes that must be drawn after i
	indeg := make([]int, n)
	key := make([]float64, n)
	for i := range boxes {
		key[i] = boxes[i].centerDepth()
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			switch {
			case behind(boxes[i], boxes[j]):
				after[i] = append(after[i], j)
				indeg[j]++
			case behind(boxes[j], boxes[i]):
				after[j] = append(after[j], i)
				indeg[i]++
			}
		}
	}

	done := make([]bool, n)
	order := make([]int, 0, n)
	for len(order) < n {
		best := -1
		for i := 0; i < n; i++ {
			if done[i] || indeg[i] != 0 {
				continue
			}
			if best < 0 || key[i] < key[best] {
				best = i
			}
		}
		if best < 0 { // cycle
			for i := 0; i < n; i++ {
				if !done[i] && (best < 0 || key[i] < key[best]) {
					best = i
				}
			}
		}
		done[best] = true
		order = append(order, best)
		for _, j := range after[best] {
			indeg[j]--
		}
	}
	return order
}

// hiddenPanels reports, per panel, whether every one of its faces is fully
// covered by a single face drawn later. It is deliberately conservative:
// partial or multi-face occlusion counts as visible, and a panel without
// faces is never reported hidden.
func hiddenPanels(polys []Poly, owner []int, n int) []bool {
	hidden := make([]bool, n)
	seen := make([]bool, n)
	for i := range hidden {
		hidden[i] = true
	}
	for i, p := range polys {
		o := owner[i]
		seen[o] = true
		if !hidden[o] {
			continue
		}
		covered := false
		for j := i + 1; j < len(polys) && !covered; j++ {
			covered = polyContains(polys[j].Points, p.Points)
		}
		if !covered {
			hidden[o] = false
		}
	}
	for i := range hidden {
		if !seen[i] {
			hidden[i] = false
		}
	}
	return hidden
}

// polyContains reports whether every vertex of inner lies inside (or on the
// boundary of) the convex polygon outer.
func polyContains(outer, inner []vec2) bool {
	const eps = 1e-6
	sign := 0.0
	for i := range outer {
		if cross2(outer[i], outer[(i+1)%len(outer)], outer[(i+2)%len(outer)]) != 0 {
			sign = math.Copysign(1, cross2(outer[i], outer[(i+1)%len(outer)], outer[(i+2)%len(outer)]))
			break
		}
	}
	if sign == 0 {
		return false // degenerate outer polygon covers nothing
	}
	for _, q := range inner {
		for i := range outer {
			a, b := outer[i], outer[(i+1)%len(outer)]
			if sign*cross2(a, b, q) < -eps {
				return false
			}
		}
	}
	return true
}

// cross2 is the z component of (b-a) x (c-a).
func cross2(a, b, c vec2) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// explodedPosition pushes a panel's origin away from the box's center along
// its own thin axis (Y for width-run panels, X for depth-run panels),
// separating the packed grid into individually legible sheets.
func explodedPosition(p geometry.Panel, box pack.BoxResult) geometry.Placement3D {
	const multiplier = 1.6
	pos := p.Position
	if p.Axis == geometry.AxisFloor {
		return pos
	}
	if p.Axis == geometry.AxisWidthRun {
		center := box.InteriorD / 2
		pos.OriginY = center + (pos.OriginY-center)*multiplier
	} else {
		center := box.InteriorW / 2
		pos.OriginX = center + (pos.OriginX-center)*multiplier
	}
	return pos
}

// BoxBounds returns the assembled-position world-space bounding box
// (min and max corners) of every decomposed rectangle of panels, exactly as
// BuildScene places them.
func BoxBounds(panels []geometry.Panel) (min, max [3]float64) {
	for i := range min {
		min[i], max[i] = math.Inf(1), math.Inf(-1)
	}
	for _, p := range panels {
		for _, r := range geometry.DecomposeFaceCut(p.Length, p.Height, p.Cut, p.Notches) {
			ox, oy, oz, w, d, h := p.RectBox(r)
			lo, hi := [3]float64{ox, oy, oz}, [3]float64{ox + w, oy + d, oz + h}
			for i := range lo {
				min[i], max[i] = math.Min(min[i], lo[i]), math.Max(max[i], hi[i])
			}
		}
	}
	return min, max
}

type boxFace struct {
	normal  vec3
	corners [4]vec3
}

// boxFaces returns the 6 outward-facing quads of an axis-aligned box,
// each listed counter-clockwise when viewed from outside the box, so the
// winding agrees with the outward normal (right-hand rule).
func boxFaces(ox, oy, oz, w, d, h float64) []boxFace {
	c := [8]vec3{
		{ox, oy, oz}, {ox + w, oy, oz}, {ox + w, oy + d, oz}, {ox, oy + d, oz},
		{ox, oy, oz + h}, {ox + w, oy, oz + h}, {ox + w, oy + d, oz + h}, {ox, oy + d, oz + h},
	}
	return []boxFace{
		{vec3{0, 0, -1}, [4]vec3{c[0], c[3], c[2], c[1]}}, // bottom
		{vec3{0, 0, 1}, [4]vec3{c[4], c[5], c[6], c[7]}},  // top
		{vec3{0, -1, 0}, [4]vec3{c[0], c[1], c[5], c[4]}}, // front (-Y)
		{vec3{0, 1, 0}, [4]vec3{c[3], c[7], c[6], c[2]}},  // back (+Y)
		{vec3{-1, 0, 0}, [4]vec3{c[0], c[4], c[7], c[3]}}, // left (-X)
		{vec3{1, 0, 0}, [4]vec3{c[1], c[2], c[6], c[5]}},  // right (+X)
	}
}

func axisAbbrev(a geometry.Axis) string {
	if a == geometry.AxisDepthRun {
		return "D"
	}
	if a == geometry.AxisFloor {
		return "F"
	}
	return "W"
}

func addPanelLabel(s *Scene, placed, orig geometry.Panel) {
	x, y, z := placed.To3D(orig.Length/2, orig.Height/2, orig.Thickness)
	anchor := project(vec3{x, y, z})
	s.extend(anchor)

	line1 := orig.ID
	line2 := fmt.Sprintf("%gx%gx%g mm  %s  %d notch(es)",
		round1(orig.Length), round1(orig.Height), round1(orig.Thickness), axisAbbrev(orig.Axis), len(orig.Notches))

	s.Labels = append(s.Labels,
		Label{Pos: vec2{anchor.X, anchor.Y - 1}, Text: line1, Size: 3.4, Color: labelColor},
		Label{Pos: vec2{anchor.X, anchor.Y + 3}, Text: line2, Size: 2.6, Color: labelColor},
	)
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func addCompartments(s *Scene, box pack.BoxResult) {
	for _, c := range box.Compartments {
		corners := [4]vec3{
			{c.Bounds.X, c.Bounds.Y, 0},
			{c.Bounds.X + c.Bounds.W, c.Bounds.Y, 0},
			{c.Bounds.X + c.Bounds.W, c.Bounds.Y + c.Bounds.D, 0},
			{c.Bounds.X, c.Bounds.Y + c.Bounds.D, 0},
		}
		var pts [4]vec2
		for i, corner := range corners {
			pts[i] = project(corner)
			s.extend(pts[i])
		}
		for i := 0; i < 4; i++ {
			s.Lines = append(s.Lines, Line{A: pts[i], B: pts[(i+1)%4], Stroke: compartmentLine, Dashed: true})
		}

		name := c.Name
		if name == "" {
			name = c.ID
		}
		center := project(vec3{c.Bounds.X + c.Bounds.W/2, c.Bounds.Y + c.Bounds.D/2, 0})
		s.Labels = append(s.Labels, Label{Pos: center, Text: fmt.Sprintf("%s (%s)", name, c.Kind), Size: 2.4, Color: compartmentLine})
	}
}

// addBoxDimensions draws witness lines with end ticks and text for the
// box's overall width, depth, and height.
func addBoxDimensions(s *Scene, box pack.BoxResult, gap float64) {
	tick := gap * 0.35

	addDimension(s, vec3{0, -gap, 0}, vec3{box.InteriorW, -gap, 0}, vec3{0, 0, tick},
		fmt.Sprintf("W %g mm", round1(box.InteriorW)))
	addDimension(s, vec3{-gap, 0, 0}, vec3{-gap, box.InteriorD, 0}, vec3{0, 0, tick},
		fmt.Sprintf("D %g mm", round1(box.InteriorD)))
	addDimension(s, vec3{-gap, -gap, 0}, vec3{-gap, -gap, box.InteriorH}, vec3{tick, 0, 0},
		fmt.Sprintf("H %g mm", round1(box.InteriorH)))
}

func addDimension(s *Scene, a, b, tickDir vec3, text string) {
	pa, pb := project(a), project(b)
	s.extend(pa)
	s.extend(pb)
	s.Lines = append(s.Lines, Line{A: pa, B: pb, Stroke: dimensionColor})

	for _, end := range [2]vec3{a, b} {
		t1 := project(vec3{end.X - tickDir.X/2, end.Y - tickDir.Y/2, end.Z - tickDir.Z/2})
		t2 := project(vec3{end.X + tickDir.X/2, end.Y + tickDir.Y/2, end.Z + tickDir.Z/2})
		s.extend(t1)
		s.extend(t2)
		s.Lines = append(s.Lines, Line{A: t1, B: t2, Stroke: dimensionColor})
	}

	mid := vec2{(pa.X + pb.X) / 2, (pa.Y+pb.Y)/2 - 2}
	s.extend(mid)
	s.Labels = append(s.Labels, Label{Pos: mid, Text: text, Size: 3, Color: dimensionColor})
}

// addLegend writes a fixed title block above the projected geometry:
// box/material summary and panel/missing counts.
func addLegend(s *Scene, box pack.BoxResult, mat manifest.Material, panels []geometry.Panel, exploded bool) {
	title := box.BoxName
	if title == "" {
		title = "insert"
	}
	if exploded {
		title += " — isometric preview (exploded)"
	} else {
		title += " — isometric preview"
	}

	missing := 0
	for _, m := range box.TotalMissing {
		missing += m.Rejected
	}

	lines := []string{
		title,
		fmt.Sprintf("Material: %s, %g mm thick, %g mm kerf", mat.Name, round1(mat.Thickness), round1(mat.Kerf)),
		fmt.Sprintf("%d panel(s)", len(panels)),
	}
	if missing > 0 {
		lines[2] += fmt.Sprintf(", %d component instance(s) did not fit", missing)
	}

	const lineHeight = 6.0
	x := s.MinX
	y0 := s.MinY - lineHeight*float64(len(lines)) - 4
	y := y0
	for _, line := range lines {
		s.Labels = append(s.Labels, Label{Pos: vec2{x, y}, Text: line, Size: 4, Color: legendColor})
		s.extend(vec2{x, y - 3})
		y += lineHeight
	}
	s.CoreMaxX = s.MaxX
	// Rough per-character width, used by the SVG/PDF renderers, which have no
	// font metrics at hand.
	for i, line := range lines {
		s.extend(vec2{x + float64(len(line))*2.6, y0 + float64(i)*lineHeight})
	}
}

// addBoxCase outlines the box interior as a wireframe cuboid.
func addBoxCase(s *Scene, box pack.BoxResult) {
	w, d, h := box.InteriorW, box.InteriorD, box.InteriorH
	var c [8]vec2
	for i := range c {
		c[i] = project(vec3{float64(i&1) * w, float64(i>>1&1) * d, float64(i>>2&1) * h})
		s.extend(c[i])
	}
	for i := 0; i < 8; i++ {
		for bit := 1; bit < 8; bit <<= 1 {
			if j := i | bit; j != i {
				s.Lines = append(s.Lines, Line{A: c[i], B: c[j], Stroke: caseColor})
			}
		}
	}
	top := project(vec3{w, d, h})
	s.Labels = append(s.Labels, Label{Pos: vec2{top.X + 2, top.Y}, Text: "box case", Size: 3, Color: caseColor})
}
