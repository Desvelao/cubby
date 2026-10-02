package dxf

import (
	"bufio"
	"bytes"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/export/svg"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func TestExportBalancedGroupCodesAndSections(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60,
			Outline: geometry.BuildOutline(300, 60, nil)},
		{ID: "v-1", Axis: geometry.AxisDepthRun, Length: 145, Height: 60,
			Outline: geometry.BuildOutline(145, 60, []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}})},
	}

	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines)%2 != 0 {
		t.Fatalf("expected an even number of lines (group code + value pairs), got %d", len(lines))
	}

	sectionDepth := 0
	polylineDepth := 0
	sc := bufio.NewScanner(strings.NewReader(buf.String()))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var code string
	for sc.Scan() {
		line := sc.Text()
		if code == "" {
			code = line
			continue
		}
		value := line
		n, err := strconv.Atoi(code)
		if err != nil {
			t.Fatalf("non-numeric group code %q", code)
		}
		if n == 0 {
			switch value {
			case "SECTION":
				sectionDepth++
			case "ENDSEC":
				sectionDepth--
			case "POLYLINE":
				polylineDepth++
			case "SEQEND":
				polylineDepth--
			}
		}
		code = ""
	}
	if sectionDepth != 0 {
		t.Fatalf("unbalanced SECTION/ENDSEC: depth %d", sectionDepth)
	}
	if polylineDepth != 0 {
		t.Fatalf("unbalanced POLYLINE/SEQEND: depth %d", polylineDepth)
	}
	if !strings.HasSuffix(strings.TrimRight(buf.String(), "\n"), "EOF") {
		t.Fatal("expected file to end with an EOF entity")
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

func TestExportEmptyPanelList(t *testing.T) {
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, nil, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "POLYLINE") || !strings.Contains(out, "ENTITIES") {
		t.Errorf("expected empty ENTITIES section:\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), "EOF") {
		t.Error("expected file to end with EOF")
	}
}

func TestExportSheetWidthChangesNesting(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Outline: geometry.BuildOutline(300, 60, nil)},
		{ID: "b", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Outline: geometry.BuildOutline(300, 60, nil)},
	}
	maxY := func(e Exporter) float64 {
		var buf bytes.Buffer
		if err := e.Export(&buf, pack.BoxResult{}, panels, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		max := 0.0
		for i := 0; i+1 < len(lines); i += 2 {
			if lines[i] == "20" {
				y, err := strconv.ParseFloat(lines[i+1], 64)
				if err != nil {
					t.Fatal(err)
				}
				if y > max {
					max = y
				}
			}
		}
		return max
	}
	narrow, wide := maxY(Exporter{}), maxY(Exporter{SheetWidth: 800})
	if wide >= narrow {
		t.Errorf("wider sheet should nest both panels on one row (max Y %g) vs default two rows (max Y %g)", wide, narrow)
	}
}

func TestExportHeaderDeclaresMillimetres(t *testing.T) {
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, nil, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	header := out[:strings.Index(out, "ENDSEC")]
	for _, want := range []string{"9\n$INSUNITS\n70\n4\n", "9\n$MEASUREMENT\n70\n1\n"} {
		if !strings.Contains(header, want) {
			t.Errorf("HEADER missing %q:\n%s", want, header)
		}
	}
}

// dxfPolylines returns each POLYLINE's vertices in file order.
func dxfPolylines(t *testing.T, s string) [][][2]float64 {
	t.Helper()
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var out [][][2]float64
	inVertex := false
	for i := 0; i+1 < len(lines); i += 2 {
		code, val := lines[i], lines[i+1]
		if code == "0" {
			inVertex = val == "VERTEX"
		}
		switch {
		case code == "0" && val == "POLYLINE":
			out = append(out, nil)
		case code == "0" && val == "VERTEX":
			out[len(out)-1] = append(out[len(out)-1], [2]float64{})
		case inVertex && (code == "10" || code == "20"):
			f, err := strconv.ParseFloat(val, 64)
			if err != nil {
				t.Fatal(err)
			}
			v := &out[len(out)-1][len(out[len(out)-1])-1]
			if code == "10" {
				v[0] = f
			} else {
				v[1] = f
			}
		}
	}
	return out
}

