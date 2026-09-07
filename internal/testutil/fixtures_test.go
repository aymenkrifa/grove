package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tasks 3, 4, 6, 9 and 11 all build their fixtures here, so a fixture that
// quietly means something other than it says would corrupt tests across the
// whole project. These pin the two options whose meaning is not obvious.

// Dirty must produce a MODIFIED TRACKED file (" M"), not an untracked one
// ("??"). Those are different git statuses, and a collector test built on the
// wrong one would assert the wrong thing while looking correct.
func TestDirtyImpliesACommit(t *testing.T) {
	dir := NewRepo(t, filepath.Join(t.TempDir(), "repo"), Dirty())

	if got := strings.TrimSpace(Run(t, dir, "rev-list", "--count", "HEAD")); got != "1" {
		t.Fatalf("commit count = %s, want 1 — Dirty() must imply WithCommit()", got)
	}
	status := Run(t, dir, "status", "--porcelain")
	if !strings.Contains(status, " M README.md") {
		t.Errorf("status = %q, want a modified tracked README.md", status)
	}
	if strings.Contains(status, "??") {
		t.Errorf("status = %q, want no untracked files", status)
	}
}

func TestBareHasNoWorkingTree(t *testing.T) {
	dir := NewRepo(t, filepath.Join(t.TempDir(), "mirror.git"), Bare())

	if _, err := os.Lstat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Errorf("a bare repo must have no .git entry (err = %v)", err)
	}
	for _, name := range []string{"HEAD", "objects"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Errorf("a bare repo should carry %s at its top level: %v", name, err)
		}
	}
}

// Bare has no working tree, so every working-tree option is a mistake in the
// calling test. NewRepo turns this into a t.Fatalf, which cannot be asserted on
// directly, so the predicate behind it is tested instead.
func TestBareRejectsWorkingTreeOptions(t *testing.T) {
	incompatible := map[string]RepoOpt{
		"WithCommit": WithCommit(),
		"WithBranch": WithBranch("feature"),
		"Dirty":      Dirty(),
		"Staged":     Staged(),
		"Untracked":  Untracked(),
	}
	for name, opt := range incompatible {
		t.Run(name, func(t *testing.T) {
			c := repoCfg{}
			Bare()(&c)
			opt(&c)
			msg := conflictingOptions(c)
			if msg == "" {
				t.Fatalf("Bare() + %s() should be rejected", name)
			}
			if !strings.Contains(msg, "Bare()") {
				t.Errorf("message %q should name the offending option", msg)
			}
		})
	}

	alone := repoCfg{}
	Bare()(&alone)
	if msg := conflictingOptions(alone); msg != "" {
		t.Errorf("Bare() on its own is valid, got %q", msg)
	}
	if msg := conflictingOptions(repoCfg{commit: true, dirty: true}); msg != "" {
		t.Errorf("working-tree options combine freely with each other, got %q", msg)
	}
}
