package assembly

import (
	"unicode/utf8"

	"github.com/go-pdf/fpdf"
)

// pdfDoc wraps an fpdf document whose core (Courier) fonts only speak
// cp1252, so every string drawn or measured through it is first translated
// from UTF-8. Only the text methods used by this package are overridden;
// everything else is the embedded *fpdf.Fpdf.
type pdfDoc struct {
	*fpdf.Fpdf
	tr func(string) string
}

func newPDFDoc(pdf *fpdf.Fpdf) *pdfDoc {
	tr := pdf.UnicodeTranslatorFromDescriptor("") // "" is cp1252
	return &pdfDoc{Fpdf: pdf, tr: tr}
}

// encode converts UTF-8 text to cp1252 bytes. Runes cp1252 cannot represent
// (emoji, CJK, ...) and invalid UTF-8 become '?'.
func (d *pdfDoc) encode(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r < utf8.RuneSelf {
			out = append(out, byte(r))
			continue
		}
		// The translator maps unknown runes to '.', which cannot be a
		// legitimate result for a non-ASCII rune.
		t := d.tr(string(r))
		if r == utf8.RuneError || t == "." {
			t = "?"
		}
		out = append(out, t...)
	}
	return string(out)
}

func (d *pdfDoc) Text(x, y float64, s string) { d.Fpdf.Text(x, y, d.encode(s)) }

func (d *pdfDoc) GetStringWidth(s string) float64 { return d.Fpdf.GetStringWidth(d.encode(s)) }

func (d *pdfDoc) CellFormat(w, h float64, s, borderStr string, ln int, alignStr string, fill bool, link int, linkStr string) {
	d.Fpdf.CellFormat(w, h, d.encode(s), borderStr, ln, alignStr, fill, link, linkStr)
}
