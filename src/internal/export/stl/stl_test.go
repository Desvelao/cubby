package stl

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func TestExportBinarySTLStructure(t *testing.T) {
	panel := geometry.Panel{
		ID: "h-1", Axis: geometry.AxisWidthRun,
		Length: 300, Height: 60, Thickness: 3,
		Outline:  geometry.BuildOutline(300, 60, nil),
		Position: geometry.Placement3D{},
	}

	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, []geometry.Panel{panel}, manifest.Material{Thickness: 3}); err != nil {
		t.Fatal(err)
	}

	data := buf.Bytes()
	if len(data) < 84 {
		t.Fatalf("output too short to be a valid binary STL: %d bytes", len(data))
	}
	count := binary.LittleEndian.Uint32(data[80:84])
	// no notches -> 1 decomposed rectangle -> 2 (top) + 2 (bottom) + 4*2 (sides) = 12 triangles
	if count != 12 {
		t.Fatalf("expected 12 triangles for a single un-notched panel, got %d", count)
	}

	expectedLen := 84 + int(count)*50 // 50 bytes/triangle: 12 floats (48B) + 2B attribute count
	if len(data) != expectedLen {
		t.Fatalf("expected %d bytes, got %d", expectedLen, len(data))
	}
}

func notchedPanel(axis geometry.Axis, edge geometry.Edge) geometry.Panel {
	return geometry.Panel{
		ID: "p", Axis: axis, Length: 300, Height: 60, Thickness: 3,
		Notches: []geometry.Notch{
			{Pos: 40, Width: 3, Edge: edge},
			{Pos: 120, Width: 5, Edge: edge},
		},
		Position: geometry.Placement3D{OriginX: 10, OriginY: 20, OriginZ: 30},
	}
}

func TestPanelTrianglesCountAndVolume(t *testing.T) {
	for _, edge := range []geometry.Edge{geometry.EdgeTop, geometry.EdgeBottom} {
		for _, axis := range []geometry.Axis{geometry.AxisFloor, geometry.AxisDepthRun, geometry.AxisWidthRun} {
			p := notchedPanel(axis, edge)
			n := len(p.OutlinePolygon())
			tris, err := panelTriangles(p)
			if err != nil {
				t.Fatal(err)
			}
			// Two caps of n-2 triangles each plus two triangles per side wall.
			if want := 2*(n-2) + 2*n; len(tris) != want {
				t.Errorf("%s/%s: %d triangles, want %d", axis, edge, len(tris), want)
			}
			// (rect area - notch areas) * thickness; notches are height/2 deep.
			want := (300*60 - (3+5)*30) * 3.0
			if vol := signedVolume(tris); math.Abs(vol-want) > 1e-3*want {
				t.Errorf("%s/%s: volume %v, want %v", axis, edge, vol, want)
			}
		}
	}
}

// TestPanelTrianglesAreManifold checks every undirected edge is used by
// exactly two triangles, once in each direction (watertight, consistently
// wound), and that no triangle is degenerate.
func TestPanelTrianglesAreManifold(t *testing.T) {
	for _, edge := range []geometry.Edge{geometry.EdgeTop, geometry.EdgeBottom} {
		for _, axis := range []geometry.Axis{geometry.AxisFloor, geometry.AxisDepthRun, geometry.AxisWidthRun} {
			tris, err := panelTriangles(notchedPanel(axis, edge))
			if err != nil {
				t.Fatal(err)
			}
			type key [3]int64
			k := func(v vec3) key {
				r := func(f float32) int64 { return int64(math.Round(float64(f) * 1000)) }
				return key{r(v.X), r(v.Y), r(v.Z)}
			}
			directed := map[[2]key]int{}
			for i, tr := range tris {
				ks := [3]key{k(tr.V1), k(tr.V2), k(tr.V3)}
				if ks[0] == ks[1] || ks[1] == ks[2] || ks[0] == ks[2] {
					t.Fatalf("%s/%s: triangle %d is degenerate", axis, edge, i)
				}
				for j := 0; j < 3; j++ {
					directed[[2]key{ks[j], ks[(j+1)%3]}]++
				}
			}
			for e, c := range directed {
				if c != 1 || directed[[2]key{e[1], e[0]}] != 1 {
					t.Fatalf("%s/%s: edge %v used %d times, reverse %d times", axis, edge, e, c, directed[[2]key{e[1], e[0]}])
				}
			}
		}
	}
}

