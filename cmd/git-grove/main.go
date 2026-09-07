// Command git-grove lets grove be invoked as `git grove ...`. Git treats any
// git-<name> executable on PATH as a subcommand, so this needs no shell support.
package main

import (
	"os"

	"github.com/aymenkrifa/grove/cmd"
)

func main() { os.Exit(cmd.ExecuteAsGitSubcommand()) }
