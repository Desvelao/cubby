package geometry

import "sort"

// PanelRect is one axis-aligned rectangle of a panel's face, in the panel's
// own local (length, height) coordinate space.
type PanelRect struct {
	X, Y, W, H float64
}

// DecomposeFace splits a notched panel face into non-overlapping rectangles:
// one base rectangle for the un-notched half, plus one "tooth" rectangle per
// gap between consecutive notches on the notched half. Because notches are
// axis-aligned cuts from a single edge and never overlap, this always yields
// a clean, gap-free rectangle set — no general polygon triangulator needed.
// Used by both the STL and STEP exporters.
func DecomposeFace(length, height float64, notches []Notch) []PanelRect {
	return DecomposeFaceCut(length, height, 0, notches)
}

// DecomposeFaceCut is DecomposeFace for a panel whose top edge is lowered by
// cut, as BuildOutlineCut builds it: height is the reduced height and notches
// keep the depth of the full-height (height+cut) panel.
func DecomposeFaceCut(length, height, cut float64, notches []Notch) []PanelRect {
	if len(notches) == 0 {
		return []PanelRect{{X: 0, Y: 0, W: length, H: height}}
	}

	sorted := append([]Notch(nil), notches...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Pos < sorted[j].Pos })
	sorted = snapNotches(sorted, length)
	notchDepth := (height + cut) / 2
	edge := sorted[0].Edge

	var rects []PanelRect
	if edge == EdgeBottom {
		// Base: the untouched top half spans the full length.
		rects = append(rects, PanelRect{X: 0, Y: notchDepth, W: length, H: height - notchDepth})
		// Teeth: full-height rectangles between/around notches on the bottom half.
		x := 0.0
		for _, n := range sorted {
			if n.Pos > x {
				rects = append(rects, PanelRect{X: x, Y: 0, W: n.Pos - x, H: notchDepth})
			}
			x = n.Pos + n.Width
		}
		if x < length {
			rects = append(rects, PanelRect{X: x, Y: 0, W: length - x, H: notchDepth})
		}
		return rects
	}

	// edge == "top"
	rects = append(rects, PanelRect{X: 0, Y: 0, W: length, H: notchDepth})
	x := 0.0
	for _, n := range sorted {
		if n.Pos > x {
			rects = append(rects, PanelRect{X: x, Y: notchDepth, W: n.Pos - x, H: height - notchDepth})
		}
		x = n.Pos + n.Width
	}
	if x < length {
		rects = append(rects, PanelRect{X: x, Y: notchDepth, W: length - x, H: height - notchDepth})
	}
	return rects
}
