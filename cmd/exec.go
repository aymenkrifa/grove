package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	var (
		dryRun    bool
		keepGoing bool
	)
	c := &cobra.Command{
		Use:   "exec [selector] -- <command> [args...]",
		Short: "Run a command in every selected repository",
		Long: "The only route to a command that changes anything. grove has no pull,\n" +
			"push, checkout or commit of its own: a mutating command fanned out over\n" +
			"many repositories should be one the user typed in full.\n\n" +
			"Output streams live, repository by repository, in order.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			n := cmd.ArgsLenAtDash()
			if n < 0 || n >= len(args) {
				return errors.New("no command given; usage: grove exec [selector] -- <command> [args...]")
			}
			if n > 1 {
				return fmt.Errorf("at most one selector is allowed before --, got %d: %v", n, args[:n])
			}
			selector := firstArg(args[:n])
			argv := args[n:]

			_, found, err := resolveAndFind(selector)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if dryRun {
				fmt.Fprintf(out, "would run: %s\n", strings.Join(argv, " "))
				for _, f := range found {
					fmt.Fprintf(out, "  %s\n", f.RelPath)
				}
				return nil
			}

			for _, f := range found {
				fmt.Fprintf(out, "\n%s\n", f.RelPath)
				sub := exec.CommandContext(cmd.Context(), argv[0], argv[1:]...)
				sub.Dir = f.AbsPath
				sub.Stdout = out
				sub.Stderr = cmd.ErrOrStderr()
				sub.Stdin = os.Stdin
				if err := sub.Run(); err != nil {
					markPartialFailure()
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", f.RelPath, err)
					if !keepGoing {
						return nil
					}
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print the command and the repositories, run nothing")
	c.Flags().BoolVar(&keepGoing, "keep-going", false, "continue after a repository fails")
	return c
}
