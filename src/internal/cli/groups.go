package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
)

func newGroupsCmd() *cobra.Command {
	var format, groupFilter string

	cmd := &cobra.Command{
		Use:   "groups <manifest.yaml>",
		Short: "Report packed/remaining/missing space per compartment, without exporting a design",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.Load(args[0])
			if err != nil {
				return newExitError(ExitUsageError, err)
			}
			if res := manifest.Validate(m); !res.OK() {
				for _, iss := range res.Issues {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), iss)
				}
				return newExitError(ExitValidationFailed, fmt.Errorf("manifest is invalid; run `cubby validate` for details"))
			}

			boxResult, err := pack.LayoutBox(m)
			if err != nil {
				return newExitError(manifestErrorCode(err), err)
			}

			compartments := boxResult.Compartments
			if groupFilter != "" {
				var filtered []pack.CompartmentResult
				for _, c := range compartments {
					if c.ID == groupFilter {
						filtered = append(filtered, c)
					}
				}
				if len(filtered) == 0 {
					return newExitError(ExitUsageError, fmt.Errorf("unknown --group %q: no compartment with that id", groupFilter))
				}
				compartments = filtered

				// Scope the missing-item totals to the filtered set so the
				// summary, JSON and exit code agree. Box-level geometry
				// (interior size, used and remaining space) stays as-is.
				scoped := make([]pack.MissingItem, 0)
				for _, c := range filtered {
					scoped = append(scoped, c.Missing...)
				}
				boxResult.Compartments = filtered
				boxResult.TotalMissing = scoped
			}

			out := cmd.OutOrStdout()
			switch format {
			case "table", "":
				tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
				tew := &export.ErrWriter{W: tw}
				tew.Println("ID\tNAME\tKIND\tUSED (W x D)\tREMAINING (W x D)\tMISSING")
				for _, c := range compartments {
					tew.Printf("%s\t%s\t%s\t%.1f x %.1f\t%.1f x %.1f\t%d\n",
						c.ID, c.Name, c.Kind, c.UsedW, c.UsedD, c.Remaining.Width, c.Remaining.Depth, missingCount(c.Missing))
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				if tew.Err != nil {
					return tew.Err
				}
				ew := &export.ErrWriter{W: out}
				if groupFilter != "" {
					c := compartments[0]
					ew.Printf("\ncompartment %q: used %.1f x %.1f mm, %.1f x %.1f mm remaining, %d component(s) missing\n",
						c.ID, c.UsedW, c.UsedD, c.Remaining.Width, c.Remaining.Depth, missingCount(c.Missing))
				} else {
					ew.Printf("\nbox %q: used %.1f x %.1f of %.1f x %.1f mm, %.1f x %.1f mm remaining, %d component(s) missing\n",
						boxResult.BoxName, boxResult.UsedW, boxResult.UsedD, boxResult.InteriorW, boxResult.InteriorD,
						boxResult.Remaining.Width, boxResult.Remaining.Depth, missingCount(boxResult.TotalMissing))
				}
				if ew.Err != nil {
					return ew.Err
				}
			case "json":
				if err := writeJSON(out, nonNilBoxResult(boxResult)); err != nil {
					return err
				}
			default:
				return newExitError(ExitUsageError, fmt.Errorf("unknown --format %q (expected table or json)", format))
			}

			if len(boxResult.TotalMissing) > 0 {
				return newExitError(ExitPackingIncomplete, fmt.Errorf("%d component(s) did not fit", missingCount(boxResult.TotalMissing)))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "table", "output format: table|json")
	cmd.Flags().StringVar(&groupFilter, "group", "", "only show this compartment id")
	return cmd
}

func missingCount(items []pack.MissingItem) int {
	n := 0
	for _, m := range items {
		n += m.Rejected
	}
	return n
}

// nonNilBoxResult returns a copy of r whose slices are all non-nil, so the
// JSON output has [] rather than null for empty compartments, rows, items and
// missing lists. The input is not modified.
func nonNilBoxResult(r pack.BoxResult) pack.BoxResult {
	if r.Compartments == nil {
		r.Compartments = []pack.CompartmentResult{}
	}
	if r.TotalMissing == nil {
		r.TotalMissing = []pack.MissingItem{}
	}
	comps := make([]pack.CompartmentResult, len(r.Compartments))
	for i, c := range r.Compartments {
		if c.Missing == nil {
			c.Missing = []pack.MissingItem{}
		}
		rows := make([]pack.ShelfRow, len(c.Rows))
		for j, row := range c.Rows {
			if row.Items == nil {
				row.Items = []pack.PlacedItem{}
			}
			rows[j] = row
		}
		c.Rows = rows
		comps[i] = c
	}
	r.Compartments = comps
	return r
}
