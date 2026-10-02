package isometric

import (
	"fmt"
	"io"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// SVGExporter renders the assembled insert as an annotated isometric SVG
// drawing. When Exploded is true, panels are separated along their thin axis
// to show each piece individually.
type SVGExporter struct {
	Exploded bool
	// BoxCase outlines the box interior as a wireframe.
	BoxCase bool
}

func (e SVGExporter) Format() string {
	if e.Exploded {
		return "iso-svg-exploded"
	}
	return "iso-svg"
}

const svgMargin = 12.0

func (e SVGExporter) Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error {
	scene := BuildSceneCase(box, panels, mat, e.Exploded, e.BoxCase)

	width := scene.MaxX - scene.MinX + 2*svgMargin
	height := scene.MaxY - scene.MinY + 2*svgMargin
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	offX, offY := svgMargin-scene.MinX, svgMargin-scene.MinY

	ew := &export.ErrWriter{W: w}
	ew.Printf(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	ew.Printf(`<svg xmlns="http://www.w3.org/2000/svg" width="%g" height="%g" viewBox="0 0 %g %g" font-family="monospace">`+"\n",
		width, height, width, height)
	ew.Printf(`<rect x="0" y="0" width="%g" height="%g" fill="white"/>`+"\n", width, height)
	ew.Printf(`<g transform="translate(%g,%g)">`+"\n", offX, offY)

	for _, p := range scene.Polys {
		ew.Printf(`<polygon points="%s" fill="%s" stroke="%s" stroke-width="0.3"/>`+"\n",
			pointsAttr(p.Points), p.Fill.hex(), p.Stroke.hex())
	}
	for _, l := range scene.Lines {
		dash := ""
		if l.Dashed {
			dash = ` stroke-dasharray="1.5,1.5"`
		}
		ew.Printf(`<line x1="%g" y1="%g" x2="%g" y2="%g" stroke="%s" stroke-width="0.35"%s/>`+"\n",
			l.A.X, l.A.Y, l.B.X, l.B.Y, l.Stroke.hex(), dash)
	}
	for _, lbl := range scene.Labels {
		ew.Printf(`<text x="%g" y="%g" font-size="%g" fill="%s">%s</text>`+"\n",
			lbl.Pos.X, lbl.Pos.Y, lbl.Size, lbl.Color.hex(), xmlEscape(lbl.Text))
	}

	ew.Println(`</g>`)
	ew.Println(`</svg>`)
	return ew.Err
}

func pointsAttr(pts []vec2) string {
	s := ""
	for i, p := range pts {
		if i > 0 {
			s += " "
		}
		s += fmt.Sprintf("%g,%g", p.X, p.Y)
	}
	return s
}

// xmlEscape strips characters invalid in XML 1.0 and escapes markup characters.
func xmlEscape(s string) string {
	out := make([]byte, 0, len(s))
	for _, r := range export.XMLText(s) {
		switch r {
		case '&':
			out = append(out, "&amp;"...)
		case '<':
			out = append(out, "&lt;"...)
		case '>':
			out = append(out, "&gt;"...)
		default:
			out = append(out, string(r)...)
		}
	}
	return string(out)
}
