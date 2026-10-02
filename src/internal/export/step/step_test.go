package step

import (
	"bytes"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func samplePanels() []geometry.Panel {
	return []geometry.Panel{
		{
			ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Thickness: 3,
			Notches:  []geometry.Notch{{Pos: 92, Width: 3, Edge: "top"}},
			Position: geometry.Placement3D{OriginX: 0, OriginY: 142, OriginZ: 0},
		},
		{
			ID: "v-1", Axis: geometry.AxisDepthRun, Length: 145, Height: 60, Thickness: 3,
			Notches:  []geometry.Notch{{Pos: 0, Width: 3, Edge: "bottom"}},
			Position: geometry.Placement3D{OriginX: 95, OriginY: 0, OriginZ: 0},
		},
	}
}

var defRe = regexp.MustCompile(`(?m)^#(\d+)=`)
var refRe = regexp.MustCompile(`#(\d+)`)

func TestExportSelfConsistentReferences(t *testing.T) {
	var buf bytes.Buffer
	err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Test Box"}, samplePanels(), manifest.Material{Thickness: 3, Kerf: 0})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.HasPrefix(out, "ISO-10303-21;\n") {
		t.Fatal("expected file to start with the ISO-10303-21 header")
	}
	if !strings.Contains(out, "END-ISO-10303-21;") {
		t.Fatal("expected file to end with END-ISO-10303-21;")
	}

	defined := map[string]bool{}
	for _, m := range defRe.FindAllStringSubmatch(out, -1) {
		if defined[m[1]] {
			t.Fatalf("entity #%s defined more than once", m[1])
		}
		defined[m[1]] = true
	}
	if len(defined) == 0 {
		t.Fatal("expected at least one defined entity")
	}

	for _, m := range refRe.FindAllStringSubmatch(out, -1) {
		if !defined[m[1]] {
			t.Fatalf("reference to undefined entity #%s", m[1])
		}
	}
}

var (
	edgeCurveRe = regexp.MustCompile(`(?m)^#(\d+)=EDGE_CURVE\('',#(\d+),#(\d+),#\d+,\.[TF]\.\);$`)
	orientedRe  = regexp.MustCompile(`(?m)^#(\d+)=ORIENTED_EDGE\('',\*,\*,#(\d+),\.([TF])\.\);$`)
	loopRe      = regexp.MustCompile(`(?m)^#\d+=EDGE_LOOP\('',\(([^)]*)\)\);$`)
	lineRe      = regexp.MustCompile(`(?m)^#\d+=LINE\(`)
	vertexRe    = regexp.MustCompile(`(?m)^#\d+=VERTEX_POINT\(`)
)

func TestExportShellIsTopologicallyClosed(t *testing.T) {
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Test Box"}, samplePanels(), manifest.Material{Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	type edge struct{ start, end string }
	edges := map[string]edge{}
	for _, m := range edgeCurveRe.FindAllStringSubmatch(out, -1) {
		edges[m[1]] = edge{m[2], m[3]}
	}
	oriented := map[string]struct {
		edge string
		fwd  bool
	}{}
	uses := map[string][]bool{}
	for _, m := range orientedRe.FindAllStringSubmatch(out, -1) {
		fwd := m[3] == "T"
		oriented[m[1]] = struct {
			edge string
			fwd  bool
		}{m[2], fwd}
		uses[m[2]] = append(uses[m[2]], fwd)
	}

	// One prism per panel with N outline vertices: 3N edges/lines, 2N vertices.
	panels := samplePanels()
	var wantEdges, wantVerts, wantLoops int
	for _, p := range panels {
		n := len(p.OutlinePolygon())
		wantEdges += 3 * n
		wantVerts += 2 * n
		wantLoops += n + 2
	}
	if bodies := strings.Count(out, "MANIFOLD_SOLID_BREP("); bodies != len(panels) {
		t.Fatalf("want one MANIFOLD_SOLID_BREP per panel (%d), got %d", len(panels), bodies)
	}
	if len(edges) != wantEdges || len(lineRe.FindAllString(out, -1)) != wantEdges || len(vertexRe.FindAllString(out, -1)) != wantVerts {
		t.Fatalf("want %d edges/lines and %d vertices, got %d edges", wantEdges, wantVerts, len(edges))
	}

	for id := range edges {
		u := uses[id]
		if len(u) != 2 || u[0] == u[1] {
			t.Fatalf("EDGE_CURVE #%s must be used by exactly two ORIENTED_EDGEs with opposite orientation, got %v", id, u)
		}
	}
	for _, u := range oriented {
		if _, ok := edges[u.edge]; !ok {
			t.Fatalf("ORIENTED_EDGE references non-EDGE_CURVE #%s", u.edge)
		}
	}

	loops := loopRe.FindAllStringSubmatch(out, -1)
	if len(loops) != wantLoops {
		t.Fatalf("want %d loops, got %d", wantLoops, len(loops))
	}
	for _, l := range loops {
		ids := refRe.FindAllStringSubmatch(l[1], -1)
		if len(ids) < 4 {
			t.Fatalf("loop should have at least 4 edges: %s", l[0])
		}
		var first, prevEnd string
		for i, m := range ids {
			o := oriented[m[1]]
			s, e := edges[o.edge].start, edges[o.edge].end
			if !o.fwd {
				s, e = e, s
			}
			if i == 0 {
				first = s
			} else if s != prevEnd {
				t.Fatalf("loop not head-to-tail: %s", l[0])
			}
			prevEnd = e
		}
		if prevEnd != first {
			t.Fatalf("loop not closed: %s", l[0])
		}
	}
}

