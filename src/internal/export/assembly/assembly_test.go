package assembly

import (
	"bytes"
	"fmt"
	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/export/isometric"
	"github.com/go-pdf/fpdf"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func TestExportProducesAPDF(t *testing.T) {
	box := pack.BoxResult{
		BoxName:   "Test Box",
		InteriorW: 300, InteriorD: 200, InteriorH: 60,
		Compartments: []pack.CompartmentResult{
			{ID: "c1", Name: "Chess", Kind: "group", Bounds: pack.Rect{X: 0, Y: 0, W: 150, D: 200}},
		},
	}
	panels := []geometry.Panel{
		{
			ID: "h-1", Axis: geometry.AxisWidthRun,
			Length: 300, Height: 60, Thickness: 3,
			Outline:  geometry.BuildOutline(300, 60, nil),
			Position: geometry.Placement3D{},
		},
		{
			ID: "v-1", Axis: geometry.AxisDepthRun,
			Length: 200, Height: 60, Thickness: 3,
			Notches:  []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}},
			Outline:  geometry.BuildOutline(200, 60, []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}}),
			Position: geometry.Placement3D{OriginX: 150},
		},
	}
	mat := manifest.Material{Name: "Foamboard", Thickness: 3, Kerf: 0.1}

	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, box, panels, mat); err != nil {
		t.Fatalf("Export: %v", err)
	}

	data := buf.Bytes()
	if !bytes.HasPrefix(data, []byte("%PDF-1.")) {
		t.Fatalf("expected output to start with a PDF magic header, got: %q", data[:min(20, len(data))])
	}
	trimmed := bytes.TrimRight(data, "\n\r")
	if !bytes.HasSuffix(trimmed, []byte("%%EOF")) {
		t.Fatalf("expected output to end with %%%%EOF, got: %q", trimmed[max(0, len(trimmed)-20):])
	}
	if len(data) < 500 {
		t.Fatalf("expected a non-trivial PDF, got only %d bytes", len(data))
	}
}

func TestExportEmptyPanelsStillProducesAValidPDF(t *testing.T) {
	var buf bytes.Buffer
	err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Empty", InteriorW: 100, InteriorD: 100, InteriorH: 30}, nil, manifest.Material{Thickness: 3})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-1.")) {
		t.Fatal("expected a valid PDF header even with zero panels")
	}
}

func TestPieceListPaginatesAndRepeatsHeader(t *testing.T) {
	var panels []geometry.Panel
	for i := 0; i < 90; i++ {
		// Distinct lengths so each panel is its own group; per-panel thickness.
		panels = append(panels, geometry.Panel{
			ID: fmt.Sprintf("p-%d", i), Axis: geometry.AxisWidthRun,
			Length: float64(100 + i), Height: 40, Thickness: 4.5,
			Outline: geometry.BuildOutline(float64(100+i), 40, nil),
		})
	}
	pdf := newPDFDoc(fpdf.New("P", "mm", "A4", ""))
	pageW, pageH := pdf.GetPageSize()
	drawPieceList(pdf, export.GroupPanels(panels), pageW, pageH)

	if pdf.PageCount() < 2 {
		t.Fatalf("expected the piece list to span multiple pages, got %d", pdf.PageCount())
	}
	if pdf.Error() != nil {
		t.Fatalf("pdf error: %v", pdf.Error())
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("Output: %v", err)
	}
	if got := bytes.Count(buf.Bytes(), []byte("/Type /Page\n")); got != pdf.PageCount() {
		t.Fatalf("expected %d page objects, found %d", pdf.PageCount(), got)
	}
}

func TestExportZeroSizeBoxDoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Zero"}, nil, manifest.Material{Thickness: 3})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-1.")) {
		t.Fatal("expected a PDF header")
	}
}

func TestExportContentHasPagesAndGroupInfo(t *testing.T) {
	box := pack.BoxResult{BoxName: "Content Box", InteriorW: 300, InteriorD: 200, InteriorH: 60}
	var panels []geometry.Panel
	for i := 0; i < 3; i++ {
		panels = append(panels, geometry.Panel{
			ID: fmt.Sprintf("h-%d", i), Axis: geometry.AxisWidthRun,
			Length: 300, Height: 60, Thickness: 3,
			Outline:  geometry.BuildOutline(300, 60, nil),
			Position: geometry.Placement3D{OriginY: float64(i * 50)},
		})
	}
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, box, panels, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	data := buf.Bytes()
	if !bytes.HasSuffix(bytes.TrimRight(data, "\r\n"), []byte("%%EOF")) {
		t.Fatal("expected trailing EOF marker")
	}
	// 2 isometric pages + piece list + at least one piece drawing page.
	if got := bytes.Count(data, []byte("/Type /Page\n")); got < 3 {
		t.Fatalf("expected at least 3 pages, got %d", got)
	}

	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available; skipping text content checks")
	}
	path := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("pdftotext", "-layout", path, "-").Output()
	if err != nil {
		t.Skipf("pdftotext failed: %v", err)
	}
	text := string(out)
	for _, want := range []string{"Content Box", "Foamboard"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected extracted text to contain %q", want)
		}
	}
}

