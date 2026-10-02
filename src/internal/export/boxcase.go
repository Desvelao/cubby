package export

import (
	"fmt"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/pack"
)

// CaseWallThickness is the wall thickness (mm) of the box case the 3D
// exporters draw around the box interior when --box-case is set.
const CaseWallThickness = 3.0

// CasePanels returns the box case as five plain rectangular panels (floor
// plus four walls) wrapped around the box interior, so the 3D exporters can
// extrude them like any other panel. They are for reference only and are
// never nested or cut.
func CasePanels(box pack.BoxResult) []geometry.Panel {
	w, d, h, t := box.InteriorW, box.InteriorD, box.InteriorH, CaseWallThickness
	return []geometry.Panel{
		{ID: "box-case-floor", Axis: geometry.AxisFloor, Length: w + 2*t, Height: d + 2*t, Thickness: t,
			Position: geometry.Placement3D{OriginX: -t, OriginY: -t, OriginZ: -t}},
		{ID: "box-case-front", Axis: geometry.AxisWidthRun, Length: w + 2*t, Height: h, Thickness: t,
			Position: geometry.Placement3D{OriginX: -t, OriginY: -t}},
		{ID: "box-case-back", Axis: geometry.AxisWidthRun, Length: w + 2*t, Height: h, Thickness: t,
			Position: geometry.Placement3D{OriginX: -t, OriginY: d}},
		{ID: "box-case-left", Axis: geometry.AxisDepthRun, Length: d, Height: h, Thickness: t,
			Position: geometry.Placement3D{OriginX: -t}},
		{ID: "box-case-right", Axis: geometry.AxisDepthRun, Length: d, Height: h, Thickness: t,
			Position: geometry.Placement3D{OriginX: w}},
	}
}

// CaseRect is a labelled reference rectangle for the 2D exporters, in the
// SVG's Y-down coordinates.
type CaseRect struct {
	X, Y, W, H float64
	Label      string
}

// CaseRects returns the box case's top view (W x D) and front view (W x H)
// stacked below y = top (leaving gap between them), plus the y of the bottom
// edge of the last rectangle.
func CaseRects(box pack.BoxResult, top, gap float64) ([]CaseRect, float64) {
	y := top + gap
	plan := CaseRect{X: 0, Y: y, W: box.InteriorW, H: box.InteriorD,
		Label: fmt.Sprintf("box case, top view: %g x %g mm", box.InteriorW, box.InteriorD)}
	y += plan.H + gap
	front := CaseRect{X: 0, Y: y, W: box.InteriorW, H: box.InteriorH,
		Label: fmt.Sprintf("box case, front view: %g x %g mm", box.InteriorW, box.InteriorH)}
	return []CaseRect{plan, front}, front.Y + front.H
}
