// Package discover finds git repositories beneath a root and narrows them by
// a user-supplied selector.
package discover

import (
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

// Walk returns every repository at or beneath root, to at most depth levels.
// Descent stops at the first repository on any path, so submodules and nested
// clones are not reported separately.
func Walk(root string, depth int, ignore []string) ([]Found, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var out []Found
	var recurse func(dir string, level int)
	recurse = func(dir string, level int) {
		if isRepo(dir) {
			out = append(out, newFound(abs, dir))
			return // stop descending: submodules are not separate repos
		}
		if level >= depth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return // unreadable directory: skip, never abort the walk
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			child := filepath.Join(dir, e.Name())
			if matchesAny(ignore, child, abs) {
				continue
			}
			// Do not follow symlinks: they invite cycles.
			if info, err := os.Lstat(child); err != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			recurse(child, level+1)
		}
	}
	recurse(abs, 0)
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
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
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		rel = dir
	}
	rel = filepath.ToSlash(rel)
	group := ""
	if i := strings.Index(rel, "/"); i > 0 {
		group = rel[:i]
	}
	return Found{AbsPath: dir, RelPath: rel, Group: group}
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

// globMatch supports the leading and trailing "**/" forms used in config, on
// top of filepath.Match's single-segment semantics.
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
