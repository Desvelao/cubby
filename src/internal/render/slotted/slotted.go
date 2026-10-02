// Package slotted implements cubby's v1 render.Backend: an egg-crate
// style grid of interlocking divider panels, one per compartment boundary,
// plus (per compartment, when enabled) plain internal dividers between that
// compartment's own shelf-packed rows.
package slotted

import (
	"fmt"
	"math"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
	"github.com/Desvelao/cubby/internal/render"
)

type Backend struct{}

func (Backend) Name() string { return "slotted" }

// Render turns a packed box into panels.
//
// Floors in the shared grid: if ANY tray has Floor, zBase becomes the
// material thickness for the whole grid. Every grid panel (h-*, v-*, boundary
// walls and internal dividers), including those bordering only floorless
// trays, is lifted to OriginZ = thickness and shortened to height - thickness.
// Only floored trays get an fl-* plate. Percent reductions use that lowered
// height as their base (height - zBase). Removable trays differ: each tray
// uses its own base (its WallT when it has a floor, else 0), so the same
// percentage gives different absolute cuts in the two modes. Absolute (mm)
// reductions are unaffected by the base.
func (Backend) Render(box pack.BoxResult, mat manifest.Material, _ render.RenderOptions) ([]geometry.Panel, error) {
	height := box.InteriorH

	removable := 0
	for _, cr := range box.Compartments {
		if cr.Removable {
			removable++
		}
	}
	if removable > 0 && removable != len(box.Compartments) {
		return nil, fmt.Errorf("cannot mix removable and non-removable trays in one box: set removable on every tray (or none)")
	}

	if removable > 0 {
		var panels []geometry.Panel
		for _, cr := range box.Compartments {
			tray, err := removableTray(cr, height, mat)
			if err != nil {
				return nil, err
			}
			panels = append(panels, tray...)
		}
		return panels, nil
	}

	// Shared grid: if any tray has a floor, every grid panel is raised by
	// the floor thickness (and shortened) so the floor plates sit under it.
	zBase := 0.0
	for _, cr := range box.Compartments {
		if cr.Floor {
			zBase = mat.Thickness
		}
	}
	if err := checkHeightAboveFloor(height, zBase); err != nil {
		return nil, err
	}
	panels, err := renderGrid(box, mat, height-zBase)
	if err != nil {
		return nil, err
	}
	panels = lift(panels, zBase)
	for _, cr := range box.Compartments {
		if cr.Floor {
			panels = append(panels, floorPanel(cr, mat.Thickness))
		}
	}
	return panels, nil
}

// checkHeightAboveFloor rejects a panel base height that the floor would
// consume entirely. It is reported as a ReductionError because the height
// (the box interior height) and floor are the manifest-side inputs
// at fault, the same class as a reduction too large for the height.
func checkHeightAboveFloor(height, floor float64) error {
	if floor > 0 && height <= floor {
		return &pack.ReductionError{Err: fmt.Errorf("panel height %g must exceed floor thickness %g", height, floor)}
	}
	return nil
}

// cutFor returns how much the highest-reducing of reds trims panel id, and
// rejects reductions that would leave no panel or eat into the half-lap
// notches (which keep the full-height depth so joints still mate).
func cutFor(id string, height float64, notched bool, reds ...pack.HeightReduction) (float64, error) {
	cut := 0.0
	for _, r := range reds {
		if c := r.For(id, height); c > cut {
			cut = c
		}
	}
	if cut <= 0 {
		return 0, nil
	}
	if err := geometry.ValidateCut(height, cut, notched); err != nil {
		return 0, &pack.ReductionError{Err: fmt.Errorf("panel %s: %w", id, err)}
	}
	return cut, nil
}

// lift raises panels' origins by dz.
func lift(panels []geometry.Panel, dz float64) []geometry.Panel {
	for i := range panels {
		panels[i].Position.OriginZ += dz
	}
	return panels
}

func floorPanel(cr pack.CompartmentResult, thickness float64) geometry.Panel {
	return geometry.Panel{
		ID:        "fl-" + cr.ID,
		Axis:      geometry.AxisFloor,
		Length:    cr.Bounds.W,
		Height:    cr.Bounds.D,
		Thickness: thickness,
		Outline:   geometry.BuildOutline(cr.Bounds.W, cr.Bounds.D, nil),
		Position:  geometry.Placement3D{OriginX: cr.Bounds.X, OriginY: cr.Bounds.Y},
	}
}

