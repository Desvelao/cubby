package e2e

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/cli"
)

func run(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	root := cli.NewRootCmd()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		// main prints the returned error to stderr; mirror that here.
		errBuf.WriteString("error: " + err.Error() + "\n")
	}
	return outBuf.String(), errBuf.String(), cli.ExitCode(err)
}

func examplePath(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", "examples", name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuildEndToEndAllFormats(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")

	for _, format := range []string{"console", "csv", "svg", "dxf", "stl", "step", "iso-svg", "iso-svg-exploded", "iso-png", "iso-png-exploded", "assembly"} {
		out, errOut, code := run(t, "build", manifest, "--format", format)
		if code != 0 {
			t.Fatalf("format %s: expected exit 0, got %d (stderr: %s)", format, code, errOut)
		}
		if out == "" {
			t.Fatalf("format %s: expected non-empty output", format)
		}
	}
}

func TestBuildEndToEndOutDirDoesNotCollideOnSharedExtensions(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	outDir := t.TempDir()

	formats := []string{"console", "csv", "svg", "dxf", "stl", "step", "iso-svg", "iso-svg-exploded", "iso-png", "iso-png-exploded", "assembly"}
	_, errOut, code := run(t, "build", manifest, "--format", strings.Join(formats, ","), "--out-dir", outDir)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, errOut)
	}

	// Formats whose name equals their extension use the bare slug; the rest
	// carry the format name so shared extensions never collide.
	want := map[string]bool{
		"chess-checkers-combo.console.txt":          true,
		"chess-checkers-combo.csv":                  true,
		"chess-checkers-combo.svg":                  true,
		"chess-checkers-combo.dxf":                  true,
		"chess-checkers-combo.stl":                  true,
		"chess-checkers-combo.step":                 true,
		"chess-checkers-combo.iso-svg.svg":          true,
		"chess-checkers-combo.iso-svg-exploded.svg": true,
		"chess-checkers-combo.iso-png.png":          true,
		"chess-checkers-combo.iso-png-exploded.png": true,
		"chess-checkers-combo.assembly.pdf":         true,
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name()] = true
	}
	for name := range want {
		if !got[name] {
			t.Errorf("missing expected output file %q", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("unexpected output file %q", name)
		}
	}
}

func TestPlainJointsEndToEnd(t *testing.T) {
	manifest := examplePath(t, "plain-joints.yaml")

	for _, format := range []string{"console", "csv", "svg", "iso-png"} {
		out, errOut, code := run(t, "build", manifest, "--format", format)
		if code != 0 {
			t.Fatalf("format %s: expected exit 0, got %d (stderr: %s)", format, code, errOut)
		}
		if out == "" {
			t.Fatalf("format %s: expected non-empty output", format)
		}
	}

	csvOut, _, code := run(t, "build", manifest, "--format", "csv")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(csvOut, ",0,") {
		t.Fatalf("expected at least one panel with notch_count=0 (defaults.jointType: plain), got:\n%s", csvOut)
	}
}

func TestRowDividersEndToEnd(t *testing.T) {
	manifest := examplePath(t, "row-dividers.yaml")

	for _, format := range []string{"console", "csv", "svg"} {
		out, errOut, code := run(t, "build", manifest, "--format", format)
		if code != 0 {
			t.Fatalf("format %s: expected exit 0, got %d (stderr: %s)", format, code, errOut)
		}
		if out == "" {
			t.Fatalf("format %s: expected non-empty output", format)
		}
	}

	csvOut, _, code := run(t, "build", manifest, "--format", "csv")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	got := 0
	for _, line := range strings.Split(csvOut, "\n") {
		if strings.HasPrefix(line, "ih-") { // panel_id column only (ids column repeats it)
			got++
		}
	}
	if got != 1 {
		t.Fatalf("expected exactly 1 internal row-divider panel (from 'with-dividers'; 'no-dividers' opts out via dividers:false), got %d:\n%s", got, csvOut)
	}
}

func TestBuildEndToEndOverfullExitsThree(t *testing.T) {
	manifest := examplePath(t, "overfull.yaml")
	out, _, code := run(t, "build", manifest)
	if code != 3 {
		t.Fatalf("expected exit code 3 for an overfull manifest, got %d (stdout: %s)", code, out)
	}
}

