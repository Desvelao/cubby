// Package dxf writes panel outlines as a minimal DXF R12 file — the most
// broadly compatible DXF flavor, opening cleanly in LibreCAD. A TABLES section
// declares the linetype and the layers the entities use: one layer per panel
// axis (outlines, to cut) and LABELS (panel ID text, to hide or ignore when
// cutting).
//
// Kerf: the outlines are the nominal, uncompensated panel shapes. Material
// kerf is already accounted for in the joint fit by geometry.SlotWidth, which
// widens every notch by the kerf, so no outline offset is applied here (that
// would double-count the kerf).
//
// Units: the HEADER declares millimetres ($INSUNITS = 4) and metric
// measurement ($MEASUREMENT = 1), so importers do not assume inches or
// unitless drawings.
//
// Orientation: SVG's Y axis points down while DXF's points up. To make a DXF
// opened in a Y-up CAD viewer look the same as the SVG, every vertex is
// written at y = UsedHeight - (placementY + outlineY), where UsedHeight is
// the nested sheet height. This flips the whole sheet (first shelf ends up at
// the top, as in the SVG) and, because it is one affine transform, each panel
// is flipped around its own extent without panels overlapping. X is unchanged.
package dxf

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// DefaultSheetWidth is the virtual sheet width (mm) panels are nested onto,
// matching the SVG exporter's default so both formats lay panels out the
// same way.
const DefaultSheetWidth = export.DefaultSheetWidth

type Exporter struct {
	// SheetWidth is the nesting canvas width in mm. Zero uses DefaultSheetWidth.
	SheetWidth float64
	// NestGap is the spacing in mm between nested panels. Nil uses
	// export.NestGap (5); a pointer so an explicit 0 is honoured.
	NestGap *float64
	// BoxCase draws the box interior (top and front views) on a BOX_CASE
	// layer below the nested panels.
	BoxCase bool
}

const (
	// labelLayer holds the panel ID TEXT entities, apart from the cut outlines.
	labelLayer = "LABELS"
	// caseLayer holds the reference rectangles of the box case.
	caseLayer = "BOX_CASE"
	// The label sits where the SVG exporter puts its <text>: 2mm right of and
	// 12mm below the panel's placement origin (SVG Y-down), baseline-left.
	labelDX, labelDY = 2.0, 12.0
	// labelHeight is the DXF text (cap) height in mm. The SVG label uses
	// font-size 6 (em size); cap height is about 0.72 em, so 4.3mm draws
	// letters the same visual size.
	labelHeight = 4.3
	// maxTextLen is the DXF R12 limit for a group code 1 string.
	maxTextLen = 255
)

// text returns s as a DXF R12 string value: R12 is not UTF-8, so non-ASCII
// runes and control characters (which would break the line-based format)
// become '?'; '^' (control-character notation) also becomes '?', and '%' is
// written as the %%37 escape so it is not read as a %%c/%%d/%%p code. The
// result is capped at maxTextLen characters.
func text(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		var piece string
		switch {
		case r == '%':
			piece = "%%37"
		case r < 0x20 || r > 0x7E || r == '^':
			piece = "?"
		default:
			piece = string(r)
		}
		if n+len(piece) > maxTextLen {
			break
		}
		b.WriteString(piece)
		n += len(piece)
	}
	return b.String()
}

