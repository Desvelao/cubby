package geometry

import "sort"

// BuildOutline constructs a panel's flat rectilinear outline: a
// length x height rectangle with notches cut into exactly one edge (EdgeTop,
// y = height, or EdgeBottom, y = 0; see ValidateNotchEdges). Notches must be non-overlapping; they are
// sorted by position before the outline is walked. Winding order is
// counter-clockwise starting at the origin.
func BuildOutline(length, height float64, notches []Notch) []Point2D {
	if len(notches) == 0 {
		return []Point2D{
			{X: 0, Y: 0}, {X: length, Y: 0}, {X: length, Y: height}, {X: 0, Y: height},
		}
	}

	sorted := append([]Notch(nil), notches...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Pos < sorted[j].Pos })
	notchDepth := height / 2
	edge := sorted[0].Edge

	sorted = snapNotches(sorted, length)

	var pts []Point2D
	if edge == EdgeBottom {
		// Walk the bottom edge left to right, stepping up into each notch
		// (material is removed below notchDepth there), then straight up
		// the right side and back along the clean top edge.
		pts = []Point2D{{X: 0, Y: 0}}
		for _, n := range sorted {
			pts = append(pts,
				Point2D{X: n.Pos, Y: 0},
				Point2D{X: n.Pos, Y: notchDepth},
				Point2D{X: n.Pos + n.Width, Y: notchDepth},
				Point2D{X: n.Pos + n.Width, Y: 0},
			)
		}
		pts = append(pts, Point2D{X: length, Y: 0}, Point2D{X: length, Y: height}, Point2D{X: 0, Y: height})
	} else {
		// edge == "top": walk the top edge right to left (to keep CCW
		// winding), stepping into each notch.
		pts = []Point2D{{X: 0, Y: 0}, {X: length, Y: 0}, {X: length, Y: height}}
		for i := len(sorted) - 1; i >= 0; i-- {
			n := sorted[i]
			pts = append(pts,
				Point2D{X: n.Pos + n.Width, Y: height},
				Point2D{X: n.Pos + n.Width, Y: notchDepth},
				Point2D{X: n.Pos, Y: notchDepth},
				Point2D{X: n.Pos, Y: height},
			)
		}
		pts = append(pts, Point2D{X: 0, Y: height})
	}

	return dedupConsecutive(pts)
}

// snapBound clamps v to [0, length] and snaps it onto 0 or length when it is
// within notchEps of them, so notches ValidateNotches accepts by tolerance end
// exactly flush with the panel instead of 1 ulp outside or inside it.
func snapBound(v, length float64) float64 {
	if v <= notchEps {
		return 0
	}
	if v >= length-notchEps {
		return length
	}
	return v
}

// snapNotches returns a copy of notches with Pos and Pos+Width snapped by
// snapBound. Width is recomputed from the snapped bounds.
func snapNotches(notches []Notch, length float64) []Notch {
	out := make([]Notch, len(notches))
	for i, n := range notches {
		lo, hi := snapBound(n.Pos, length), snapBound(n.Pos+n.Width, length)
		n.Pos, n.Width = lo, hi-lo
		out[i] = n
	}
	return out
}

// dedupConsecutive drops zero-length edges that arise when a notch sits
// exactly at a panel's end (Pos 0 or Pos+Width == length).
func dedupConsecutive(pts []Point2D) []Point2D {
	out := pts[:0:0]
	for i, p := range pts {
		if i > 0 && p == pts[i-1] {
			continue
		}
		out = append(out, p)
	}
	if len(out) > 1 && out[0] == out[len(out)-1] {
		out = out[:len(out)-1]
	}
	return out
}

// BuildOutlineCut is BuildOutline for a panel whose top edge is lowered by
// cut. Notches keep the depth of the full-height panel, so the half-lap joints
// still mate with neighbours; the panel is just trimmed from the top. The
// caller must keep cut below the notch depth (height/2) on notched panels;
// ValidateCut and ValidateNotches check this.
func BuildOutlineCut(length, height, cut float64, notches []Notch) []Point2D {
	pts := BuildOutline(length, height, notches)
	if cut <= 0 {
		return pts
	}
	top := height - cut
	for i := range pts {
		if pts[i].Y > top {
			pts[i].Y = top
		}
	}
	return dedupConsecutive(pts)
}
