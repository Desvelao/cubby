// Package console renders a packed box layout as a human-readable report.
package console

import (
	"io"
	"strings"
	"text/tabwriter"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

type Exporter struct {
	// BoxCase adds the box case dimensions to the report.
	BoxCase bool
}

func (Exporter) Format() string { return "console" }

func (e Exporter) Export(w io.Writer, box pack.BoxResult, panels []geometry.Panel, mat manifest.Material) error {
	ew := &export.ErrWriter{W: w}

	ew.Printf("Box: %s (interior %.1f x %.1f x %.1f mm)\n", box.BoxName, box.InteriorW, box.InteriorD, box.InteriorH)
	ew.Printf("Material: %s, %.2fmm thick, %.2fmm kerf\n", mat.Name, mat.Thickness, mat.Kerf)
	if e.BoxCase {
		ew.Printf("Box case: %.1f x %.1f x %.1f mm interior, %.1f mm walls\n", box.InteriorW, box.InteriorD, box.InteriorH, export.CaseWallThickness)
	}
	printProject(ew, box.Project)
	ew.Println()

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	tew := &export.ErrWriter{W: tw}
	tew.Println("COMPARTMENT\tKIND\tUSED (W x D)\tREMAINING (W x D)\tMISSING")
	for _, c := range box.Compartments {
		tew.Printf("%s\t%s\t%.1f x %.1f\t%.1f x %.1f\t%d\n",
			nameOrID(c.Name, c.ID), c.Kind, c.UsedW, c.UsedD, c.Remaining.Width, c.Remaining.Depth, rejectedCount(c.Missing))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if ew.Err != nil {
		return ew.Err
	}
	if tew.Err != nil {
		return tew.Err
	}

	if len(panels) > 0 {
		ew.Printf("\nPanels: %d\n", len(panels))
	}

	ew.Printf("\nBox total: used %.1f x %.1f of %.1f x %.1f mm, %.1f x %.1f mm remaining\n",
		box.UsedW, box.UsedD, box.InteriorW, box.InteriorD, box.Remaining.Width, box.Remaining.Depth)

	if n := rejectedCount(box.TotalMissing); n > 0 {
		ew.Printf("\nWARNING: %d component instance(s) did not fit:\n", n)
		for _, m := range box.TotalMissing {
			ew.Printf("  - %s: requested %d, placed %d, rejected %d (%s)\n", m.ComponentID, m.Requested, m.Placed, m.Rejected, m.Reason)
		}
	}

	return ew.Err
}

// printProject writes the project summary lines, only for the fields that are
// set: "Project: <name> (rev <revision>) by <author>" and, if present, a
// "Description:" line (newlines collapsed so it stays on one line).
func printProject(ew *export.ErrWriter, p *manifest.Project) {
	if p == nil {
		return
	}
	line := p.Name
	if p.Revision != "" {
		if line != "" {
			line += " "
		}
		line += "(rev " + p.Revision + ")"
	}
	if p.Author != "" {
		if line != "" {
			line += " "
		}
		line += "by " + p.Author
	}
	if line != "" {
		ew.Printf("Project: %s\n", line)
	}
	if d := strings.Join(strings.Fields(p.Description), " "); d != "" {
		ew.Printf("Description: %s\n", d)
	}
}

func nameOrID(name, id string) string {
	if name != "" {
		return name
	}
	return id
}

func rejectedCount(items []pack.MissingItem) int {
	n := 0
	for _, m := range items {
		n += m.Rejected
	}
	return n
}