// Notches flush with both panel ends make BuildOutline emit zero-width
// spikes; the prism must still be a closed manifold of the right volume.
func TestPanelTrianglesFlushEndNotches(t *testing.T) {
	for _, edge := range []geometry.Edge{geometry.EdgeTop, geometry.EdgeBottom} {
		for _, axis := range []geometry.Axis{geometry.AxisFloor, geometry.AxisDepthRun, geometry.AxisWidthRun} {
			p := geometry.Panel{
				ID: "p", Axis: axis, Length: 150, Height: 40, Thickness: 3,
				Notches: []geometry.Notch{{Pos: 0, Width: 3, Edge: edge}, {Pos: 147, Width: 3, Edge: edge}},
			}
			tris, err := panelTriangles(p)
			if err != nil {
				t.Fatal(err)
			}
			want := (150*40 - 2*3*20) * 3.0
			if vol := signedVolume(tris); math.Abs(vol-want) > 1e-3*want {
				t.Errorf("%s/%s: volume %v, want %v", axis, edge, vol, want)
			}
			if n := len(p.OutlinePolygon()); len(tris) != 2*(n-2)+2*n {
				t.Errorf("%s/%s: %d triangles for %d vertices", axis, edge, len(tris), n)
			}
			directed := map[[6]float32]int{}
			for _, tr := range tris {
				for _, e := range [][2]vec3{{tr.V1, tr.V2}, {tr.V2, tr.V3}, {tr.V3, tr.V1}} {
					directed[[6]float32{e[0].X, e[0].Y, e[0].Z, e[1].X, e[1].Y, e[1].Z}]++
				}
			}
			for e, c := range directed {
				if c != 1 || directed[[6]float32{e[3], e[4], e[5], e[0], e[1], e[2]}] != 1 {
					t.Fatalf("%s/%s: edge %v not paired exactly once with its reverse", axis, edge, e)
				}
			}
		}
	}
}

func signedVolume(tris []triangle) float64 {
	var vol float64
	for _, t := range tris {
		a, b, c := t.V1, t.V2, t.V3
		cx := float64(b.Y)*float64(c.Z) - float64(b.Z)*float64(c.Y)
		cy := float64(b.Z)*float64(c.X) - float64(b.X)*float64(c.Z)
		cz := float64(b.X)*float64(c.Y) - float64(b.Y)*float64(c.X)
		vol += (float64(a.X)*cx + float64(a.Y)*cy + float64(a.Z)*cz) / 6
	}
	return vol
}

