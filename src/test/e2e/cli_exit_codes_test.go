package e2e

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/cli"
)

// fixture resolves a manifest under src/testdata/manifests.
func fixture(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "testdata", "manifests", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("missing fixture: %v", err)
	}
	return p
}

// twoCompartmentPanels is the panel count for two-compartment.yaml: two
// compartments side by side in one row need exactly one internal divider.
const twoCompartmentPanels = 1

func TestCLIExitCodes(t *testing.T) {
	valid := fixture(t, "two-compartment.yaml")
	invalid := fixture(t, "invalid.yaml")
	overfull := fixture(t, "overfull.yaml")
	unknownField := fixture(t, "unknown-field.yaml")
	tooTall := reductionTooTallManifest(t)
	missing := filepath.Join(t.TempDir(), "nope.yaml")

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"build ok", []string{"build", valid, "--format", "csv"}, cli.ExitOK},
		{"validate ok", []string{"validate", valid}, cli.ExitOK},
		{"list ok", []string{"list", valid}, cli.ExitOK},
		{"groups ok", []string{"groups", valid}, cli.ExitOK},
		{"version ok", []string{"version"}, cli.ExitOK},

		{"build unknown format", []string{"build", valid, "--format", "bogus"}, cli.ExitUsageError},
		{"build out with multiple formats", []string{"build", valid, "--format", "csv,svg", "--out", filepath.Join(t.TempDir(), "o")}, cli.ExitUsageError},
		{"build missing manifest", []string{"build", missing}, cli.ExitUsageError},
		{"validate missing manifest", []string{"validate", missing}, cli.ExitUsageError},
		{"list missing manifest", []string{"list", missing}, cli.ExitUsageError},
		{"groups missing manifest", []string{"groups", missing}, cli.ExitUsageError},
		{"build unknown field", []string{"build", unknownField}, cli.ExitUsageError},
		{"validate unknown field", []string{"validate", unknownField}, cli.ExitUsageError},
		{"validate unknown format", []string{"validate", valid, "--format", "xml"}, cli.ExitUsageError},
		{"list unknown format", []string{"list", valid, "--format", "xml"}, cli.ExitUsageError},
		{"groups unknown format", []string{"groups", valid, "--format", "xml"}, cli.ExitUsageError},
		{"unknown subcommand", []string{"frobnicate"}, cli.ExitUsageError},
		{"unknown flag", []string{"build", valid, "--nope"}, cli.ExitUsageError},
		{"build without manifest arg", []string{"build"}, cli.ExitUsageError},

		{"build invalid manifest", []string{"build", invalid}, cli.ExitValidationFailed},
		{"validate invalid manifest", []string{"validate", invalid}, cli.ExitValidationFailed},
		{"validate invalid manifest json", []string{"validate", invalid, "--format", "json"}, cli.ExitValidationFailed},
		{"list invalid manifest", []string{"list", invalid}, cli.ExitValidationFailed},
		{"groups invalid manifest", []string{"groups", invalid}, cli.ExitValidationFailed},
		{"build reduction too large", []string{"build", tooTall}, cli.ExitValidationFailed},
		{"build reduction too large csv", []string{"build", tooTall, "--format", "csv"}, cli.ExitValidationFailed},
		{"validate reduction too large", []string{"validate", tooTall}, cli.ExitValidationFailed},

		{"build overfull", []string{"build", overfull, "--format", "csv"}, cli.ExitPackingIncomplete},
		{"groups overfull", []string{"groups", overfull}, cli.ExitPackingIncomplete},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, code := run(t, tc.args...)
			if code != tc.want {
				t.Fatalf("args %v: expected exit %d, got %d (stderr: %s)", tc.args, tc.want, code, stderr)
			}
			if tc.want != cli.ExitOK && stderr == "" {
				t.Errorf("expected an error message on stderr")
			}
		})
	}
}

