package svg

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func TestExportProducesWellFormedXML(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60,
			Outline: geometry.BuildOutline(300, 60, nil)},
		{ID: "v-1", Axis: geometry.AxisDepthRun, Length: 145, Height: 60,
			Outline: geometry.BuildOutline(145, 60, []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}})},
	}

	var buf bytes.Buffer
	err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Test"}, panels, manifest.Material{Name: "Foamboard", Thickness: 3})
	if err != nil {
		t.Fatal(err)
	}

	dec := xml.NewDecoder(&buf)
	for {
		_, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("output is not well-formed XML: %v", err)
		}
	}
}

func TestExportErrorsOnPanelWiderThanSheet(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Outline: geometry.BuildOutline(300, 60, nil)},
		{ID: "big-1", Axis: geometry.AxisWidthRun, Length: 700, Height: 650, Outline: geometry.BuildOutline(700, 650, nil)},
	}
	var buf bytes.Buffer
	err := (Exporter{}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{Name: "Foamboard", Thickness: 3})
	if err == nil {
		t.Fatal("expected error for panel wider than sheet")
	}
	for _, want := range []string{"big-1", "700x650", "600", "--sheet-width to at least 655mm"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "h-1") {
		t.Errorf("error %q should not name panels that fit", err)
	}

	buf.Reset()
	if err := (Exporter{SheetWidth: 800}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
		t.Fatalf("expected success with wider sheet: %v", err)
	}
}

func TestExportRotatesPanelWiderThanSheet(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "long-1", Axis: geometry.AxisWidthRun, Length: 700, Height: 60, Outline: geometry.BuildOutline(700, 60, nil)},
	}
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
		t.Fatalf("expected rotated panel to export: %v", err)
	}
}

func TestExportEscapesUserNames(t *testing.T) {
	id := "a&b<c>d--e-"
	panels := []geometry.Panel{
		{ID: id, Axis: geometry.AxisWidthRun, Length: 100, Height: 50,
			Outline: geometry.BuildOutline(100, 50, nil)},
	}

	var buf bytes.Buffer
	err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "x--y<&>-"}, panels, manifest.Material{Name: "m--&", Thickness: 3})
	if err != nil {
		t.Fatal(err)
	}

	var text string
	dec := xml.NewDecoder(&buf)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("output is not well-formed XML: %v", err)
		}
		if cd, ok := tok.(xml.CharData); ok && strings.TrimSpace(string(cd)) != "" {
			text = string(cd)
		}
	}
	if text != id {
		t.Errorf("label = %q, want %q", text, id)
	}
}

func TestExportEmptyPanelList(t *testing.T) {
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Test"}, nil, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "0 panels") || strings.Contains(out, "<polygon") {
		t.Errorf("unexpected output for empty panel list:\n%s", out)
	}
	dec := xml.NewDecoder(strings.NewReader(out))
	for {
		if _, err := dec.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("not well-formed XML: %v", err)
		}
	}
}

func TestExportSheetWidthChangesCanvas(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Outline: geometry.BuildOutline(300, 60, nil)},
		{ID: "b", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Outline: geometry.BuildOutline(300, 60, nil)},
	}
	mat := manifest.Material{Name: "Foamboard", Thickness: 3}
	render := func(e Exporter) string {
		var buf bytes.Buffer
		if err := e.Export(&buf, pack.BoxResult{}, panels, mat); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	def, wide := render(Exporter{}), render(Exporter{SheetWidth: 800})
	if !strings.Contains(def, `width="600mm"`) || !strings.Contains(def, `viewBox="0 0 600 `) {
		t.Errorf("default canvas should be 600 wide:\n%s", def)
	}
	if !strings.Contains(wide, `width="800mm"`) || !strings.Contains(wide, `viewBox="0 0 800 `) {
		t.Errorf("override canvas should be 800 wide:\n%s", wide)
	}
	// Wider sheet lets both panels share a row, so the canvas gets shorter.
	if def == wide {
		t.Error("SheetWidth override had no effect")
	}
	if !strings.Contains(wide, "translate(307.5,2.5)") {
		t.Errorf("wide sheet should place panels side by side:\n%s", wide)
	}
	if strings.Contains(def, "translate(307.5,") {
		t.Errorf("default sheet should not fit both panels on one row:\n%s", def)
	}
}