// removableTray builds one self-contained open box: four butt-jointed walls
// inside the tray's footprint, its internal dividers, and (optionally) a
// floor plate the walls stand on.
func removableTray(cr pack.CompartmentResult, height float64, mat manifest.Material) ([]geometry.Panel, error) {
	t := cr.WallT
	var out []geometry.Panel
	base := 0.0
	if cr.Floor {
		base = t
		out = append(out, floorPanel(cr, t))
	}
	if err := checkHeightAboveFloor(height, base); err != nil {
		return nil, fmt.Errorf("tray %s: %w", cr.ID, err)
	}
	h := height - base
	var cutErr error
	wall := func(id string, axis geometry.Axis, length, x, y float64) geometry.Panel {
		cut, err := cutFor(id, h, false, cr.ExternalReduction)
		if err != nil && cutErr == nil {
			cutErr = err
		}
		return geometry.Panel{
			ID: id, Axis: axis, Length: length, Height: h - cut, Thickness: t,
			Outline:  geometry.BuildOutlineCut(length, h, cut, nil),
			Position: geometry.Placement3D{OriginX: x, OriginY: y},
		}
	}
	b := cr.Bounds
	if b.W <= 2*t || b.D <= 2*t {
		return nil, fmt.Errorf("removable tray %s is too small: %g x %g mm must exceed two wall thicknesses (%g mm) in both directions", cr.ID, b.W, b.D, 2*t)
	}
	walls := []geometry.Panel{
		wall("rw-"+cr.ID+"-front", geometry.AxisWidthRun, b.W, b.X, b.Y),
		wall("rw-"+cr.ID+"-back", geometry.AxisWidthRun, b.W, b.X, b.Y+b.D-t),
		wall("rv-"+cr.ID+"-left", geometry.AxisDepthRun, b.D-2*t, b.X, b.Y+t),
		wall("rv-"+cr.ID+"-right", geometry.AxisDepthRun, b.D-2*t, b.X+b.W-t, b.Y+t),
	}
	if cutErr != nil {
		return nil, cutErr
	}
	dividers, err := internalDividers(cr, h, mat)
	if err != nil {
		return nil, err
	}
	walls = append(walls, dividers...)
	return append(out, lift(walls, base)...), nil
}

