// Command cubby designs boardgame box inserts from a YAML manifest.
package main

import (
	"fmt"
	"os"

	"github.com/Desvelao/cubby/internal/cli"
)

func main() {
	root := cli.NewRootCmd()
	err := root.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(cli.ExitCode(err))
}