func TestExitCodeConstants(t *testing.T) {
	// The documented contract: 0 ok, 1 usage/IO, 2 validation, 3 packing.
	if cli.ExitOK != 0 || cli.ExitUsageError != 1 || cli.ExitValidationFailed != 2 || cli.ExitPackingIncomplete != 3 {
		t.Fatalf("exit code constants changed: %d %d %d %d", cli.ExitOK, cli.ExitUsageError, cli.ExitValidationFailed, cli.ExitPackingIncomplete)
	}
}

func TestValidateJSONFixtures(t *testing.T) {
	type result struct {
		Issues []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"issues"`
	}

	out, _, code := run(t, "validate", "--format", "json", fixture(t, "two-compartment.yaml"))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, `"issues": []`) {
		t.Fatalf("expected empty issues to serialise as [], got:\n%s", out)
	}
	var ok result
	if err := json.Unmarshal([]byte(out), &ok); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(ok.Issues) != 0 {
		t.Fatalf("expected no issues, got %+v", ok.Issues)
	}

	out, _, code = run(t, "validate", "--format", "json", fixture(t, "invalid.yaml"))
	if code != cli.ExitValidationFailed {
		t.Fatalf("expected exit %d, got %d", cli.ExitValidationFailed, code)
	}
	var bad result
	if err := json.Unmarshal([]byte(out), &bad); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(bad.Issues) != 1 || bad.Issues[0].Path != "box.name" || !strings.Contains(bad.Issues[0].Message, "empty") {
		t.Fatalf("expected a single box.name issue, got %+v", bad.Issues)
	}

	text, _, _ := run(t, "validate", fixture(t, "invalid.yaml"))
	if !strings.Contains(text, "FAIL: 1 issue(s) found") || !strings.Contains(text, "box.name") {
		t.Fatalf("unexpected text output: %q", text)
	}
	text, _, _ = run(t, "validate", fixture(t, "two-compartment.yaml"))
	if !strings.Contains(text, "OK: manifest is valid") {
		t.Fatalf("unexpected text output: %q", text)
	}
}

func TestListFormatsFixture(t *testing.T) {
	manifest := fixture(t, "two-compartment.yaml")

	out, _, code := run(t, "list", "--format", "json", manifest)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var rows []struct {
		ID     string  `json:"id"`
		Name   string  `json:"name"`
		Group  string  `json:"group"`
		Width  float64 `json:"width"`
		Depth  float64 `json:"depth"`
		Height float64 `json:"height"`
		Qty    int     `json:"qty"`
	}
	for _, key := range []string{`"id"`, `"name"`, `"group"`, `"width"`, `"depth"`, `"height"`, `"qty"`} {
		if !strings.Contains(out, key) {
			t.Errorf("list json missing lowercase key %s:\n%s", key, out)
		}
	}
	if strings.Contains(out, `"ID"`) || strings.Contains(out, `"Width"`) {
		t.Errorf("list json still has Go-cased keys:\n%s", out)
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 components, got %+v", rows)
	}
	if r := rows[0]; r.ID != "card-tray" || r.Group != "cards" || r.Width != 40 || r.Depth != 50 || r.Height != 20 || r.Qty != 1 {
		t.Errorf("unexpected first row: %+v", r)
	}
	if r := rows[1]; r.ID != "token-tray" || r.Group != "tokens" || r.Width != 30 {
		t.Errorf("unexpected second row: %+v", r)
	}

	out, _, code = run(t, "list", "--format", "csv", manifest)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[1][0] != "card-tray" || recs[2][0] != "token-tray" {
		t.Fatalf("unexpected CSV records: %q", recs)
	}

	out, _, code = run(t, "list", "--group", "tokens", "--format", "json", manifest)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	rows = nil
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "token-tray" {
		t.Fatalf("expected only token-tray, got %+v", rows)
	}
}

func TestListJSONEmptyResultIsArray(t *testing.T) {
	// Every component in the fixture is grouped, so --group - matches nothing.
	out, _, code := run(t, "list", "--format", "json", "--group", "-", fixture(t, "two-compartment.yaml"))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("expected [] for an empty result, got %q", out)
	}
}

func TestGroupsJSONFixture(t *testing.T) {
	out, _, code := run(t, "groups", "--format", "json", fixture(t, "two-compartment.yaml"))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	for _, want := range []string{`"boxName"`, `"interiorW"`, `"compartments"`, `"usedW"`, `"remaining"`, `"totalMissing": []`, `"missing": []`, `"contentOffsetX"`} {
		if !strings.Contains(out, want) {
			t.Errorf("groups json missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "null") || strings.Contains(out, `"BoxName"`) || strings.Contains(out, `"ID"`) {
		t.Errorf("groups json has null or Go-cased keys:\n%s", out)
	}
	var res struct {
		Compartments []struct {
			ID string `json:"id"`
		} `json:"compartments"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(res.Compartments) != 2 || res.Compartments[0].ID != "cards" || res.Compartments[1].ID != "tokens" {
		t.Fatalf("expected compartments [cards tokens], got %+v", res.Compartments)
	}
}

func TestBuildFixtureContent(t *testing.T) {
	manifest := fixture(t, "two-compartment.yaml")

	// CSV: header + one row per panel. The single divider runs along the
	// depth; panel length 55 = 52 (compartment depth) + 3 (thickness);
	// height defaults to the 40 mm interior height.
	out, _, code := run(t, "build", manifest, "--format", "csv")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v\n%s", err, out)
	}
	if len(recs) != 1+twoCompartmentPanels {
		t.Fatalf("expected header + %d panel rows, got %d records:\n%s", twoCompartmentPanels, len(recs), out)
	}
	if got := strings.Join(recs[0], ","); got != "panel_id,axis,length_mm,height_mm,thickness_mm,material,notch_count,qty,cut_mm,notch_positions,ids" {
		t.Errorf("unexpected header %q", got)
	}
	row := recs[1]
	if row[0] != "v-1" || row[1] != "depth-run" || row[2] != "55" || row[3] != "40" || row[4] != "3" || row[7] != "1" {
		t.Errorf("unexpected panel row %q", row)
	}

	// SVG: one polygon per panel.
	svg, _, code := run(t, "build", manifest, "--format", "svg")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if got := strings.Count(svg, "<polygon"); got != twoCompartmentPanels {
		t.Errorf("expected %d polygon(s), got %d:\n%s", twoCompartmentPanels, got, svg)
	}
	if !strings.Contains(svg, "v-1") {
		t.Errorf("expected panel label v-1 in SVG")
	}

	// DXF: one POLYLINE entity per panel.
	dxf, _, code := run(t, "build", manifest, "--format", "dxf")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if got := strings.Count(dxf, "\nPOLYLINE\n"); got != twoCompartmentPanels {
		t.Errorf("expected %d POLYLINE(s), got %d", twoCompartmentPanels, got)
	}
}