func TestExportOneNamedSolidPerPanel(t *testing.T) {
	panels := samplePanels()
	panels[1].ID = "v'1 é"
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Test Box"}, panels, manifest.Material{Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	solidRe := regexp.MustCompile(`(?m)^#\d+=MANIFOLD_SOLID_BREP\('([^\n]*)',#\d+\);$`)
	ms := solidRe.FindAllStringSubmatch(buf.String(), -1)
	if len(ms) != len(panels) {
		t.Fatalf("want %d solids, got %d", len(panels), len(ms))
	}
	for i, p := range panels {
		if ms[i][1] != escape(p.ID) {
			t.Errorf("solid %d named %q, want %q", i, ms[i][1], escape(p.ID))
		}
	}
	// Each solid is a single closed shell with N+2 faces.
	shellRe := regexp.MustCompile(`(?m)^#\d+=CLOSED_SHELL\('(?:[^']|'')*',\(([^)]*)\)\);$`)
	shells := shellRe.FindAllStringSubmatch(buf.String(), -1)
	if len(shells) != len(panels) {
		t.Fatalf("want %d shells, got %d", len(panels), len(shells))
	}
	for i, p := range panels {
		if got, want := len(refRe.FindAllString(shells[i][1], -1)), len(p.OutlinePolygon())+2; got != want {
			t.Errorf("panel %s: %d faces, want %d", p.ID, got, want)
		}
	}
}

