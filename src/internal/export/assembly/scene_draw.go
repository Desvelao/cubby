package assembly

import (
	"github.com/go-pdf/fpdf"

	"github.com/Desvelao/cubby/internal/export/isometric"
)

// mmToPt converts a length in millimeters to PDF points, since fpdf.SetFont
// takes a point size regardless of the document's own unit.
const mmToPt = 72.0 / 25.4

// drawScene renders an isometric.Scene onto the current page: every Poly as
// a filled+stroked polygon, every Line as a stroked segment (dashed when
// requested), and every Label as text — a direct, mechanical translation of
// the same scene data the SVG/PNG isometric exporters already draw, so the
// PDF's isometric pages match those exports exactly.
func drawScene(pdf *pdfDoc, s *isometric.Scene, offsetX, offsetY, scale float64) {
	mapPt := func(x, y float64) fpdf.PointType {
		return fpdf.PointType{X: offsetX + x*scale, Y: offsetY + y*scale}
	}

	pdf.SetLineWidth(0.15)
	for _, poly := range s.Polys {
		pts := make([]fpdf.PointType, len(poly.Points))
		for i, p := range poly.Points {
			pts[i] = mapPt(p.X, p.Y)
		}
		pdf.SetFillColor(int(poly.Fill.R), int(poly.Fill.G), int(poly.Fill.B))
		pdf.SetDrawColor(int(poly.Stroke.R), int(poly.Stroke.G), int(poly.Stroke.B))
		pdf.Polygon(pts, "FD")
	}

	for _, l := range s.Lines {
		if l.Dashed {
			pdf.SetDashPattern([]float64{1, 1}, 0)
		} else {
			pdf.SetDashPattern(nil, 0)
		}
		pdf.SetDrawColor(int(l.Stroke.R), int(l.Stroke.G), int(l.Stroke.B))
		a, b := mapPt(l.A.X, l.A.Y), mapPt(l.B.X, l.B.Y)
		pdf.Line(a.X, a.Y, b.X, b.Y)
	}
	pdf.SetDashPattern(nil, 0)

	pdf.SetFont("Courier", "", 1) // size overridden per label below
	for _, lbl := range s.Labels {
		pdf.SetFontSize((lbl.Size * scale) * mmToPt)
		pdf.SetTextColor(int(lbl.Color.R), int(lbl.Color.G), int(lbl.Color.B))
		p := mapPt(lbl.Pos.X, lbl.Pos.Y)
		pdf.Text(p.X, p.Y, lbl.Text)
	}
}