func TestPanelTrianglesOutwardWindingPerAxis(t *testing.T) {
	notches := []geometry.Notch{
		{Pos: 40, Width: 3, Edge: "top"},
		{Pos: 120, Width: 3, Edge: "top"},
		{Pos: 200, Width: 3, Edge: "bottom"},
	}
	var ref float64
	for _, axis := range []geometry.Axis{geometry.AxisFloor, geometry.AxisDepthRun, geometry.AxisWidthRun} {
		t.Run(string(axis), func(t *testing.T) {
			p := geometry.Panel{
				ID: "p", Axis: axis,
				Length: 300, Height: 60, Thickness: 3,
				Notches:  notches,
				Position: geometry.Placement3D{OriginX: 10, OriginY: 20, OriginZ: 30},
			}
			tris, err := panelTriangles(p)
			if err != nil {
				t.Fatal(err)
			}
			if len(tris) <= 12 {
				t.Fatalf("expected several rects (>12 triangles), got %d", len(tris))
			}
			vol := signedVolume(tris)
			if vol <= 0 {
				t.Fatalf("signed volume = %v, want positive (inward-facing winding)", vol)
			}
			if ref == 0 {
				ref = vol
			} else if math.Abs(vol-ref) > 1e-3*ref {
				t.Fatalf("volume %v differs from reference %v", vol, ref)
			}
			for i, tr := range tris {
				ux, uy, uz := tr.V2.X-tr.V1.X, tr.V2.Y-tr.V1.Y, tr.V2.Z-tr.V1.Z
				vx, vy, vz := tr.V3.X-tr.V1.X, tr.V3.Y-tr.V1.Y, tr.V3.Z-tr.V1.Z
				cx, cy, cz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
				if dot := cx*tr.Normal.X + cy*tr.Normal.Y + cz*tr.Normal.Z; dot <= 0 {
					t.Fatalf("triangle %d: stored normal disagrees with winding (dot=%v)", i, dot)
				}
			}
		})
	}
}

