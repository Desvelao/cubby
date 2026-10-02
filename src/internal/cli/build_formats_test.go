package cli

import (
	"testing"

	"github.com/Desvelao/cubby/internal/export"
)

func TestAvailableFormatsIsSortedAndStable(t *testing.T) {
	registry := map[string]export.Exporter{
		"svg": nil, "console": nil, "stl": nil, "csv": nil, "dxf": nil,
		"step": nil, "iso-svg": nil, "assembly": nil, "iso-png": nil,
	}
	const want = "assembly, console, csv, dxf, iso-png, iso-svg, step, stl, svg"
	for i := 0; i < 20; i++ {
		if got := availableFormats(registry); got != want {
			t.Fatalf("run %d: availableFormats = %q, want %q", i, got, want)
		}
	}
}