// TestExportMatchesSVGWithYFlipped checks every DXF vertex equals the SVG
// vertex (translated by its group offset) with Y mirrored about the sheet
// height, and that flipped panels do not overlap.
func TestExportMatchesSVGWithYFlipped(t *testing.T) {
	notched := func(l, h float64, edge geometry.Edge) []geometry.Point2D {
		return geometry.BuildOutline(l, h, []geometry.Notch{{Pos: 20, Width: 3, Edge: edge}, {Pos: 60, Width: 3, Edge: edge}})
	}
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Outline: notched(300, 60, geometry.EdgeBottom)},
		{ID: "b", Axis: geometry.AxisDepthRun, Length: 250, Height: 45, Outline: notched(250, 45, geometry.EdgeTop)},
		{ID: "c", Axis: geometry.AxisWidthRun, Length: 200, Height: 80, Outline: notched(200, 80, geometry.EdgeBottom)},
		{ID: "d", Axis: geometry.AxisDepthRun, Length: 145, Height: 30, Outline: geometry.BuildOutline(145, 30, nil)},
		{ID: "e", Axis: geometry.AxisWidthRun, Length: 700, Height: 60, Outline: notched(700, 60, geometry.EdgeBottom)},
	}
	mat := manifest.Material{Name: "Foamboard", Thickness: 3}
	var dbuf, sbuf bytes.Buffer
	if err := (Exporter{}).Export(&dbuf, pack.BoxResult{}, panels, mat); err != nil {
		t.Fatal(err)
	}
	if err := (svg.Exporter{}).Export(&sbuf, pack.BoxResult{}, panels, mat); err != nil {
		t.Fatal(err)
	}
	svgOut := sbuf.String()
	height := regexp.MustCompile(`viewBox="0 0 \S+ (\S+)"`).FindStringSubmatch(svgOut)
	sheetH, _ := strconv.ParseFloat(height[1], 64)

	groups := regexp.MustCompile(`translate\(([^,]+),([^)]+)\)\">\s*<polygon points="([^"]+)"`).FindAllStringSubmatch(svgOut, -1)
	polys := dxfPolylines(t, dbuf.String())
	if len(groups) != len(panels) || len(polys) != len(panels) {
		t.Fatalf("got %d svg groups, %d dxf polylines, want %d", len(groups), len(polys), len(panels))
	}
	type box struct{ x0, y0, x1, y1 float64 }
	var boxes []box
	for i, g := range groups {
		ox, _ := strconv.ParseFloat(g[1], 64)
		oy, _ := strconv.ParseFloat(g[2], 64)
		pts := strings.Fields(g[3])
		if len(pts) != len(polys[i]) {
			t.Fatalf("panel %d: %d svg points vs %d dxf vertices", i, len(pts), len(polys[i]))
		}
		b := box{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		for j, p := range pts {
			xy := strings.Split(p, ",")
			x, _ := strconv.ParseFloat(xy[0], 64)
			y, _ := strconv.ParseFloat(xy[1], 64)
			wantX, wantY := ox+x, sheetH-(oy+y)
			if math.Abs(polys[i][j][0]-wantX) > 1e-9 || math.Abs(polys[i][j][1]-wantY) > 1e-9 {
				t.Errorf("panel %d vertex %d: dxf %v, want (%g,%g)", i, j, polys[i][j], wantX, wantY)
			}
			b.x0, b.x1 = math.Min(b.x0, polys[i][j][0]), math.Max(b.x1, polys[i][j][0])
			b.y0, b.y1 = math.Min(b.y0, polys[i][j][1]), math.Max(b.y1, polys[i][j][1])
		}
		boxes = append(boxes, b)
	}
	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			a, b := boxes[i], boxes[j]
			if a.x0 < b.x1 && b.x0 < a.x1 && a.y0 < b.y1 && b.y0 < a.y1 {
				t.Errorf("flipped panels %d and %d overlap: %v %v", i, j, a, b)
			}
		}
	}
}

type pair struct{ code, val string }

func dxfPairs(s string) []pair {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var out []pair
	for i := 0; i+1 < len(lines); i += 2 {
		out = append(out, pair{lines[i], lines[i+1]})
	}
	return out
}

