package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Desvelao/cubby/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the cubby version",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "cubby %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date)
			return err
		},
	}
}