func TestValidateEndToEnd(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	_, _, code := run(t, "validate", manifest)
	if code != 0 {
		t.Fatalf("expected exit code 0 for a valid manifest, got %d", code)
	}
}

func TestListEndToEnd(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	out, _, code := run(t, "list", manifest)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if out == "" {
		t.Fatal("expected non-empty component list")
	}
}

func TestListGroupFilterEndToEnd(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")

	out, _, code := run(t, "list", "--group", "-", manifest)
	if code != 0 {
		t.Fatalf("--group -: expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "rulebook") {
		t.Errorf("--group -: expected ungrouped component rulebook, got:\n%s", out)
	}
	for _, id := range []string{"chess-pieces-box", "checkers-red", "checkers-black"} {
		if strings.Contains(out, id) {
			t.Errorf("--group -: unexpected grouped component %s in:\n%s", id, out)
		}
	}

	out, _, code = run(t, "list", "--group", "checkers-compartment", manifest)
	if code != 0 {
		t.Fatalf("--group <id>: expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "checkers-red") || !strings.Contains(out, "checkers-black") {
		t.Errorf("--group <id>: expected both checkers components, got:\n%s", out)
	}
	if strings.Contains(out, "rulebook") || strings.Contains(out, "chess-pieces-box") {
		t.Errorf("--group <id>: unexpected components in:\n%s", out)
	}
}

func TestListGroupFilterUnknownIDEndToEnd(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")

	for _, format := range []string{"table", "csv", "json"} {
		out, stderr, code := run(t, "list", "--format", format, "--group", "nope", manifest)
		if code != 1 {
			t.Errorf("unknown group (%s): expected exit code 1, got %d", format, code)
		}
		if out != "" {
			t.Errorf("unknown group (%s): expected no stdout, got:\n%s", format, out)
		}
		if !strings.Contains(stderr, "nope") {
			t.Errorf("unknown group (%s): expected stderr to name the group, got: %s", format, stderr)
		}

		for _, id := range []string{"-", "checkers-compartment"} {
			out, _, code := run(t, "list", "--format", format, "--group", id, manifest)
			if code != 0 || out == "" {
				t.Errorf("known group %q (%s): expected exit 0 and output, got %d %q", id, format, code, out)
			}
		}
	}
}

func TestGroupsEndToEnd(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	out, _, code := run(t, "groups", manifest)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if out == "" {
		t.Fatal("expected non-empty groups report")
	}
}

func TestGroupsGroupFilterEndToEnd(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")

	out, _, code := run(t, "groups", "--format", "json", "--group", "checkers-compartment", manifest)
	if code != 0 {
		t.Fatalf("json --group: expected exit code 0, got %d", code)
	}
	var res struct {
		Compartments []struct {
			ID string `json:"id"`
		} `json:"compartments"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("json --group: invalid JSON: %v\n%s", err, out)
	}
	if len(res.Compartments) != 1 || res.Compartments[0].ID != "checkers-compartment" {
		t.Errorf("json --group: expected only checkers-compartment, got %+v", res.Compartments)
	}

	out, _, code = run(t, "groups", "--group", "checkers-compartment", manifest)
	if code != 0 {
		t.Fatalf("table --group: expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "chess-compartment") || !strings.Contains(out, `compartment "checkers-compartment"`) {
		t.Errorf("table --group: expected filtered rows and summary, got:\n%s", out)
	}

	for _, format := range []string{"table", "json"} {
		out, stderr, code := run(t, "groups", "--format", format, "--group", "nope", manifest)
		if code != 1 {
			t.Errorf("unknown group (%s): expected exit code 1, got %d", format, code)
		}
		if out != "" {
			t.Errorf("unknown group (%s): expected no stdout, got:\n%s", format, out)
		}
		if !strings.Contains(stderr, "nope") {
			t.Errorf("unknown group (%s): expected stderr to name the group, got: %s", format, stderr)
		}
	}
}

func TestFillRemainingAndFullWallsEndToEnd(t *testing.T) {
	manifest := examplePath(t, "full-walls-fill-remaining.yaml")

	out, _, code := run(t, "groups", manifest)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stdout: %s)", code, out)
	}
	if !strings.Contains(out, "0.0 x 0.0 mm remaining") {
		t.Fatalf("expected fillRemaining to consume all box space, got:\n%s", out)
	}

	for _, format := range []string{"console", "svg"} {
		out, errOut, code := run(t, "build", manifest, "--format", format)
		if code != 0 {
			t.Fatalf("format %s: expected exit 0, got %d (stderr: %s)", format, code, errOut)
		}
		if out == "" {
			t.Fatalf("format %s: expected non-empty output", format)
		}
	}
}

// manifest.Validate reports only errors (Issue has no severity), so --strict
// currently cannot change the outcome of a valid manifest; these tests pin
// that it neither breaks clean builds nor lets invalid ones write output.
func TestBuildStrictCleanManifestSucceeds(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	outDir := t.TempDir()

	_, errOut, code := run(t, "build", manifest, "--strict", "--format", "csv", "--out-dir", outDir)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, errOut)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(entries))
	}
}

func TestBuildStrictInvalidManifestWritesNothing(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("version: 1\nbox:\n  name: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "out")

	_, _, code := run(t, "build", bad, "--strict", "--format", "csv", "--out-dir", outDir)
	if code != cli.ExitValidationFailed {
		t.Fatalf("expected exit %d, got %d", cli.ExitValidationFailed, code)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("expected no output dir to be created, stat err: %v", err)
	}
}

func TestListCSVQuotesSpecialCharacters(t *testing.T) {
	name := "Tray, \"big\"\nline2"
	yaml := "version: 1\n" +
		"box:\n  name: \"B\"\n  units: mm\n  interior:\n    width: 100\n    depth: 100\n    height: 50\n" +
		"material:\n  name: \"M\"\n  thickness: 3.0\n  kerf: 0.0\n" +
		"defaults:\n  padding: 1.0\n  margin: 1.5\n" +
		"components:\n  - id: tray\n    name: \"Tray, \\\"big\\\"\\nline2\"\n    width: 20\n    depth: 20\n    height: 10\n    qty: 2\n"
	path := filepath.Join(t.TempDir(), "m.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := run(t, "list", "--format", "csv", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, errOut)
	}
	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v\n%s", err, out)
	}
	if len(recs) != 2 {
		t.Fatalf("expected header + 1 row, got %d records", len(recs))
	}
	want := []string{"tray", name, "20", "20", "10", "2", "-"}
	if strings.Join(recs[1], "|") != strings.Join(want, "|") {
		t.Fatalf("row = %q, want %q", recs[1], want)
	}
}

func TestBuildRejectsNegativeNumericFlags(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	for _, flag := range []string{"--sheet-width", "--nest-gap"} {
		out, errOut, code := run(t, "build", manifest, flag, "-5")
		if code != cli.ExitUsageError {
			t.Fatalf("%s -5: expected exit %d, got %d (stdout: %s)", flag, cli.ExitUsageError, code, out)
		}
		if !strings.Contains(errOut, flag) {
			t.Fatalf("%s: expected message naming the flag, got %q", flag, errOut)
		}
		if out != "" {
			t.Fatalf("%s: expected no output, got %q", flag, out)
		}
	}
}

func TestBuildOutWithMultipleFormatsWritesNothing(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")

	_, _, code := run(t, "build", manifest, "--format", "csv,svg", "--out", out)
	if code != cli.ExitUsageError {
		t.Fatalf("expected exit %d, got %d", cli.ExitUsageError, code)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files written, got %d", len(entries))
	}
}

func TestBuildFlagCombinationRules(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")

	tests := []struct {
		name string
		args func(dir string) []string
	}{
		{"multiple formats without out-dir", func(dir string) []string {
			return []string{"--format", "csv,svg"}
		}},
		{"multiple binary formats without out-dir", func(dir string) []string {
			return []string{"--format", "csv,iso-png"}
		}},
		{"out with out-dir", func(dir string) []string {
			return []string{"--format", "csv", "--out", filepath.Join(dir, "o.csv"), "--out-dir", filepath.Join(dir, "d")}
		}},
		{"duplicate formats", func(dir string) []string {
			return []string{"--format", "svg,svg", "--out-dir", filepath.Join(dir, "d")}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"build", manifest}, tt.args(dir)...)
			stdout, _, code := run(t, args...)
			if code != cli.ExitUsageError {
				t.Fatalf("expected exit %d, got %d", cli.ExitUsageError, code)
			}
			if stdout != "" {
				t.Fatalf("expected empty stdout, got %d bytes", len(stdout))
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("expected no files written, got %d", len(entries))
			}
		})
	}
}

func TestBuildMultipleFormatsWithOutDirWorks(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	dir := t.TempDir()
	stdout, errOut, code := run(t, "build", manifest, "--format", "csv,svg", "--out-dir", dir)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, errOut)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %d bytes", len(stdout))
	}
	for _, ext := range []string{"csv", "svg"} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*."+ext))
		if len(matches) != 1 {
			t.Fatalf("expected one .%s file, got %v", ext, matches)
		}
	}
}

func TestBuildSingleFormatToStdoutStillWorks(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	stdout, errOut, code := run(t, "build", manifest, "--format", "csv")
	if code != 0 || stdout == "" {
		t.Fatalf("expected exit 0 with output, got %d (stderr: %s)", code, errOut)
	}
}

func TestBuildBinaryFormatToNonTerminalWriterStillWorks(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	out, errOut, code := run(t, "build", manifest, "--format", "iso-png")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, errOut)
	}
	if !strings.HasPrefix(out, "\x89PNG") {
		t.Fatalf("expected PNG data on the piped writer")
	}
}

func TestBuildMaterialThicknessFollowsBoxUnits(t *testing.T) {
	const yamlTmpl = `version: 1
box:
  name: "U"
  units: %s
  interior: {width: %s, depth: %s, height: %s}
material:
  name: "ply"
  thickness: %s
  kerf: 0
defaults:
  removable: true
components:
  - {id: c, width: %s, depth: %s, height: %s, qty: 1}
groups:
  - {id: g, components: [c]}
`
	for _, tc := range []struct {
		units, dim, thick, comp, want string
	}{
		{"in", "8", "0.125", "2", "3.175"},
		{"cm", "20", "0.3", "5", "3"},
		{"mm", "200", "3", "50", "3"},
	} {
		src := fmt.Sprintf(yamlTmpl, tc.units, tc.dim, tc.dim, tc.dim, tc.thick, tc.comp, tc.comp, tc.comp)
		path := filepath.Join(t.TempDir(), "m.yaml")
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		out, errOut, code := run(t, "build", path, "--format", "csv")
		if code != 0 {
			t.Fatalf("%s: exit %d (stderr: %s)", tc.units, code, errOut)
		}
		rows, err := csv.NewReader(strings.NewReader(out)).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		col := -1
		for i, h := range rows[0] {
			if strings.Contains(strings.ToLower(h), "thick") {
				col = i
			}
		}
		if col < 0 || len(rows) < 2 {
			t.Fatalf("%s: no thickness column in %v", tc.units, rows[0])
		}
		for _, r := range rows[1:] {
			if r[col] != tc.want {
				t.Errorf("units %s: thickness %q, want %s mm (row %v)", tc.units, r[col], tc.want, r)
			}
		}
	}
}

// TestBuildWarnsOnUnmatchedHeightReductionPanels checks that panels entries
// matching no panel of the setting's class warn on stderr, and that --strict
// turns the warning into a failure before any output is written.
func TestBuildWarnsOnUnmatchedHeightReductionPanels(t *testing.T) {
	src, err := os.ReadFile(examplePath(t, "height-reduction.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// Keep the card-tray's valid "ih-*-1" entry and add three that match
	// nothing: a typo'd id, a glob owned by no tray, and a divider-class
	// pattern that only matches external panels.
	text := strings.Replace(string(src), `panels: ["ih-*-1"]`, `panels: ["ih-*-1", "h-99", "iv-gears-*"]`, 1)
	text = strings.Replace(text, "dividerHeightReduction: \"30%\"", "dividerHeightReduction:\n    by: \"30%\"\n    panels: [\"[w]-*\"]", 1)
	if text == string(src) {
		t.Fatal("fixture edit did not apply")
	}
	manifest := filepath.Join(t.TempDir(), "unmatched.yaml")
	if err := os.WriteFile(manifest, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errOut, code := run(t, "build", manifest, "--format", "csv")
	if code != 0 {
		t.Fatalf("expected exit 0 without --strict, got %d (stderr: %s)", code, errOut)
	}
	for _, want := range []string{
		`dividerHeightReduction panels entry "h-99" matches no divider panel`,
		`dividerHeightReduction panels entry "iv-gears-*" matches no divider panel`,
		`dividerHeightReduction panels entry "[w]-*" matches no divider panel (it only matches external panels)`,
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, `"ih-*-1"`) {
		t.Errorf("matched entry ih-*-1 must not warn:\n%s", errOut)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	_, _, code = run(t, "build", manifest, "--strict", "--format", "csv", "--out-dir", outDir)
	if code != cli.ExitValidationFailed {
		t.Fatalf("expected exit %d under --strict, got %d", cli.ExitValidationFailed, code)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("expected no output dir under --strict failure, stat err: %v", err)
	}

	// The shipped example has only matching entries and stays clean.
	_, errOut, code = run(t, "build", examplePath(t, "height-reduction.yaml"), "--strict", "--format", "csv")
	if code != 0 || strings.Contains(errOut, "warning") {
		t.Fatalf("clean example: exit %d, stderr: %s", code, errOut)
	}
}

// TestBuildWarnsOnUnusedExternalHeightReduction checks that an
// externalHeightReduction on trays with no external panels warns on stderr
// and fails under --strict, while trays where it applies stay quiet.
func TestBuildWarnsOnUnusedExternalHeightReduction(t *testing.T) {
	src, err := os.ReadFile(examplePath(t, "height-reduction.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// Turn fullWalls off for every tray: the card-tray's reduction of 5 now
	// has no external panel to lower.
	text := strings.Replace(string(src), "fullWalls: true", "fullWalls: false", 1)
	if text == string(src) {
		t.Fatal("fixture edit did not apply")
	}
	manifest := filepath.Join(t.TempDir(), "unused-external.yaml")
	if err := os.WriteFile(manifest, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errOut, code := run(t, "build", manifest, "--format", "csv")
	if code != 0 {
		t.Fatalf("expected exit 0 without --strict, got %d (stderr: %s)", code, errOut)
	}
	if want := "externalHeightReduction on tray(s) card-tray has no effect"; !strings.Contains(errOut, want) {
		t.Errorf("stderr missing %q:\n%s", want, errOut)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	_, _, code = run(t, "build", manifest, "--strict", "--format", "csv", "--out-dir", outDir)
	if code != cli.ExitValidationFailed {
		t.Fatalf("expected exit %d under --strict, got %d", cli.ExitValidationFailed, code)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("expected no output dir under --strict failure, stat err: %v", err)
	}
}

// reductionTooTallManifest returns a manifest whose divider reduction eats
// the whole panel height, so rendering fails in cutFor. The reduction is
// limited to panel ids because an unrestricted amount that large is already
// rejected by the manifest feasibility check.
func reductionTooTallManifest(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(examplePath(t, "height-reduction.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(src), "dividerHeightReduction: \"30%\"", "dividerHeightReduction: {by: 1000, panels: [\"h-*\", \"v-*\", \"iv-*\"]}", 1)
	if text == string(src) {
		t.Fatal("fixture edit did not apply")
	}
	p := filepath.Join(t.TempDir(), "too-tall.yaml")
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestBuildHeightReductionRenderErrorAllFormats checks that an impossible
// reduction fails build the same way under the default console format as
// under csv, and that validate reports it too.
func TestBuildHeightReductionRenderErrorAllFormats(t *testing.T) {
	manifest := reductionTooTallManifest(t)
	for _, args := range [][]string{{"build", manifest}, {"build", manifest, "--format", "csv"}} {
		_, errOut, code := run(t, args...)
		if code != cli.ExitValidationFailed {
			t.Fatalf("%v: expected exit %d, got %d (stderr: %s)", args, cli.ExitValidationFailed, code, errOut)
		}
		if !strings.Contains(errOut, "height reduction") || !strings.Contains(errOut, "leaves nothing") {
			t.Errorf("%v: stderr missing reduction error:\n%s", args, errOut)
		}
	}
}

func TestValidateHeightReductionRenderError(t *testing.T) {
	manifest := reductionTooTallManifest(t)
	out, _, code := run(t, "validate", manifest)
	if code != cli.ExitValidationFailed {
		t.Fatalf("expected exit %d, got %d (stdout: %s)", cli.ExitValidationFailed, code, out)
	}
	if !strings.Contains(out, "leaves nothing") {
		t.Errorf("stdout missing reduction error:\n%s", out)
	}
	_, _, code = run(t, "validate", examplePath(t, "height-reduction.yaml"))
	if code != 0 {
		t.Fatalf("shipped example should validate, got exit %d", code)
	}
}

func TestBuildFailingFormatDoesNotCorruptOtherOutputs(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	dir := t.TempDir()
	if _, errOut, code := run(t, "build", manifest, "--format", "csv,svg", "--out-dir", dir); code != 0 {
		t.Fatalf("setup build failed: %d (%s)", code, errOut)
	}
	csvPaths, _ := filepath.Glob(filepath.Join(dir, "*.csv"))
	svgPaths, _ := filepath.Glob(filepath.Join(dir, "*.svg"))
	if len(csvPaths) != 1 || len(svgPaths) != 1 {
		t.Fatalf("setup: unexpected outputs %v %v", csvPaths, svgPaths)
	}
	csvPath, svgPath := csvPaths[0], svgPaths[0]
	// Make the svg output impossible to write: its path is a directory.
	if err := os.Remove(svgPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(svgPath, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(csvPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	// svg fails first, so the csv that comes after it is never touched.
	_, _, code := run(t, "build", manifest, "--format", "svg,csv", "--out-dir", dir)
	if code == 0 {
		t.Fatal("expected failure when the svg output cannot be written")
	}
	if got, _ := os.ReadFile(csvPath); string(got) != "old" {
		t.Fatalf("existing csv must be untouched, got %d bytes", len(got))
	}

	// All-or-nothing: csv is exported before the failing svg, but nothing is
	// committed, so the existing csv stays byte-identical.
	_, _, code = run(t, "build", manifest, "--format", "csv,svg", "--out-dir", dir)
	if code == 0 {
		t.Fatal("expected failure when the svg output cannot be written")
	}
	if got, _ := os.ReadFile(csvPath); string(got) != "old" {
		t.Fatalf("csv must not be replaced when a later format fails, got %d bytes", len(got))
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func listDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			got[e.Name()] = "<dir>"
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		got[e.Name()] = string(b)
	}
	return got
}

func TestBuildMultiFormatFailureLeavesNoNewFiles(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	dir := t.TempDir()
	// The dxf target is a directory, so the second format fails.
	if err := os.MkdirAll(filepath.Join(dir, "chess-checkers-combo.dxf", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chess-checkers-combo.csv"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := listDir(t, dir)
	_, _, code := run(t, "build", manifest, "--format", "csv,dxf,svg", "--out-dir", dir)
	if code == 0 {
		t.Fatal("expected failure")
	}
	after := listDir(t, dir)
	if len(before) != len(after) {
		t.Fatalf("directory changed: before %v after %v", before, after)
	}
	for name, c := range before {
		if after[name] != c {
			t.Fatalf("%s changed", name)
		}
	}
	// A new format listed before the failing one must not appear either.
	_, _, code = run(t, "build", manifest, "--format", "svg,dxf", "--out-dir", dir)
	if code == 0 {
		t.Fatal("expected failure")
	}
	if got := listDir(t, dir); len(got) != len(before) {
		t.Fatalf("svg left behind: %v", got)
	}
}

func TestBuildMultiFormatSuccessCommitsAllFiles(t *testing.T) {
	manifest := examplePath(t, "chess-checkers.yaml")
	dir := t.TempDir()
	old := filepath.Join(dir, "chess-checkers-combo.csv")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := run(t, "build", manifest, "--format", "csv,svg,dxf", "--out-dir", dir); code != 0 {
		t.Fatalf("build failed: %d (%s)", code, errOut)
	}
	got := listDir(t, dir)
	if len(got) != 3 {
		t.Fatalf("expected 3 files, got %v", got)
	}
	for name, c := range got {
		if c == "" || c == "old" || strings.HasPrefix(name, ".") {
			t.Fatalf("unexpected file %s (%d bytes)", name, len(c))
		}
	}
}

func TestBuildMultiFormatOverfullStillWritesUsableFormats(t *testing.T) {
	manifest := examplePath(t, "overfull.yaml")
	dir := t.TempDir()
	_, errOut, code := run(t, "build", manifest, "--format", "csv,stl,svg", "--out-dir", dir)
	if code != 3 {
		t.Fatalf("expected exit 3, got %d (%s)", code, errOut)
	}
	if !strings.Contains(errOut, "skipping stl") {
		t.Fatalf("expected a skipping note, got %q", errOut)
	}
	got := listDir(t, dir)
	if len(got) != 2 {
		t.Fatalf("expected the usable formats to be written, got %v", got)
	}
	for name := range got {
		if strings.HasPrefix(name, ".") {
			t.Fatalf("temp file left behind: %s", name)
		}
	}
}

// TestListAndGroupsPrintValidationIssues checks that list and groups print
// each validation issue (path: message, as build does) to stderr before
// failing with the validation exit code.
func TestListAndGroupsPrintValidationIssues(t *testing.T) {
	invalid := fixture(t, "invalid.yaml")
	_, buildErr, code := run(t, "build", invalid)
	if code != cli.ExitValidationFailed {
		t.Fatalf("build: expected exit %d, got %d", cli.ExitValidationFailed, code)
	}
	var issues []string
	for _, line := range strings.Split(buildErr, "\n") {
		if strings.Contains(line, ": ") && !strings.HasPrefix(line, "error:") {
			issues = append(issues, line)
		}
	}
	if len(issues) == 0 {
		t.Fatalf("build printed no issues:\n%s", buildErr)
	}
	for _, cmd := range []string{"list", "groups"} {
		_, errOut, code := run(t, cmd, invalid)
		if code != cli.ExitValidationFailed {
			t.Fatalf("%s: expected exit %d, got %d", cmd, cli.ExitValidationFailed, code)
		}
		for _, iss := range issues {
			if !strings.Contains(errOut, iss+"\n") {
				t.Errorf("%s: stderr missing issue %q:\n%s", cmd, iss, errOut)
			}
		}
	}
}

// TestValidateStrictReductionWarnings checks that validate reports the
// unmatched/unused height-reduction warnings build raises: as warning lines
// with exit 0 by default, and as a failure (exit 2) under --strict.
func TestValidateStrictReductionWarnings(t *testing.T) {
	src, err := os.ReadFile(examplePath(t, "height-reduction.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(src), `panels: ["ih-*-1"]`, `panels: ["ih-*-1", "h-99"]`, 1)
	if text == string(src) {
		t.Fatal("fixture edit did not apply")
	}
	manifest := filepath.Join(t.TempDir(), "unmatched.yaml")
	if err := os.WriteFile(manifest, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	want := `dividerHeightReduction panels entry "h-99" matches no divider panel`

	out, errOut, code := run(t, "validate", manifest)
	if code != 0 || !strings.Contains(out, "OK") || !strings.Contains(errOut, "warning: "+want) {
		t.Fatalf("without --strict: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	out, errOut, code = run(t, "validate", manifest, "--strict")
	if code != cli.ExitValidationFailed || !strings.Contains(errOut, "warning: "+want) || strings.Contains(out, "OK") {
		t.Fatalf("--strict: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	out, _, code = run(t, "validate", manifest, "--format", "json")
	var got struct {
		Issues   []any    `json:"issues"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 || len(got.Warnings) != 1 || len(got.Issues) != 0 {
		t.Fatalf("json: exit %d, err %v, out %s", code, err, out)
	}

	// A clean manifest has no warnings and no warnings key in JSON.
	clean := examplePath(t, "height-reduction.yaml")
	out, errOut, code = run(t, "validate", clean, "--strict", "--format", "json")
	if code != 0 || errOut != "" || strings.Contains(out, "warnings") {
		t.Fatalf("clean --strict: exit %d, stdout %q, stderr %q", code, out, errOut)
	}

	// Unused external reduction.
	text = strings.Replace(string(src), "fullWalls: true", "fullWalls: false", 1)
	unused := filepath.Join(t.TempDir(), "unused.yaml")
	if err := os.WriteFile(unused, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, code = run(t, "validate", unused, "--strict")
	if code != cli.ExitValidationFailed || !strings.Contains(errOut, "externalHeightReduction on tray(s) card-tray has no effect") {
		t.Fatalf("unused --strict: exit %d, stderr %q", code, errOut)
	}
}

// groupsJSONTotals is the part of `groups --format json` that --group scopes.
type groupsJSONTotals struct {
	Compartments []struct {
		ID      string `json:"id"`
		Missing []struct {
			ComponentID string `json:"componentId"`
			Rejected    int    `json:"rejected"`
		} `json:"missing"`
		DividerReduction struct {
			AmountMM float64 `json:"amountMM"`
		} `json:"dividerReduction"`
	} `json:"compartments"`
	TotalMissing []struct {
		ComponentID string `json:"componentId"`
		Rejected    int    `json:"rejected"`
	} `json:"totalMissing"`
}

// TestGroupsGroupFilterJSONTotalsAndExitCodeWithHeightReduction checks that
// --group scopes totalMissing and the exit code to the selected compartment
// on a manifest that sets height reductions.
func TestGroupsGroupFilterJSONTotalsAndExitCodeWithHeightReduction(t *testing.T) {
	const defaults = "  externalHeightReduction: 4\n  dividerHeightReduction: 6"
	groups := func(path string, args ...string) (groupsJSONTotals, int) {
		t.Helper()
		out, stderr, code := run(t, append([]string{"groups", "--format", "json"}, append(args, path)...)...)
		var res groupsJSONTotals
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("%v: invalid JSON: %v\nstdout: %s\nstderr: %s", args, err, out, stderr)
		}
		return res, code
	}

	// Everything fits: a filtered report succeeds and carries the reduction.
	res, code := groups(writeHRManifest(t, defaults, ""), "--group", "gears")
	if code != 0 {
		t.Errorf("--group gears: exit %d, want 0", code)
	}
	if len(res.Compartments) != 1 || res.Compartments[0].ID != "gears" {
		t.Fatalf("--group gears: compartments = %+v", res.Compartments)
	}
	if got := res.Compartments[0].DividerReduction.AmountMM; got != 6 {
		t.Errorf("--group gears: dividerReduction amountMM = %v, want 6", got)
	}
	if len(res.TotalMissing) != 0 {
		t.Errorf("--group gears: totalMissing = %+v, want empty", res.TotalMissing)
	}

	// 12 cards overflow the box: 2 cards and the 4 gears squeezed out do not
	// fit. --group card-tray scopes totals and exit code to the card tray.
	path := writeHRManifest(t, defaults, "")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(src), "{id: cards, width: 35, depth: 15, height: 8, qty: 4,", "{id: cards, width: 35, depth: 15, height: 8, qty: 12,", 1)
	if text == string(src) {
		t.Fatal("fixture edit did not apply")
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	res, code = groups(path, "--group", "card-tray")
	if code != 3 {
		t.Errorf("--group card-tray: exit %d, want 3", code)
	}
	if len(res.Compartments) != 1 || res.Compartments[0].ID != "card-tray" {
		t.Fatalf("--group card-tray: compartments = %+v", res.Compartments)
	}
	if got := res.Compartments[0].DividerReduction.AmountMM; got != 6 {
		t.Errorf("--group card-tray: dividerReduction amountMM = %v, want 6", got)
	}
	if len(res.TotalMissing) != 1 || res.TotalMissing[0].ComponentID != "cards" || res.TotalMissing[0].Rejected != 2 {
		t.Fatalf("--group card-tray: totalMissing = %+v, want 2 cards", res.TotalMissing)
	}

	// Unfiltered, the box-level total also counts the displaced gears.
	all, code := groups(path)
	if code != 3 {
		t.Errorf("no --group: exit %d, want 3", code)
	}
	n := 0
	for _, m := range all.TotalMissing {
		n += m.Rejected
	}
	if n != 6 {
		t.Errorf("no --group: total rejected = %d, want 6 (%+v)", n, all.TotalMissing)
	}
}