// entities groups the pairs after each group-0 marker, keyed by entity type.
func dxfEntities(s, kind string) []map[string]string {
	var out []map[string]string
	var cur map[string]string
	for _, p := range dxfPairs(s) {
		if p.code == "0" {
			cur = nil
			if p.val == kind {
				cur = map[string]string{}
				out = append(out, cur)
			}
			continue
		}
		if cur != nil {
			if _, dup := cur[p.code]; !dup {
				cur[p.code] = p.val
			}
		}
	}
	return out
}

func labelPanels() []geometry.Panel {
	return []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Outline: geometry.BuildOutline(300, 60, nil)},
		{ID: "b", Axis: geometry.AxisDepthRun, Length: 250, Height: 45, Outline: geometry.BuildOutline(250, 45, nil)},
		{ID: "long", Axis: geometry.AxisWidthRun, Length: 700, Height: 60, Outline: geometry.BuildOutline(700, 60, nil)}, // rotated on a 600 sheet
		{ID: "f", Axis: geometry.AxisFloor, Length: 100, Height: 100, Outline: geometry.BuildOutline(100, 100, nil)},
	}
}

func TestExportLayerTableListsUsedLayers(t *testing.T) {
	var buf bytes.Buffer
	mat := manifest.Material{Name: "Foamboard", Thickness: 3}
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, labelPanels(), mat); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Index(out, "TABLES") > strings.Index(out, "ENTITIES") || strings.Index(out, "HEADER") > strings.Index(out, "TABLES") {
		t.Error("TABLES must sit between HEADER and ENTITIES")
	}
	declared := map[string]bool{}
	for _, l := range dxfEntities(out, "LAYER") {
		declared[l["2"]] = true
	}
	want := map[string]bool{"width-run": true, "depth-run": true, "floor": true, "LABELS": true}
	if len(declared) != len(want) {
		t.Errorf("declared layers %v, want %v", declared, want)
	}
	for n := range want {
		if !declared[n] {
			t.Errorf("layer %q not declared", n)
		}
	}
	if !strings.Contains(out, "2\nLAYER\n70\n4\n") {
		t.Errorf("LAYER table count should be 4:\n%s", out)
	}
	// Every layer referenced by an entity is declared, and entity layers are unchanged.
	pairs := dxfPairs(out)
	for i, p := range pairs {
		if p.code == "8" && !declared[p.val] {
			t.Errorf("entity layer %q (pair %d) not declared", p.val, i)
		}
	}
	for _, e := range dxfEntities(out, "POLYLINE") {
		if e["8"] != "width-run" && e["8"] != "depth-run" && e["8"] != "floor" {
			t.Errorf("unexpected outline layer %q", e["8"])
		}
	}
}

func TestExportEmptyDeclaresNoLayers(t *testing.T) {
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, nil, manifest.Material{}); err != nil {
		t.Fatal(err)
	}
	if len(dxfEntities(buf.String(), "LAYER")) != 0 || !strings.Contains(buf.String(), "2\nLAYER\n70\n0\n") {
		t.Errorf("expected empty LAYER table:\n%s", buf.String())
	}
}