func TestEscape(t *testing.T) {
	cases := map[string]string{
		"plain":      "plain",
		"it's":       "it''s",
		`a\b`:        `a\\b`,
		"Café":       `Caf\X2\00E9\X0\`,
		"a\u2014b":   `a\X2\2014\X0\b`,
		"\U0001F600": `\X2\D83DDE00\X0\`,
		"é—x":        `\X2\00E92014\X0\x`,
		"a\nb\x01c":  "a bc",
	}
	for in, want := range cases {
		if got := escape(in); got != want {
			t.Errorf("escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExportDeterministicWithInjectedTime(t *testing.T) {
	fixed := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	exp := Exporter{Now: func() time.Time { return fixed }}
	box := pack.BoxResult{BoxName: "Café — it's"}
	var a, b bytes.Buffer
	for _, buf := range []*bytes.Buffer{&a, &b} {
		if err := exp.Export(buf, box, samplePanels(), manifest.Material{Thickness: 3}); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("exports differ despite fixed time")
	}
	out := a.String()
	if !strings.Contains(out, "2024-01-02T03:04:05") {
		t.Error("fixed timestamp missing")
	}
	if !strings.Contains(out, `'Caf\X2\00E9\X0\ \X2\2014\X0\ it''s insert'`) {
		t.Errorf("escaped name missing:\n%s", out[:300])
	}
	for _, r := range out {
		if r > 0x7e {
			t.Fatalf("non-ASCII rune %q in output", r)
		}
	}
}

func TestFormatReal(t *testing.T) {
	negZero := math.Copysign(0, -1)
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0."}, {negZero, "0."}, {3, "3."}, {-1, "-1."}, {0.5, "0.5"},
		{-0.25, "-0.25"}, {1e-5, "1.E-5"}, {1e-4, "0.0001"}, {1e-7, "1.E-7"}, {-1.5e-7, "-1.5E-7"},
		{1e21, "1.E21"}, {1.5e300, "1.5E300"}, {1e6, "1000000."},
		{123456.789, "123456.789"}, {1e14, "100000000000000."},
	}
	realRe := regexp.MustCompile(`^-?\d+\.\d*(E-?\d+)?$`)
	for _, c := range cases {
		got, err := formatReal(c.in)
		if err != nil {
			t.Errorf("formatReal(%v) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("formatReal(%v) = %q, want %q", c.in, got, c.want)
		}
		if !realRe.MatchString(got) {
			t.Errorf("formatReal(%v) = %q is not a valid REAL", c.in, got)
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := formatReal(v); err == nil {
			t.Errorf("formatReal(%v): expected error", v)
		}
	}
}

func TestExportRejectsNonFinite(t *testing.T) {
	panels := samplePanels()
	panels[0].Position.OriginX = math.NaN()
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "x"}, panels, manifest.Material{Thickness: 3}); err == nil {
		t.Fatal("expected error for NaN coordinate")
	}
}

// TestExportAllRealsAreValidLiterals scans every numeric token outside string
// literals and entity references and requires it to be a valid REAL, except
// for the few INTEGER attributes the writer emits.
func TestExportAllRealsAreValidLiterals(t *testing.T) {
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "Test 1.5 Box"}, samplePanels(), manifest.Material{Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	strRe := regexp.MustCompile(`'(?:[^']|'')*'`)
	numRe := regexp.MustCompile(`(?:^|[(,])([-+]?\d[\w.+-]*)`)
	realRe := regexp.MustCompile(`^-?\d+\.\d*(E-?\d+)?$`)
	intAttrRe := regexp.MustCompile(`GEOMETRIC_REPRESENTATION_CONTEXT\(3\)|,2003,`)
	seen := map[string]int{}
	for _, line := range strings.Split(buf.String(), "\n") {
		m := defRe.FindStringIndex(line)
		if m == nil {
			continue // header lines carry only strings
		}
		body := line[m[1]:]
		body = strRe.ReplaceAllString(body, "''")
		body = refRe.ReplaceAllString(body, "#")
		body = intAttrRe.ReplaceAllString(body, "")
		name := body[:strings.IndexAny(body, "(")]
		for _, sm := range numRe.FindAllStringSubmatch(body, -1) {
			tok := sm[1]
			if !realRe.MatchString(tok) {
				t.Errorf("%s: invalid REAL token %q in %s", name, tok, line)
			}
			seen[name]++
		}
	}
	for _, n := range []string{"CARTESIAN_POINT", "DIRECTION", "VECTOR"} {
		if seen[n] == 0 {
			t.Errorf("no numeric tokens scanned for %s", n)
		}
	}
	if seen["UNCERTAINTY_MEASURE_WITH_UNIT"] == 0 {
		t.Error("uncertainty REAL not scanned")
	}
}

