package export_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/export/console"
	"github.com/Desvelao/cubby/internal/export/dxf"
	"github.com/Desvelao/cubby/internal/export/stl"
	"github.com/Desvelao/cubby/internal/export/svg"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func caseFixture() (pack.BoxResult, []geometry.Panel, manifest.Material) {
	box := pack.BoxResult{BoxName: "T", InteriorW: 300, InteriorD: 200, InteriorH: 60}
	panels := []geometry.Panel{{ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60,
		Thickness: 3, Outline: geometry.BuildOutline(300, 60, nil)}}
	return box, panels, manifest.Material{Name: "m", Thickness: 3}
}

func render(t *testing.T, e export.Exporter) string {
	t.Helper()
	box, panels, mat := caseFixture()
	var buf bytes.Buffer
	if err := e.Export(&buf, box, panels, mat); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestBoxCaseTextOutputs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		off, on export.Exporter
		marker  string
	}{
		{"console", console.Exporter{}, console.Exporter{BoxCase: true}, "Box case:"},
		{"svg", svg.Exporter{}, svg.Exporter{BoxCase: true}, `id="box-case"`},
		{"dxf", dxf.Exporter{}, dxf.Exporter{BoxCase: true}, "BOX_CASE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Contains(render(t, tc.off), tc.marker) {
				t.Errorf("%s present without BoxCase", tc.marker)
			}
			if !strings.Contains(render(t, tc.on), tc.marker) {
				t.Errorf("%s missing with BoxCase", tc.marker)
			}
		})
	}
}

func TestBoxCaseSTLAddsTriangles(t *testing.T) {
	off, on := len(render(t, stl.Exporter{})), len(render(t, stl.Exporter{BoxCase: true}))
	if on <= off {
		t.Errorf("stl with box case (%d bytes) not larger than without (%d)", on, off)
	}
}
