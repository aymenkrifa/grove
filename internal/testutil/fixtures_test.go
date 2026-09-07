package testutil

import (
	"os"
	"os/exec"
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

// The predicate above is only useful if NewRepo actually consults it, and that
// wiring cannot be asserted in-process: the failure it produces is a t.Fatalf,
// which would end this test rather than be observed by it. So the check runs in
// a child copy of this test binary, where failing is the expected outcome.
// Deleting the call site inside NewRepo makes the child succeed, and this test
// fail — which is the whole point of it.
func TestNewRepoRejectsBareWithWorkingTreeOptions(t *testing.T) {
	const marker = "GROVE_TESTUTIL_EXPECT_FATAL"

	if os.Getenv(marker) == "1" {
		// Child process: this call is required to fail the test.
		NewRepo(t, filepath.Join(t.TempDir(), "repo"), Bare(), WithCommit())
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestNewRepoRejectsBareWithWorkingTreeOptions")
	cmd.Env = append(os.Environ(), marker+"=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("NewRepo(Bare(), WithCommit()) should have failed the test, but the "+
			"child passed — is the conflictingOptions call still wired into NewRepo?\n%s", out)
	}
	if !strings.Contains(string(out), "Bare() cannot be combined") {
		t.Errorf("the failure should explain the conflict, got:\n%s", out)
	}
}
