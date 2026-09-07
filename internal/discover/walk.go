// Package discover finds git repositories beneath a root and narrows them by
// a user-supplied selector.
package discover

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Found is a repository located by the walk.
type Found struct {
	AbsPath string
	RelPath string // relative to the root; "." when the root is itself a repo
	Group   string // first path segment, or "" when the repo is at the root
}

// Walk returns every repository at or beneath root, to at most depth levels,
// alongside one warning line for each directory it could not read.
//
// Descent stops at the first repository on any path, so submodules and nested
// clones are not reported separately.
//
// The two unreadable-directory cases are deliberately different. An unreadable
// *root* is a returned error: the caller named that directory, and answering
// with an empty slice would misreport "you cannot read this" as "there is
// nothing here". An unreadable directory *below* the root is skipped, described
// in the returned warnings, and the walk carries on, so one locked subtree
// never costs the caller the rest of the tree. This holds at every level,
// including the depth limit itself, where nothing is read at all. Warnings are
// returned rather than printed because only Walk knows a directory was skipped,
// and only the caller knows where its diagnostics go.
func Walk(root string, depth int, ignore []string) ([]Found, []string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	var (
		out      []Found
		warnings []string
	)
	// recurse returns an error only for the directory it was asked to read.
	// Its caller decides what that means: fatal at the root, a warning below.
	var recurse func(dir string, level int) error
	recurse = func(dir string, level int) error {
		repo, err := isRepo(dir)
		if err != nil {
			// Cannot examine this directory. Reported, never swallowed: the
			// depth guard below returns without reading anything, so an
			// unreadable directory at exactly the limit would otherwise
			// disappear in silence — and if it were a repository, disappear
			// from the results entirely.
			return err
		}
		if repo {
			out = append(out, newFound(abs, dir))
			return nil // stop descending: submodules are not separate repos
		}
		if level >= depth {
			return nil
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			// A symlink's DirEntry type is ModeSymlink and never a directory,
			// so this test also declines to follow symlinks, which would
			// invite cycles and double-report a repo reachable by two names.
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			child := filepath.Join(dir, e.Name())
			if matchesAny(ignore, child, abs) {
				continue
			}
			if err := recurse(child, level+1); err != nil {
				warnings = append(warnings,
					fmt.Sprintf("skipped %s: %s", relOf(abs, child), reason(err)))
			}
		}
		return nil
	}
	if err := recurse(abs, 0); err != nil {
		return nil, nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, warnings, nil
}

// isRepo reports whether dir is a repository, in either of the two shapes git
// uses.
//
// A repository with a working tree carries a .git entry: a directory normally,
// or a file for a linked worktree or a submodule. A bare repository carries no
// .git at all — the directory *is* the git directory — so it is recognised by
// its contents: HEAD beside an objects directory, the shape `git init --bare`
// produces.
//
// The second test also matches the .git directory of an ordinary repository,
// which is correct in isolation but must not produce a duplicate entry during
// the walk. It cannot: dot-prefixed directories are never descended into, and
// descent stops at the working tree above it either way.
//
// A returned error means neither question could be answered — the directory is
// unreadable, almost always a permissions problem. That is deliberately NOT
// folded into a false result. "I cannot tell" and "no" lead to different
// places: one is a warning the user can act on, the other is a repository
// quietly missing from the listing.
func isRepo(dir string) (bool, error) {
	git, err := entryExists(filepath.Join(dir, ".git"))
	if err != nil {
		return false, err
	}
	if git {
		return true, nil
	}
	// No .git. It may still be a bare repository.
	head, err := entryExists(filepath.Join(dir, "HEAD"))
	if err != nil || !head {
		return false, err
	}
	// HEAD was statable, so the directory is searchable and a failure on
	// objects means it is simply not there.
	info, serr := os.Stat(filepath.Join(dir, "objects"))
	return serr == nil && info.IsDir(), nil
}

// entryExists reports whether path exists, without following a final symlink.
// A missing entry is (false, nil); anything else — permission denied above all
// — is an error, because "I cannot look" must not be flattened into "it is not
// there". Both of isRepo's probes route through here so that the distinction is
// made in exactly one place.
func entryExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func newFound(root, dir string) Found {
	rel := relOf(root, dir)
	group := ""
	if i := strings.Index(rel, "/"); i > 0 {
		group = rel[:i]
	}
	return Found{AbsPath: dir, RelPath: rel, Group: group}
}

// relOf renders dir relative to root in slash form, falling back to the
// absolute path when the two share no common ancestor.
func relOf(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return dir
	}
	return filepath.ToSlash(rel)
}

// reason strips the path that fs errors repeat, leaving the cause on its own.
// The caller has already named the directory in human terms.
func reason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

func matchesAny(patterns []string, path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, p := range patterns {
		if globMatch(p, rel) {
			return true
		}
	}
	return false
}

// globMatch matches one ignore pattern against a path relative to the root,
// using the dialect every git user already knows from .gitignore:
//
//   - a pattern with no slash matches a single path segment at ANY depth, so
//     "node_modules" excludes both "node_modules" and "web/app/node_modules";
//   - a pattern containing a slash is anchored to the root, so "web/app"
//     excludes that one directory and not "other/web/app".
//
// The "**/" prefix and "/**" suffix that config files conventionally wrap such
// patterns in are stripped first: "**/node_modules/**" is the same instruction
// as "node_modules". filepath.Match supplies the wildcards within a segment.
func globMatch(pattern, name string) bool {
	pattern = strings.TrimPrefix(pattern, "**/")
	pattern = strings.TrimSuffix(pattern, "/**")
	if pattern == "" {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if ok, _ := filepath.Match(pattern, seg); ok {
			return true
		}
	}
	ok, _ := filepath.Match(pattern, name)
	return ok
}
