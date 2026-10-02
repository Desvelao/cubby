package isometric

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"sort"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

// PNGExporter rasterizes the same isometric scene SVGExporter draws, as a
// plain raster image. Label text is drawn at a fixed on-screen size
// (screen-space annotations), independent of model scale.
type PNGExporter struct {
	Exploded bool
	// BoxCase outlines the box interior as a wireframe.
	BoxCase bool
}

func (e PNGExporter) Format() string {
	if e.Exploded {
		return "iso-png-exploded"
	}
	return "iso-png"
}

const (
	pngMargin  = 12.0
	pngScale   = 4.0    // px per mm before the max-dimension clamp
	pngMaxSide = 1800.0 // px
)

func (e PNGExporter) Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error {
	scene := BuildSceneCase(box, panels, mat, e.Exploded, e.BoxCase)

	// The scene's MaxX includes an estimated legend text width; the PNG
	// measures text exactly, so size the drawing from the core bounds.
	sceneW := scene.CoreMaxX - scene.MinX + 2*pngMargin
	sceneH := scene.MaxY - scene.MinY + 2*pngMargin
	if sceneW <= 0 {
		sceneW = 1
	}
	if sceneH <= 0 {
		sceneH = 1
	}

	// Labels are drawn at a fixed pixel size, so their real extents (not a
	// per-character estimate in scene units) decide how much room the image
	// needs. Shrinking the scale to honor the max side moves label anchors,
	// so iterate to a fixed point.
	scale := pngScale
	var ext pxExtent
	for i := 0; i < 20; i++ {
		ext = imageExtent(scene, scale, sceneW, sceneH)
		over := math.Max(ext.w/pngMaxSide, ext.h/pngMaxSide)
		if over <= 1 {
			break
		}
		scale /= over
	}

	imgW := int(math.Ceil(ext.w))
	imgH := int(math.Ceil(ext.h))
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	toPx := func(p vec2) (float64, float64) {
		return (p.X-scene.MinX+pngMargin)*scale + ext.offX, (p.Y-scene.MinY+pngMargin)*scale + ext.offY
	}

	for _, poly := range scene.Polys {
		pts := make([]image.Point, len(poly.Points))
		for i, p := range poly.Points {
			x, y := toPx(p)
			pts[i] = image.Point{X: int(math.Round(x)), Y: int(math.Round(y))}
		}
		fillPolygon(img, pts, poly.Fill.toColor())
		for i := range pts {
			j := (i + 1) % len(pts)
			drawLine(img, pts[i].X, pts[i].Y, pts[j].X, pts[j].Y, poly.Stroke.toColor(), false)
		}
	}

	for _, l := range scene.Lines {
		ax, ay := toPx(l.A)
		bx, by := toPx(l.B)
		drawLine(img, int(math.Round(ax)), int(math.Round(ay)), int(math.Round(bx)), int(math.Round(by)), l.Stroke.toColor(), l.Dashed)
	}

	for _, lbl := range scene.Labels {
		x, y := toPx(lbl.Pos)
		drawText(img, int(math.Round(x)), int(math.Round(y)), lbl.Text, lbl.Color.toColor())
	}

	return png.Encode(w, img)
}

func (c rgb) toColor() color.RGBA { return color.RGBA{R: c.R, G: c.G, B: c.B, A: 255} }

// fillPolygon scanline-fills a simple (convex, non-self-intersecting)
// polygon; every face cubby projects is an axis-aligned box quad, so this
// is sufficient without a general polygon rasterizer.
func fillPolygon(img *image.RGBA, pts []image.Point, c color.RGBA) {
	if len(pts) < 3 {
		return
	}
	minY, maxY := pts[0].Y, pts[0].Y
	for _, p := range pts {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	b := img.Bounds()
	if minY < b.Min.Y {
		minY = b.Min.Y
	}
	if maxY > b.Max.Y-1 {
		maxY = b.Max.Y - 1
	}

	n := len(pts)
	for y := minY; y <= maxY; y++ {
		var xs []int
		for i := 0; i < n; i++ {
			a, pb := pts[i], pts[(i+1)%n]
			if a.Y == pb.Y {
				continue
			}
			y0, y1, x0, x1 := a.Y, pb.Y, a.X, pb.X
			if y0 > y1 {
				y0, y1 = y1, y0
				x0, x1 = x1, x0
			}
			if y < y0 || y >= y1 {
				continue
			}
			t := float64(y-y0) / float64(y1-y0)
			xs = append(xs, int(math.Round(float64(x0)+t*float64(x1-x0))))
		}
		sort.Ints(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			x0, x1 := xs[i], xs[i+1]
			if x0 < b.Min.X {
				x0 = b.Min.X
			}
			if x1 > b.Max.X-1 {
				x1 = b.Max.X - 1
			}
			for x := x0; x <= x1; x++ {
				img.Set(x, y, c)
			}
		}
	}
}

// drawLine plots a line with a simple parametric walk (not true Bresenham,
// but adequate for thin 1px technical-drawing strokes), optionally dashed.
func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, dashed bool) {
	dx, dy := float64(x1-x0), float64(y1-y0)
	length := math.Hypot(dx, dy)
	if length == 0 {
		setPixelSafe(img, x0, y0, c)
		return
	}
	const dashPx = 5.0
	steps := int(length)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		if dashed && int(t*length/dashPx)%2 == 1 {
			continue
		}
		x := int(math.Round(float64(x0) + dx*t))
		y := int(math.Round(float64(y0) + dy*t))
		setPixelSafe(img, x, y, c)
	}
}

