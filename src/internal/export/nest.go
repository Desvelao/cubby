package export

import (
	"fmt"
	"strings"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/pack"
)

// DefaultSheetWidth is the virtual sheet width (mm) panels are nested onto
// when no --sheet-width override is given.
const DefaultSheetWidth = 600.0

// NestGap is the default spacing (mm) kept between nested panels. It is
// deliberately independent of the material kerf: kerf is already compensated
// in the notch widths (geometry.SlotWidth), so the gap is only cutting
// clearance and can be changed with --nest-gap.
const NestGap = 5.0

// GapOrDefault returns *gap, or NestGap when gap is nil. A pointer lets an
// explicit gap of 0 be told apart from "not set".
func GapOrDefault(gap *float64) float64 {
	if gap == nil {
		return NestGap
	}
	return *gap
}

// Placement is a panel's bounding box on the sheet. Rect.W x Rect.D is the
// footprint as placed: the panel's Length x Height, or Height x Length when
// Rotated.
type Placement struct {
	pack.Rect
	// Rotated reports the panel was turned 90 degrees to fit the sheet width.
	Rotated bool
}

// Outline returns p's outline in sheet-local coordinates relative to the
// placement origin, applying the 90-degree rotation (x,y) -> (Height-y, x)
// when Rotated (a proper rotation, so the outline is not mirrored).
func (pl Placement) Outline(p geometry.Panel) []geometry.Point2D {
	pts := p.OutlinePoints()
	if !pl.Rotated {
		return pts
	}
	out := make([]geometry.Point2D, len(pts))
	for i, pt := range pts {
		out[i] = geometry.Point2D{X: p.Height - pt.Y, Y: pt.X}
	}
	return out
}

// Nesting is the result of nesting panels onto a virtual sheet.
type Nesting struct {
	// Placement maps a panel ID to where it sits on the sheet.
	Placement map[string]Placement
	// SheetWidth is the effective sheet width in mm.
	SheetWidth float64
	// UsedHeight is the sheet height consumed by the nested panels in mm.
	UsedHeight float64
}

// NestPanels shelf-packs panel bounding boxes onto a sheet of sheetW mm
// (DefaultSheetWidth when sheetW <= 0), keeping gap mm between panels (NestGap
// when gap < 0; 0 is a valid, explicit gap). Panels are kept unrotated unless their
// Length exceeds the sheet width, in which case they are turned 90 degrees.
// format prefixes the errors returned when two panels share an ID (placements
// are keyed by ID, so a duplicate would silently overwrite another panel's
// position) or when one or more panels do not fit the sheet width even rotated.
func NestPanels(format string, panels []geometry.Panel, sheetW, gap float64) (Nesting, error) {
	if gap < 0 {
		gap = NestGap
	}
	if sheetW <= 0 {
		sheetW = DefaultSheetWidth
	}
	items := make([]pack.Item, len(panels))
	rotated := map[string]bool{}
	seen := make(map[string]bool, len(panels))
	for i, p := range panels {
		if seen[p.ID] {
			return Nesting{}, fmt.Errorf("%s export: duplicate panel ID %q; panel IDs must be unique to nest", format, p.ID)
		}
		seen[p.ID] = true
		w, d := p.Length, p.Height
		if w+gap > sheetW && (d+gap <= sheetW || d < w) {
			// Too wide as drawn (the packer pads each side by gap/2, so the
			// usable width is sheetW-gap); turn it so the shorter side spans the
			// sheet. When neither orientation fits, the shorter side is still
			// the one to report.
			w, d = d, w
			rotated[p.ID] = true
		}
		items[i] = pack.Item{ID: p.ID, Qty: 1, W: w, D: d, H: 0, AllowRotate: false}
	}
	nested := pack.PackShelf(items, sheetW, 1e12, 1, gap/2)
	if len(nested.Missing) > 0 {
		sizes := map[string]geometry.Panel{}
		for _, p := range panels {
			sizes[p.ID] = p
		}
		var parts []string
		minWidth, minSide := 0.0, 0.0
		for _, m := range nested.Missing {
			p := sizes[m.ComponentID]
			minWidth = max(minWidth, min(p.Length, p.Height)+gap)
			minSide = max(minSide, min(p.Length, p.Height))
			parts = append(parts, fmt.Sprintf("%s (%gx%gmm, %s)", m.ComponentID, p.Length, p.Height, m.Reason))
		}
		hint := ""
		if minSide <= sheetW {
			// Panels are never squeezed into the gap: it is the clearance the
			// user asked for, so say that the gap is what makes them not fit.
			hint = fmt.Sprintf(" (the panels fit without the %gmm gap; or lower --nest-gap to at most %gmm)", gap, sheetW-minSide)
		}
		return Nesting{}, fmt.Errorf("%s export: %d panel(s) do not fit the %gmm sheet: %s; increase --sheet-width to at least %gmm%s", format, len(nested.Missing), sheetW, strings.Join(parts, ", "), minWidth, hint)
	}
	placement := map[string]Placement{}
	for _, row := range nested.Rows {
		for _, it := range row.Items {
			placement[it.ID] = Placement{Rect: it.Rect, Rotated: rotated[it.ID]}
		}
	}
	return Nesting{Placement: placement, SheetWidth: sheetW, UsedHeight: nested.UsedD}, nil
}
