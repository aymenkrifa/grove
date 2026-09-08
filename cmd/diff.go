package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/git"
	"github.com/aymenkrifa/grove/internal/render"
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
			res, found, err := resolveAndFind(selector)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			// git turns colour off whenever its stdout is not a terminal, and
			// grove's always is a buffer — so a diff arrives plain unless we
			// ask. We ask on grove's own terms: the same decision that colours
			// the status table, which already accounts for --color, NO_COLOR
			// and whether a human is watching. Redirecting to a file therefore
			// still yields a clean patch.
			//
			// It goes before the passthrough args so a user's own --color or
			// --no-color after -- still wins: git honours the last one.
			colorArg := "--color=never"
			if renderOptions(res, out).Color {
				colorArg = "--color=always"
			}

			// A selector that narrows to exactly one repository, and no
			// --stat: show the real diff. len(found)==1 alone is not enough
			// — a workspace that simply *contains* only one repository must
			// still get the --stat form when no selector was given, per
			// §5.2 ("without a selector: a per-repo --stat summary").
			if selector != "" && len(found) == 1 && !statOnly {
				gitArgs := append([]string{"diff", colorArg}, passthrough...)
				body, err := git.Run(cmd.Context(), found[0].AbsPath, gitArgs...)
				if err != nil {
					markPartialFailure()
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", found[0].RelPath, err)
					return nil
				}
				return page(out, body)
			}

			ropts := renderOptions(res, out)
			var b strings.Builder
			for _, f := range found {
				gitArgs := append([]string{"diff", "--stat", colorArg}, passthrough...)
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
				// A blank line before every heading but the first: the
				// blocks are separate documents and reading them as one
				// wall of stat output is the thing the banner exists to
				// prevent.
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(render.RepoHeading(f.RelPath, body, ropts))
				b.WriteString(body)
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
	// less renders ANSI as literal garbage unless told otherwise, so a
	// coloured diff needs -R. git sets exactly this default for exactly this
	// reason; an LESS the user has set themselves is left alone.
	cmd.Env = os.Environ()
	if _, set := os.LookupEnv("LESS"); !set {
		cmd.Env = append(cmd.Env, "LESS=FRX")
	}
	cmd.Stdin = strings.NewReader(body)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_, werr := fmt.Fprint(out, body) // pager failed: print plainly
		return werr
	}
	return nil
}