func renderGrid(box pack.BoxResult, mat manifest.Material, height float64) ([]geometry.Panel, error) {
	slotWidth := geometry.SlotWidth(mat)

	region := geometry.BuildRegion(box)
	var panels []geometry.Panel
	// rowBoundaryCrossings[i] holds the crossings of the horizontal panel
	// between rows i and i+1, so the vertical panels can tell which of them
	// actually got a slot cut for them.
	var rowBoundaryCrossings [][]geometry.BoundaryCrossing

	// Horizontal (width-run) panels: one per interior row boundary. These
	// are unaffected by FullWalls, since they only ever sit between two
	// rows — never at the box's own top/bottom edge.
	for i := 0; i+1 < len(region.Rows); i++ {
		a, b := region.Rows[i], region.Rows[i+1]
		boundaryY := a.Y + a.Height
		crossings := geometry.RowBoundaryPositions(a, b)
		rowBoundaryCrossings = append(rowBoundaryCrossings, crossings)
		notches := notchesAt(crossings, slotWidth, geometry.EdgeTop)
		if err := checkNotches(fmt.Sprintf("h-%d", i+1), region.Bounds.W, notches); err != nil {
			return nil, err
		}

		var reds []pack.HeightReduction
		for _, c := range append(append([]geometry.Cell(nil), a.Cells...), b.Cells...) {
			reds = append(reds, c.Compartment.DividerReduction)
		}
		hID := fmt.Sprintf("h-%d", i+1)
		cut, err := cutFor(hID, height, len(notches) > 0, reds...)
		if err != nil {
			return nil, err
		}
		outline := geometry.BuildOutlineCut(region.Bounds.W, height, cut, notches)
		panels = append(panels, geometry.Panel{
			ID:        hID,
			Axis:      geometry.AxisWidthRun,
			Length:    region.Bounds.W,
			Height:    height - cut,
			Cut:       cut,
			Thickness: mat.Thickness,
			Notches:   notches,
			Outline:   outline,
			Position:  geometry.Placement3D{OriginX: 0, OriginY: boundaryY, OriginZ: 0},
		})
	}

	// Vertical (depth-run) panels: one per cell boundary within each row.
	vIdx := 0
	lastRowIdx := len(region.Rows) - 1
	for rowIdx, row := range region.Rows {
		isFirstRow := rowIdx == 0
		isLastRow := rowIdx == lastRowIdx
		lastCellIdx := len(row.Cells) - 1

		for i := 0; i+1 < len(row.Cells); i++ {
			left, right := row.Cells[i], row.Cells[i+1]
			boundaryX := left.Bounds.X + left.Bounds.W
			eitherFullWalls := left.Compartment.FullWalls || right.Compartment.FullWalls
			// A notch here only helps if both neighboring compartments want
			// one; if either is JointTypePlain, this joint is left uncut.
			jointOK := left.Compartment.JointType != manifest.JointTypePlain && right.Compartment.JointType != manifest.JointTypePlain

			var notches []geometry.Notch
			length := row.Height
			// Rows are packed contiguously and exclude divider thickness, so
			// the horizontal panel on the boundary at Y occupies
			// [Y, Y+thickness] -- inside the row *below* it -- and is cut
			// with a slot at every boundary X of either row (unless a
			// plain joint suppresses it).
			//
			// A vertical panel's Y-start end overlaps the horizontal panel
			// on its row's top boundary, or, in the first row, its own
			// FullWalls front wall at [0, t]. Its far end only butts against
			// the next row's horizontal panel, so it is notched there just
			// when it is lengthened by one thickness to reach into it
			// (which requires the next row to have no panel of its own at
			// this X, or the two would collide) or, in the last row, when it
			// meets a FullWalls back wall at [D-t, D].
			startNotch := jointOK && eitherFullWalls
			if !isFirstRow {
				startNotch = hasSlot(rowBoundaryCrossings[rowIdx-1], boundaryX)
			}
			if startNotch {
				notches = append(notches, geometry.Notch{Pos: 0, Width: slotWidth, Edge: geometry.EdgeBottom})
			}
			if isLastRow {
				if eitherFullWalls && jointOK {
					notches = append(notches, geometry.Notch{Pos: length - slotWidth, Width: slotWidth, Edge: geometry.EdgeBottom})
				}
			} else if hasSlot(rowBoundaryCrossings[rowIdx], boundaryX) &&
				!hasCrossing(geometry.RowBoundaryPositions(region.Rows[rowIdx+1], geometry.RowNode{}), boundaryX) {
				length += mat.Thickness
				notches = append(notches, geometry.Notch{Pos: length - slotWidth, Width: slotWidth, Edge: geometry.EdgeBottom})
			}

			if err := checkNotches(fmt.Sprintf("v-%d", vIdx+1), length, notches); err != nil {
				return nil, err
			}
			vIdx++
			vID := fmt.Sprintf("v-%d", vIdx)
			cut, err := cutFor(vID, height, len(notches) > 0, left.Compartment.DividerReduction, right.Compartment.DividerReduction)
			if err != nil {
				return nil, err
			}
			outline := geometry.BuildOutlineCut(length, height, cut, notches)
			panels = append(panels, geometry.Panel{
				ID:        vID,
				Axis:      geometry.AxisDepthRun,
				Length:    length,
				Height:    height - cut,
				Cut:       cut,
				Thickness: mat.Thickness,
				Notches:   notches,
				Outline:   outline,
				Position:  geometry.Placement3D{OriginX: boundaryX, OriginY: row.Y, OriginZ: 0},
			})
		}

		for cellIdx, cell := range row.Cells {
			if !cell.Compartment.FullWalls {
				continue
			}
			isFirstCell := cellIdx == 0
			isLastCell := cellIdx == lastCellIdx
			notch := cell.Compartment.JointType != manifest.JointTypePlain
			t := mat.Thickness

			// A boundary wall is only notched where another panel really
			// crosses it within its own extent, and that panel is notched
			// in turn (see the divider logic above):
			//   - at a box corner, the adjoining boundary wall of the same
			//     cell;
			//   - at an interior cell boundary, the divider, which occupies
			//     [boundaryX, boundaryX+t] -- i.e. beyond this cell's right
			//     edge. So a wall's left end meets it directly, but its
			//     right end must be lengthened by t to reach it. If the
			//     right neighbour is FullWalls too, its own left-end notch
			//     already laps the divider, so this wall stops short and
			//     is left un-notched there instead of colliding with it.
			// Joints against a JointTypePlain neighbour are left uncut,
			// matching the divider.
			startNotch, endNotch := notch, notch
			if !isFirstCell {
				startNotch = notch && row.Cells[cellIdx-1].Compartment.JointType != manifest.JointTypePlain
			}
			wallLen := cell.Bounds.W
			if !isLastCell {
				right := row.Cells[cellIdx+1].Compartment
				endNotch = notch && !right.FullWalls && right.JointType != manifest.JointTypePlain
				if endNotch {
					wallLen += t
				}
			}
			if isFirstRow {
				bp, err := boundaryPanel(&vIdx, geometry.AxisWidthRun,
					wallLen, height, cell.Compartment.ExternalReduction, t, slotWidth, startNotch, endNotch,
					cell.Bounds.X, 0)
				if err != nil {
					return nil, err
				}
				panels = append(panels, bp)
			}
			if isLastRow {
				bp, err := boundaryPanel(&vIdx, geometry.AxisWidthRun,
					wallLen, height, cell.Compartment.ExternalReduction, t, slotWidth, startNotch, endNotch,
					cell.Bounds.X, region.Bounds.D-t)
				if err != nil {
					return nil, err
				}
				panels = append(panels, bp)
			}

			// Side walls run along the box's left/right edge. Toward the
			// front they meet the front wall only in the first row; in
			// later rows they start just past the row-boundary horizontal
			// panel (which spans the full width and has no slot at the box
			// edge) so the two don't overlap. Likewise they only meet the
			// back wall in the last row; otherwise they butt against the
			// next row's horizontal panel.
			sideY, sideLen := cell.Bounds.Y, cell.Bounds.D
			if !isFirstRow {
				sideY += t
				sideLen -= t
			}
			for _, side := range []struct {
				on bool
				x  float64
			}{{isFirstCell, 0}, {isLastCell, region.Bounds.W - t}} {
				if !side.on {
					continue
				}
				bp, err := boundaryPanel(&vIdx, geometry.AxisDepthRun,
					sideLen, height, cell.Compartment.ExternalReduction, t, slotWidth, notch && isFirstRow, notch && isLastRow,
					side.x, sideY)
				if err != nil {
					return nil, err
				}
				panels = append(panels, bp)
			}
		}
	}

	// Internal (within-compartment) dividers: column dividers between
	// side-by-side items in a row, plus one row divider per interior row
	// boundary inside a single compartment's own shelf-packed Rows, when
	// that compartment's Dividers option is enabled. These panels sit
	// strictly inside the compartment's content footprint (offset by
	// ContentOffsetX/Y from Bounds, spanning UsedW) -- they never reach the
	// compartment's own boundary walls or the box wall, since that space is
	// occupied by margin -- so there's nothing to interlock with at either
	// end and they always get a plain butt edge, the same treatment panels
	// meeting the box wall already get.
	for _, cr := range box.Compartments {
		divs, err := internalDividers(cr, height, mat)
		if err != nil {
			return nil, err
		}
		panels = append(panels, divs...)
	}

	return panels, nil
}

