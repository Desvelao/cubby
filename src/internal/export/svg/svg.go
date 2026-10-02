// Package svg renders panel outlines as a real-world-scale SVG cutting
// template, nested onto a virtual sheet.
package svg

import (
	"fmt"
	"html"
	"io"
	"math"
	"strings"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// DefaultSheetWidth is the default virtual sheet width (mm) panels are nested
// onto when no --sheet-width override is given.
const DefaultSheetWidth = export.DefaultSheetWidth

type Exporter struct {
	// SheetWidth is the nesting canvas width in mm. Zero uses DefaultSheetWidth.
	SheetWidth float64
	// NestGap is the spacing in mm between nested panels. Nil uses
	// export.NestGap (5); a pointer so an explicit 0 is honoured.
	NestGap *float64
	// BoxCase draws the box interior (top and front views) as dashed
	// reference rectangles below the nested panels.
	BoxCase bool
}

func (Exporter) Format() string { return "svg" }

func (e Exporter) Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error {
	nest, err := export.NestPanels("svg", panels, e.SheetWidth, export.GapOrDefault(e.NestGap))
	if err != nil {
		return err
	}
	sheetW, placement := nest.SheetWidth, nest.Placement

	sheetHeight := nest.UsedHeight
	if sheetHeight <= 0 {
		sheetHeight = 1
	}
	var caseRects []export.CaseRect
	if e.BoxCase {
		caseRects, sheetHeight = export.CaseRects(box, sheetHeight, export.GapOrDefault(e.NestGap)+10)
		sheetHeight += export.GapOrDefault(e.NestGap)
		sheetW = math.Max(sheetW, box.InteriorW)
	}

	ew := &export.ErrWriter{W: w}
	ew.Printf(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	ew.Printf(`<svg xmlns="http://www.w3.org/2000/svg" width="%gmm" height="%gmm" viewBox="0 0 %g %g">`+"\n",
		sheetW, sheetHeight, sheetW, sheetHeight)
	ew.Printf(`<!-- %s: %d panels, %s, %.2fmm thick -->`+"\n", commentText(box.BoxName), len(panels), commentText(mat.Name), mat.Thickness)

	writeProject(ew, box.Project)

	for _, p := range panels {
		origin, ok := placement[p.ID]
		if !ok {
			continue
		}
		ew.Printf(`<g transform="translate(%g,%g)">`+"\n", origin.X, origin.Y)
		ew.Printf(`  <polygon points="%s" fill="none" stroke="black" stroke-width="0.2"/>`+"\n", pointsAttr(origin.Outline(p)))
		ew.Printf(`  <text x="2" y="12" font-size="6">%s</text>`+"\n", esc(p.ID))
		ew.Println(`</g>`)
	}

	if len(caseRects) > 0 {
		ew.Println(`<g id="box-case" fill="none" stroke="#888" stroke-width="0.3" stroke-dasharray="3,2">`)
		for _, r := range caseRects {
			ew.Printf(`  <rect x="%g" y="%g" width="%g" height="%g"/>`+"\n", r.X, r.Y, r.W, r.H)
			ew.Printf(`  <text x="%g" y="%g" font-size="5" fill="#888" stroke="none">%s</text>`+"\n", r.X+2, r.Y-2, esc(r.Label))
		}
		ew.Println(`</g>`)
	}

	ew.Println(`</svg>`)
	return ew.Err
}

// esc strips characters invalid in XML 1.0 and escapes markup characters.
func esc(s string) string { return html.EscapeString(export.XMLText(s)) }

// writeProject emits <title>, <desc> and a Dublin Core <metadata> block for
// the project fields that are set; it writes nothing when none apply. All
// values are XML-escaped.
func writeProject(ew *export.ErrWriter, p *manifest.Project) {
	if p == nil {
		return
	}
	if p.Name != "" {
		ew.Printf("<title>%s</title>\n", esc(p.Name))
	}
	if p.Description != "" {
		ew.Printf("<desc>%s</desc>\n", esc(p.Description))
	}
	date := p.Updated
	if date == "" {
		date = p.Created
	}
	fields := []struct{ tag, value string }{
		{"dc:title", p.Name},
		{"dc:creator", p.Author},
		{"dc:description", p.Description},
		{"dc:rights", p.License},
		{"dc:identifier", p.URL},
	}
	var b strings.Builder
	for _, f := range fields {
		if f.value != "" {
			fmt.Fprintf(&b, "  <%s>%s</%s>\n", f.tag, esc(f.value), f.tag)
		}
	}
	for _, tag := range p.Tags {
		fmt.Fprintf(&b, "  <dc:subject>%s</dc:subject>\n", esc(tag))
	}
	if date != "" {
		fmt.Fprintf(&b, "  <dc:date>%s</dc:date>\n", esc(date))
	}
	if p.Revision != "" {
		fmt.Fprintf(&b, "  <cubby:revision>%s</cubby:revision>\n", esc(p.Revision))
	}
	if b.Len() == 0 {
		return
	}
	ew.Printf(`<metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:cubby="https://github.com/Desvelao/cubby">`+"\n%s</metadata>\n", b.String())
}

func pointsAttr(pts []geometry.Point2D) string {
	s := ""
	for i, p := range pts {
		if i > 0 {
			s += " "
		}
		s += fmt.Sprintf("%g,%g", p.X, p.Y)
	}
	return s
}

// commentText makes s safe inside an XML comment, where "--" is forbidden and
// the text may not end with "-".
func commentText(s string) string {
	s = export.XMLText(s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "- -")
	}
	return strings.TrimSuffix(s, "-") + strings.Repeat("_", len(s)-len(strings.TrimSuffix(s, "-")))
}
