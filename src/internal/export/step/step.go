// Package step writes panel geometry as a minimal STEP AP214 file: each
// panel's rectilinear outline (see internal/geometry.BuildOutline) is
// extruded into one prism and emitted as a single MANIFOLD_SOLID_BREP named
// by the panel ID, since every cubby panel is a simple orthogonal
// planar-faced prism (no curves or booleans needed).
//
// This is cubby's highest-risk exporter: STEP's AP214 boilerplate
// (application context, product definition tree, geometric representation
// context, units) is fiddly to hand-roll correctly, and can only be fully
// validated by opening the output in real CAD software (FreeCAD,
// SolidWorks), which can't run in CI. Treat --format step as experimental
// until manually validated.
package step

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// Exporter writes STEP files. Now, when non-nil, supplies the timestamp
// embedded in FILE_NAME (inject a fixed time for reproducible output);
// the default is time.Now.
type Exporter struct {
	// BoxCase adds the box case (floor and four walls around the interior)
	// as extra solids.
	BoxCase bool
	Now     func() time.Time
}

func (Exporter) Format() string { return "step" }

func (e Exporter) Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error {
	for _, p := range panels {
		if err := p.ValidateOutline(); err != nil {
			return fmt.Errorf("panel %s: %w", p.ID, err)
		}
	}
	if e.BoxCase {
		panels = append(append([]geometry.Panel(nil), panels...), export.CasePanels(box)...)
	}
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	wr := newWriter(w)

	wr.raw("ISO-10303-21;")
	wr.raw("HEADER;")
	wr.raw("FILE_DESCRIPTION((''),'2;1');")
	// FILE_NAME's name stays "<box> insert" (it names the file, not the
	// design); the project author fills the author list, and the organization
	// list stays empty as no manifest field maps to it.
	var author, description string
	if box.Project != nil {
		author, description = box.Project.Author, box.Project.Description
	}
	authorList := "('')"
	if author != "" {
		authorList = fmt.Sprintf("('%s')", escape(author))
	}
	wr.raw(fmt.Sprintf("FILE_NAME('%s','%s',%s,(''),'cubby','cubby','');",
		escape(box.BoxName+" insert"), now().UTC().Format("2006-01-02T15:04:05"), authorList))
	wr.raw("FILE_SCHEMA(('AUTOMOTIVE_DESIGN'));")
	wr.raw("ENDSEC;")
	wr.raw("DATA;")

	var solidIDs []int
	for _, p := range panels {
		if id, ok := emitPrism(wr, p); ok {
			solidIDs = append(solidIDs, id)
		}
	}

	if len(solidIDs) == 0 {
		return fmt.Errorf("step export: %w: no panel has an outline of at least 3 points, and STEP needs at least one solid", export.ErrNoPanels)
	}

	emitProductTree(wr, box.BoxName, description, solidIDs)

	wr.raw("ENDSEC;")
	wr.raw("END-ISO-10303-21;")

	return wr.flush()
}

