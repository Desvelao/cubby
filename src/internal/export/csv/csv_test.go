package csv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func TestExportGroupsIdenticalPanelsIntoOneRow(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Notches: nil},
		{ID: "h-2", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Notches: nil},
		{ID: "v-1", Axis: geometry.AxisDepthRun, Length: 145, Height: 60, Notches: []geometry.Notch{{Pos: 0, Width: 3}}},
	}

	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
		t.Fatal(err)
	}

	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// header + 2 distinct panel types (h-1/h-2 collapse into one row of qty 2, v-1 its own row)
	if len(rows) != 3 {
		t.Fatalf("expected header + 2 rows, got %d: %+v", len(rows), rows)
	}
	var widthRunQty string
	for _, r := range rows[1:] {
		if r[1] == "width-run" {
			widthRunQty = r[7]
		}
	}
	if widthRunQty != "2" {
		t.Fatalf("expected qty 2 for the width-run row, got %q in rows %+v", widthRunQty, rows)
	}
}

func exportRows(t *testing.T, panels []geometry.Panel) [][]string {
	t.Helper()
	var buf bytes.Buffer
	if err := (Exporter{}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{Name: "Foamboard", Thickness: 3}); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestExportAggregatesUsingFirstSeenPanelID(t *testing.T) {
	notch := []geometry.Notch{{Pos: 5, Width: 3, Edge: "bottom"}}
	panels := []geometry.Panel{
		{ID: "first", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Notches: notch},
		{ID: "second", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Notches: notch},
	}
	rows := exportRows(t, panels)
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 row, got %+v", rows)
	}
	if rows[1][0] != "first" || rows[1][7] != "2" {
		t.Fatalf("expected first-seen id with qty 2, got %+v", rows[1])
	}
}

func TestExportSeparatesPanelsDifferingOnlyByNotchPosition(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Notches: []geometry.Notch{{Pos: 5, Width: 3, Edge: "bottom"}}},
		{ID: "b", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Notches: []geometry.Notch{{Pos: 40, Width: 3, Edge: "bottom"}}},
	}
	rows := exportRows(t, panels)
	if len(rows) != 3 {
		t.Fatalf("expected header + 2 rows, got %+v", rows)
	}
	if rows[1][0] != "a" || rows[1][7] != "1" || rows[2][0] != "b" || rows[2][7] != "1" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

type failAfter struct {
	n   int
	err error
}

func (f *failAfter) Write(p []byte) (int, error) {
	if f.n < len(p) {
		return 0, f.err
	}
	f.n -= len(p)
	return len(p), nil
}

func TestExportPropagatesWriteErrors(t *testing.T) {
	boom := errors.New("boom")
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 100, Height: 50},
		{ID: "b", Axis: geometry.AxisDepthRun, Length: 80, Height: 50},
	}
	mat := manifest.Material{Name: "Foamboard", Thickness: 3}
	// The csv writer buffers (4096 bytes), so any failure surfaces at Flush;
	// a zero budget covers both the header write and the final flush.
	for _, n := range []int{0, 1, 10, 50} {
		err := (Exporter{}).Export(&failAfter{n: n, err: boom}, pack.BoxResult{}, panels, mat)
		if !errors.Is(err, boom) {
			t.Fatalf("budget %d: expected boom, got %v", n, err)
		}
	}
}

func TestExportAppendsCutNotchPositionsAndAllIDs(t *testing.T) {
	notch := []geometry.Notch{{Pos: 40, Width: 3, Edge: "bottom"}, {Pos: 5, Width: 3, Edge: "bottom"}}
	panels := []geometry.Panel{
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Cut: 10, Notches: notch},
		{ID: "b", Axis: geometry.AxisWidthRun, Length: 100, Height: 50, Cut: 10, Notches: notch},
		{ID: "c", Axis: geometry.AxisWidthRun, Length: 100, Height: 50},
	}
	rows := exportRows(t, panels)
	want := "panel_id,axis,length_mm,height_mm,thickness_mm,material,notch_count,qty,cut_mm,notch_positions,ids"
	if got := strings.Join(rows[0], ","); got != want {
		t.Fatalf("header = %q, want %q", got, want)
	}
	if len(rows) != 3 {
		t.Fatalf("expected header + 2 rows, got %+v", rows)
	}
	// Sorted by cut: uncut group first.
	if r := rows[1]; r[8] != "0" || r[9] != "" || r[10] != "c" {
		t.Fatalf("unexpected uncut row %q", r)
	}
	if r := rows[2]; r[0] != "a" || r[6] != "2" || r[7] != "2" || r[8] != "10" || r[9] != "bottom:5:3|bottom:40:3" || r[10] != "a;b" {
		t.Fatalf("unexpected cut row %q", r)
	}
}

func TestExportNeutralisesFormulaInjectionInTextCells(t *testing.T) {
	for _, lead := range []string{"=", "+", "-", "@", "\t"} {
		id := lead + "cmd"
		var buf bytes.Buffer
		panels := []geometry.Panel{{ID: id, Axis: geometry.AxisWidthRun, Length: 10, Height: 5}}
		if err := (Exporter{}).Export(&buf, pack.BoxResult{}, panels, manifest.Material{Name: lead + "mat", Thickness: 3}); err != nil {
			t.Fatal(err)
		}
		rows, err := csv.NewReader(&buf).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		r := rows[1]
		if r[0] != "'"+id || r[5] != "'"+lead+"mat" || r[10] != "'"+id {
			t.Errorf("lead %q: got panel_id=%q material=%q ids=%q", lead, r[0], r[5], r[10])
		}
		if r[2] != "10" || r[4] != "3" {
			t.Errorf("numeric columns altered: %v", r)
		}
	}
}

func TestExportLeavesNormalTextUnchanged(t *testing.T) {
	rows := exportRows(t, []geometry.Panel{{ID: "iv-a-1-1", Axis: geometry.AxisWidthRun, Length: 10, Height: 5}})
	if rows[1][0] != "iv-a-1-1" || rows[1][5] != "Foamboard" || rows[1][10] != "iv-a-1-1" {
		t.Errorf("normal cells changed: %v", rows[1])
	}
}

func TestExportNeutralisesLeadingFormulaInFirstOfMergedIDs(t *testing.T) {
	rows := exportRows(t, []geometry.Panel{
		{ID: "=A1", Axis: geometry.AxisWidthRun, Length: 10, Height: 5},
		{ID: "h-2", Axis: geometry.AxisWidthRun, Length: 10, Height: 5},
	})
	if rows[1][10] != "'=A1;h-2" {
		t.Errorf("ids cell = %q", rows[1][10])
	}
}
