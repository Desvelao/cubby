package cli

import (
	"encoding/csv"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/manifest"
)

func newListCmd() *cobra.Command {
	var format, groupFilter string

	cmd := &cobra.Command{
		Use:   "list <manifest.yaml>",
		Short: "List the components declared in a manifest",
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

			if groupFilter != "" && groupFilter != "-" {
				known := false
				for _, g := range m.Groups {
					if g.ID == groupFilter {
						known = true
						break
					}
				}
				if !known {
					return newExitError(ExitUsageError, fmt.Errorf("unknown --group %q: no group with that id", groupFilter))
				}
			}

			groupOf := map[string]string{}
			for _, g := range m.Groups {
				for _, cid := range g.Components {
					groupOf[cid] = g.ID
				}
			}

			type row struct {
				ID     string  `json:"id"`
				Name   string  `json:"name"`
				Group  string  `json:"group"`
				Width  float64 `json:"width"`
				Depth  float64 `json:"depth"`
				Height float64 `json:"height"`
				Qty    int     `json:"qty"`
			}
			rows := []row{} // non-nil so an empty result serialises as [] not null
			for _, c := range m.Components {
				group := groupOf[c.ID]
				if groupFilter == "-" {
					if group != "" {
						continue
					}
				} else if groupFilter != "" && group != groupFilter {
					continue
				}
				g := group
				if g == "" {
					g = "-"
				}
				rows = append(rows, row{c.ID, c.Name, g, c.Width, c.Depth, c.Height, c.Qty})
			}

			out := cmd.OutOrStdout()
			switch format {
			case "table", "":
				tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
				ew := &export.ErrWriter{W: tw}
				ew.Println("ID\tNAME\tWIDTH\tDEPTH\tHEIGHT\tQTY\tGROUP")
				for _, r := range rows {
					ew.Printf("%s\t%s\t%g\t%g\t%g\t%d\t%s\n", r.ID, r.Name, r.Width, r.Depth, r.Height, r.Qty, r.Group)
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				return ew.Err
			case "csv":
				cw := csv.NewWriter(out)
				if err := cw.Write([]string{"id", "name", "width", "depth", "height", "qty", "group"}); err != nil {
					return err
				}
				for _, r := range rows {
					rec := []string{r.ID, r.Name, fmt.Sprintf("%g", r.Width), fmt.Sprintf("%g", r.Depth), fmt.Sprintf("%g", r.Height), fmt.Sprintf("%d", r.Qty), r.Group}
					if err := cw.Write(rec); err != nil {
						return err
					}
				}
				cw.Flush()
				if err := cw.Error(); err != nil {
					return err
				}
			case "json":
				return writeJSON(out, rows)
			default:
				return newExitError(ExitUsageError, fmt.Errorf("unknown --format %q (expected table, csv, or json)", format))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "table", "output format: table|csv|json")
	cmd.Flags().StringVar(&groupFilter, "group", "", "only show components in this group id (use '-' for ungrouped)")
	return cmd
}
