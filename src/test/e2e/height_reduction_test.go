package e2e

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// hrManifest is a one-row, two-tray fullWalls layout (100 x 90 x 40 mm
// interior) whose reduction settings are spliced in as YAML fragments:
// defaultsExtra goes under defaults, cardExtra under the card-tray group.
const hrManifest = `version: 1
box:
  name: "HR"
  units: mm
  interior: {width: 100, depth: 90, height: 40}
material: {name: "F", thickness: 3.0, kerf: 0.0}
defaults:
  padding: 1.0
  margin: 1.5
  fullWalls: true
%s
components:
  - {id: gear-a, width: 30, depth: 20, height: 10, qty: 4, allowRotate: false}
  - {id: cards, width: 35, depth: 15, height: 8, qty: 4, allowRotate: false}
groups:
  - {id: gears, components: [gear-a]}
  - id: card-tray
    components: [cards]
%s
`

func writeHRManifest(t *testing.T, defaultsExtra, cardExtra string) string {
	t.Helper()
	body := strings.Replace(hrManifest, "%s", defaultsExtra, 1)
	body = strings.Replace(body, "%s", cardExtra, 1)
	path := filepath.Join(t.TempDir(), "hr.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// csvHeights builds manifest as CSV and returns height_mm per panel id.
func csvHeights(t *testing.T, manifest string) map[string]float64 {
	t.Helper()
	return csvHeightsWith(t, manifest)
}

func assertHeights(t *testing.T, got, want map[string]float64) {
	t.Helper()
	for id, h := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("panel %s missing from output (have %v)", id, got)
			continue
		}
		if g != h {
			t.Errorf("panel %s: height_mm = %v, want %v", id, g, h)
		}
	}
}

func TestBuildHeightReductionForms(t *testing.T) {
	tests := []struct {
		name     string
		defaults string
		card     string
		want     map[string]float64
	}{
		{
			name:     "no reduction",
			defaults: "",
			want: map[string]float64{
				"w-1": 40, "w-4": 40, "wv-2": 40, "wv-5": 40,
				"h-1": 40, "iv-gears-1-1": 40, "iv-card-tray-1-1": 40,
				"ih-gears-1": 40, "ih-card-tray-1": 40,
			},
		},
		{
			name:     "numbers",
			defaults: "  externalHeightReduction: 4\n  dividerHeightReduction: 6",
			want: map[string]float64{
				"w-1": 36, "w-4": 36, "wv-2": 36, "wv-5": 36,
				"h-1": 34, "iv-gears-1-1": 34, "iv-card-tray-1-1": 34,
				"ih-gears-1": 34, "ih-card-tray-1": 34,
			},
		},
		{
			name:     "percent",
			defaults: "  externalHeightReduction: \"10%\"\n  dividerHeightReduction: \"25%\"",
			want: map[string]float64{
				"w-1": 36, "w-4": 36, "wv-2": 36, "wv-5": 36,
				"h-1": 30, "iv-gears-1-1": 30, "iv-card-tray-1-1": 30,
				"ih-gears-1": 30, "ih-card-tray-1": 30,
			},
		},
		{
			name: "by and panels",
			defaults: "  externalHeightReduction: {by: 5, panels: [\"w-*\"]}\n" +
				"  dividerHeightReduction: {by: \"50%\", panels: [\"iv-*\"]}",
			want: map[string]float64{
				// only w-* is external-reduced; wv-* stays full height.
				"w-1": 35, "w-4": 35, "wv-2": 40, "wv-5": 40,
				// only iv-* is divider-reduced.
				"iv-gears-1-1": 20, "iv-card-tray-1-1": 20,
				"ih-gears-1": 40, "ih-card-tray-1": 40, "h-1": 40,
			},
		},
		{
			// A group value replaces the default wholesale: card-tray loses
			// the default divider reduction and the default panels filter.
			name: "group override replaces defaults",
			defaults: "  externalHeightReduction: \"10%\"\n" +
				"  dividerHeightReduction: 6",
			card: "    externalHeightReduction: {by: 5, panels: [\"wv-*\"]}\n" +
				"    dividerHeightReduction: {by: \"25%\", panels: [\"iv-*\"]}",
			want: map[string]float64{
				// gears inherits defaults.
				"w-1": 36, "wv-2": 36, "iv-gears-1-1": 34, "ih-gears-1": 34,
				// card-tray: wv-* by 5, w-* not matched (default 10% not merged).
				"wv-5": 35, "w-4": 40,
				// iv-* by 25%, ih-* not matched (default 6 not merged).
				"iv-card-tray-1-1": 30, "ih-card-tray-1": 40,
				// h-1 is shared; gears' 6 is the largest that applies.
				"h-1": 34,
			},
		},
		{
			name:     "shared grid panel takes the largest reduction (other tray larger)",
			defaults: "  dividerHeightReduction: 6",
			card:     "    dividerHeightReduction: {by: \"25%\", panels: [\"h-*\"]}",
			want: map[string]float64{
				"h-1": 30, "ih-gears-1": 34, "ih-card-tray-1": 40,
			},
		},
		{
			name:     "shared grid panel takes the largest reduction (default larger)",
			defaults: "  dividerHeightReduction: \"25%\"",
			card:     "    dividerHeightReduction: 6",
			want: map[string]float64{
				"h-1": 30, "ih-gears-1": 30, "ih-card-tray-1": 34,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := csvHeights(t, writeHRManifest(t, tc.defaults, tc.card))
			assertHeights(t, got, tc.want)
		})
	}
}