func setPixelSafe(img *image.RGBA, x, y int, c color.RGBA) {
	if (image.Point{X: x, Y: y}).In(img.Bounds()) {
		img.Set(x, y, c)
	}
}

// labelFace is the single font used both to draw label text and to measure
// it for image bounds.
var labelFace font.Face = basicfont.Face7x13

// pxTextPad is the clear space, in pixels, kept around label text that
// extends past the scene's own margin.
const pxTextPad = 4.0

// pxExtent is the image size in pixels, plus the offset applied to scene
// coordinates so label text that starts left of or above the scene still
// fits.
type pxExtent struct{ w, h, offX, offY float64 }

// imageExtent returns the pixel size needed for the scene at the given
// scale: the scaled scene rectangle grown to include every label's measured
// text box.
func imageExtent(scene *Scene, scale, sceneW, sceneH float64) pxExtent {
	minX, minY := 0.0, 0.0
	maxX, maxY := sceneW*scale, sceneH*scale
	metrics := labelFace.Metrics()
	ascent := float64(metrics.Ascent.Ceil())
	descent := float64(metrics.Descent.Ceil())
	for _, l := range scene.Labels {
		x := (l.Pos.X - scene.MinX + pngMargin) * scale
		y := (l.Pos.Y - scene.MinY + pngMargin) * scale
		tw := float64(font.MeasureString(labelFace, pngText(l.Text)).Ceil())
		minX = math.Min(minX, x-pxTextPad)
		minY = math.Min(minY, y-ascent-pxTextPad)
		maxX = math.Max(maxX, x+tw+pxTextPad)
		maxY = math.Max(maxY, y+descent+pxTextPad)
	}
	return pxExtent{w: maxX - minX, h: maxY - minY, offX: -minX, offY: -minY}
}

func drawText(img *image.RGBA, x, y int, text string, c color.RGBA) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: labelFace,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(pngText(text))
}

// pngFold transliterates the non-ASCII runes most likely to appear in box
// and panel names (typographic punctuation and Latin letters with
// diacritics) to the closest ASCII form, since basicfont.Face7x13 only has
// glyphs for printable ASCII.
var pngFold = func() map[rune]string {
	m := map[rune]string{
		'\u2013': "-", '\u2014': "-", '\u2212': "-", '\u2010': "-", '\u2011': "-",
		'\u2018': "'", '\u2019': "'", '\u201C': "\"", '\u201D': "\"",
		'\u2026': "...", '\u00A0': " ", '\u00D7': "x", '\u00DF': "ss",
		'\u00C6': "AE", '\u00E6': "ae", '\u0152': "OE", '\u0153': "oe",
		'\u00D8': "O", '\u00F8': "o", '\u00D0': "D", '\u00F0': "d", '\u00DE': "Th", '\u00FE': "th",
	}
	groups := map[byte]string{
		'A': "ÀÁÂÃÄÅ", 'a': "àáâãäå", 'C': "Ç", 'c': "ç",
		'E': "ÈÉÊË", 'e': "èéêë", 'I': "ÌÍÎÏ", 'i': "ìíîï",
		'N': "Ñ", 'n': "ñ", 'O': "ÒÓÔÕÖ", 'o': "òóôõö",
		'U': "ÙÚÛÜ", 'u': "ùúûü", 'Y': "Ý", 'y': "ýÿ",
	}
	for base, rs := range groups {
		for _, r := range rs {
			m[r] = string(base)
		}
	}
	return m
}()

// pngText returns text restricted to what labelFace can draw: known
// non-ASCII runes are transliterated and anything else becomes '?'. It is
// used for both drawing and measuring so image bounds match what is drawn.
func pngText(text string) string {
	clean := true
	for i := 0; i < len(text); i++ {
		if text[i] < 0x20 || text[i] > 0x7E {
			clean = false
			break
		}
	}
	if clean {
		return text
	}
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= 0x20 && r <= 0x7E:
			b.WriteRune(r)
		case pngFold[r] != "":
			b.WriteString(pngFold[r])
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}
