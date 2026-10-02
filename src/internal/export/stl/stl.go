// Package stl extrudes panel geometry to material thickness and writes it
// as a binary STL triangle mesh, positioned within the assembled insert.
package stl

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

type Exporter struct {
	// BoxCase adds the box case (floor and four walls around the interior)
	// as extra reference geometry.
	BoxCase bool
}

func (Exporter) Format() string { return "stl" }

const (
	stlHeaderSize   = 84 // 80-byte header + uint32 triangle count
	stlTriangleSize = 50 // 12 float32 + uint16 attribute byte count
)

type vec3 struct{ X, Y, Z float32 }

type triangle struct {
	Normal     vec3
	V1, V2, V3 vec3
}

func (e Exporter) Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error {
	for _, p := range panels {
		if err := p.ValidateOutline(); err != nil {
			return fmt.Errorf("panel %s: %w", p.ID, err)
		}
	}
	if e.BoxCase {
		panels = append(append([]geometry.Panel(nil), panels...), export.CasePanels(box)...)
	}
	var tris []triangle
	for _, p := range panels {
		pt, err := panelTriangles(p)
		if err != nil {
			return fmt.Errorf("panel %s: %w", p.ID, err)
		}
		tris = append(tris, pt...)
	}
	if len(tris) == 0 {
		return fmt.Errorf("stl export: %w: no panel produced any triangles", export.ErrNoPanels)
	}

	// Serialize into one preallocated buffer (80-byte header, uint32 count,
	// then 50 bytes per triangle) and write it with a single call.
	buf := make([]byte, stlHeaderSize+stlTriangleSize*len(tris))
	copy(buf, "cubby slotted insert")
	binary.LittleEndian.PutUint32(buf[80:], uint32(len(tris)))
	off := stlHeaderSize
	for _, t := range tris {
		for _, f := range [12]float32{
			t.Normal.X, t.Normal.Y, t.Normal.Z,
			t.V1.X, t.V1.Y, t.V1.Z,
			t.V2.X, t.V2.Y, t.V2.Z,
			t.V3.X, t.V3.Y, t.V3.Z,
		} {
			binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(f))
			off += 4
		}
		off += 2 // attribute byte count, left zero
	}
	n, err := w.Write(buf)
	if err != nil {
		return err
	}
	if n < len(buf) {
		return io.ErrShortWrite
	}
	return nil
}

// panelTriangles extrudes a panel's outline polygon to material thickness and
// places it in 3D via its Position, producing one closed prism: an
// ear-clipped triangulation of the outline for each thickness cap (using
// outline vertices only, so cap edges match the side walls exactly) plus one
// quad per outline edge. It errors when the ear clipper cannot cover the
// outline, since the side walls would then enclose an open mesh.
func panelTriangles(p geometry.Panel) ([]triangle, error) {
	poly := p.OutlinePolygon()
	n := len(poly)
	if n < 3 {
		return nil, nil
	}
	capTris := earClip(poly)
	if err := checkTriangulation(poly, capTris); err != nil {
		return nil, err
	}

	to3D := func(pt geometry.Point2D, z float64) vec3 {
		x, y, z3d := p.To3D(pt.X, pt.Y, z)
		return vec3{X: float32(x), Y: float32(y), Z: float32(z3d)}
	}
	bottom := make([]vec3, n)
	top := make([]vec3, n)
	for i, pt := range poly {
		bottom[i] = to3D(pt, 0)
		top[i] = to3D(pt, p.Thickness)
	}

	// To3D is a reflection (determinant -1) for width-run panels, which flips
	// triangle winding; reverse it there so normals face outward in world space.
	mirrored := p.Axis == geometry.AxisWidthRun
	tri := func(a, b, c vec3) triangle {
		if mirrored {
			return newTriangle(a, c, b)
		}
		return newTriangle(a, b, c)
	}

	var tris []triangle
	for _, t := range capTris {
		// Outline is counter-clockwise: top cap keeps it, bottom reverses it.
		tris = append(tris,
			tri(top[t[0]], top[t[1]], top[t[2]]),
			tri(bottom[t[0]], bottom[t[2]], bottom[t[1]]))
	}
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		tris = append(tris,
			tri(bottom[i], bottom[j], top[j]),
			tri(bottom[i], top[j], top[i]))
	}
	return tris, nil
}

// checkTriangulation verifies that tris cover the polygon: their areas must
// sum to the polygon's area. A count check against n-2 would wrongly reject
// valid outlines, where a zero-area final triangle is legitimately dropped.
func checkTriangulation(poly []geometry.Point2D, tris [][3]int) error {
	area := func(a, b, c geometry.Point2D) float64 {
		return ((b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)) / 2
	}
	var want, got float64
	for i := 1; i+1 < len(poly); i++ {
		want += area(poly[0], poly[i], poly[i+1])
	}
	for _, t := range tris {
		got += area(poly[t[0]], poly[t[1]], poly[t[2]])
	}
	if math.Abs(got-want) > 1e-9*math.Max(math.Abs(want), 1) {
		return fmt.Errorf("cap triangulation failed: %d triangles cover area %g of outline area %g (degenerate outline)", len(tris), got, want)
	}
	return nil
}

// earClip triangulates a simple counter-clockwise polygon into index
// triples over its own vertices (n-2 triangles when no vertices are
// collinear). A vertex is an ear when it is strictly convex and no other
// vertex lies inside or on the ear triangle.
func earClip(poly []geometry.Point2D) [][3]int {
	idx := make([]int, len(poly))
	for i := range idx {
		idx[i] = i
	}
	cross := func(a, b, c geometry.Point2D) float64 {
		return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	}
	var out [][3]int
	for len(idx) > 3 {
		clipped := false
		for k := range idx {
			ia, ib, ic := idx[(k+len(idx)-1)%len(idx)], idx[k], idx[(k+1)%len(idx)]
			a, b, c := poly[ia], poly[ib], poly[ic]
			if cross(a, b, c) <= 0 {
				continue
			}
			ear := true
			for _, m := range idx {
				if m == ia || m == ib || m == ic {
					continue
				}
				q := poly[m]
				if q == a || q == b || q == c {
					continue
				}
				if cross(a, b, q) >= 0 && cross(b, c, q) >= 0 && cross(c, a, q) >= 0 {
					ear = false
					break
				}
			}
			if !ear {
				continue
			}
			out = append(out, [3]int{ia, ib, ic})
			idx = append(idx[:k], idx[k+1:]...)
			clipped = true
			break
		}
		if !clipped {
			return out // degenerate outline (zero-area spike); nothing sensible left
		}
	}
	if len(idx) == 3 && cross(poly[idx[0]], poly[idx[1]], poly[idx[2]]) > 0 {
		out = append(out, [3]int{idx[0], idx[1], idx[2]})
	}
	return out
}

func newTriangle(a, b, c vec3) triangle {
	ux, uy, uz := b.X-a.X, b.Y-a.Y, b.Z-a.Z
	vx, vy, vz := c.X-a.X, c.Y-a.Y, c.Z-a.Z
	nx, ny, nz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
	length := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz)))
	if length > 0 {
		nx, ny, nz = nx/length, ny/length, nz/length
	}
	return triangle{Normal: vec3{X: nx, Y: ny, Z: nz}, V1: a, V2: b, V3: c}
}