func TestExportNestGapChangesPlacement(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Outline: geometry.BuildOutline(100, 60, nil)},
		{ID: "b", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Outline: geometry.BuildOutline(100, 60, nil)},
	}
	render := func(e Exporter) string {
		var buf bytes.Buffer
		if err := e.Export(&buf, pack.BoxResult{}, panels, manifest.Material{}); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	gap := func(g float64) *float64 { return &g }
	def := render(Exporter{})
	if def != render(Exporter{NestGap: gap(5)}) {
		t.Error("explicit gap 5 must equal the default")
	}
	// second panel origin x = gap/2 + 100 + gap
	for g, want := range map[float64]string{0: "translate(100,0)", 5: "translate(107.5,2.5)", 20: "translate(130,10)"} {
		if out := render(Exporter{NestGap: gap(g)}); !strings.Contains(out, want) {
			t.Errorf("gap %g: missing %s in:\n%s", g, want, out)
		}
	}
}

// failAfter fails once more than n bytes have been written.
type failAfter struct {
	n   int
	err error
}

func (f *failAfter) Write(p []byte) (int, error) {
	if f.n < len(p) {
		return 0, f.err
	}
	f.n -= len(p)
	return len(p), nil
}

func TestExportPropagatesWriteErrors(t *testing.T) {
	boom := errors.New("boom")
	panels := []geometry.Panel{
		{ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Outline: geometry.BuildOutline(300, 60, nil)},
		{ID: "v-1", Axis: geometry.AxisDepthRun, Length: 145, Height: 60,
			Outline: geometry.BuildOutline(145, 60, []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}})},
	}
	mat := manifest.Material{Name: "Foamboard", Thickness: 3}
	var full bytes.Buffer
	if err := (Exporter{}).Export(&full, pack.BoxResult{}, panels, mat); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < full.Len(); n += 13 {
		err := (Exporter{}).Export(&failAfter{n: n, err: boom}, pack.BoxResult{}, panels, mat)
		if !errors.Is(err, boom) {
			t.Fatalf("budget %d: expected boom, got %v", n, err)
		}
	}
}

func TestExportProjectMetadata(t *testing.T) {
	p := manifest.Project{
		Name:        `Insert <"A"> & 'B' — 東`,
		Description: "Line one & <two>\nline \"three\" " + strings.Repeat("long ", 80),
		Revision:    "1.2.0-rc&1",
		Author:      "José Ñandú — 東",
		License:     "CC-BY-4.0",
		URL:         "https://example.com/x?a=1&b=2",
		Tags:        []string{"a&b", "<c>", "東"},
		Created:     "2026-09-01",
		Updated:     "2026-09-30",
	}
	panels := []geometry.Panel{{ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60,
		Outline: geometry.BuildOutline(300, 60, nil)}}
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Box", Project: &p}, panels, manifest.Material{Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	// Parse it back and collect the text of every element by name.
	got := map[string][]string{}
	var stack []string
	dec := xml.NewDecoder(strings.NewReader(out))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("not well-formed XML: %v\n%s", err, out)
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			stack = append(stack, tk.Name.Space+" "+tk.Name.Local)
			got[stack[len(stack)-1]] = append(got[stack[len(stack)-1]], "")
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				k := stack[len(stack)-1]
				got[k][len(got[k])-1] += string(tk)
			}
		}
	}
	const dc, sg, svgNS = "http://purl.org/dc/elements/1.1/", "https://github.com/Desvelao/cubby", "http://www.w3.org/2000/svg"
	want := map[string][]string{
		svgNS + " title":    {p.Name},
		svgNS + " desc":     {p.Description},
		dc + " title":       {p.Name},
		dc + " creator":     {p.Author},
		dc + " description": {p.Description},
		dc + " rights":      {p.License},
		dc + " identifier":  {p.URL},
		dc + " subject":     p.Tags,
		dc + " date":        {p.Updated},
		sg + " revision":    {p.Revision},
	}
	for k, w := range want {
		if strings.Join(got[k], "|") != strings.Join(w, "|") {
			t.Errorf("%s = %q, want %q", k, got[k], w)
		}
	}

	// Panels stay after the metadata and keep their structure.
	if i, j := strings.Index(out, "</metadata>"), strings.Index(out, `<g transform`); i < 0 || j < i {
		t.Errorf("metadata must precede the panels")
	}
}

