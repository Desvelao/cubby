package geometry

import (
	"fmt"
	"math"
	"sort"
)

// Edge names the panel edge a notch is cut from.
type Edge string

const (
	// EdgeBottom is the y = 0 edge.
	EdgeBottom Edge = "bottom"
	// EdgeTop is the y = height edge.
	EdgeTop Edge = "top"
)

// ValidateNotchEdges rejects notches whose Edge is not EdgeTop or EdgeBottom,
// and notch lists that mix edges. A panel is notched from exactly one edge:
// both edges are cut to half the panel height, so a top and a bottom notch
// overlapping in x would sever the panel. BuildOutline and DecomposeFace
// assume their input has passed this check.
func ValidateNotchEdges(notches []Notch) error {
	for i, n := range notches {
		if n.Edge != EdgeTop && n.Edge != EdgeBottom {
			return fmt.Errorf("notch at %.4g: unknown edge %q (want %q or %q)", n.Pos, n.Edge, EdgeTop, EdgeBottom)
		}
		if n.Edge != notches[0].Edge {
			return fmt.Errorf("notches mix edges %q and %q (notch %d at %.4g); a panel may be notched from one edge only",
				notches[0].Edge, n.Edge, i, n.Pos)
		}
	}
	return nil
}

// notchEps tolerates float rounding when comparing notch bounds.
const notchEps = 1e-9

// ValidateNotches rejects notch lists that would yield an invalid outline on
// a length x height panel: unknown or mixed edges (see ValidateNotchEdges),
// notches with a non-finite position or width or a width <= 0, notches outside
// [0, length], and notches that overlap each other. Error
// text carries no panel prefix; callers add their own context.
func ValidateNotches(length, height float64, notches []Notch) error {
	if err := ValidateNotchEdges(notches); err != nil {
		return err
	}
	if len(notches) > 0 && height <= 0 {
		return fmt.Errorf("height %.4g leaves no room for slot notches", height)
	}
	for _, n := range notches {
		if math.IsNaN(n.Pos) || math.IsInf(n.Pos, 0) || math.IsNaN(n.Width) || math.IsInf(n.Width, 0) {
			return fmt.Errorf("slot notch at %v has a non-finite position or width (%v)", n.Pos, n.Width)
		}
		if n.Width <= 0 {
			return fmt.Errorf("slot notch at %.4g has width %.4g; the width must be positive", n.Pos, n.Width)
		}
	}
	sorted := append([]Notch(nil), notches...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Pos < sorted[j].Pos })
	for i, n := range sorted {
		if n.Pos < -notchEps || n.Pos+n.Width > length+notchEps {
			return fmt.Errorf("length %.4g is too short for its %.4g-wide slot notch at %.4g; the row/compartment must be at least %.4g (twice that where notched at both ends)",
				length, n.Width, n.Pos, n.Width)
		}
		if i > 0 && sorted[i-1].Pos+sorted[i-1].Width > n.Pos+notchEps {
			return fmt.Errorf("slot notches at %.4g and %.4g overlap (slot width %.4g, length %.4g); boundaries/rows must be at least %.4g apart",
				sorted[i-1].Pos, n.Pos, n.Width, length, n.Width)
		}
	}
	return nil
}

// ValidateCut rejects a top-edge cut that BuildOutlineCut cannot honour on a
// panel of full height: a cut that leaves nothing (cut >= height), or, on a
// notched panel, one that reaches the notch depth (height/2) and so flattens
// the notch floor. A cut <= 0 is always valid. Error text carries no panel
// prefix; callers add their own context.
func ValidateCut(height, cut float64, notched bool) error {
	if cut <= 0 {
		return nil
	}
	if cut >= height {
		return fmt.Errorf("height reduction %.4g leaves nothing of its %.4g height", cut, height)
	}
	if notched && cut >= height/2 {
		return fmt.Errorf("height reduction %.4g must stay below half the panel height (%.4g) so its slot notches still mate; reduce it or list only panels without notches", cut, height/2)
	}
	return nil
}