// internalDividers builds a compartment's own (within-tray) divider panels
// at z = 0; the caller lifts them by its floor thickness.
func internalDividers(cr pack.CompartmentResult, height float64, mat manifest.Material) ([]geometry.Panel, error) {
	if !cr.Dividers {
		return nil, nil
	}
	var out []geometry.Panel
	var cutErr error
	// plain builds an un-notched divider, trimmed by the tray's divider reduction.
	plain := func(id string, axis geometry.Axis, length float64, x, y float64) geometry.Panel {
		cut, err := cutFor(id, height, false, cr.DividerReduction)
		if err != nil && cutErr == nil {
			cutErr = err
		}
		return geometry.Panel{
			ID: id, Axis: axis, Length: length, Height: height - cut, Thickness: mat.Thickness,
			Outline:  geometry.BuildOutlineCut(length, height, cut, nil),
			Position: geometry.Placement3D{OriginX: x, OriginY: y},
		}
	}
	// Expansion dividers: when the tray grew past its packed size, wall
	// the extra width off as its own cell (plain butt edges).
	if cr.Expand && cr.Bounds.W-cr.CoreW > 1e-9 {
		// A removable tray's old right wall position is now interior,
		// so the divider sits just inside it, between the side walls.
		x, y, length := cr.Bounds.X+cr.CoreW, cr.Bounds.Y, cr.Bounds.D
		if cr.Removable {
			x, y, length = x-cr.WallT, y+cr.WallT, length-2*cr.WallT
		}
		out = append(out, plain("ie-v-"+cr.ID, geometry.AxisDepthRun, length, x, y))
	}
	// Column dividers: one between each pair of side-by-side items
	// within a single row, at the midpoint of the gap between them.
	for ri, row := range cr.Rows {
		for i := 0; i+1 < len(row.Items); i++ {
			a, b := row.Items[i].Rect, row.Items[i+1].Rect
			x := cr.Bounds.X + cr.ContentOffsetX + (a.X+a.W+b.X)/2
			out = append(out, plain(fmt.Sprintf("iv-%s-%d-%d", cr.ID, ri+1, i+1), geometry.AxisDepthRun, row.Height, x, cr.Bounds.Y+cr.ContentOffsetY+row.Y))
		}
	}
	for i := 0; i+1 < len(cr.Rows); i++ {
		row := cr.Rows[i]
		boundaryY := cr.Bounds.Y + cr.ContentOffsetY + row.Y + row.Height
		out = append(out, plain(fmt.Sprintf("ih-%s-%d", cr.ID, i+1), geometry.AxisWidthRun, cr.UsedW, cr.Bounds.X+cr.ContentOffsetX, boundaryY))
	}
	return out, cutErr
}

