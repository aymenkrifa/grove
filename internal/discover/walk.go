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
// never costs the caller the rest of the tree. Warnings are returned rather
// than printed because only Walk knows a directory was skipped, and only the
// caller knows where its diagnostics go.
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
		if isRepo(dir) {
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
// its contents instead.
//
// The second test also matches the .git directory of an ordinary repository,
// which is correct in isolation but must not produce a duplicate entry during
// the walk. It cannot: dot-prefixed directories are never descended into, and
// descent stops at the working tree above it either way.
func isRepo(dir string) bool {
	if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	return isGitDir(dir)
}

// isGitDir reports whether dir looks like a git directory itself: HEAD beside
// an objects directory. This is the shape `git init --bare` produces.
func isGitDir(dir string) bool {
	if _, err := os.Lstat(filepath.Join(dir, "HEAD")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "objects"))
	return err == nil && info.IsDir()
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
