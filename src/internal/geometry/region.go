package geometry

import (
	"math"
	"sort"

	"github.com/Desvelao/cubby/internal/pack"
)

// Region is the box interior laid out as rows of compartments, derived from
// a pack.BoxResult. Compartments sharing the same Y start (within floating
// point tolerance) belong to the same row; within a row, cells are ordered
// left to right. Box-level packing always stacks rows with no vertical gap
// and packs cells within a row with no horizontal gap, so row/cell
// boundaries are exact touching lines — this is what makes flat (non
// recursive) panel generation valid for cubby's current layout engine.
//
// Limitation: "touching exactly" holds for row Y starts and cell X edges, not
// for cell depths. Group compartments are sized tightly to their content, so
// cells of one row can have different Bounds.D. The row is as deep as its
// deepest cell (RowNode.Height); a shallower cell leaves an uncovered strip
// between its own far edge and the row's far edge. Row-level geometry
// (cellBoundaries, RowBoundaryPositions and the panels built from them)
// ignores per-cell depth and treats every cell as spanning the full row.
type Region struct {
	Bounds pack.Rect
	Rows   []RowNode
}

// RowNode is one horizontal band of the region. Height is the maximum
// Bounds.D of its cells, not a depth shared by all of them: shallower cells
// do not reach Y+Height (see Region).
type RowNode struct {
	Y, Height float64
	Cells     []Cell
}

// Cell is one compartment's slot within a row.
type Cell struct {
	Bounds      pack.Rect
	Compartment pack.CompartmentResult
}

// BuildRegion groups a packed box's compartments into rows.
func BuildRegion(box pack.BoxResult) Region {
	byY := map[float64][]pack.CompartmentResult{}
	var yOrder []float64
	for _, c := range box.Compartments {
		key := snap(c.Bounds.Y)
		if _, ok := byY[key]; !ok {
			yOrder = append(yOrder, key)
		}
		byY[key] = append(byY[key], c)
	}
	sort.Float64s(yOrder)

	var rows []RowNode
	for _, y := range yOrder {
		comps := byY[y]
		sort.Slice(comps, func(i, j int) bool { return comps[i].Bounds.X < comps[j].Bounds.X })

		height := 0.0
		cells := make([]Cell, 0, len(comps))
		for _, c := range comps {
			if c.Bounds.D > height {
				height = c.Bounds.D
			}
			cells = append(cells, Cell{Bounds: c.Bounds, Compartment: c})
		}
		rows = append(rows, RowNode{Y: y, Height: height, Cells: cells})
	}

	return Region{
		Bounds: pack.Rect{X: 0, Y: 0, W: box.InteriorW, D: box.InteriorD},
		Rows:   rows,
	}
}

// snap rounds to a small epsilon so float noise doesn't split one physical
// row into two groups.
func snap(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