func TestExportRejectsBadNotchEdges(t *testing.T) {
	for name, notches := range map[string][]geometry.Notch{
		"unknown": {{Pos: 10, Width: 3, Edge: "left"}},
		"mixed":   {{Pos: 10, Width: 3, Edge: geometry.EdgeTop}, {Pos: 50, Width: 3, Edge: geometry.EdgeBottom}},
	} {
		p := geometry.Panel{ID: "p", Axis: geometry.AxisWidthRun, Length: 100, Height: 40, Thickness: 3, Notches: notches}
		var buf bytes.Buffer
		if err := (Exporter{}).Export(&buf, pack.BoxResult{}, []geometry.Panel{p}, manifest.Material{}); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func multiPanelInput() []geometry.Panel {
	return []geometry.Panel{
		notchedPanel(geometry.AxisWidthRun, geometry.EdgeTop),
		notchedPanel(geometry.AxisWidthRun, geometry.EdgeBottom),
		{
			ID: "plain", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Thickness: 3,
			Outline: geometry.BuildOutline(300, 60, nil),
		},
	}
}

func TestExportLengthMatchesHeaderCount(t *testing.T) {
	panels := multiPanelInput()
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	count := int(binary.LittleEndian.Uint32(data[80:84]))
	if len(data) != 84+50*count {
		t.Fatalf("len = %d, want 84+50*%d = %d", len(data), count, 84+50*count)
	}
	// Independent expectation: per panel, ear clipping gives N-2 triangles per
	// cap (2(N-2) total) plus 2 per side wall (2N), N = outline vertices.
	want := 0
	for _, p := range panels {
		n := len(p.OutlinePolygon())
		want += 2*(n-2) + 2*n
	}
	if count != want {
		t.Fatalf("header count = %d, want %d", count, want)
	}
}

type failAfterWriter struct{ limit int }

func (f *failAfterWriter) Write(p []byte) (int, error) {
	if len(p) > f.limit {
		n := f.limit
		f.limit = 0
		return n, errors.New("disk full")
	}
	f.limit -= len(p)
	return len(p), nil
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

func TestExportPropagatesWriterErrors(t *testing.T) {
	panels := multiPanelInput()
	mat := manifest.Material{Thickness: 3}
	if err := (Exporter{}).Export(&failAfterWriter{limit: 100}, pack.BoxResult{}, panels, mat); err == nil {
		t.Fatal("expected error from failing writer")
	}
	if err := (Exporter{}).Export(shortWriter{}, pack.BoxResult{}, panels, mat); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write: got %v, want io.ErrShortWrite", err)
	}
}

// TestHeightReducedPanelTopAndBottomNotches checks a panel trimmed by Cut
// (full height 60, cut 20, reduced height 40, notch depth 30) with notches on
// the top or the bottom edge: the solid must be watertight, have the volume of
// the trimmed outline, and never rise above the reduced height.
func TestHeightReducedPanelTopAndBottomNotches(t *testing.T) {
	for _, tc := range []struct {
		edge      geometry.Edge
		notchArea float64
	}{
		// A top notch opens in the trimmed edge, so only 3x10 is removed.
		{geometry.EdgeTop, 3 * 10},
		{geometry.EdgeBottom, 3 * 30},
	} {
		for _, axis := range []geometry.Axis{geometry.AxisFloor, geometry.AxisDepthRun, geometry.AxisWidthRun} {
			t.Run(string(tc.edge)+"/"+string(axis), func(t *testing.T) {
				p := geometry.Panel{
					ID: "p", Axis: axis, Length: 300, Height: 40, Cut: 20, Thickness: 3,
					Notches: []geometry.Notch{
						{Pos: 40, Width: 3, Edge: tc.edge},
						{Pos: 120, Width: 5, Edge: tc.edge},
					},
				}
				tris, err := panelTriangles(p)
				if err != nil {
					t.Fatal(err)
				}

				want := (300*40 - (tc.notchArea/3)*(3+5)) * 3.0
				if vol := signedVolume(tris); math.Abs(vol-want) > 1e-3*want {
					t.Errorf("volume %v, want %v", vol, want)
				}
				if n := len(p.OutlinePolygon()); len(tris) != 2*(n-2)+2*n {
					t.Errorf("%d triangles for %d outline vertices", len(tris), n)
				}

				// With a zero origin the panel's local height is Z, or Y for
				// a floor panel.
				var maxH float32
				for _, tr := range tris {
					for _, v := range []vec3{tr.V1, tr.V2, tr.V3} {
						h := v.Z
						if axis == geometry.AxisFloor {
							h = v.Y
						}
						maxH = max(maxH, h)
					}
				}
				if maxH != 40 {
					t.Errorf("max height %v, want reduced height 40", maxH)
				}

				directed := map[[6]float32]int{}
				for _, tr := range tris {
					for _, e := range [][2]vec3{{tr.V1, tr.V2}, {tr.V2, tr.V3}, {tr.V3, tr.V1}} {
						directed[[6]float32{e[0].X, e[0].Y, e[0].Z, e[1].X, e[1].Y, e[1].Z}]++
					}
				}
				for e, c := range directed {
					if c != 1 || directed[[6]float32{e[3], e[4], e[5], e[0], e[1], e[2]}] != 1 {
						t.Fatalf("edge %v not paired exactly once with its reverse", e)
					}
				}
			})
		}
	}
}

func TestExportNoPanelsErrors(t *testing.T) {
	for name, panels := range map[string][]geometry.Panel{
		"nil": nil,
	} {
		var buf bytes.Buffer
		err := (Exporter{}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{Thickness: 3})
		if !errors.Is(err, export.ErrNoPanels) {
			t.Fatalf("%s: expected export.ErrNoPanels, got %v", name, err)
		}
		if buf.Len() != 0 {
			t.Fatalf("%s: wrote %d bytes despite the error", name, buf.Len())
		}
	}
}

func TestCheckTriangulationDetectsEarClipFailure(t *testing.T) {
	// A zero-area spike leaves the clipper without a valid ear.
	spike := []geometry.Point2D{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 5, Y: 10}, {X: 5, Y: 20}, {X: 5, Y: 10}, {X: 0, Y: 10}}
	if err := checkTriangulation(spike, earClip(spike)); err == nil {
		t.Fatal("expected an error for a degenerate outline")
	}
	// A valid outline whose final triangle is zero-area must still pass.
	square := []geometry.Point2D{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if err := checkTriangulation(square, earClip(square)); err != nil {
		t.Fatalf("valid outline rejected: %v", err)
	}
}
