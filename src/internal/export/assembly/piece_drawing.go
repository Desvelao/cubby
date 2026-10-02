package assembly

import (
	"fmt"
	"math"

	"github.com/go-pdf/fpdf"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
)

var (
	pieceStroke    = [3]int{45, 45, 45}
	pieceFill      = [3]int{225, 225, 225}
	dimStroke      = [3]int{30, 90, 200}
	titleColor     = [3]int{20, 20, 20}
	notchTickColor = [3]int{160, 40, 40}
)

// Font sizes (pt) used by drawPanel2D; shared with pieceLayout's text-extent
// estimates so the two never drift apart.
const (
	dimFontPt   = 6.0
	notchFontPt = 5.0
	titleH      = 6.0 // fixed mm, unscaled, reserved for the title line
	cellPad     = 2.0 // mm of breathing room kept inside the cell
)

// extents is an axis-aligned bounding box in page mm.
type extents struct{ minX, minY, maxX, maxY float64 }

func (e *extents) add(x, y float64) {
	e.minX, e.maxX = math.Min(e.minX, x), math.Max(e.maxX, x)
	e.minY, e.maxY = math.Min(e.minY, y), math.Max(e.maxY, y)
}

// within reports whether e lies inside the cell (x0, y0, w, h).
func (e extents) within(x0, y0, w, h float64) bool {
	const eps = 1e-6
	return e.minX >= x0-eps && e.minY >= y0-eps && e.maxX <= x0+w+eps && e.maxY <= y0+h+eps
}

// textWidthMM conservatively estimates the width of text set in Courier
// (0.6 em per glyph) at the given point size.
func textWidthMM(text string, pt float64) float64 {
	return float64(len(text)) * 0.6 * pt * 25.4 / 72
}

// textHeightMM is the approximate ascent+descent of a line at the given size.
func textHeightMM(pt float64) float64 { return pt * 25.4 / 72 }

// lengthLabelX is the left anchor of the L label: centred-ish under the
// outline but never left of the cell's left edge (small outlines are narrower
// than their label).
func lengthLabelX(midX, x0 float64) float64 { return math.Max(midX-8, x0+cellPad/2) }

// notchLabelX is the left anchor of a notch's "@pos" label, kept off the
// left edge of the outline.
func notchLabelX(ax, bx, originX float64) float64 { return math.Max((ax+bx)/2-3, originX) }

func lengthLabel(p geometry.Panel) string { return fmt.Sprintf("L %g mm", round1(p.Length)) }
func heightLabel(p geometry.Panel) string { return fmt.Sprintf("H %g mm", round1(p.Height)) }

// notchDepthLabel is the notch depth callout, (Height+Cut)/2 as in the
// decomposition, or "" for a panel without notches.
func notchDepthLabel(p geometry.Panel) string {
	if len(p.Notches) == 0 {
		return ""
	}
	return fmt.Sprintf("D %g mm", round1((p.Height+p.Cut)/2))
}

// notchDepthY is the baseline of the depth label: one line below the H label.
func notchDepthY(midY float64) float64 { return midY + textHeightMM(dimFontPt)*1.25 }

// layout is the resolved geometry of one piece drawing: everything is in page
// mm, with gapS = gap*scale (the scaled margin used for dimension lines).
type layout struct {
	scale, gapS      float64
	originX, originY float64 // page position of the panel's local (0, Height)
	lengthY, heightX float64 // page position of the L / H dimension lines
	bounds           extents // everything drawn except the title line
}

// pieceLayout solves the scale so that the outline, both dimension lines (with
// tick marks and estimated label extents) and notch labels all stay inside the
// cell (x0, y0, maxW, maxH). The layout is computed in scaled units: the
// margin reserved for the dimension lines is gap*scale, the same value used to
// place them.
func pieceLayout(p geometry.Panel, x0, y0, maxW, maxH float64) layout {
	gap := 0.15 * math.Max(p.Length, p.Height)
	if gap < 4 {
		gap = 4
	}

	inCell := func(l layout) bool {
		return l.bounds.within(x0+cellPad/2, y0+cellPad/2, maxW-cellPad, maxH-cellPad)
	}
	valid := func(v float64) bool { return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) }
	// Bisect on the scale (the outline alone bounds it from above). The old
	// formula reserved the *unscaled* margin, which wasted most of the cell
	// for large panels and overflowed it for small ones.
	hi := math.Min((maxW-cellPad)/p.Length, (maxH-titleH-cellPad)/p.Height)
	if !valid(hi) {
		return layoutAt(p, gap, 1, x0, y0)
	}
	lo := 0.0
	for i := 0; i < 50; i++ {
		mid := (lo + hi) / 2
		if inCell(layoutAt(p, gap, mid, x0, y0)) {
			lo = mid
		} else {
			hi = mid
		}
	}
	if lo <= 0 {
		lo = hi
	}
	return layoutAt(p, gap, lo, x0, y0)
}