func determinismFixture() (pack.BoxResult, []geometry.Panel, manifest.Material) {
	box := pack.BoxResult{BoxName: "Det", InteriorW: 300, InteriorD: 200, InteriorH: 60}
	panels := []geometry.Panel{
		{ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Thickness: 3,
			Outline: geometry.BuildOutline(300, 60, nil)},
		{ID: "v-1", Axis: geometry.AxisDepthRun, Length: 200, Height: 60, Thickness: 3,
			Notches:  []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}},
			Outline:  geometry.BuildOutline(200, 60, []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}}),
			Position: geometry.Placement3D{OriginX: 150}},
	}
	return box, panels, manifest.Material{Name: "Foamboard", Thickness: 3, Kerf: 0.1}
}

func TestExportDeterministicWithFixedClock(t *testing.T) {
	box, panels, mat := determinismFixture()
	fixed := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	e := Exporter{Now: func() time.Time { return fixed }}
	var first []byte
	for i := 0; i < 20; i++ {
		var buf bytes.Buffer
		if err := e.Export(&buf, box, panels, mat); err != nil {
			t.Fatalf("Export: %v", err)
		}
		if i == 0 {
			first = buf.Bytes()
			if !bytes.Contains(first, []byte("D:20240102030405")) {
				t.Errorf("fixed clock not embedded in PDF")
			}
			continue
		}
		if !bytes.Equal(first, buf.Bytes()) {
			t.Fatalf("run %d differs from run 0", i)
		}
	}
}

func TestExportNilNowStillWorks(t *testing.T) {
	box, panels, mat := determinismFixture()
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, box, panels, mat); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/CreationDate (D:")) {
		t.Errorf("expected a creation date in output")
	}
}

func TestPieceLayoutStaysWithinCell(t *testing.T) {
	const cellW, cellH = 88.0, 70.0
	notches := func(l float64) []geometry.Notch {
		return []geometry.Notch{
			{Pos: 0, Width: 3, Edge: geometry.EdgeBottom},
			{Pos: l - 3, Width: 3, Edge: geometry.EdgeTop},
		}
	}
	sizes := [][2]float64{{10, 10}, {30, 20}, {20, 30}, {60, 40}, {200, 60}, {300, 60}, {1000, 20}, {20, 1000}, {5, 5}, {400, 400}}
	for _, sz := range sizes {
		for _, withNotches := range []bool{false, true} {
			l, h := sz[0], sz[1]
			var ns []geometry.Notch
			if withNotches && l > 8 {
				ns = notches(l)
			}
			p := geometry.Panel{ID: "x", Length: l, Height: h, Thickness: 3, Notches: ns, Outline: geometry.BuildOutline(l, h, ns)}
			x0, y0 := 15.0, 40.0
			lay := pieceLayout(p, x0, y0, cellW, cellH)
			if lay.scale <= 0 {
				t.Fatalf("%gx%g: non-positive scale %g", l, h, lay.scale)
			}
			if used := math.Max(l*lay.scale/cellW, h*lay.scale/cellH); used < 0.4 {
				t.Errorf("%gx%g: outline uses only %.0f%% of the cell (scale %g)", l, h, used*100, lay.scale)
			}
			if !lay.bounds.within(x0, y0, cellW, cellH) {
				t.Errorf("%gx%g notches=%v: bounds %+v outside cell (%g,%g,%g,%g), scale %g",
					l, h, withNotches, lay.bounds, x0, y0, cellW, cellH, lay.scale)
			}
		}
	}
}