// boundaryPanel builds one FullWalls boundary-wall panel, with a slot notch
// at its start and/or end (see the call sites for where a crossing panel
// really meets it), and allocates it the next panel index via idx.
func boundaryPanel(idx *int, axis geometry.Axis, length, height float64, red pack.HeightReduction, thickness, slotWidth float64, notchStart, notchEnd bool, originX, originY float64) (geometry.Panel, error) {
	var notches []geometry.Notch
	edge := geometry.EdgeTop
	if axis == geometry.AxisDepthRun {
		edge = geometry.EdgeBottom
	}
	if notchStart {
		notches = append(notches, geometry.Notch{Pos: 0, Width: slotWidth, Edge: edge})
	}
	if notchEnd {
		notches = append(notches, geometry.Notch{Pos: length - slotWidth, Width: slotWidth, Edge: edge})
	}
	*idx++
	prefix := "w"
	if axis == geometry.AxisDepthRun {
		prefix = "wv"
	}
	id := fmt.Sprintf("%s-%d", prefix, *idx)
	if err := checkNotches(id, length, notches); err != nil {
		return geometry.Panel{}, err
	}
	cut, err := cutFor(id, height, len(notches) > 0, red)
	if err != nil {
		return geometry.Panel{}, err
	}
	return geometry.Panel{
		ID:        id,
		Axis:      axis,
		Length:    length,
		Height:    height - cut,
		Cut:       cut,
		Thickness: thickness,
		Notches:   notches,
		Outline:   geometry.BuildOutlineCut(length, height, cut, notches),
		Position:  geometry.Placement3D{OriginX: originX, OriginY: originY},
	}, nil
}

// checkNotches rejects notches that fall outside [0, length] or overlap each
// other, which would otherwise yield a self-intersecting outline.
func checkNotches(id string, length float64, notches []geometry.Notch) error {
	// Height is unknown here (cutFor checks it); any positive value passes.
	if err := geometry.ValidateNotches(length, 1, notches); err != nil {
		return fmt.Errorf("panel %s: %w", id, err)
	}
	return nil
}

func notchesAt(crossings []geometry.BoundaryCrossing, width float64, edge geometry.Edge) []geometry.Notch {
	notches := make([]geometry.Notch, 0, len(crossings))
	for _, c := range crossings {
		if !c.Notch {
			continue
		}
		notches = append(notches, geometry.Notch{Pos: c.X, Width: width, Edge: edge})
	}
	return notches
}

// hasCrossing reports whether crossings has an entry at x.
func hasCrossing(crossings []geometry.BoundaryCrossing, x float64) bool {
	for _, c := range crossings {
		if math.Abs(c.X-x) < 1e-6 {
			return true
		}
	}
	return false
}

// hasSlot reports whether crossings has an entry at x that is actually cut
// as an interlocking notch.
func hasSlot(crossings []geometry.BoundaryCrossing, x float64) bool {
	for _, c := range crossings {
		if c.Notch && math.Abs(c.X-x) < 1e-6 {
			return true
		}
	}
	return false
}
