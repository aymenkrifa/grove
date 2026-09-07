package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/git"
)

func newDiffCmd() *cobra.Command {
	var statOnly bool
	c := &cobra.Command{
		Use:   "diff [selector] [-- <git diff args>]",
		Short: "Show what has changed, across repositories or within one",
		Long: "With no selector, prints a per-repository diffstat and omits repositories\n" +
			"with no changes. With a selector matching a single repository, prints that\n" +
			"repository's full diff. Everything after -- is passed to git diff verbatim.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			selector, passthrough, err := splitPassthrough(cmd, args)
			if err != nil {
				return err
			}
			_, found, err := resolveAndFind(selector)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			// A selector that narrows to exactly one repository, and no
			// --stat: show the real diff. len(found)==1 alone is not enough
			// — a workspace that simply *contains* only one repository must
			// still get the --stat form when no selector was given, per
			// §5.2 ("without a selector: a per-repo --stat summary").
			if selector != "" && len(found) == 1 && !statOnly {
				gitArgs := append([]string{"diff"}, passthrough...)
				body, err := git.Run(cmd.Context(), found[0].AbsPath, gitArgs...)
				if err != nil {
					markPartialFailure()
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", found[0].RelPath, err)
					return nil
				}
				return page(out, body)
			}

			var b strings.Builder
			for _, f := range found {
				gitArgs := append([]string{"diff", "--stat"}, passthrough...)
				body, err := git.Run(cmd.Context(), f.AbsPath, gitArgs...)
				if err != nil {
					// Diagnostics go to stderr, never into the (possibly
					// paged) stdout body, so `grove diff 2>/dev/null` and a
					// piped --stat summary both stay clean.
					markPartialFailure()
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", f.RelPath, err)
					continue
				}
				if strings.TrimSpace(body) == "" {
					continue // nothing changed here
				}
				fmt.Fprintf(&b, "%s\n%s\n", f.RelPath, body)
			}
			return page(out, b.String())
		},
	}
	c.Flags().BoolVar(&statOnly, "stat", false, "always show the summary form, even for a single repository")
	return c
}

// splitPassthrough separates the selector from arguments after --. At most
// one argument may come before the --, matching every other command in this
// batch: a second bare argument there is rejected rather than silently
// dropped.
func splitPassthrough(cmd *cobra.Command, args []string) (string, []string, error) {
	n := cmd.ArgsLenAtDash()
	pre, post := args, []string(nil)
	if n >= 0 {
		pre, post = args[:n], args[n:]
	}
	if len(pre) > 1 {
		return "", nil, fmt.Errorf("at most one selector is allowed, got %d: %v", len(pre), pre)
	}
	return firstArg(pre), post, nil
}

// page sends body through the user's pager when stdout is a terminal.
//
// This is the one place grove builds a shell command line, and only to
// honour $PAGER, which is by definition a shell command line the user chose.
// No grove data — repository names, diff content, anything git produced — is
// interpolated into it; body is only ever passed to the pager's stdin.
func page(out io.Writer, body string) error {
	pager := os.Getenv("GIT_PAGER")
	if pager == "" {
		pager = os.Getenv("PAGER")
	}
	if pager == "" || !isTerminal(out) {
		_, err := fmt.Fprint(out, body)
		return err
	}
	cmd := exec.Command("sh", "-c", pager)
	cmd.Stdin = strings.NewReader(body)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_, werr := fmt.Fprint(out, body) // pager failed: print plainly
		return werr
	}
	return nil
}
