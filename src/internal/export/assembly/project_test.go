package assembly

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/go-pdf/fpdf"

	"github.com/Desvelao/cubby/internal/manifest"
)

func projectFixture() manifest.Project {
	return manifest.Project{
		Name:        `Gateway & "Co" Insert`,
		Description: "Two-tier insert for the starter box. " + strings.Repeat("Cut from 5 mm foamboard, glue floors first. ", 12),
		Revision:    "1.2.0",
		Author:      "José Ñandú — 東",
		License:     "CC-BY-4.0",
		Tags:        []string{"lorcana", "insert & tray", "東"},
	}
}

func exportWithProject(t *testing.T, p *manifest.Project) []byte {
	t.Helper()
	box, panels, mat := determinismFixture()
	box.Project = p
	fixed := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	var buf bytes.Buffer
	if err := (Exporter{Now: func() time.Time { return fixed }}).Export(&buf, box, panels, mat); err != nil {
		t.Fatalf("Export: %v", err)
	}
	return buf.Bytes()
}

// utf16be encodes s as fpdf writes a UTF-8 Info string: a BOM then UTF-16BE.
func utf16be(s string) []byte {
	out := []byte{0xFE, 0xFF}
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u>>8), byte(u))
	}
	return out
}

func TestExportProjectInfoDictionary(t *testing.T) {
	p := projectFixture()
	pdf := exportWithProject(t, &p)
	// The Info strings are UTF-16BE (with a BOM), so check the key and the
	// encoded value rather than scanning for ASCII text.
	for _, want := range []struct{ key, val string }{
		{"/Title", p.Name},
		{"/Author", p.Author},
		{"/Keywords", "lorcana, insert & tray, 東"},
	} {
		if !bytes.Contains(pdf, append([]byte(want.key+" ("), utf16be(want.val)...)) {
			t.Errorf("Info dictionary missing %s = %q", want.key, want.val)
		}
	}
	if !bytes.Contains(pdf, []byte("/Subject (")) || !bytes.Contains(pdf, []byte("/Creator (")) {
		t.Error("Info dictionary missing /Subject or /Creator")
	}
}

func TestExportProjectAbsentLeavesInfoUntouched(t *testing.T) {
	for _, p := range []*manifest.Project{nil, {}, {Game: "only unrelated"}} {
		pdf := exportWithProject(t, p)
		for _, key := range []string{"/Title", "/Author", "/Subject", "/Keywords", "/Creator"} {
			if p != nil && p.Game != "" && key == "/Creator" {
				continue // a non-empty project always stamps the generator
			}
			if bytes.Contains(pdf, []byte(key+" (")) {
				t.Errorf("project %+v: unexpected %s in Info", p, key)
			}
		}
	}
	// A nil and an empty project produce identical bytes.
	if !bytes.Equal(exportWithProject(t, nil), exportWithProject(t, &manifest.Project{})) {
		t.Error("empty project changed the output")
	}
}

func TestExportProjectDeterministic(t *testing.T) {
	p := projectFixture()
	if !bytes.Equal(exportWithProject(t, &p), exportWithProject(t, &p)) {
		t.Fatal("output with project differs between runs")
	}
}

func TestExportProjectTextAndInfoViaPoppler(t *testing.T) {
	p := projectFixture()
	pdf := exportWithProject(t, &p)
	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}

	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not available")
	}
	out, err := exec.Command(bin, "-enc", "UTF-8", "-f", "1", "-l", "1", path, "-").Output()
	if err != nil {
		t.Skipf("pdftotext failed: %v", err)
	}
	text := string(out)
	for _, want := range []string{`Gateway & "Co" Insert`, "rev 1.2.0", "José Ñandú", "CC-BY-4.0", "Two-tier insert", "..."} {
		if !strings.Contains(text, want) {
			t.Errorf("page 1 text missing %q:\n%s", want, text)
		}
	}
	// The isometric legend is still there, below the block.
	if !strings.Contains(text, "isometric preview") {
		t.Errorf("isometric legend missing:\n%s", text)
	}

	if info, err := exec.LookPath("pdfinfo"); err == nil {
		o, err := exec.Command(info, "-enc", "UTF-8", path).Output()
		if err != nil {
			t.Fatalf("pdfinfo: %v", err)
		}
		for _, want := range []string{`Gateway & "Co" Insert`, "José Ñandú — 東", "lorcana, insert & tray, 東"} {
			if !strings.Contains(string(o), want) {
				t.Errorf("pdfinfo missing %q:\n%s", want, o)
			}
		}
	}
}

func TestDrawProjectBlockHeight(t *testing.T) {
	pdf := newPDFDoc(fpdf.New("P", "mm", "A4", ""))
	pdf.AddPage()
	if h := drawProjectBlock(pdf, nil, 15, 15, 180); h != 0 {
		t.Errorf("nil project height = %v", h)
	}
	if h := drawProjectBlock(pdf, &manifest.Project{Tags: []string{"x"}, Game: "g"}, 15, 15, 180); h != 0 {
		t.Errorf("unrelated-only project height = %v", h)
	}
	p := projectFixture()
	h := drawProjectBlock(pdf, &p, 15, 15, 180)
	// name + meta + at most projectDescMaxLines description lines + gap.
	if h <= 0 || h > 6+1+5+4*projectDescMaxLines+projectGapMM+0.001 {
		t.Errorf("height = %v", h)
	}
}

func TestWrapTextLongWordAndLimits(t *testing.T) {
	pdf := newPDFDoc(fpdf.New("P", "mm", "A4", ""))
	pdf.AddPage()
	pdf.SetFont("Courier", "", 8)
	long := strings.Repeat("W", 300)
	for _, l := range wrapText(pdf, "a "+long+" b", 100) {
		if pdf.GetStringWidth(l) > 100 {
			t.Errorf("line too wide: %q", l)
		}
	}
	if got := strings.Join(wrapText(pdf, "a "+long+" b", 100), ""); strings.ReplaceAll(strings.ReplaceAll(got, "W", ""), " ", "") != "ab" {
		t.Errorf("wrapping lost text: %q", got)
	}
}
