package geometry

import "testing"

func TestRectBoxPerAxisWithNonZeroOrigin(t *testing.T) {
	r := PanelRect{X: 5, Y: 7, W: 30, H: 20}
	pos := Placement3D{OriginX: 100, OriginY: 200, OriginZ: 300}
	cases := []struct {
		axis                Axis
		ox, oy, oz, w, d, h float64
	}{
		{AxisWidthRun, 105, 200, 307, 30, 3, 20},
		{AxisDepthRun, 100, 205, 307, 3, 30, 20},
		{AxisFloor, 105, 207, 300, 30, 20, 3},
	}
	for _, c := range cases {
		p := Panel{Axis: c.axis, Thickness: 3, Position: pos}
		ox, oy, oz, w, d, h := p.RectBox(r)
		if ox != c.ox || oy != c.oy || oz != c.oz || w != c.w || d != c.d || h != c.h {
			t.Errorf("%s: got origin (%g,%g,%g) extent (%g,%g,%g), want (%g,%g,%g) (%g,%g,%g)",
				c.axis, ox, oy, oz, w, d, h, c.ox, c.oy, c.oz, c.w, c.d, c.h)
		}
		// Must agree with To3D: the box spans its opposite corners.
		x0, y0, z0 := p.To3D(r.X, r.Y, 0)
		x1, y1, z1 := p.To3D(r.X+r.W, r.Y+r.H, p.Thickness)
		if ox != x0 || oy != y0 || oz != z0 || ox+w != x1 || oy+d != y1 || oz+h != z1 {
			t.Errorf("%s: RectBox disagrees with To3D corners", c.axis)
		}
	}
}