// removableHRManifest returns the height-reduction example turned into a
// roomy removable-tray box with floors.
func removableHRManifest(t *testing.T, reductions bool) string {
	t.Helper()
	src, err := os.ReadFile(examplePath(t, "height-reduction.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, r := range [][2]string{
		{"width: 100", "width: 200"},
		{"depth: 90", "depth: 150"},
		{"fullWalls: true", "fullWalls: false\n  removable: true\n  floor: true"},
	} {
		if !strings.Contains(text, r[0]) {
			t.Fatalf("example no longer contains %q", r[0])
		}
		text = strings.Replace(text, r[0], r[1], 1)
	}
	if !reductions {
		text = regexp.MustCompile(`(?m)^\s*(externalHeightReduction|dividerHeightReduction):.*\n(\s+(by|panels):.*\n)*`).ReplaceAllString(text, "")
	}
	path := filepath.Join(t.TempDir(), "removable.yaml")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildHeightReductionRemovableTraysAndFloors(t *testing.T) {
	base := csvHeights(t, removableHRManifest(t, false))
	got := csvHeights(t, removableHRManifest(t, true))

	// Interior 40 - floor 3 = 37 for removable tray walls.
	assertHeights(t, base, map[string]float64{
		"rw-gears-front": 37, "rv-gears-left": 37,
		"rw-card-tray-front": 37, "rv-card-tray-left": 37,
		"iv-gears-1-1": 37, "iv-card-tray-1-1": 37,
	})
	assertHeights(t, got, map[string]float64{
		// defaults: dividers -30% (37 -> 25.9); no external reduction.
		"rw-gears-front": 37, "rv-gears-left": 37,
		"iv-gears-1-1": 25.9,
		// card-tray: walls -5; its only divider entry is ih-*-1, which
		// matches nothing here (single row), so iv-* stays full height.
		"rw-card-tray-front": 32, "rv-card-tray-left": 32,
		"iv-card-tray-1-1": 37,
	})
	// Floors are never reduced.
	for _, id := range []string{"fl-gears", "fl-card-tray"} {
		b, ok := base[id]
		if !ok {
			t.Fatalf("baseline missing %s: %v", id, base)
		}
		if got[id] != b {
			t.Errorf("%s: height_mm = %v with reductions, want unreduced %v", id, got[id], b)
		}
	}
}

var (
	svgGroupRe = regexp.MustCompile(`(?s)<polygon points="([^"]+)"[^>]*/>\s*<text[^>]*>([^<]+)</text>`)
	dxfNumRe   = regexp.MustCompile(`^-?[0-9.]+$`)
)

// pointsHeight returns max y - min y over "x,y x,y ..." points.
func pointsHeight(t *testing.T, points string) float64 {
	t.Helper()
	minY, maxY := 1e18, -1e18
	for _, p := range strings.Fields(points) {
		xy := strings.Split(p, ",")
		y, err := strconv.ParseFloat(xy[len(xy)-1], 64)
		if err != nil {
			t.Fatalf("bad point %q", p)
		}
		minY, maxY = min(minY, y), max(maxY, y)
	}
	return maxY - minY
}

// svgHeights returns the outline height (bounding box) per labelled panel.
func svgHeights(t *testing.T, svg string) map[string]float64 {
	t.Helper()
	got := map[string]float64{}
	for _, m := range svgGroupRe.FindAllStringSubmatch(svg, -1) {
		got[m[2]] = pointsHeight(t, m[1])
	}
	return got
}

// dxfHeights returns the outline height per panel: each panel is a POLYLINE
// whose VERTEX y values (group code 20) are followed by its TEXT label.
func dxfHeights(t *testing.T, dxf string) map[string]float64 {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(dxf, "\r\n", "\n"), "\n")
	got := map[string]float64{}
	var ys []float64
	inPoly, inText := false, false
	for i := 0; i+1 < len(lines); i += 2 {
		code, val := strings.TrimSpace(lines[i]), strings.TrimSpace(lines[i+1])
		switch {
		case code == "0":
			inText = val == "TEXT"
			switch val {
			case "POLYLINE":
				inPoly, ys = true, nil
			case "SEQEND":
				inPoly = false
			}
		case code == "20" && inPoly && dxfNumRe.MatchString(val):
			y, _ := strconv.ParseFloat(val, 64)
			ys = append(ys, y)
		case code == "1" && inText && len(ys) > 0:
			lo, hi := ys[0], ys[0]
			for _, y := range ys {
				lo, hi = min(lo, y), max(hi, y)
			}
			got[val] = hi - lo
			ys = nil
		}
	}
	return got
}

func TestBuildHeightReductionSVGAndDXFMatchCSV(t *testing.T) {
	manifest := writeHRManifest(t,
		"  externalHeightReduction: \"10%\"\n  dividerHeightReduction: 6",
		"    dividerHeightReduction: {by: \"25%\", panels: [\"iv-*\", \"h-*\"]}")
	want := csvHeights(t, manifest)
	if want["h-1"] != 30 || want["w-1"] != 36 || want["ih-card-tray-1"] != 40 {
		t.Fatalf("unexpected CSV heights: %v", want)
	}

	for _, format := range []string{"svg", "dxf"} {
		out, errOut, code := run(t, "build", manifest, "--format", format)
		if code != 0 {
			t.Fatalf("%s: expected exit 0, got %d (stderr: %s)", format, code, errOut)
		}
		var got map[string]float64
		if format == "svg" {
			got = svgHeights(t, out)
		} else {
			got = dxfHeights(t, out)
		}
		// CSV lists one row per distinct panel (qty column); SVG/DXF draw
		// every instance, so compare each CSV panel against its drawings.
		for id, h := range want {
			g, ok := got[id]
			if !ok {
				t.Errorf("%s: panel %s not found", format, id)
				continue
			}
			if d := g - h; d > 1e-6 || d < -1e-6 {
				t.Errorf("%s: panel %s height = %v, want %v", format, id, g, h)
			}
		}
	}
}

// csvHeightsWith is csvHeights with extra build arguments.
func csvHeightsWith(t *testing.T, manifest string, args ...string) map[string]float64 {
	t.Helper()
	full := append([]string{"build", manifest, "--format", "csv"}, args...)
	out, errOut, code := run(t, full...)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, errOut)
	}
	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil || len(recs) < 2 {
		t.Fatalf("unexpected CSV (err %v):\n%s", err, out)
	}
	idCol, hCol, idsCol := -1, -1, -1
	for i, name := range recs[0] {
		switch name {
		case "panel_id":
			idCol = i
		case "ids":
			idsCol = i
		case "height_mm":
			hCol = i
		}
	}
	got := map[string]float64{}
	for _, r := range recs[1:] {
		h, err := strconv.ParseFloat(r[hCol], 64)
		if err != nil {
			t.Fatalf("bad height %q in row %v", r[hCol], r)
		}
		got[r[idCol]] = h
		if idsCol >= 0 {
			for _, id := range strings.Split(r[idsCol], ";") {
				got[id] = h
			}
		}
	}
	return got
}
