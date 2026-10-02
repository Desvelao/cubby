package slotted

import (
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/pack"
)

func TestUnusedExternalReductions(t *testing.T) {
	ext := pack.HeightReduction{AmountMM: 5}
	other := pack.HeightReduction{Percent: 10}
	tray := func(id string, full, removable bool, r pack.HeightReduction) pack.CompartmentResult {
		cr := fwCell(id, 0, 0, 50, 50, full)
		cr.Removable = removable
		cr.ExternalReduction = r
		return cr
	}
	tests := []struct {
		name  string
		trays []pack.CompartmentResult
		want  []string // substrings, one per expected warning
	}{
		{"no reduction", []pack.CompartmentResult{tray("a", false, false, pack.HeightReduction{})}, nil},
		{"panels-only filter is not a setting", []pack.CompartmentResult{tray("a", false, false, pack.HeightReduction{Panels: []string{"w-*"}})}, nil},
		{"fullWalls applies", []pack.CompartmentResult{tray("a", true, false, ext)}, nil},
		{"removable applies", []pack.CompartmentResult{tray("a", false, true, ext)}, nil},
		{"plain tray warns", []pack.CompartmentResult{tray("a", false, false, ext)}, []string{"tray(s) a has no effect"}},
		{"percent warns", []pack.CompartmentResult{tray("a", false, false, other)}, []string{"tray(s) a "}},
		{"inherited value used by one tray stays quiet",
			[]pack.CompartmentResult{tray("a", false, false, ext), tray("b", true, false, ext)}, nil},
		{"inherited value unused by all warns once",
			[]pack.CompartmentResult{tray("a", false, false, ext), tray("b", false, false, ext)}, []string{"tray(s) a, b "}},
		{"different values judged separately",
			[]pack.CompartmentResult{tray("a", false, false, ext), tray("b", true, false, other)}, []string{"tray(s) a "}},
		{"zero override on plain tray is quiet",
			[]pack.CompartmentResult{tray("a", false, false, pack.HeightReduction{}), tray("b", true, false, ext)}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := UnusedExternalReductions(pack.BoxResult{Compartments: tc.trays})
			if len(got) != len(tc.want) {
				t.Fatalf("got %d warnings %q, want %d", len(got), got, len(tc.want))
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) || !strings.Contains(got[i], "externalHeightReduction") {
					t.Errorf("warning %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}
