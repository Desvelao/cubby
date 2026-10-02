// Package geometry turns a packed box layout into slotted divider-panel
// geometry: flat 2D outlines with interlocking notches, extruded to
// material thickness for 3D export.
package geometry

// Axis identifies which direction a panel runs, which determines which edge
// its notches are cut from.
type Axis string

const (
	// AxisWidthRun panels run along the box's X axis and are notched from
	// their top edge.
	AxisWidthRun Axis = "width-run"
	// AxisDepthRun panels run along the box's Y axis and are notched from
	// their bottom edge.
	AxisDepthRun Axis = "depth-run"
	// AxisFloor panels lie flat: Length runs along X, "Height" (the local
	// face's second dimension) along Y, extruded upward along Z.
	AxisFloor Axis = "floor"
)

// Point2D is a vertex in a panel's flat outline.
type Point2D struct{ X, Y float64 }

// Notch is a half-lap slot cut into one edge of a panel so it can interlock
// with a crossing panel.
type Notch struct {
	Pos, Width float64
	Edge       Edge
}

// Placement3D positions an extruded panel's local (length, height) face
// within the assembled 3D insert's box-interior coordinate space (X = box
// width, Y = box depth, Z = box height). A width-run panel's Length runs
// along X, extruded along Y at OriginY; a depth-run panel's Length runs
// along Y, extruded along X at OriginX. Both stand upright along Z.
type Placement3D struct {
	OriginX, OriginY, OriginZ float64
}

// Panel is one flat divider, ready to be cut and extruded.
type Panel struct {
	ID                        string
	Axis                      Axis
	Length, Height, Thickness float64
	// Cut is how much the panel's top edge is lowered below its full height
	// (Height is already reduced); notches keep the full-height depth.
	Cut      float64
	Notches  []Notch
	Outline  []Point2D
	Position Placement3D
}

// To3D maps a point on this panel's local (length, height) face, offset by z
// along the panel's thickness, into the assembled insert's 3D box-interior
// coordinate space (X = box width, Y = box depth, Z = box height), per the
// placement convention documented on Placement3D.
func (p Panel) To3D(localX, localY, z float64) (x, y, z3d float64) {
	if p.Axis == AxisFloor {
		return p.Position.OriginX + localX, p.Position.OriginY + localY, p.Position.OriginZ + z
	}
	if p.Axis == AxisDepthRun {
		return p.Position.OriginX + z, p.Position.OriginY + localX, p.Position.OriginZ + localY
	}
	return p.Position.OriginX + localX, p.Position.OriginY + z, p.Position.OriginZ + localY
}

// OutlinePoints returns the panel's raw counter-clockwise outline in local
// (length, height) coordinates: Outline when populated, otherwise one built
// from Length, Height, Cut and Notches with notches at the full-height depth
// (Height + Cut), as BuildOutlineCut builds it. Unlike OutlinePolygon the
// vertices are not simplified. Every exporter reads the outline through this
// accessor or OutlinePolygon.
func (p Panel) OutlinePoints() []Point2D {
	if len(p.Outline) >= 3 {
		return p.Outline
	}
	return BuildOutlineCut(p.Length, p.Height+p.Cut, p.Cut, p.Notches)
}

// ValidateOutline reports whether the panel's outline can be produced without
// degenerating. Notch edges are always checked; when Outline is not populated
// (OutlinePoints would rebuild it) notch bounds, overlaps and Cut are checked
// too, so exporters do not silently emit a degenerate fallback outline.
func (p Panel) ValidateOutline() error {
	if len(p.Outline) >= 3 {
		return ValidateNotchEdges(p.Notches)
	}
	full := p.Height + p.Cut
	if err := ValidateNotches(p.Length, full, p.Notches); err != nil {
		return err
	}
	return ValidateCut(full, p.Cut, len(p.Notches) > 0)
}

// OutlinePolygon returns the panel's counter-clockwise rectilinear outline in
// local (length, height) coordinates, ready to be extruded into a prism:
// OutlinePoints with vertices that add nothing to the shape are removed: duplicates,
// collinear vertices, and the zero-width spikes BuildOutline produces when a
// notch is flush with a panel end. The result has only true corners.
func (p Panel) OutlinePolygon() []Point2D {
	out := append([]Point2D(nil), p.OutlinePoints()...)
	for changed := true; changed && len(out) > 3; {
		changed = false
		for i := 0; i < len(out) && len(out) > 3; i++ {
			a, b, c := out[(i+len(out)-1)%len(out)], out[i], out[(i+1)%len(out)]
			if (b.X-a.X)*(c.Y-a.Y)-(b.Y-a.Y)*(c.X-a.X) == 0 {
				out = append(out[:i], out[i+1:]...)
				changed = true
				i--
			}
		}
	}
	return out
}

// RectBox maps one decomposed rectangle r of this panel's local (length,
// height) face, extruded through the panel's thickness, into an axis-aligned
// box in the assembled insert's 3D coordinate space: the box's minimum
// corner (ox, oy, oz) and its non-negative extent along X, Y and Z (w, d, h).
// It is the single source of truth for world-space placement of a
// decomposed rectangle and follows the same convention as To3D.
func (p Panel) RectBox(r PanelRect) (ox, oy, oz, w, d, h float64) {
	ox, oy, oz = p.To3D(r.X, r.Y, 0)
	switch p.Axis {
	case AxisFloor:
		return ox, oy, oz, r.W, r.H, p.Thickness
	case AxisDepthRun:
		return ox, oy, oz, p.Thickness, r.W, r.H
	default:
		return ox, oy, oz, r.W, p.Thickness, r.H
	}
}
