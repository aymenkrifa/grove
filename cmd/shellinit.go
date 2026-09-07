package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/shell"
)

func newShellInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell-init <zsh|bash|fish>",
		Short: "Print the git wrapper for your shell",
		Long: "Add to your shell startup file:\n\n" +
			"  zsh, bash:  eval \"$(grove shell-init zsh)\"\n" +
			"  fish:       grove shell-init fish | source\n\n" +
			"The wrapper forwards git status, diff, fetch, log and branch to grove only\n" +
			"when the working directory is inside a configured grove but not inside a\n" +
			"repository. It prints the grove command it runs. Set GROVE_NO_GIT_HOOK=1 to\n" +
			"disable it, or use `command git` to bypass it once.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"zsh", "bash", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := shell.Hook(args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), body)
			return nil
		},
	}
}