// layoutAt places every element for a given scale and computes the bounds.
func layoutAt(p geometry.Panel, gap, scale, x0, y0 float64) layout {
	g := gap * scale
	tick := g * 0.3
	l := layout{scale: scale, gapS: g}
	// Left margin: dimension line inset by its end tick (plus the cell pad)
	// so the ticks stay inside the cell.
	l.heightX = x0 + cellPad/2 + tick
	l.originX = l.heightX + g
	l.originY = y0 + titleH
	l.lengthY = l.originY + p.Height*scale + g

	outW, outH := p.Length*scale, p.Height*scale
	b := extents{l.originX, l.originY, l.originX, l.originY}
	b.add(l.originX+outW, l.originY+outH)

	// Length dimension: line, ticks, and label (drawn at lengthLabelX, baseline lengthY+3).
	b.add(l.originX, l.lengthY-tick)
	b.add(l.originX+outW, l.lengthY+tick)
	lw := textWidthMM(lengthLabel(p), dimFontPt)
	midX := l.originX + outW/2
	lx0 := lengthLabelX(midX, x0)
	b.add(lx0, l.lengthY+3-textHeightMM(dimFontPt))
	b.add(lx0+lw, l.lengthY+3+textHeightMM(dimFontPt)*0.25)

	// Height dimension: line, ticks, and label (drawn at heightX+1, baseline midY).
	b.add(l.heightX-tick, l.originY)
	b.add(l.heightX+tick, l.originY+outH)
	hw := textWidthMM(heightLabel(p), dimFontPt)
	midY := l.originY + outH/2
	b.add(l.heightX+1, midY-textHeightMM(dimFontPt))
	b.add(l.heightX+1+hw, midY+textHeightMM(dimFontPt)*0.25)

	if dl := notchDepthLabel(p); dl != "" {
		dy := notchDepthY(midY)
		b.add(l.heightX+1, dy-textHeightMM(dimFontPt))
		b.add(l.heightX+1+textWidthMM(dl, dimFontPt), dy+textHeightMM(dimFontPt)*0.25)
	}

	// Notch ticks and position labels.
	for _, n := range p.Notches {
		edgeY := 0.0
		if n.Edge == geometry.EdgeTop {
			edgeY = p.Height
		}
		ay := l.originY + (p.Height-edgeY)*scale
		ax, bx := l.originX+n.Pos*scale, l.originX+(n.Pos+n.Width)*scale
		td := -g * 0.25
		if n.Edge == geometry.EdgeTop {
			td = -td
		}
		b.add(ax, ay+td)
		b.add(bx, ay-td)
		labelY := ay + td*2.2
		text := fmt.Sprintf("@%g", round1(n.Pos))
		lx := notchLabelX(ax, bx, l.originX)
		b.add(lx, labelY-textHeightMM(notchFontPt))
		b.add(lx+textWidthMM(text, notchFontPt), labelY+textHeightMM(notchFontPt)*0.25)
	}
	l.bounds = b
	return l
}

