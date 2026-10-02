package e2e

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const projectManifest = `version: 1
project:
  name: "Chess & Checkers <Insert>"
  description: "Organizer with 'quotes' and \"doubles\"."
  revision: "2.0"
  author: "José Ñandú — 東"
  license: CC0-1.0
  url: "https://example.com/x?a=1&b=2"
  tags: [chess, "a&b"]
  created: 2026-09-30
box:
  name: Box
  units: mm
  interior: {width: 100, depth: 100, height: 50}
material: {name: Foam, thickness: 3}
components:
  - {id: a, name: A, width: 30, depth: 30, height: 20, qty: 2}
`

func TestBuildProjectMetadataEndToEnd(t *testing.T) {
	dir := t.TempDir()
	m := filepath.Join(dir, "m.yaml")
	if err := os.WriteFile(m, []byte(projectManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if _, errOut, code := run(t, "build", m, "--format", "svg,step,console", "--out-dir", outDir); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	svg, err := os.ReadFile(filepath.Join(outDir, "box.svg"))
	if err != nil {
		t.Fatal(err)
	}
	dec := xml.NewDecoder(strings.NewReader(string(svg)))
	for {
		if _, err := dec.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("svg is not well-formed XML: %v", err)
		}
	}
	for _, want := range []string{
		"<title>Chess &amp; Checkers &lt;Insert&gt;</title>",
		"<dc:creator>José Ñandú — 東</dc:creator>",
		"<dc:rights>CC0-1.0</dc:rights>",
		"<dc:subject>a&amp;b</dc:subject>",
		"<dc:date>2026-09-30</dc:date>",
		"<cubby:revision>2.0</cubby:revision>",
	} {
		if !strings.Contains(string(svg), want) {
			t.Errorf("svg missing %q", want)
		}
	}

	stepOut, err := os.ReadFile(filepath.Join(outDir, "box.step"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stepOut), `('Jos\X2\00E9\X0\ \X2\00D1\X0\and\X2\00FA\X0\ \X2\2014\X0\ \X2\6771\X0\')`) {
		t.Errorf("step FILE_NAME author missing:\n%s", string(stepOut)[:400])
	}
	if !strings.Contains(string(stepOut), "'Organizer with ''quotes'' and \"doubles\".'") {
		t.Errorf("step PRODUCT description missing")
	}

	out, _, _ := run(t, "build", m, "--format", "console")
	if !strings.Contains(out, "Project: Chess & Checkers <Insert> (rev 2.0) by José Ñandú — 東\n") {
		t.Errorf("console project line missing:\n%s", out)
	}
}

func TestGroupsJSONProjectKey(t *testing.T) {
	// With project metadata the key is present with camelCase fields ...
	out, _, code := run(t, "groups", examplePath(t, "chess-checkers.yaml"), "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var got struct {
		Project map[string]any `json:"project"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Project["name"] != "Chess & Checkers Insert" || got.Project["revision"] != "1.0" || got.Project["created"] != "2026-09-30" {
		t.Errorf("project = %v", got.Project)
	}
	if tags, _ := got.Project["tags"].([]any); len(tags) != 2 {
		t.Errorf("tags = %v", got.Project["tags"])
	}

	// ... and without it the key is absent entirely.
	out, _, _ = run(t, "groups", examplePath(t, "plain-joints.yaml"), "--format", "json")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["project"]; ok {
		t.Error(`"project" key present for a manifest without project`)
	}
}