// layerNames returns the sorted layers used by the placed panels' outlines
// and, when any panel is placed, the label layer.
func layerNames(panels []geometry.Panel, placed map[string]export.Placement, boxCase bool) []string {
	set := map[string]bool{}
	if boxCase {
		set[caseLayer] = true
	}
	for _, p := range panels {
		if _, ok := placed[p.ID]; ok {
			set[string(p.Axis)] = true
			set[labelLayer] = true
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (Exporter) Format() string { return "dxf" }

func gc(ew *export.ErrWriter, code int, value string) {
	ew.Printf("%d\n%s\n", code, value)
}

func gcInt(ew *export.ErrWriter, code int, value int) {
	gc(ew, code, fmt.Sprintf("%d", value))
}

func gcFloat(ew *export.ErrWriter, code int, value float64) {
	gc(ew, code, fmt.Sprintf("%g", value))
}

func (e Exporter) Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error {
	nest, err := export.NestPanels("dxf", panels, e.SheetWidth, export.GapOrDefault(e.NestGap))
	if err != nil {
		return err
	}
	origin := nest.Placement

	ew := &export.ErrWriter{W: w}

	gc(ew, 0, "SECTION")
	gc(ew, 2, "HEADER")
	gc(ew, 9, "$ACADVER")
	gc(ew, 1, "AC1009") // R12
	gc(ew, 9, "$INSUNITS")
	gcInt(ew, 70, 4) // millimetres
	gc(ew, 9, "$MEASUREMENT")
	gcInt(ew, 70, 1) // metric
	gc(ew, 0, "ENDSEC")

	layers := layerNames(panels, origin, e.BoxCase)
	gc(ew, 0, "SECTION")
	gc(ew, 2, "TABLES")
	gc(ew, 0, "TABLE")
	gc(ew, 2, "LTYPE")
	gcInt(ew, 70, 1)
	gc(ew, 0, "LTYPE")
	gc(ew, 2, "CONTINUOUS")
	gcInt(ew, 70, 0)
	gc(ew, 3, "Solid line")
	gcInt(ew, 72, 65)
	gcInt(ew, 73, 0)
	gcFloat(ew, 40, 0)
	gc(ew, 0, "ENDTAB")
	gc(ew, 0, "TABLE")
	gc(ew, 2, "LAYER")
	gcInt(ew, 70, len(layers))
	for _, name := range layers {
		color := 7 // white/black
		if name == labelLayer || name == caseLayer {
			color = 8 // grey
		}
		gc(ew, 0, "LAYER")
		gc(ew, 2, name)
		gcInt(ew, 70, 0)
		gcInt(ew, 62, color)
		gc(ew, 6, "CONTINUOUS")
	}
	gc(ew, 0, "ENDTAB")
	gc(ew, 0, "ENDSEC")

	gc(ew, 0, "SECTION")
	gc(ew, 2, "ENTITIES")

	for _, p := range panels {
		o, ok := origin[p.ID]
		if !ok {
			continue
		}
		gc(ew, 0, "POLYLINE")
		gc(ew, 8, string(p.Axis)) // layer per axis
		gcInt(ew, 66, 1)          // "vertices follow" flag
		gcInt(ew, 70, 1)          // closed polyline

		for _, pt := range o.Outline(p) {
			gc(ew, 0, "VERTEX")
			gc(ew, 8, string(p.Axis))
			gcFloat(ew, 10, o.X+pt.X)
			gcFloat(ew, 20, nest.UsedHeight-(o.Y+pt.Y)) // flip: SVG Y-down -> DXF Y-up
		}
		gc(ew, 0, "SEQEND")

		gc(ew, 0, "TEXT")
		gc(ew, 8, labelLayer)
		gcFloat(ew, 10, o.X+labelDX)
		gcFloat(ew, 20, nest.UsedHeight-(o.Y+labelDY)) // same flip as the vertices
		gcFloat(ew, 40, labelHeight)
		gc(ew, 1, text(p.ID))
	}

	if e.BoxCase {
		// Same Y flip as the panels, applied to rectangles stacked below the sheet.
		rects, _ := export.CaseRects(box, nest.UsedHeight, export.GapOrDefault(e.NestGap)+10)
		for _, r := range rects {
			corners := [4][2]float64{{r.X, r.Y}, {r.X + r.W, r.Y}, {r.X + r.W, r.Y + r.H}, {r.X, r.Y + r.H}}
			gc(ew, 0, "POLYLINE")
			gc(ew, 8, caseLayer)
			gcInt(ew, 66, 1)
			gcInt(ew, 70, 1)
			for _, c := range corners {
				gc(ew, 0, "VERTEX")
				gc(ew, 8, caseLayer)
				gcFloat(ew, 10, c[0])
				gcFloat(ew, 20, nest.UsedHeight-c[1])
			}
			gc(ew, 0, "SEQEND")
			gc(ew, 0, "TEXT")
			gc(ew, 8, caseLayer)
			gcFloat(ew, 10, r.X+labelDX)
			gcFloat(ew, 20, nest.UsedHeight-(r.Y-2))
			gcFloat(ew, 40, labelHeight)
			gc(ew, 1, text(r.Label))
		}
	}

	gc(ew, 0, "ENDSEC")
	gc(ew, 0, "EOF")
	return ew.Err
}
