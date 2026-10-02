package assembly

import (
	"strings"
	"unicode/utf8"

	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/version"
)

// setDocumentInfo fills the PDF Info dictionary from the project metadata.
// Strings are passed as UTF-8 (fpdf then writes them as UTF-16BE), so
// non-ASCII text survives even though the page fonts are cp1252-only. Only
// fields that are set are written, and a manifest without project metadata
// leaves the dictionary exactly as fpdf produces it.
func setDocumentInfo(pdf *pdfDoc, p *manifest.Project) {
	if p == nil || p.IsZero() {
		return
	}
	if p.Name != "" {
		pdf.SetTitle(p.Name, true)
	}
	if p.Author != "" {
		pdf.SetAuthor(p.Author, true)
	}
	if d := oneLine(p.Description); d != "" {
		pdf.SetSubject(d, true)
	}
	if len(p.Tags) > 0 {
		pdf.SetKeywords(strings.Join(p.Tags, ", "), true)
	}
	pdf.SetCreator("cubby "+version.Version, true)
}

// oneLine collapses all runs of whitespace (including newlines) to a space.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

const (
	projectDescMaxLines = 5
	projectGapMM        = 4.0
)

// drawProjectBlock prints the project name, a revision/author/license line and
// the description (wrapped, truncated to projectDescMaxLines lines) at (x, y)
// within width w, and returns the height it used including a trailing gap; it
// draws nothing and returns 0 when none of those fields is set.
func drawProjectBlock(pdf *pdfDoc, p *manifest.Project, x, y, w float64) float64 {
	if p == nil {
		return 0
	}
	var meta []string
	if p.Revision != "" {
		meta = append(meta, "rev "+p.Revision)
	}
	if p.Author != "" {
		meta = append(meta, "by "+p.Author)
	}
	if p.License != "" {
		meta = append(meta, p.License)
	}
	name, desc := oneLine(p.Name), oneLine(p.Description)
	if name == "" && len(meta) == 0 && desc == "" {
		return 0
	}

	pdf.SetTextColor(20, 20, 20)
	cur := y
	if name != "" {
		pdf.SetFont("Courier", "B", 14)
		cur += 6
		pdf.Text(x, cur, truncateToWidth(pdf, name, w))
		cur += 1
	}
	if len(meta) > 0 {
		pdf.SetFont("Courier", "", 9)
		cur += 5
		pdf.Text(x, cur, truncateToWidth(pdf, strings.Join(meta, "  |  "), w))
	}
	if desc != "" {
		pdf.SetFont("Courier", "", 8)
		lines := wrapText(pdf, desc, w)
		if len(lines) > projectDescMaxLines {
			lines = lines[:projectDescMaxLines]
			lines[projectDescMaxLines-1] = truncateToWidth(pdf, lines[projectDescMaxLines-1]+"...", w)
		}
		for _, l := range lines {
			cur += 4
			pdf.Text(x, cur, l)
		}
	}
	return cur - y + projectGapMM
}

// wrapText greedily breaks s at spaces into lines no wider than w in the
// current font; a single word wider than w is split by characters.
func wrapText(pdf *pdfDoc, s string, w float64) []string {
	var lines []string
	line := ""
	flush := func() {
		if line != "" {
			lines = append(lines, line)
			line = ""
		}
	}
	for _, word := range strings.Fields(s) {
		for pdf.GetStringWidth(word) > w {
			// Split an over-long word at the widest prefix that fits.
			flush()
			n := 0
			for i := range word {
				if i > 0 && pdf.GetStringWidth(word[:i]) > w {
					break
				}
				n = i
			}
			if n == 0 {
				_, n = utf8.DecodeRuneInString(word)
			}
			lines = append(lines, word[:n])
			word = word[n:]
		}
		switch {
		case line == "":
			line = word
		case pdf.GetStringWidth(line+" "+word) <= w:
			line += " " + word
		default:
			flush()
			line = word
		}
	}
	flush()
	return lines
}

// truncateToWidth shortens s with a trailing "..." until it fits within w.
func truncateToWidth(pdf *pdfDoc, s string, w float64) string {
	if pdf.GetStringWidth(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		if c := string(r) + "..."; pdf.GetStringWidth(c) <= w {
			return c
		}
	}
	return ""
}