var svgWidthRe = regexp.MustCompile(`<svg[^>]* width="([0-9.]+)mm" height="([0-9.]+)mm"`)

func svgSize(t *testing.T, svg string) (w, h string) {
	t.Helper()
	m := svgWidthRe.FindStringSubmatch(svg)
	if m == nil {
		t.Fatalf("no width/height on <svg> in:\n%s", svg)
	}
	return m[1], m[2]
}

func TestBuildSheetWidthAffectsSVGCanvas(t *testing.T) {
	manifest := fixture(t, "two-compartment.yaml")

	def, _, code := run(t, "build", manifest, "--format", "svg")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if w, _ := svgSize(t, def); w != "600" {
		t.Errorf("default canvas width = %s, want 600", w)
	}

	narrow, _, code := run(t, "build", manifest, "--format", "svg", "--sheet-width", "300")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if w, _ := svgSize(t, narrow); w != "300" {
		t.Errorf("--sheet-width 300: canvas width = %s, want 300", w)
	}
	if !strings.Contains(narrow, `viewBox="0 0 300 `) {
		t.Errorf("expected viewBox width 300 in:\n%s", narrow)
	}
}

func TestBuildOutFileAndStdout(t *testing.T) {
	manifest := fixture(t, "two-compartment.yaml")
	want, _, code := run(t, "build", manifest, "--format", "csv")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	path := filepath.Join(t.TempDir(), "cuts.csv")
	stdout, _, code := run(t, "build", manifest, "--format", "csv", "--out", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stdout != "" {
		t.Errorf("--out file: expected empty stdout, got %q", stdout)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("--out file content differs from stdout content:\n%s\nvs\n%s", got, want)
	}

	dash, _, code := run(t, "build", manifest, "--format", "csv", "--out", "-")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if dash != want {
		t.Errorf("--out - should write to stdout; got %q", dash)
	}
}

func TestBuildOverfullStillWritesOutput(t *testing.T) {
	out, stderr, code := run(t, "build", fixture(t, "overfull.yaml"), "--format", "csv")
	if code != cli.ExitPackingIncomplete {
		t.Fatalf("expected exit %d, got %d", cli.ExitPackingIncomplete, code)
	}
	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil || len(recs) != 1 {
		t.Fatalf("expected header-only CSV (err %v):\n%s", err, out)
	}
	if !strings.Contains(stderr, "did not fit") {
		t.Errorf("expected 'did not fit' on stderr, got %q", stderr)
	}
}

func TestBuildStrictFixture(t *testing.T) {
	_, _, code := run(t, "build", fixture(t, "two-compartment.yaml"), "--strict", "--format", "csv")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	_, _, code = run(t, "build", fixture(t, "invalid.yaml"), "--strict")
	if code != cli.ExitValidationFailed {
		t.Fatalf("expected exit %d, got %d", cli.ExitValidationFailed, code)
	}
}

func TestVersionCommand(t *testing.T) {
	out, _, code := run(t, "version")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.HasPrefix(out, "cubby ") || !strings.Contains(out, "commit") || !strings.Contains(out, "built") {
		t.Fatalf("unexpected version output %q", out)
	}
}

// TestBuildAllGroupsUnplaceableExitsPacking builds a manifest whose only group
// has no placeable component: no compartments are produced, nothing crashes,
// and the exit code reports incomplete packing.
func TestBuildAllGroupsUnplaceableExitsPacking(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "empty-group.yaml")
	src := `version: 1
box:
  name: "Empty Group Box"
  units: mm
  interior: {width: 100, depth: 100, height: 20}
material:
  thickness: 3
  kerf: 0
defaults:
  margin: 0
components:
  - id: tall
    name: Tall
    width: 10
    depth: 10
    height: 40
    qty: 2
groups:
  - id: dead
    name: Dead
    components: [tall]
`
	if err := os.WriteFile(manifest, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"console", "csv", "svg", "dxf", "stl", "step", "iso-svg", "iso-png", "assembly"} {
		_, stderr, code := run(t, "build", manifest, "--format", format)
		if code != cli.ExitPackingIncomplete {
			t.Errorf("format %s: expected exit %d, got %d (stderr: %s)", format, cli.ExitPackingIncomplete, code, stderr)
		}
	}
	_, stderr, code := run(t, "groups", manifest)
	if code != cli.ExitPackingIncomplete || !strings.Contains(stderr, "did not fit") {
		t.Errorf("groups: exit %d, stderr %q", code, stderr)
	}
}

