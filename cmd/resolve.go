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
//
// Both forms answer the same question, so both enforce the same rule below.
// The cheaper form (no --scope, no walk) is what a shell whose command
// substitution cannot report an exit status uses as its in-grove probe — see
// hooks/fish.fish — which only works if it is exactly as strict as --scope.
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
			rel, inside := relWithin(res.Root, cwd)
			// There are two distinct ways to be outside a grove, and the hook
			// must refuse in both.
			//
			// The bare cwd fallback does not count: rule 6 resolves any
			// directory on the machine to itself, so accepting it would arm
			// the hook everywhere. config.Resolved.Source carries which
			// precedence rule matched; "working directory" is rule 6's exact
			// string.
			//
			// Nor is a resolved root necessarily an ancestor of cwd. Rule 2
			// ($GROVE_ROOT) and rule 5 (the configured default workspace)
			// never consult cwd at all, so both hand back a root the user may
			// be nowhere near — only rules 3 and 4 find the root *from* cwd.
			// Answering in that case makes the hook rewrite `git status` in
			// an unrelated directory into a report about a tree the user is
			// not in: git lying about the working directory, which is the
			// worst outcome this project has.
			if res.Source == "working directory" || !inside {
				return errors.New("not inside a configured grove")
			}
			if !scope {
				fmt.Fprintln(cmd.OutOrStdout(), res.Root)
				return nil
			}
			// Stdout only, and nothing else on it: the hook reads this
			// through a command substitution, so a diagnostic printed here
			// would be parsed as a selector, and a selector printed on stderr
			// would silently become the empty one — reporting the whole grove
			// where the user asked about one group.
			fmt.Fprintln(cmd.OutOrStdout(), scopeFor(res, rel))
			return nil
		},
	}
	c.Flags().BoolVar(&scope, "scope", false, "print the selector for the current subdirectory")
	return c
}

// relWithin reports whether cwd is root itself or lives beneath it, and
// returns the slash-separated path from root to cwd when it does.
//
// It is one function rather than a call to config.isWithin followed by a
// separate filepath.Rel because the two callers need different halves of the
// same answer — the command needs the boolean, scopeFor needs the path — and
// computing the same relationship twice is how the two answers drift apart.
// config.isWithin stays unexported for the same reason: exporting it would
// widen config's API for a predicate that on its own answers only half of
// what this file asks.
func relWithin(root, cwd string) (string, bool) {
	// Resolve both sides before comparing. On macOS /var is a symlink to
	// /private/var, so os.Getwd() hands back the resolved path while a
	// configured root keeps the one the user typed — and the two then fail to
	// match despite naming the same directory. The visible symptom is the
	// worst one this tool has: the git hook silently declines to fire, and a
	// workspace reached through any symlink looks like no grove at all.
	//
	// EvalSymlinks fails on a path that does not exist, which is not this
	// function's business to judge, so an unresolvable side falls back to
	// itself and the comparison proceeds on the literal paths.
	root, cwd = resolved(root), resolved(cwd)

	rel, err := filepath.Rel(root, cwd)
	if err != nil {
		return "", false
	}
	// The escape marker is the whole first segment, not the first two
	// characters: a directory genuinely inside the grove but named "..cache"
	// also starts with "..". Same test as config.isWithin.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// resolved follows symlinks, falling back to the path itself when it cannot
// — a path that does not exist is not a reason to refuse to compare.
func resolved(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// scopeFor turns rel — the path from the grove root down to the working
// directory, as relWithin returns it — into a selector, or "" when the whole
// grove is meant. A subdirectory that no repository lives under yields "", so
// the hook degrades to showing everything rather than erroring.
func scopeFor(res *config.Resolved, rel string) string {
	if rel == "." {
		return ""
	}
	found, _, err := discover.Walk(res.Root, res.Depth, res.Ignore)
	if err != nil {
		return ""
	}
	for _, f := range found {
		// rel+"/" rather than rel: a group directory named "we" must not
		// claim the repositories that live under its sibling "web".
		if f.RelPath == rel || strings.HasPrefix(f.RelPath, rel+"/") {
			return rel
		}
	}
	return ""
}
