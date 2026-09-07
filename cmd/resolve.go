package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/discover"
)

// newResolveCmd backs the shell hook in Task 12: the hook has to ask "am I
// inside a configured grove, and if so what sub-scope?" without running a
// full status. It is hidden because it is a protocol between grove and its
// own generated shell function, not a user-facing command.
//
// It calls config.Resolve directly rather than resolveAndFind. resolveAndFind
// already calls git.Available() as a preflight, and every other command needs
// that: they are all about to run git. __resolve is different — it only
// resolves paths, never touches a repository, and is specifically the check
// the shell hook runs *before* it knows whether git matters at all. Requiring
// git on PATH here would make the hook itself unusable on a machine that
// somehow has grove but not git, for no benefit: nothing below calls git.
func newResolveCmd() *cobra.Command {
	var scope bool
	c := &cobra.Command{
		Use:    "__resolve",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			res, err := config.Resolve(cfg, config.Opts{Cwd: cwd})
			if err != nil {
				return err
			}
			// The bare cwd fallback does not count: the hook must only fire
			// inside a grove the user actually configured or marked, never in
			// any random directory. config.Resolved.Source carries which
			// precedence rule matched; "working directory" is rule 6, the
			// fallthrough that applies everywhere.
			if res.Source == "working directory" {
				return errors.New("not inside a configured grove")
			}
			if !scope {
				fmt.Fprintln(cmd.OutOrStdout(), res.Root)
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), scopeFor(res, cwd))
			return nil
		},
	}
	c.Flags().BoolVar(&scope, "scope", false, "print the selector for the current subdirectory")
	return c
}

// scopeFor turns the working directory into a selector, or "" when the whole
// grove is meant. A subdirectory that no repository lives under yields "", so
// the hook degrades to showing everything rather than erroring.
func scopeFor(res *config.Resolved, cwd string) string {
	rel, err := filepath.Rel(res.Root, cwd)
	// A root chosen by GROVE_ROOT or a default workspace need not be an
	// ancestor of cwd at all, so an escaping "rel" is a real case here, not
	// just defensive. The check mirrors config.isWithin: testing only for a
	// ".." *prefix* would also reject a legitimate sibling whose name merely
	// starts with two dots, such as "..cache".
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	rel = filepath.ToSlash(rel)
	found, _, err := discover.Walk(res.Root, res.Depth, res.Ignore)
	if err != nil {
		return ""
	}
	for _, f := range found {
		if f.RelPath == rel || strings.HasPrefix(f.RelPath, rel+"/") {
			return rel
		}
	}
	return ""
}