// TestExportTextLabelsMatchSVGLabels checks each panel gets a TEXT with its ID
// on LABELS, at the SVG label position (group origin + text x,y) with Y
// flipped, including for a panel the nester rotated.
func TestExportTextLabelsMatchSVGLabels(t *testing.T) {
	panels := labelPanels()
	mat := manifest.Material{Name: "Foamboard", Thickness: 3}
	var dbuf, sbuf bytes.Buffer
	if err := (Exporter{}).Export(&dbuf, pack.BoxResult{}, panels, mat); err != nil {
		t.Fatal(err)
	}
	if err := (svg.Exporter{}).Export(&sbuf, pack.BoxResult{}, panels, mat); err != nil {
		t.Fatal(err)
	}
	svgOut := sbuf.String()
	sheetH, _ := strconv.ParseFloat(regexp.MustCompile(`viewBox="0 0 \S+ (\S+)"`).FindStringSubmatch(svgOut)[1], 64)
	labels := regexp.MustCompile(`translate\(([^,]+),([^)]+)\)">\s*<polygon[^\n]*\n\s*<text x="([^"]+)" y="([^"]+)" font-size="([^"]+)">([^<]*)</text>`).FindAllStringSubmatch(svgOut, -1)
	texts := dxfEntities(dbuf.String(), "TEXT")
	if len(labels) != len(panels) || len(texts) != len(panels) {
		t.Fatalf("got %d svg labels, %d dxf TEXT, want %d", len(labels), len(texts), len(panels))
	}
	f := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	for i, l := range labels {
		tx := texts[i]
		if tx["8"] != "LABELS" || tx["1"] != l[6] || tx["1"] != panels[i].ID {
			t.Errorf("text %d: layer %q value %q, want LABELS/%q", i, tx["8"], tx["1"], panels[i].ID)
		}
		wantX, wantY := f(l[1])+f(l[3]), sheetH-(f(l[2])+f(l[4]))
		if math.Abs(f(tx["10"])-wantX) > 1e-9 || math.Abs(f(tx["20"])-wantY) > 1e-9 {
			t.Errorf("text %d at (%s,%s), want (%g,%g)", i, tx["10"], tx["20"], wantX, wantY)
		}
		// cap height ~0.72 of the SVG font size
		if h, fs := f(tx["40"]), f(l[5]); math.Abs(h/fs-0.72) > 0.01 {
			t.Errorf("text height %g vs svg font-size %g", h, fs)
		}
	}
	// Rotated panel: its label is at the placement origin, not rotated with the outline.
	nest, err := export.NestPanels("dxf", panels, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	pl := nest.Placement["long"]
	if !pl.Rotated {
		t.Fatal("expected 'long' to be rotated")
	}
	tx := texts[2]
	if math.Abs(f(tx["10"])-(pl.X+2)) > 1e-9 || math.Abs(f(tx["20"])-(nest.UsedHeight-(pl.Y+12))) > 1e-9 {
		t.Errorf("rotated panel text at (%s,%s), want (%g,%g)", tx["10"], tx["20"], pl.X+2, nest.UsedHeight-(pl.Y+12))
	}
}

func TestExportTextSanitised(t *testing.T) {
	for in, want := range map[string]string{
		"plain-1":       "plain-1",
		"café 中":        "caf? ?",
		"a\nb\x00c\x7f": "a?b?c?",
		"50% ^J":        "50%%37 ?J",
	} {
		if got := text(in); got != want {
			t.Errorf("text(%q) = %q, want %q", in, got, want)
		}
	}
	if got := text(strings.Repeat("x", 400)); len(got) != maxTextLen {
		t.Errorf("len = %d, want %d", len(got), maxTextLen)
	}
	panels := []geometry.Panel{{ID: "naïve\nid", Axis: geometry.AxisFloor, Length: 10, Height: 10, Outline: geometry.BuildOutline(10, 10, nil)}}
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{}); err != nil {
		t.Fatal(err)
	}
	if got := dxfEntities(buf.String(), "TEXT")[0]["1"]; got != "na?ve?id" {
		t.Errorf("TEXT value %q", got)
	}
	if len(dxfPairs(buf.String()))*2 != len(strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")) {
		t.Error("group code/value pairs broken by label")
	}
}

func TestExportNestGapChangesPlacement(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Outline: geometry.BuildOutline(100, 60, nil)},
		{ID: "b", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Outline: geometry.BuildOutline(100, 60, nil)},
	}
	textX := func(e Exporter) float64 {
		var buf bytes.Buffer
		if err := e.Export(&buf, pack.BoxResult{}, panels, manifest.Material{}); err != nil {
			t.Fatal(err)
		}
		x, _ := strconv.ParseFloat(dxfEntities(buf.String(), "TEXT")[1]["10"], 64)
		return x
	}
	gap := func(g float64) *float64 { return &g }
	def, five, twenty, zero := textX(Exporter{}), textX(Exporter{NestGap: gap(5)}), textX(Exporter{NestGap: gap(20)}), textX(Exporter{NestGap: gap(0)})
	if def != five {
		t.Errorf("default gap changed: %g vs %g", def, five)
	}
	if twenty-five != 22.5 || five-zero != 7.5 {
		t.Errorf("second panel x: gap0=%g gap5=%g gap20=%g", zero, five, twenty)
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