func TestExportProjectAbsentOrUnrelatedAddsNothing(t *testing.T) {
	render := func(p *manifest.Project) string {
		var buf bytes.Buffer
		if err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Box", Project: p}, nil, manifest.Material{Thickness: 3}); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	base := render(nil)
	for _, p := range []*manifest.Project{{}, {Game: "G", Publisher: "P", Notes: "n", Contact: "c@d.io"}} {
		if got := render(p); got != base {
			t.Errorf("project %+v changed output:\n%s", p, got)
		}
	}
	if strings.Contains(base, "<title>") || strings.Contains(base, "<metadata") {
		t.Error("baseline has metadata elements")
	}
}

// TestExportHeightReducedPanelTopAndBottomNotches exports a panel trimmed by
// Cut (full height 60, cut 20, reduced height 40, notch depth 30) with notches
// on the top or bottom edge, both as drawn and rotated onto a narrow sheet.
// The polygon must span the reduced height, reach the full-height notch depth,
// and match the trimmed outline's area.
func TestExportHeightReducedPanelTopAndBottomNotches(t *testing.T) {
	polyRe := regexp.MustCompile(`<polygon points="([^"]*)"`)
	for _, edge := range []geometry.Edge{geometry.EdgeTop, geometry.EdgeBottom} {
		for _, sc := range []struct {
			name       string
			sheetWidth float64
			rotated    bool
		}{
			{"as drawn", 600, false},
			{"rotated", 80, true},
		} {
			t.Run(string(edge)+"/"+sc.name, func(t *testing.T) {
				p := geometry.Panel{
					ID: "r-1", Axis: geometry.AxisWidthRun, Length: 100, Height: 40, Cut: 20, Thickness: 3,
					Notches: []geometry.Notch{{Pos: 20, Width: 3, Edge: edge}, {Pos: 70, Width: 3, Edge: edge}},
				}
				var buf bytes.Buffer
				if err := (Exporter{SheetWidth: sc.sheetWidth}).Export(&buf, pack.BoxResult{}, []geometry.Panel{p}, manifest.Material{Thickness: 3}); err != nil {
					t.Fatal(err)
				}
				m := polyRe.FindStringSubmatch(buf.String())
				if m == nil {
					t.Fatalf("no polygon in output:\n%s", buf.String())
				}
				var pts []geometry.Point2D
				for _, f := range strings.Fields(m[1]) {
					var pt geometry.Point2D
					if _, err := fmt.Sscanf(f, "%g,%g", &pt.X, &pt.Y); err != nil {
						t.Fatalf("bad point %q: %v", f, err)
					}
					pts = append(pts, pt)
				}
				// Rotated, the footprint is height x length.
				w, d := 100.0, 40.0
				if sc.rotated {
					w, d = d, w
				}
				var maxX, maxY float64
				for _, pt := range pts {
					maxX, maxY = max(maxX, pt.X), max(maxY, pt.Y)
				}
				if maxX != w || maxY != d {
					t.Errorf("polygon extent %gx%g, want %gx%g", maxX, maxY, w, d)
				}
				// Notch depth 30 is the full-height depth, measured from the
				// bottom edge (rotation maps y to Height-x).
				depth := 30.0
				found := false
				for _, pt := range pts {
					v := pt.Y
					if sc.rotated {
						v = 40 - pt.X
					}
					if v == depth {
						found = true
					}
				}
				if !found {
					t.Errorf("no vertex at full-height notch depth %g in %v", depth, pts)
				}
				var area float64
				for i, a := range pts {
					b := pts[(i+1)%len(pts)]
					area += a.X*b.Y - b.X*a.Y
				}
				notch := 3.0 * 30
				if edge == geometry.EdgeTop {
					notch = 3 * 10
				}
				if want := 100*40 - 2*notch; area/2 != want {
					t.Errorf("polygon area = %g, want %g", area/2, want)
				}
			})
		}
	}
}

func TestExportStripsInvalidXMLCharacters(t *testing.T) {
	bad := "a\x00b\x01c\x1bd\x0be\x7ff\xffg"
	panels := []geometry.Panel{
		{ID: "id" + bad, Axis: geometry.AxisWidthRun, Length: 100, Height: 50,
			Outline: geometry.BuildOutline(100, 50, nil)},
	}
	box := pack.BoxResult{BoxName: "box" + bad, Project: &manifest.Project{
		Name: "n" + bad, Description: "d" + bad, Author: "a" + bad, License: "l" + bad,
		URL: "u" + bad, Tags: []string{"t" + bad}, Revision: "r" + bad, Updated: "2024" + bad,
	}}

	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, box, panels, manifest.Material{Name: "m" + bad, Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	dec := xml.NewDecoder(bytes.NewReader(buf.Bytes()))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("output is not well-formed XML: %v", err)
		}
	}
	if !strings.Contains(buf.String(), "<title>na") {
		t.Errorf("title missing or malformed:\n%s", buf.String())
	}
}
