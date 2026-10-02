// Package cli wires up the cubby command-line interface.
package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd builds the cubby root command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "cubby",
		Short:         "Design boardgame box inserts from a YAML manifest",
		Long:          "cubby reads a YAML manifest describing a box and its components, packs the components into compartments, and exports a slotted divider-panel insert design.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newValidateCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newGroupsCmd())
	root.AddCommand(newBuildCmd())
	root.AddCommand(newVersionCmd())

	return root
}