func TestPieceLayoutSmallPanelRegression(t *testing.T) {
	p := geometry.Panel{ID: "s", Length: 30, Height: 20, Thickness: 3, Outline: geometry.BuildOutline(30, 20, nil)}
	lay := pieceLayout(p, 0, 0, 88, 70)
	if lay.bounds.maxX > 88 || lay.bounds.maxY > 70 || lay.bounds.minX < 0 {
		t.Fatalf("small panel overflows cell: %+v", lay.bounds)
	}
	if lay.lengthY > 70 {
		t.Fatalf("L dimension at %g beyond cell height", lay.lengthY)
	}
}

func TestExportSmallPanelsProducesAValidPDF(t *testing.T) {
	box := pack.BoxResult{BoxName: "Small", InteriorW: 100, InteriorD: 100, InteriorH: 30}
	var panels []geometry.Panel
	for i, sz := range [][2]float64{{10, 10}, {30, 20}, {1000, 20}, {20, 200}} {
		panels = append(panels, geometry.Panel{
			ID: fmt.Sprintf("s-%d", i), Axis: geometry.AxisWidthRun,
			Length: sz[0], Height: sz[1], Thickness: 3,
			Outline: geometry.BuildOutline(sz[0], sz[1], nil),
		})
	}
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, box, panels, manifest.Material{Thickness: 3}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-1.")) {
		t.Fatal("expected a PDF header")
	}
}

func TestPDFEncodeNonASCII(t *testing.T) {
	d := newPDFDoc(fpdf.New("P", "mm", "A4", ""))
	d.SetFont("Courier", "", 9)
	cases := map[string]string{
		"plain":            "plain",
		"Café":             "Caf\xe9",
		"a — b":            "a \x97 b",
		"smile \U0001F600": "smile ?",
		"日本":               "??",
		"bad\xffutf8":      "bad?utf8",
		"dot.":             "dot.",
	}
	for in, want := range cases {
		if got := d.encode(in); got != want {
			t.Errorf("encode(%q) = %q, want %q", in, got, want)
		}
	}
	if d.GetStringWidth("Café") != d.GetStringWidth("Cafe") {
		t.Error("width of translated string should match a 4-glyph string")
	}
}

