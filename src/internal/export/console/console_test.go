package console

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

var testMat = manifest.Material{Name: "Foamboard", Thickness: 3, Kerf: 0.2}

func sampleBox() pack.BoxResult {
	return pack.BoxResult{
		BoxName:   "Test",
		InteriorW: 200, InteriorD: 100, InteriorH: 50,
		Compartments: []pack.CompartmentResult{
			{ID: "c1", Name: "Cards", Kind: "group", UsedW: 60, UsedD: 40, Remaining: pack.Space{Width: 140, Depth: 60}},
			{ID: "c2", Kind: "auto", UsedW: 10, UsedD: 10},
		},
		UsedW: 70, UsedD: 50,
		Remaining: pack.Space{Width: 130, Depth: 50},
	}
}

func render(t *testing.T, box pack.BoxResult, panels []geometry.Panel) string {
	t.Helper()
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, box, panels, testMat); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestExportHeaderTableAndTotals(t *testing.T) {
	out := render(t, sampleBox(), nil)
	for _, want := range []string{
		"Box: Test (interior 200.0 x 100.0 x 50.0 mm)",
		"Material: Foamboard, 3.00mm thick, 0.20mm kerf",
		"COMPARTMENT", "Cards", "c2", // unnamed compartment falls back to ID
		"Box total: used 70.0 x 50.0 of 200.0 x 100.0 mm, 130.0 x 50.0 mm remaining",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestExportPanelsCountLine(t *testing.T) {
	if out := render(t, sampleBox(), nil); strings.Contains(out, "Panels:") {
		t.Errorf("no panels: expected no Panels line:\n%s", out)
	}
	panels := []geometry.Panel{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if out := render(t, sampleBox(), panels); !strings.Contains(out, "Panels: 3\n") {
		t.Errorf("expected 'Panels: 3' line:\n%s", out)
	}
}

func TestExportMissingWarningBlock(t *testing.T) {
	if out := render(t, sampleBox(), nil); strings.Contains(out, "WARNING") {
		t.Errorf("no missing items: expected no WARNING:\n%s", out)
	}

	// Items with zero rejected must not trigger the warning either.
	box := sampleBox()
	box.TotalMissing = []pack.MissingItem{{ComponentID: "ok", Requested: 2, Placed: 2}}
	if out := render(t, box, nil); strings.Contains(out, "WARNING") {
		t.Errorf("zero rejected: expected no WARNING:\n%s", out)
	}

	box = sampleBox()
	box.Compartments[0].Missing = []pack.MissingItem{{ComponentID: "die", Requested: 5, Placed: 3, Rejected: 2, Reason: "no-space"}}
	box.TotalMissing = []pack.MissingItem{
		{ComponentID: "die", Requested: 5, Placed: 3, Rejected: 2, Reason: "no-space"},
		{ComponentID: "tray", Requested: 1, Placed: 0, Rejected: 1, Reason: "too-wide"},
	}
	out := render(t, box, nil)
	for _, want := range []string{
		"WARNING: 3 component instance(s) did not fit:",
		"- die: requested 5, placed 3, rejected 2 (no-space)",
		"- tray: requested 1, placed 0, rejected 1 (too-wide)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
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
	var full bytes.Buffer
	box := sampleBox()
	box.TotalMissing = []pack.MissingItem{{ComponentID: "die", Requested: 1, Rejected: 1, Reason: "no-space"}}
	panels := []geometry.Panel{{ID: "a"}}
	if err := (Exporter{}).Export(&full, box, panels, testMat); err != nil {
		t.Fatal(err)
	}
	// Fail at every possible byte budget: header, table (tabwriter flush),
	// panels/totals, and warning block must all surface the error.
	for n := 0; n < full.Len(); n += 7 {
		err := (Exporter{}).Export(&failAfter{n: n, err: boom}, box, panels, testMat)
		if !errors.Is(err, boom) {
			t.Fatalf("budget %d: expected boom, got %v", n, err)
		}
	}
}

func TestExportProjectLines(t *testing.T) {
	tests := []struct {
		name    string
		project *manifest.Project
		want    string // exact text between the Material line and the blank line before the table
	}{
		{"nil", nil, ""},
		{"empty", &manifest.Project{}, ""},
		{"full", &manifest.Project{Name: "Gateway Insert", Revision: "1.2", Author: "José Ñandú — 東", Description: "Two tiers,\n  for tokens"},
			"Project: Gateway Insert (rev 1.2) by José Ñandú — 東\nDescription: Two tiers, for tokens\n"},
		{"name only", &manifest.Project{Name: "N"}, "Project: N\n"},
		{"revision only", &manifest.Project{Revision: "2"}, "Project: (rev 2)\n"},
		{"author only", &manifest.Project{Author: "Ann"}, "Project: by Ann\n"},
		{"description only", &manifest.Project{Description: "D"}, "Description: D\n"},
		{"unrelated fields only", &manifest.Project{License: "MIT", Tags: []string{"x"}}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			box := sampleBox()
			box.Project = tc.project
			out := render(t, box, nil)
			const material = "Material: Foamboard, 3.00mm thick, 0.20mm kerf\n"
			want := material + tc.want + "\nCOMPARTMENT"
			if !strings.Contains(out, want) {
				t.Errorf("output does not contain %q:\n%s", want, out)
			}
		})
	}
}
