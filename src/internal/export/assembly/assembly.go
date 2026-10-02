// Package assembly renders a single combined PDF "build sheet" for an
// insert: an isometric preview (assembled and exploded), a piece list, and a
// dimensioned flat drawing of each distinct piece. Where the other
// exporters each serve one purpose (svg/dxf for cutting, stl/step for CAD
// interchange, iso-svg/iso-png for a standalone preview), this is the
// document meant to be printed and built from.
package assembly

import (
	"fmt"
	"io"
	"math"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/export/isometric"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// Exporter writes the assembly PDF. Now, when non-nil, supplies the
// creation/modification timestamp embedded in the document (inject a fixed
// time for reproducible output); the default is time.Now.
type Exporter struct {
	// BoxCase outlines the box interior in the isometric pages.
	BoxCase bool
	Now     func() time.Time
}

func (Exporter) Format() string { return "assembly" }

const (
	pageMargin = 15.0
	headerY    = 22.0
)

func (e Exporter) Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error {
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	pdf := newPDFDoc(fpdf.New("P", "mm", "A4", ""))
	// fpdf otherwise stamps time.Now and iterates its internal maps in
	// random order, making output differ run to run.
	stamp := now()
	pdf.SetCreationDate(stamp)
	pdf.SetModificationDate(stamp)
	pdf.SetCatalogSort(true)
	pageW, pageH := pdf.GetPageSize()
	contentW := pageW - 2*pageMargin

	// No separate page header here: isometric.BuildScene already embeds its
	// own title/material/panel-count legend (matching the standalone
	// iso-svg/iso-png exports), so adding a second one would just duplicate
	// it inside the drawing area.
	setDocumentInfo(pdf, box.Project)
	isoPage := func(exploded bool) error {
		pdf.AddPage()
		scene := isometric.BuildSceneCase(box, panels, mat, exploded, e.BoxCase)
		top := pageMargin
		if !exploded {
			// The project title block, when there is one, sits above the
			// assembled preview and pushes it down; without a project
			// nothing moves.
			top += drawProjectBlock(pdf, box.Project, pageMargin, pageMargin, contentW)
		}
		return fitScene(pdf, scene, pageMargin, top, contentW, pageH-pageMargin-top)
	}
	if err := isoPage(false); err != nil {
		return err
	}
	if err := isoPage(true); err != nil {
		return err
	}

	drawPieceList(pdf, export.GroupPanels(panels), pageW, pageH)

	drawPieceDrawingPages(pdf, export.GroupPanels(panels), pageW, pageH)

	return pdf.Output(w)
}

// fitScene centers an isometric.Scene, uniformly scaled to fit, within the
// rectangle (x0, y0, w, h). It returns an error, drawing nothing, when the
// scene bounds are not finite and positive (which would otherwise yield a
// blank page or a NaN scale).
func fitScene(pdf *pdfDoc, s *isometric.Scene, x0, y0, w, h float64) error {
	sceneW, sceneH := s.MaxX-s.MinX, s.MaxY-s.MinY
	if !isFinitePositive(sceneW) || !isFinitePositive(sceneH) {
		return fmt.Errorf("assembly export: isometric scene has degenerate bounds (%v x %v)", sceneW, sceneH)
	}
	scale := math.Min(w/sceneW, h/sceneH)
	drawnW, drawnH := sceneW*scale, sceneH*scale
	offsetX := x0 + (w-drawnW)/2 - s.MinX*scale
	offsetY := y0 + (h-drawnH)/2 - s.MinY*scale
	drawScene(pdf, s, offsetX, offsetY, scale)
	return nil
}

func isFinitePositive(v float64) bool {
	return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}

// variantLetters returns, per group, a letter ("A", "B", ...) when it shares
// axis, length and height with a neighbouring group (groups are sorted, so such
// ties are adjacent) and "" otherwise.
func variantLetters(groups []export.PanelGroup) []string {
	out := make([]string, len(groups))
	same := func(a, b geometry.Panel) bool {
		return a.Axis == b.Axis && a.Length == b.Length && a.Height == b.Height
	}
	for i := 0; i < len(groups); {
		j := i + 1
		for j < len(groups) && same(groups[i].Panel, groups[j].Panel) {
			j++
		}
		if j-i > 1 {
			for k := i; k < j; k++ {
				out[k] = string(rune('A' + (k-i)%26))
			}
		}
		i = j
	}
	return out
}

func drawPieceList(pdf *pdfDoc, groups []export.PanelGroup, pageW, pageH float64) {
	const (
		baseWidthsSum = 170.0 // the column widths below were tuned for this total
		rowH          = 7.0
		autoBreakMM   = 10.0 // fpdf's default bottom margin, restored afterwards
	)
	contentW := pageW - 2*pageMargin
	widths := []float64{35, 30, 25, 25, 20, 20, 15}
	for i := range widths {
		widths[i] *= contentW / baseWidthsSum
	}
	headers := []string{"ID", "Axis", "L (mm)", "H (mm)", "T (mm)", "Notches", "Qty"}

	// Page breaks are handled explicitly so every page repeats the title and
	// header row.
	pdf.SetAutoPageBreak(false, 0)
	defer pdf.SetAutoPageBreak(true, autoBreakMM)

	startPage := func(title string) {
		pdf.AddPage()
		pdf.SetFont("Courier", "B", 14)
		pdf.SetTextColor(20, 20, 20)
		pdf.Text(pageMargin, 15, title)

		pdf.SetXY(pageMargin, 25)
		pdf.SetFont("Courier", "B", 9)
		pdf.SetFillColor(230, 230, 230)
		for i, hdr := range headers {
			pdf.CellFormat(widths[i], rowH, hdr, "1", 0, "C", true, 0, "")
		}
		pdf.Ln(rowH)
		pdf.SetFont("Courier", "", 9)
	}

	variants := variantLetters(groups)
	startPage("Piece List")
	for gi, g := range groups {
		if pdf.GetY()+rowH > pageH-pageMargin {
			startPage("Piece List (cont.)")
		}
		p := g.Panel
		// Truncate the id part only so the variant letter stays visible.
		suffix := ""
		if variants[gi] != "" {
			suffix = " " + variants[gi]
		}
		idMaxW := widths[0] - 2*pdf.GetCellMargin() - pdf.GetStringWidth(suffix)
		id := truncateToWidth(pdf, p.ID, idMaxW) + suffix
		cells := []string{
			id, string(p.Axis),
			fmt.Sprintf("%g", p.Length), fmt.Sprintf("%g", p.Height), fmt.Sprintf("%g", p.Thickness),
			fmt.Sprintf("%d", len(p.Notches)), fmt.Sprintf("%d", g.Qty),
		}
		pdf.SetX(pageMargin)
		for i, c := range cells {
			pdf.CellFormat(widths[i], rowH, c, "1", 0, "C", false, 0, "")
		}
		pdf.Ln(rowH)
	}
}

const (
	drawingCols  = 2
	drawingGapMM = 8.0
)

func drawPieceDrawingPages(pdf *pdfDoc, groups []export.PanelGroup, pageW, pageH float64) {
	cellW := (pageW - 2*pageMargin - (drawingCols-1)*drawingGapMM) / drawingCols
	cellH := 70.0
	rowsPerPage := int(math.Floor((pageH - headerY - pageMargin) / (cellH + drawingGapMM)))
	if rowsPerPage < 1 {
		rowsPerPage = 1
	}
	perPage := rowsPerPage * drawingCols

	for i, g := range groups {
		posOnPage := i % perPage
		if posOnPage == 0 {
			pdf.AddPage()
			pdf.SetFont("Courier", "B", 14)
			pdf.SetTextColor(20, 20, 20)
			pdf.Text(pageMargin, 15, "Piece Drawings")
		}
		col := posOnPage % drawingCols
		row := posOnPage / drawingCols
		x0 := pageMargin + float64(col)*(cellW+drawingGapMM)
		y0 := headerY + float64(row)*(cellH+drawingGapMM)
		drawPanel2D(pdf, g, x0, y0, cellW, cellH)
	}
}