func TestExportNonASCIINames(t *testing.T) {
	name := "Café — naïve \U0001F600"
	box := pack.BoxResult{BoxName: name, InteriorW: 60, InteriorD: 40, InteriorH: 20}
	panels := []geometry.Panel{{
		ID: name, Axis: geometry.AxisWidthRun,
		Length: 60, Height: 20, Thickness: 3,
		Outline: geometry.BuildOutline(60, 20, nil),
	}}
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, box, panels, manifest.Material{Name: "Bírch", Thickness: 3}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatal("output is not a PDF")
	}
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not available")
	}
	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "-enc", "UTF-8", path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	text := string(out)
	for _, want := range []string{"Café", "naïve"} {
		if !strings.Contains(text, want) {
			t.Errorf("pdftotext output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "â€") || strings.Contains(text, "Ã©") {
		t.Errorf("mojibake in output:\n%s", text)
	}
}

func TestFitSceneRejectsDegenerateBounds(t *testing.T) {
	inf := math.Inf(1)
	cases := map[string]isometric.Scene{
		"zero width":  {MinX: 5, MaxX: 5, MinY: 0, MaxY: 10},
		"zero height": {MinX: 0, MaxX: 10, MinY: 3, MaxY: 3},
		"negative":    {MinX: 10, MaxX: 0, MinY: 0, MaxY: 10},
		"NaN":         {MinX: math.NaN(), MaxX: 10, MinY: 0, MaxY: 10},
		"+Inf":        {MinX: 0, MaxX: inf, MinY: 0, MaxY: 10},
		"empty scene": {MinX: inf, MaxX: -inf, MinY: inf, MaxY: -inf},
	}
	for name, sc := range cases {
		t.Run(name, func(t *testing.T) {
			pdf := newPDFDoc(fpdf.New("P", "mm", "A4", ""))
			pdf.AddPage()
			err := fitScene(pdf, &sc, 15, 15, 180, 267)
			if err == nil || !strings.Contains(err.Error(), "degenerate bounds") {
				t.Fatalf("err = %v, want degenerate bounds error", err)
			}
		})
	}
}

func TestFitSceneAcceptsNormalScene(t *testing.T) {
	pdf := newPDFDoc(fpdf.New("P", "mm", "A4", ""))
	pdf.AddPage()
	sc := isometric.Scene{MinX: -10, MaxX: 90, MinY: 0, MaxY: 50}
	if err := fitScene(pdf, &sc, 15, 15, 180, 267); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHeightReducedPanelTopAndBottomNotches draws a panel trimmed by Cut (full
// height 60, cut 20, reduced height 40, notch depth 30) with notches on the
// top or the bottom edge. Its ticks and labels sit on the reduced outline's
// edges and must stay inside the piece cell, and the PDF must report the
// reduced height and the notch positions.
func TestHeightReducedPanelTopAndBottomNotches(t *testing.T) {
	const cellW, cellH = 88.0, 70.0
	for _, edge := range []geometry.Edge{geometry.EdgeTop, geometry.EdgeBottom} {
		for _, axis := range []geometry.Axis{geometry.AxisWidthRun, geometry.AxisDepthRun} {
			t.Run(string(edge)+"/"+string(axis), func(t *testing.T) {
				p := geometry.Panel{
					ID: "r-1", Axis: axis, Length: 100, Height: 40, Cut: 20, Thickness: 3,
					Notches: []geometry.Notch{{Pos: 0, Width: 3, Edge: edge}, {Pos: 70, Width: 3, Edge: edge}},
				}
				x0, y0 := 15.0, 40.0
				lay := pieceLayout(p, x0, y0, cellW, cellH)
				if lay.scale <= 0 {
					t.Fatalf("non-positive scale %g", lay.scale)
				}
				if !lay.bounds.within(x0, y0, cellW, cellH) {
					t.Errorf("bounds %+v outside cell (%g,%g,%g,%g)", lay.bounds, x0, y0, cellW, cellH)
				}
				// The top edge is the reduced top (Height, not Height+Cut):
				// the outline is drawn Height*scale tall from originY, and a
				// top-edge tick reaches above that edge.
				if got := lay.lengthY - lay.originY - lay.gapS; math.Abs(got-p.Height*lay.scale) > 1e-9 {
					t.Errorf("drawn outline height %g, want Height*scale = %g", got, p.Height*lay.scale)
				}
				if edge == geometry.EdgeTop && lay.bounds.minY >= lay.originY {
					t.Errorf("top-edge notch tick/label missing above the reduced top: minY %g, originY %g", lay.bounds.minY, lay.originY)
				}

				box := pack.BoxResult{BoxName: "Cut Box", InteriorW: 300, InteriorD: 200, InteriorH: 60}
				var buf bytes.Buffer
				if err := (Exporter{}).Export(&buf, box, []geometry.Panel{p}, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
					t.Fatalf("Export: %v", err)
				}
				data := buf.Bytes()
				if !bytes.HasPrefix(data, []byte("%PDF-1.")) {
					t.Fatal("expected a PDF header")
				}
				if _, err := exec.LookPath("pdftotext"); err != nil {
					t.Log("pdftotext not available; skipping text content checks")
					return
				}
				path := filepath.Join(t.TempDir(), "a.pdf")
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
				out, err := exec.Command("pdftotext", "-layout", path, "-").Output()
				if err != nil {
					t.Skipf("pdftotext failed: %v", err)
				}
				for _, want := range []string{"H 40 mm", "D 30 mm", "@0", "@70"} {
					if !strings.Contains(string(out), want) {
						t.Errorf("expected extracted text to contain %q", want)
					}
				}
			})
		}
	}
}

func TestNotchDepthLabel(t *testing.T) {
	notched := geometry.Panel{Length: 100, Height: 40, Cut: 20, Notches: []geometry.Notch{{Pos: 10, Width: 3, Edge: geometry.EdgeTop}}}
	if got := notchDepthLabel(notched); got != "D 30 mm" {
		t.Errorf("reduced panel label = %q, want %q", got, "D 30 mm")
	}
	notched.Cut = 0
	if got := notchDepthLabel(notched); got != "D 20 mm" {
		t.Errorf("uncut panel label = %q, want %q", got, "D 20 mm")
	}
	if got := notchDepthLabel(geometry.Panel{Length: 100, Height: 40}); got != "" {
		t.Errorf("panel without notches got label %q, want none", got)
	}
}

func TestVariantLetters(t *testing.T) {
	n := []geometry.Notch{{Pos: 5, Width: 3, Edge: "bottom"}}
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 100, Height: 50},
		{ID: "b", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Notches: n},
		{ID: "c", Axis: geometry.AxisWidthRun, Length: 200, Height: 50},
		{ID: "d", Axis: geometry.AxisDepthRun, Length: 100, Height: 50},
	}
	got := variantLetters(export.GroupPanels(panels))
	// Order is depth-run first (single, no letter) then the tied width-run pair.
	if len(got) != 4 {
		t.Fatalf("got %v", got)
	}
	if got[0] != "" || got[1] != "A" || got[2] != "B" || got[3] != "" {
		t.Fatalf("unexpected variant letters %v", got)
	}
}

func TestPieceListWithVariantsDoesNotBreak(t *testing.T) {
	n := []geometry.Notch{{Pos: 5, Width: 3, Edge: "bottom"}}
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Thickness: 3},
		{ID: "b", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Thickness: 3, Notches: n},
	}
	pdf := newPDFDoc(fpdf.New("P", "mm", "A4", ""))
	pageW, pageH := pdf.GetPageSize()
	drawPieceList(pdf, export.GroupPanels(panels), pageW, pageH)
	if pdf.Error() != nil || pdf.PageCount() != 1 {
		t.Fatalf("pdf error %v, pages %d", pdf.Error(), pdf.PageCount())
	}
}