// escape encodes s as the content of a STEP (ISO 10303-21) string literal:
// single quotes and backslashes are doubled, non-ASCII runs are written as
// \X2\<UTF-16BE hex>\X0\ (surrogate pairs above U+FFFF), whitespace control
// characters become a space and other control characters are dropped.
func escape(s string) string {
	var b strings.Builder
	inWide := false
	closeWide := func() {
		if inWide {
			b.WriteString(`\X0\`)
			inWide = false
		}
	}
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			closeWide()
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			// dropped
		case r < 0x7f:
			closeWide()
			switch r {
			case '\'':
				b.WriteString("''")
			case '\\':
				b.WriteString(`\\`)
			default:
				b.WriteRune(r)
			}
		default:
			if !inWide {
				b.WriteString(`\X2\`)
				inWide = true
			}
			for _, u := range utf16.Encode([]rune{r}) {
				fmt.Fprintf(&b, "%04X", u)
			}
		}
	}
	closeWide()
	return b.String()
}

// orientedEdge is one traversal of an EDGE_CURVE within a face loop.
type orientedEdge struct {
	edge    int
	forward bool
}

// emitPrism writes a panel's outline extruded through its thickness as one
// MANIFOLD_SOLID_BREP named by the panel ID, and returns its entity id.
// Faces are the two outline-shaped caps plus one quad per outline edge. Each
// EDGE_CURVE (bottom, top and vertical edge per outline edge) is shared by
// exactly two faces through ORIENTED_EDGEs of opposite orientation.
func emitPrism(wr *writer, p geometry.Panel) (int, bool) {
	poly := p.OutlinePolygon()
	n := len(poly)
	if n < 3 {
		return 0, false
	}

	// Ring 0 is the z=0 face of the panel, ring 1 the z=thickness face.
	var pts [2][][3]float64
	var pids, vids [2][]int
	for r := 0; r < 2; r++ {
		z := float64(r) * p.Thickness
		for _, pt := range poly {
			x, y, z3 := p.To3D(pt.X, pt.Y, z)
			pts[r] = append(pts[r], [3]float64{x, y, z3})
			pid := wr.emit(wr.point(x, y, z3))
			pids[r] = append(pids[r], pid)
			vids[r] = append(vids[r], wr.emit(fmt.Sprintf("VERTEX_POINT('',%s)", ref(pid))))
		}
	}

	dirIDs := map[[3]float64]int{}
	dir := func(x, y, z float64) int {
		k := [3]float64{x + 0, y + 0, z + 0} // +0 folds -0 into 0
		if id, ok := dirIDs[k]; ok {
			return id
		}
		id := wr.emit(wr.direction(k[0], k[1], k[2]))
		dirIDs[k] = id
		return id
	}

	ends := map[int][2][3]float64{} // EDGE_CURVE id -> start, end
	edge := func(r0, i0, r1, i1 int) int {
		a, b := pts[r0][i0], pts[r1][i1]
		ux, uy, uz := unit(sub(b, a))
		vecID := wr.emit(fmt.Sprintf("VECTOR('',%s,%s)", ref(dir(ux, uy, uz)), wr.real(dist(a, b))))
		lineID := wr.emit(fmt.Sprintf("LINE('',%s,%s)", ref(pids[r0][i0]), ref(vecID)))
		id := wr.emit(fmt.Sprintf("EDGE_CURVE('',%s,%s,%s,.T.)", ref(vids[r0][i0]), ref(vids[r1][i1]), ref(lineID)))
		ends[id] = [2][3]float64{a, b}
		return id
	}
	var ringEdges [2][]int // ringEdges[r][i] runs vertex i -> i+1 on ring r
	var vertEdges []int    // vertEdges[i] runs ring 0 -> ring 1 at vertex i
	for r := 0; r < 2; r++ {
		for i := 0; i < n; i++ {
			ringEdges[r] = append(ringEdges[r], edge(r, i, r, (i+1)%n))
		}
	}
	for i := 0; i < n; i++ {
		vertEdges = append(vertEdges, edge(0, i, 1, i))
	}

	// To3D is a reflection for width-run panels: every loop below is written
	// counter-clockwise about the outward normal in local space, so reverse it
	// there to stay outward-facing in world space.
	mirrored := p.Axis == geometry.AxisWidthRun

	face := func(loop []orientedEdge) int {
		if mirrored {
			rev := make([]orientedEdge, len(loop))
			for i, oe := range loop {
				rev[len(loop)-1-i] = orientedEdge{oe.edge, !oe.forward}
			}
			loop = rev
		}
		var oeIDs []int
		var loopPts [][3]float64 // start point of each traversed edge, in loop order
		for _, oe := range loop {
			sense := ".F."
			if oe.forward {
				sense = ".T."
			}
			oeIDs = append(oeIDs, wr.emit(fmt.Sprintf("ORIENTED_EDGE('',*,*,%s,%s)", ref(oe.edge), sense)))
			e := ends[oe.edge]
			if oe.forward {
				loopPts = append(loopPts, e[0])
			} else {
				loopPts = append(loopPts, e[1])
			}
		}
		loopID := wr.emit(fmt.Sprintf("EDGE_LOOP('',%s)", refList(oeIDs)))
		boundID := wr.emit(fmt.Sprintf("FACE_OUTER_BOUND('',%s,.T.)", ref(loopID)))

		nx, ny, nz := unit(newell(loopPts))
		ex, ey, ez := unit(sub(loopPts[1], loopPts[0]))
		zDirID := dir(nx, ny, nz)
		xDirID := dir(ex, ey, ez)
		originID := wr.emit(wr.point(loopPts[0][0], loopPts[0][1], loopPts[0][2]))
		axisID := wr.emit(fmt.Sprintf("AXIS2_PLACEMENT_3D('',%s,%s,%s)", ref(originID), ref(zDirID), ref(xDirID)))
		planeID := wr.emit(fmt.Sprintf("PLANE('',%s)", ref(axisID)))
		return wr.emit(fmt.Sprintf("ADVANCED_FACE('',%s,%s,.T.)", refList([]int{boundID}), ref(planeID)))
	}

	var faces []int
	// Top cap (local +thickness): outline counter-clockwise.
	var loop []orientedEdge
	for i := 0; i < n; i++ {
		loop = append(loop, orientedEdge{ringEdges[1][i], true})
	}
	faces = append(faces, face(loop))
	// Bottom cap: outline clockwise.
	loop = nil
	for i := n - 1; i >= 0; i-- {
		loop = append(loop, orientedEdge{ringEdges[0][i], false})
	}
	faces = append(faces, face(loop))
	// Side walls, one per outline edge.
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		faces = append(faces, face([]orientedEdge{
			{ringEdges[0][i], true}, {vertEdges[j], true}, {ringEdges[1][i], false}, {vertEdges[i], false},
		}))
	}

	name := escape(p.ID)
	shellID := wr.emit(fmt.Sprintf("CLOSED_SHELL('%s',%s)", name, refList(faces)))
	return wr.emit(fmt.Sprintf("MANIFOLD_SOLID_BREP('%s',%s)", name, ref(shellID))), true
}

// newell returns the (unnormalized) normal of the planar polygon pts, wound
// counter-clockwise about it; unlike a cross product of two edges it is
// correct for non-convex polygons.
func newell(pts [][3]float64) [3]float64 {
	var n [3]float64
	for i, a := range pts {
		b := pts[(i+1)%len(pts)]
		n[0] += (a[1] - b[1]) * (a[2] + b[2])
		n[1] += (a[2] - b[2]) * (a[0] + b[0])
		n[2] += (a[0] - b[0]) * (a[1] + b[1])
	}
	return n
}

func sub(a, b [3]float64) [3]float64 {
	return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func dist(a, b [3]float64) float64 {
	d := sub(a, b)
	return math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
}

func unit(v [3]float64) (x, y, z float64) {
	l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if l == 0 {
		return 0, 0, 0
	}
	return v[0] / l, v[1] / l, v[2] / l
}

// emitProductTree writes the standard AP214 wrapper (application context,
// product definition, geometric representation context with millimeter
// units) around an already-emitted list of solid entity ids. description, if
// non-empty, becomes the PRODUCT description.
func emitProductTree(wr *writer, name, description string, solidIDs []int) {
	if name == "" {
		name = "cubby insert"
	}
	name = escape(name)
	description = escape(description)

	appCtxID := wr.emit("APPLICATION_CONTEXT('automotive design')")
	wr.emit(fmt.Sprintf("APPLICATION_PROTOCOL_DEFINITION('international standard','automotive_design',2003,%s)", ref(appCtxID)))
	prodCtxID := wr.emit(fmt.Sprintf("PRODUCT_CONTEXT('',%s,'mechanical')", ref(appCtxID)))
	prodID := wr.emit(fmt.Sprintf("PRODUCT('%s','%s','%s',(%s))", name, name, description, ref(prodCtxID)))
	prodFormID := wr.emit(fmt.Sprintf("PRODUCT_DEFINITION_FORMATION('','',%s)", ref(prodID)))
	prodDefCtxID := wr.emit(fmt.Sprintf("PRODUCT_DEFINITION_CONTEXT('part definition',%s,'design')", ref(appCtxID)))
	prodDefID := wr.emit(fmt.Sprintf("PRODUCT_DEFINITION('design','',%s,%s)", ref(prodFormID), ref(prodDefCtxID)))
	shapeID := wr.emit(fmt.Sprintf("PRODUCT_DEFINITION_SHAPE('','',%s)", ref(prodDefID)))

	lengthUnitID := wr.emit("(LENGTH_UNIT()NAMED_UNIT(*)SI_UNIT(.MILLI.,.METRE.))")
	angleUnitID := wr.emit("(NAMED_UNIT(*)PLANE_ANGLE_UNIT()SI_UNIT($,.RADIAN.))")
	solidAngleUnitID := wr.emit("(NAMED_UNIT(*)SI_UNIT($,.STERADIAN.)SOLID_ANGLE_UNIT())")
	uncertaintyID := wr.emit(fmt.Sprintf("UNCERTAINTY_MEASURE_WITH_UNIT(LENGTH_MEASURE(%s),%s,'DISTANCE_ACCURACY_VALUE','')", wr.real(1e-6), ref(lengthUnitID)))
	repCtxID := wr.emit(fmt.Sprintf(
		"(GEOMETRIC_REPRESENTATION_CONTEXT(3)GLOBAL_UNCERTAINTY_ASSIGNED_CONTEXT((%s))GLOBAL_UNIT_ASSIGNED_CONTEXT((%s,%s,%s))REPRESENTATION_CONTEXT('','3D'))",
		ref(uncertaintyID), ref(lengthUnitID), ref(angleUnitID), ref(solidAngleUnitID)))

	repID := wr.emit(fmt.Sprintf("ADVANCED_BREP_SHAPE_REPRESENTATION('',%s,%s)", refList(solidIDs), ref(repCtxID)))
	wr.emit(fmt.Sprintf("SHAPE_DEFINITION_REPRESENTATION(%s,%s)", ref(shapeID), ref(repID)))
}
