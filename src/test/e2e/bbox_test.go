package e2e

import (
	"bytes"
	"encoding/binary"
	"math"
	"regexp"
	"strconv"
	"testing"

	"github.com/Desvelao/cubby/internal/export/isometric"
	"github.com/Desvelao/cubby/internal/export/step"
	"github.com/Desvelao/cubby/internal/export/stl"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

type bounds struct{ min, max [3]float64 }

func newBounds() bounds {
	var b bounds
	for i := range b.min {
		b.min[i], b.max[i] = math.Inf(1), math.Inf(-1)
	}
	return b
}

func (b *bounds) add(p [3]float64) {
	for i, v := range p {
		b.min[i], b.max[i] = math.Min(b.min[i], v), math.Max(b.max[i], v)
	}
}

func (b bounds) equalWithin(o bounds, tol float64) bool {
	for i := range b.min {
		if math.Abs(b.min[i]-o.min[i]) > tol || math.Abs(b.max[i]-o.max[i]) > tol {
			return false
		}
	}
	return true
}

// TestExportersAgreeOnWorldBoundingBox asserts STL, STEP and the isometric
// scene's box placement all yield the same world-space bounding box for a
// multi-axis, notched panel set at non-zero origins.
func TestExportersAgreeOnWorldBoundingBox(t *testing.T) {
	notch := []geometry.Notch{{Pos: 40, Width: 3, Edge: geometry.EdgeTop}}
	panels := []geometry.Panel{
		{ID: "w", Axis: geometry.AxisWidthRun, Length: 120, Height: 50, Thickness: 3, Notches: notch,
			Position: geometry.Placement3D{OriginX: 10, OriginY: 30, OriginZ: 5}},
		{ID: "d", Axis: geometry.AxisDepthRun, Length: 90, Height: 50, Thickness: 3, Notches: notch,
			Position: geometry.Placement3D{OriginX: 60, OriginY: 7, OriginZ: 5}},
		{ID: "f", Axis: geometry.AxisFloor, Length: 140, Height: 100, Thickness: 3,
			Position: geometry.Placement3D{OriginX: 4, OriginY: 9, OriginZ: 1}},
	}
	for i := range panels {
		panels[i].Outline = geometry.BuildOutline(panels[i].Length, panels[i].Height, panels[i].Notches)
	}
	box := pack.BoxResult{}
	mat := manifest.Material{Thickness: 3}

	var stlBuf bytes.Buffer
	if err := (stl.Exporter{}).Export(&stlBuf, box, panels, mat); err != nil {
		t.Fatal(err)
	}
	data := stlBuf.Bytes()
	n := int(binary.LittleEndian.Uint32(data[80:84]))
	stlB := newBounds()
	for i := 0; i < n; i++ {
		rec := data[84+i*50 : 84+(i+1)*50]
		for v := 0; v < 3; v++ { // skip the 12-byte normal
			var p [3]float64
			for c := 0; c < 3; c++ {
				off := 12 + v*12 + c*4
				p[c] = float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[off : off+4])))
			}
			stlB.add(p)
		}
	}

	var stepBuf bytes.Buffer
	if err := (step.Exporter{}).Export(&stepBuf, box, panels, mat); err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`CARTESIAN_POINT\('',\(([^,]+),([^,]+),([^)]+)\)\)`)
	stepB := newBounds()
	matches := re.FindAllStringSubmatch(stepBuf.String(), -1)
	if len(matches) == 0 {
		t.Fatal("no CARTESIAN_POINTs found in STEP output")
	}
	for _, m := range matches {
		var p [3]float64
		for c := 0; c < 3; c++ {
			v, err := strconv.ParseFloat(m[c+1], 64)
			if err != nil {
				t.Fatal(err)
			}
			p[c] = v
		}
		stepB.add(p)
	}

	isoB := newBounds()
	isoMin, isoMax := isometric.BoxBounds(panels)
	isoB.add(isoMin)
	isoB.add(isoMax)

	if !stlB.equalWithin(stepB, 1e-4) {
		t.Errorf("STL bounds %v != STEP bounds %v", stlB, stepB)
	}
	if !isoB.equalWithin(stepB, 1e-4) {
		t.Errorf("isometric bounds %v != STEP bounds %v", isoB, stepB)
	}
	want := bounds{min: [3]float64{4, 7, 1}, max: [3]float64{144, 109, 55}}
	if !stepB.equalWithin(want, 1e-4) {
		t.Errorf("STEP bounds %v, want %v", stepB, want)
	}
}