// pieceListText renders the piece list of groups and returns its pdftotext
// output, skipping the test when pdftotext is unavailable.
func pieceListText(t *testing.T, groups []export.PanelGroup) (string, int) {
	t.Helper()
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not available")
	}
	pdf := newPDFDoc(fpdf.New("P", "mm", "A4", ""))
	pageW, pageH := pdf.GetPageSize()
	drawPieceList(pdf, groups, pageW, pageH)
	if pdf.Error() != nil {
		t.Fatalf("pdf error: %v", pdf.Error())
	}
	path := filepath.Join(t.TempDir(), "list.pdf")
	if err := pdf.OutputFileAndClose(path); err != nil {
		t.Fatalf("OutputFileAndClose: %v", err)
	}
	out, err := exec.Command(bin, "-layout", path, "-").Output()
	if err != nil {
		t.Skipf("pdftotext failed: %v", err)
	}
	return string(out), pdf.PageCount()
}

func TestPieceListManyGroupsListsEachOnceWithContinuationHeader(t *testing.T) {
	var panels []geometry.Panel
	for i := 0; i < 90; i++ {
		panels = append(panels, geometry.Panel{
			ID: fmt.Sprintf("grp%03d", i), Axis: geometry.AxisWidthRun,
			Length: float64(100 + i), Height: 40, Thickness: 4.5,
			Outline: geometry.BuildOutline(float64(100+i), 40, nil),
		})
	}
	text, pages := pieceListText(t, export.GroupPanels(panels))
	if pages < 2 {
		t.Fatalf("expected 2+ pages, got %d", pages)
	}
	if got := strings.Count(text, "Piece List (cont.)"); got != pages-1 {
		t.Errorf("continuation header count = %d, want %d", got, pages-1)
	}
	if got := strings.Count(text, "Piece List\n"); got != 1 {
		t.Errorf("first-page title count = %d, want 1", got)
	}
	for i := 0; i < 90; i++ {
		id := fmt.Sprintf("grp%03d", i)
		if got := strings.Count(text, id); got != 1 {
			t.Errorf("%s listed %d times, want 1", id, got)
		}
	}
}

func TestPieceListTruncatesLongIDs(t *testing.T) {
	long := "very-long-panel-identifier-that-overflows"
	n := []geometry.Notch{{Pos: 5, Width: 3, Edge: "bottom"}}
	panels := []geometry.Panel{
		{ID: long, Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Thickness: 3},
		{ID: long + "-2", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Thickness: 3, Notches: n},
		{ID: "short", Axis: geometry.AxisDepthRun, Length: 90, Height: 50, Thickness: 3},
	}
	text, _ := pieceListText(t, export.GroupPanels(panels))
	if strings.Contains(text, long) {
		t.Errorf("full long id should not appear:\n%s", text)
	}
	var longRows, shortRows int
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.Contains(line, "very-"):
			longRows++
			f := strings.Fields(line)
			if !strings.Contains(f[0], "...") {
				t.Errorf("truncated id missing ellipsis: %q", line)
			}
			// ID, letter, axis, L, H, T, notches, qty: the id must not
			// have spilled into the Axis column.
			if len(f) != 8 || (f[1] != "A" && f[1] != "B") {
				t.Errorf("unexpected row layout (variant letter lost?): %q", line)
			}
		case strings.Contains(line, "short"):
			shortRows++
			if strings.Contains(line, "...") {
				t.Errorf("short id must not be truncated: %q", line)
			}
		}
	}
	if longRows != 2 || shortRows != 1 {
		t.Errorf("rows: long %d (want 2), short %d (want 1):\n%s", longRows, shortRows, text)
	}
}
