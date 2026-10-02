package e2e

import (
	"bytes"
	"testing"
	"time"

	"github.com/Desvelao/cubby/internal/export/assembly"
	"github.com/Desvelao/cubby/internal/export/dxf"
	"github.com/Desvelao/cubby/internal/export/step"
	"github.com/Desvelao/cubby/internal/export/stl"
	"github.com/Desvelao/cubby/internal/export/svg"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// TestEveryExporterFallsBackToFullDepthNotchesForReducedPanel exports a
// notched, height-reduced panel in every geometry format, once with its
// Outline populated and once with it empty. The fallback must reproduce the
// populated outline (notch depth 50 = full height 100 / 2, not the reduced
// 80 / 2), so both exports are byte-identical.
func TestEveryExporterFallsBackToFullDepthNotchesForReducedPanel(t *testing.T) {
	notches := []geometry.Notch{{Pos: 40, Width: 3, Edge: geometry.EdgeBottom}}
	withOutline := geometry.Panel{
		ID: "v-1", Axis: geometry.AxisDepthRun,
		Length: 100, Height: 80, Cut: 20, Thickness: 3,
		Notches: notches,
		Outline: geometry.BuildOutlineCut(100, 100, 20, notches),
	}
	noOutline := withOutline
	noOutline.Outline = nil

	now := func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	box := pack.BoxResult{BoxName: "Test", InteriorW: 300, InteriorD: 200, InteriorH: 100}
	mat := manifest.Material{Name: "Foamboard", Thickness: 3}
	formats := []struct {
		name   string
		export func(buf *bytes.Buffer, p geometry.Panel) error
	}{
		{"svg", func(b *bytes.Buffer, p geometry.Panel) error {
			return (svg.Exporter{}).Export(b, box, []geometry.Panel{p}, mat)
		}},
		{"dxf", func(b *bytes.Buffer, p geometry.Panel) error {
			return (dxf.Exporter{}).Export(b, box, []geometry.Panel{p}, mat)
		}},
		{"assembly", func(b *bytes.Buffer, p geometry.Panel) error {
			return (assembly.Exporter{Now: now}).Export(b, box, []geometry.Panel{p}, mat)
		}},
		{"stl", func(b *bytes.Buffer, p geometry.Panel) error {
			return (stl.Exporter{}).Export(b, box, []geometry.Panel{p}, mat)
		}},
		{"step", func(b *bytes.Buffer, p geometry.Panel) error {
			return (step.Exporter{Now: now}).Export(b, box, []geometry.Panel{p}, mat)
		}},
	}
	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			var want, got bytes.Buffer
			if err := f.export(&want, withOutline); err != nil {
				t.Fatal(err)
			}
			if err := f.export(&got, noOutline); err != nil {
				t.Fatal(err)
			}
			if want.Len() == 0 || !bytes.Equal(want.Bytes(), got.Bytes()) {
				t.Fatalf("export without Outline differs from export with Outline (%d vs %d bytes)", got.Len(), want.Len())
			}
		})
	}
}
