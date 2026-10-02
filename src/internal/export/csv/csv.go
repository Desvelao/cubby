// Package csv renders panel geometry as a cut list / bill of materials.
package csv

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

type Exporter struct{}

func (Exporter) Format() string { return "csv" }

func (Exporter) Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"panel_id", "axis", "length_mm", "height_mm", "thickness_mm", "material", "notch_count", "qty", "cut_mm", "notch_positions", "ids"}); err != nil {
		return err
	}

	for _, g := range export.GroupPanels(panels) {
		row := []string{
			safeText(g.Panel.ID), string(g.Panel.Axis),
			fmt.Sprintf("%g", g.Panel.Length), fmt.Sprintf("%g", g.Panel.Height), fmt.Sprintf("%g", mat.Thickness),
			safeText(mat.Name), fmt.Sprintf("%d", len(g.Panel.Notches)), fmt.Sprintf("%d", g.Qty),
			fmt.Sprintf("%g", g.Panel.Cut), export.NotchSignature(g.Panel.Notches), safeText(strings.Join(g.IDs, ";")),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// safeText neutralises spreadsheet formula injection: a text cell starting
// with =, +, -, @, tab or carriage return is prefixed with a single quote so
// Excel/Sheets treat it as text. Numeric columns are not passed through it.
func safeText(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
