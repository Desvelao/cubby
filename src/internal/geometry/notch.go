package geometry

import (
	"sort"

	"github.com/Desvelao/cubby/internal/manifest"
)

// SlotWidth returns how wide a notch must be cut for panels of the given
// material to interlock: the panel thickness plus kerf clearance.
func SlotWidth(mat manifest.Material) float64 {
	return mat.Thickness + mat.Kerf
}

// BoundaryCrossing is one interior cell-boundary position a horizontal panel
// might need cut, tagged with whether it should actually be cut as an
// interlocking notch: a notch only helps if every panel meeting there is
// also cut, so Notch is true only when every compartment touching this
// position resolves to manifest.JointTypeNotch.
type BoundaryCrossing struct {
	X     float64
	Notch bool
}

// cellBoundaries returns the interior X positions between consecutive cells
// in a row (never the row's own left/right edges, since a compartment
// touching the box wall or open leftover space needs no divider there).
func cellBoundaries(row RowNode) []BoundaryCrossing {
	var xs []BoundaryCrossing
	for i := 0; i+1 < len(row.Cells); i++ {
		left, right := row.Cells[i], row.Cells[i+1]
		xs = append(xs, BoundaryCrossing{
			X:     snap(left.Bounds.X + left.Bounds.W),
			Notch: left.Compartment.JointType != manifest.JointTypePlain && right.Compartment.JointType != manifest.JointTypePlain,
		})
	}
	return xs
}

// RowBoundaryPositions returns where a horizontal panel between row a and
// row b might be notched: the union of both rows' internal cell boundaries
// (a vertical panel ending on either side needs a crossing slot there). When
// the same X position is contributed by both rows, its Notch flags are
// AND-combined: the position is only notchable if every contributing
// compartment on both sides wants a notch.
func RowBoundaryPositions(a, b RowNode) []BoundaryCrossing {
	notchByX := map[float64]bool{}
	seen := map[float64]bool{}
	var order []float64
	for _, c := range append(cellBoundaries(a), cellBoundaries(b)...) {
		if !seen[c.X] {
			seen[c.X] = true
			order = append(order, c.X)
			notchByX[c.X] = c.Notch
		} else {
			notchByX[c.X] = notchByX[c.X] && c.Notch
		}
	}
	sort.Float64s(order)
	xs := make([]BoundaryCrossing, len(order))
	for i, x := range order {
		xs[i] = BoundaryCrossing{X: x, Notch: notchByX[x]}
	}
	return xs
}
