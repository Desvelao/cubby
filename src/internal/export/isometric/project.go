// Package isometric renders the assembled insert (panels placed in their
// packed 3D positions, per geometry.Panel.Position) as an annotated
// isometric technical drawing, in both SVG and PNG, in assembled and
// exploded variants. It is a pure consumer of the same panel geometry the
// stl/step exporters already use — no packing or geometry changes needed.
package isometric

import "math"

// vec3 is a point in the box-interior coordinate space (X = width,
// Y = depth, Z = height), in millimeters.
type vec3 struct{ X, Y, Z float64 }

// vec2 is a projected 2D drawing-space point.
type vec2 struct{ X, Y float64 }

// isoAngle is the classic 30-degree isometric projection angle.
var (
	cosIso = math.Cos(math.Pi / 6)
	sinIso = math.Sin(math.Pi / 6)
)

// project maps a 3D box-interior point onto the 2D isometric drawing plane.
// Z increases upward on screen (screen Y grows downward, so Z is
// subtracted).
func project(p vec3) vec2 {
	return vec2{
		X: (p.X - p.Y) * cosIso,
		Y: (p.X+p.Y)*sinIso - p.Z,
	}
}

// viewDir is the fixed camera direction used for backface culling and
// painter's-algorithm depth sorting: the camera sits far out along +viewDir
// looking back toward the origin, so it sees the box's top (+Z) and its +X
// and +Y faces. (project draws larger X+Y lower on screen, so the +X/+Y
// corner is the one nearest the viewer.)
var viewDir = vec3{X: 1, Y: 1, Z: 1}

// depth returns a point's position along the camera's view axis; larger
// values are closer to the camera.
func depth(p vec3) float64 { return p.X*viewDir.X + p.Y*viewDir.Y + p.Z*viewDir.Z }

// facesCamera reports whether a face with the given outward normal is
// visible to the camera.
func facesCamera(normal vec3) bool { return dot(normal, viewDir) > 1e-9 }

func dot(a, b vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

// lightDir is a fixed key-light direction used for simple flat shading of
// faces, independent of the camera direction, so top faces read lighter
// than side faces.
var lightDir = normalize(vec3{X: 0.35, Y: 0.2, Z: 0.9})

func normalize(v vec3) vec3 {
	l := math.Sqrt(dot(v, v))
	if l == 0 {
		return v
	}
	return vec3{v.X / l, v.Y / l, v.Z / l}
}

// shade returns a 0..1 brightness factor for a face with the given normal.
func shade(normal vec3) float64 {
	b := dot(normalize(normal), lightDir)
	if b < 0.35 {
		b = 0.35
	}
	if b > 1 {
		b = 1
	}
	return b
}