// drawPanel2D draws one distinct panel type's flat outline, scaled to fit
// within (maxW, maxH) below (x0, y0), with a title line, overall
// length/height witness lines, a thickness callout, and a tick + position
// label at each notch. It's a true flat 2D view (no projection), unlike the
// isometric pages.
func drawPanel2D(pdf *pdfDoc, g export.PanelGroup, x0, y0, maxW, maxH float64) {
	p := g.Panel
	lay := pieceLayout(p, x0, y0, maxW, maxH)
	scale, gapS := lay.scale, lay.gapS
	originX, originY := lay.originX, lay.originY

	mapPt := func(localX, localY float64) fpdf.PointType {
		return fpdf.PointType{X: originX + localX*scale, Y: originY + (p.Height-localY)*scale}
	}

	// Title, with the thickness callout right after it on the same line
	// (offset by the title's actual rendered width, so it never overlaps
	// regardless of how long the panel ID is).
	pdf.SetFont("Courier", "B", 9)
	pdf.SetTextColor(titleColor[0], titleColor[1], titleColor[2])
	titleText := fmt.Sprintf("%s  x%d  (%s)", p.ID, g.Qty, string(p.Axis))
	pdf.Text(x0, y0+4, titleText)
	titleW := pdf.GetStringWidth(titleText)

	// Outline.
	outline := p.OutlinePoints()
	pts := make([]fpdf.PointType, len(outline))
	for i, o := range outline {
		pts[i] = mapPt(o.X, o.Y)
	}
	pdf.SetLineWidth(0.2)
	pdf.SetFillColor(pieceFill[0], pieceFill[1], pieceFill[2])
	pdf.SetDrawColor(pieceStroke[0], pieceStroke[1], pieceStroke[2])
	pdf.Polygon(pts, "FD")

	// Length dimension (below the shape).
	lengthY := lay.lengthY
	drawDimension2D(pdf, mapPt(0, 0).X, lengthY, mapPt(p.Length, 0).X, lengthY, gapS*0.3,
		lengthLabel(p), true, lengthLabelX((mapPt(0, 0).X+mapPt(p.Length, 0).X)/2, x0))

	// Height dimension (left of the shape).
	heightX := lay.heightX
	drawDimension2D(pdf, heightX, mapPt(0, 0).Y, heightX, mapPt(0, p.Height).Y, gapS*0.3,
		heightLabel(p), false, 0)

	// Notch depth callout (only for panels with notches), under the H label.
	if dl := notchDepthLabel(p); dl != "" {
		pdf.SetFont("Courier", "", dimFontPt)
		pdf.SetTextColor(dimStroke[0], dimStroke[1], dimStroke[2])
		pdf.Text(heightX+1, notchDepthY((mapPt(0, 0).Y+mapPt(0, p.Height).Y)/2), dl)
	}

	// Thickness callout.
	pdf.SetFont("Courier", "", 7)
	pdf.SetTextColor(dimStroke[0], dimStroke[1], dimStroke[2])
	pdf.Text(x0+titleW+4, y0+4, fmt.Sprintf("T %g mm", round1(p.Thickness)))

	// Notch position callouts.
	pdf.SetFont("Courier", "", notchFontPt)
	for _, n := range p.Notches {
		edgeY := 0.0
		if n.Edge == geometry.EdgeTop {
			edgeY = p.Height
		}
		a := mapPt(n.Pos, edgeY)
		b := mapPt(n.Pos+n.Width, edgeY)
		tickDir := -gapS * 0.25
		if n.Edge == geometry.EdgeTop {
			tickDir = -tickDir
		}
		pdf.SetDrawColor(notchTickColor[0], notchTickColor[1], notchTickColor[2])
		pdf.Line(a.X, a.Y+tickDir, a.X, a.Y-tickDir)
		pdf.Line(b.X, b.Y+tickDir, b.X, b.Y-tickDir)
		pdf.SetTextColor(notchTickColor[0], notchTickColor[1], notchTickColor[2])
		labelY := a.Y + tickDir*2.2
		pdf.Text(notchLabelX(a.X, b.X, originX), labelY, fmt.Sprintf("@%g", round1(n.Pos)))
	}
}

// drawDimension2D draws a straight witness line with small perpendicular end
// ticks and a text label, either horizontal (anchored at labelX) or vertical
// (anchored just right of the line).
func drawDimension2D(pdf *pdfDoc, x1, y1, x2, y2, tickLen float64, text string, horizontal bool, labelX float64) {
	pdf.SetDrawColor(dimStroke[0], dimStroke[1], dimStroke[2])
	pdf.SetLineWidth(0.1)
	pdf.Line(x1, y1, x2, y2)
	if horizontal {
		pdf.Line(x1, y1-tickLen, x1, y1+tickLen)
		pdf.Line(x2, y2-tickLen, x2, y2+tickLen)
	} else {
		pdf.Line(x1-tickLen, y1, x1+tickLen, y1)
		pdf.Line(x2-tickLen, y2, x2+tickLen, y2)
	}

	pdf.SetFont("Courier", "", dimFontPt)
	pdf.SetTextColor(dimStroke[0], dimStroke[1], dimStroke[2])
	midX, midY := (x1+x2)/2, (y1+y2)/2
	if horizontal {
		pdf.Text(labelX, midY+3, text)
	} else {
		pdf.Text(midX+1, midY, text)
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