func TestGroupsJSONMissingKeys(t *testing.T) {
	out, _, code := run(t, "groups", "--format", "json", fixture(t, "overfull.yaml"))
	if code != cli.ExitPackingIncomplete {
		t.Fatalf("expected exit %d, got %d", cli.ExitPackingIncomplete, code)
	}
	for _, want := range []string{`"componentId"`, `"requested"`, `"placed"`, `"rejected"`, `"reason"`} {
		if !strings.Contains(out, want) {
			t.Errorf("groups json missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "null") {
		t.Errorf("groups json contains null:\n%s", out)
	}
}

func TestBuildNestGapAffectsSVGAndDXF(t *testing.T) {
	manifest := fixture(t, "two-compartment.yaml")
	for _, format := range []string{"svg", "dxf"} {
		def, _, code := run(t, "build", manifest, "--format", format)
		if code != 0 {
			t.Fatalf("%s: expected exit 0, got %d", format, code)
		}
		five, _, _ := run(t, "build", manifest, "--format", format, "--nest-gap", "5")
		if def != five {
			t.Errorf("%s: --nest-gap 5 differs from default", format)
		}
		wide, _, code := run(t, "build", manifest, "--format", format, "--nest-gap", "20")
		if code != 0 || wide == def {
			t.Errorf("%s: --nest-gap 20 (exit %d) should change output", format, code)
		}
		zero, _, code := run(t, "build", manifest, "--format", format, "--nest-gap", "0")
		if code != 0 || zero == def || zero == wide {
			t.Errorf("%s: --nest-gap 0 (exit %d) should be honoured", format, code)
		}
	}
}

func TestBuildRejectsBadFlagsBeforeLoadingManifest(t *testing.T) {
	valid := fixture(t, "two-compartment.yaml")
	invalid := fixture(t, "invalid.yaml")
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown format wins over invalid manifest", []string{"build", invalid, "--format", "bogus"}, `unknown --format "bogus"`},
		{"sheet-width NaN", []string{"build", valid, "--format", "svg", "--sheet-width", "NaN"}, "--sheet-width must be a finite number"},
		{"nest-gap Inf", []string{"build", valid, "--format", "svg", "--nest-gap", "Inf"}, "--nest-gap must be a finite number"},
		{"nest-gap -Inf", []string{"build", valid, "--format", "dxf", "--nest-gap", "-Inf"}, "--nest-gap must be a finite number"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := run(t, tc.args...)
			if code != cli.ExitUsageError {
				t.Fatalf("args %v: exit = %d, want %d (stderr: %s)", tc.args, code, cli.ExitUsageError, stderr)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
			if stdout != "" {
				t.Errorf("expected no output, got %q", stdout)
			}
		})
	}
}

func TestValidateJSONLoadFailure(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cases := map[string]string{
		"bad yaml":    write("bad.yaml", "box: [unclosed\n"),
		"unknown key": write("unknown.yaml", "bogus: 1\n"),
		"empty file":  write("empty.yaml", ""),
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			out, errOut, code := run(t, "validate", "--format", "json", path)
			if code != cli.ExitUsageError {
				t.Fatalf("expected exit %d, got %d", cli.ExitUsageError, code)
			}
			var res struct {
				Issues []struct {
					Path    string `json:"path"`
					Message string `json:"message"`
				} `json:"issues"`
			}
			if err := json.Unmarshal([]byte(out), &res); err != nil {
				t.Fatalf("stdout is not JSON: %v\n%s", err, out)
			}
			if len(res.Issues) != 1 || res.Issues[0].Path != "" || !strings.HasPrefix(res.Issues[0].Message, "parse manifest:") {
				t.Fatalf("unexpected issues: %+v", res.Issues)
			}
			if !strings.Contains(errOut, res.Issues[0].Message) {
				t.Fatalf("stderr %q should carry the load error %q", errOut, res.Issues[0].Message)
			}
			// Text format is unchanged: nothing on stdout.
			if tout, _, tcode := run(t, "validate", path); tout != "" || tcode != cli.ExitUsageError {
				t.Fatalf("text format: stdout %q exit %d", tout, tcode)
			}
		})
	}

	// A missing file keeps the plain error: no JSON.
	out, errOut, code := run(t, "validate", "--format", "json", filepath.Join(dir, "missing.yaml"))
	if code != cli.ExitUsageError || out != "" || !strings.Contains(errOut, "read manifest") {
		t.Fatalf("missing file: exit %d stdout %q stderr %q", code, out, errOut)
	}
}