func TestExportProjectMetadata(t *testing.T) {
	fixed := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	exp := Exporter{Now: func() time.Time { return fixed }}
	box := pack.BoxResult{BoxName: "Box", Project: &manifest.Project{
		Name:        "Insert & \"Co\"",
		Author:      "José Ñandú — 東 O'Neil",
		Description: "Two tiers,\nline two \\ backslash " + strings.Repeat("long ", 100),
	}}
	var buf bytes.Buffer
	if err := exp.Export(&buf, box, samplePanels(), manifest.Material{Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	wantAuthor := `('Jos\X2\00E9\X0\ \X2\00D1\X0\and\X2\00FA\X0\ \X2\2014\X0\ \X2\6771\X0\ O''Neil')`
	if !strings.Contains(out, "FILE_NAME('Box insert','2024-01-02T03:04:05',"+wantAuthor+",(''),'cubby','cubby','');") {
		t.Errorf("FILE_NAME author missing or wrong:\n%s", out[:400])
	}
	if !strings.Contains(out, "'Two tiers, line two \\\\ backslash long long") {
		t.Errorf("PRODUCT description missing or wrong")
	}
	if !strings.Contains(out, "PRODUCT('Box','Box','Two tiers") {
		t.Errorf("PRODUCT naming changed")
	}
	for _, r := range out {
		if r > 0x7e {
			t.Fatalf("non-ASCII rune %q in output", r)
		}
	}

	// The metadata must not disturb the geometry: entity numbering and the
	// solids are identical to an export without a project.
	var plain bytes.Buffer
	if err := exp.Export(&plain, pack.BoxResult{BoxName: "Box"}, samplePanels(), manifest.Material{Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	if strip := func(s string) string { i := strings.Index(s, "DATA;"); return s[i:] }; strings.Count(strip(out), "\n") != strings.Count(strip(plain.String()), "\n") {
		t.Error("project metadata changed the number of DATA entities")
	}
}

func TestExportAuthorOnlyKeepsProductDescriptionEmpty(t *testing.T) {
	var buf bytes.Buffer
	box := pack.BoxResult{BoxName: "Box", Project: &manifest.Project{Author: "Ann"}}
	if err := (Exporter{}).Export(&buf, box, samplePanels(), manifest.Material{Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "('Ann'),(''),'cubby'") || !strings.Contains(out, "PRODUCT('Box','Box','',(") {
		t.Errorf("unexpected header/product:\n%s", out[:400])
	}
}

// TestExportHeightReducedPanelTopAndBottomNotches exports a panel trimmed by
// Cut (full height 60, cut 20, reduced height 40, notch depth 30) with notches
// on the top or bottom edge: one prism per outline vertex set, a closed edge
// count, and no point above the reduced height.
func TestExportHeightReducedPanelTopAndBottomNotches(t *testing.T) {
	pointRe := regexp.MustCompile(`CARTESIAN_POINT\('',\(([^,]+),([^,]+),([^)]+)\)\)`)
	for _, edge := range []geometry.Edge{geometry.EdgeTop, geometry.EdgeBottom} {
		for _, axis := range []geometry.Axis{geometry.AxisWidthRun, geometry.AxisDepthRun} {
			t.Run(string(edge)+"/"+string(axis), func(t *testing.T) {
				p := geometry.Panel{
					ID: "r-1", Axis: axis, Length: 100, Height: 40, Cut: 20, Thickness: 3,
					Notches: []geometry.Notch{{Pos: 20, Width: 3, Edge: edge}, {Pos: 70, Width: 3, Edge: edge}},
				}
				var buf bytes.Buffer
				if err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "B"}, []geometry.Panel{p}, manifest.Material{Thickness: 3}); err != nil {
					t.Fatal(err)
				}
				out := buf.String()

				// Two notches on the trimmed top edge give 12 outline corners,
				// two on the bottom edge also 12; each is a 3N-edge prism.
				n := len(p.OutlinePolygon())
				if n != 12 {
					t.Fatalf("outline has %d vertices, want 12", n)
				}
				if got := len(edgeCurveRe.FindAllString(out, -1)); got != 3*n {
					t.Errorf("%d EDGE_CURVEs, want %d", got, 3*n)
				}
				if got := len(vertexRe.FindAllString(out, -1)); got != 2*n {
					t.Errorf("%d vertices, want %d", got, 2*n)
				}

				// The panel's local height is world Z for both axes.
				var maxZ, notchZ float64
				for _, m := range pointRe.FindAllStringSubmatch(out, -1) {
					z, err := strconv.ParseFloat(m[3], 64)
					if err != nil {
						continue
					}
					maxZ = math.Max(maxZ, z)
					if z == 30 {
						notchZ = z
					}
				}
				if maxZ != 40 {
					t.Errorf("max point Z = %g, want reduced height 40", maxZ)
				}
				if notchZ != 30 {
					t.Errorf("no point at the full-height notch depth Z=30")
				}
			})
		}
	}
}

func TestExportNoPanelsErrors(t *testing.T) {
	for name, panels := range map[string][]geometry.Panel{
		"nil": nil,
	} {
		var buf bytes.Buffer
		err := (Exporter{}).Export(&buf, pack.BoxResult{BoxName: "b"}, panels, manifest.Material{Thickness: 3})
		if !errors.Is(err, export.ErrNoPanels) {
			t.Fatalf("%s: expected export.ErrNoPanels, got %v", name, err)
		}
		if strings.Contains(buf.String(), "ADVANCED_BREP_SHAPE_REPRESENTATION") {
			t.Fatalf("%s: wrote an empty solid list", name)
		}
	}
}
