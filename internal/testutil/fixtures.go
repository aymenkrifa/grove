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
func Dirty() RepoOpt              { return func(c *repoCfg) { c.dirty = true } }
func Staged() RepoOpt             { return func(c *repoCfg) { c.staged = true } }
func Untracked() RepoOpt          { return func(c *repoCfg) { c.untracked = true } }
func Bare() RepoOpt               { return func(c *repoCfg) { c.bare = true } }

// NewRepo creates a git repository at dir and returns dir.
func NewRepo(t *testing.T, dir string, opts ...RepoOpt) string {
	t.Helper()
	c := repoCfg{}
	for _, o := range opts {
		o(&c)
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
