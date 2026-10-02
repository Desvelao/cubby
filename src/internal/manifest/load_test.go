package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "m.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const loadValid = `version: 1
box:
  name: Box
  units: mm
  interior: {width: 100, depth: 100, height: 50}
`

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string // empty means success
	}{
		{"valid", loadValid, ""},
		{"empty", "", "manifest is empty"},
		{"whitespace", "  \n\n   \n", "manifest is empty"},
		{"comment only", "# nothing here\n# at all\n", "manifest is empty"},
		{"trailing document", loadValid + "---\nversion: 2\n", "multiple YAML documents"},
		{"trailing empty document", loadValid + "---\n", "multiple YAML documents"},
		{"unknown field", loadValid + "bogus: 1\n", "field bogus not found"},
		{"malformed", "version: [1\n", "parse manifest:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Load(writeTemp(t, tc.content))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if m.Version != 1 || m.Box.Name != "Box" {
					t.Fatalf("unexpected manifest: %+v", m)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadProject(t *testing.T) {
	m, err := Load(writeTemp(t, loadValid+`project:
  name: Insert
  revision: 1.2
  created: 2026-09-30
  updated: "2026-10-01"
  tags: [a, 2026]
`))
	if err != nil {
		t.Fatal(err)
	}
	p := m.Project
	// Unquoted dates and numbers keep their authored text.
	if p.Name != "Insert" || p.Revision != "1.2" || p.Created != "2026-09-30" || p.Updated != "2026-10-01" {
		t.Fatalf("unexpected project: %+v", p)
	}
	if len(p.Tags) != 2 || p.Tags[1] != "2026" {
		t.Fatalf("tags = %v", p.Tags)
	}

	_, err = Load(writeTemp(t, loadValid+"project:\n  name: X\n  bogus: 1\n"))
	if err == nil || !strings.Contains(err.Error(), "field bogus not found") {
		t.Fatalf("unknown project key: err = %v", err)
	}
}
