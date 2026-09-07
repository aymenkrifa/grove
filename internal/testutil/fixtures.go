// Package testutil builds real git repositories in temporary directories.
// Tests assert against genuine git output rather than a mock, because the
// porcelain format is the thing most likely to surprise us.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Run executes a git command inside dir and fails the test if it errors.
func Run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

type repoCfg struct {
	commit    bool
	branch    string
	dirty     bool
	staged    bool
	untracked bool
	bare      bool
}

type RepoOpt func(*repoCfg)

func WithCommit() RepoOpt         { return func(c *repoCfg) { c.commit = true } }
func WithBranch(b string) RepoOpt { return func(c *repoCfg) { c.branch = b } }

// Dirty leaves README.md modified but unstaged. It implies WithCommit,
// because an unstaged modification requires an already-tracked file.
func Dirty() RepoOpt { return func(c *repoCfg) { c.dirty = true } }

func Staged() RepoOpt    { return func(c *repoCfg) { c.staged = true } }
func Untracked() RepoOpt { return func(c *repoCfg) { c.untracked = true } }

// Bare builds a repository with no working tree. It cannot be combined with
// any other option; NewRepo fails the test if it is.
func Bare() RepoOpt { return func(c *repoCfg) { c.bare = true } }

// NewRepo creates a git repository at dir and returns dir.
//
// Two constraints the option set cannot express on its own, and which quietly
// build the wrong fixture if left implicit:
//
//   - Dirty() means a tracked file carrying unstaged modifications, and nothing
//     is tracked until something is committed. Dirty() therefore implies
//     WithCommit(). Without that it would leave an *untracked* README behind
//     instead, which is a different git status altogether — and every test
//     built on it would be asserting the wrong thing.
//   - Bare() produces a repository with no working tree, so no other option can
//     apply to it. Combining them is a mistake in the calling test rather than
//     something to resolve silently, so it fails the test loudly.
func NewRepo(t *testing.T, dir string, opts ...RepoOpt) string {
	t.Helper()
	c := repoCfg{}
	for _, o := range opts {
		o(&c)
	}
	if msg := conflictingOptions(c); msg != "" {
		t.Fatalf("testutil.NewRepo(%s): %s", dir, msg)
	}
	if c.dirty {
		c.commit = true // there must be a tracked file before it can be modified
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if c.bare {
		Run(t, dir, "init", "--bare", "-q")
		return dir
	}
	Run(t, dir, "init", "-q", "-b", "main")
	if c.commit {
		write(t, filepath.Join(dir, "README.md"), "hello\n")
		Run(t, dir, "add", "README.md")
		Run(t, dir, "commit", "-qm", "initial")
	}
	if c.branch != "" {
		Run(t, dir, "checkout", "-qb", c.branch)
	}
	if c.dirty {
		write(t, filepath.Join(dir, "README.md"), "changed\n")
	}
	if c.staged {
		write(t, filepath.Join(dir, "staged.txt"), "staged\n")
		Run(t, dir, "add", "staged.txt")
	}
	if c.untracked {
		write(t, filepath.Join(dir, "untracked.txt"), "untracked\n")
	}
	return dir
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// conflictingOptions reports why an option set cannot be built, or "" when it
// is coherent. It is separate from NewRepo so that it can be tested directly:
// the alternative is asserting on a t.Fatalf, which ends the test that calls it.
func conflictingOptions(c repoCfg) string {
	if c.bare && (c.commit || c.branch != "" || c.dirty || c.staged || c.untracked) {
		return "Bare() cannot be combined with working-tree options (WithCommit, " +
			"WithBranch, Dirty, Staged, Untracked) — a bare repository has no " +
			"working tree for them to act on"
	}
	return ""
}
