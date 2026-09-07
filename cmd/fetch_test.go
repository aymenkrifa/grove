package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// TestFetchOnBrokenRemoteIsPartialFailure replaces the brief's premise, which
// does not hold under git 2.43: `git fetch --quiet` with NO remote configured
// at all exits 0 and does nothing (verified directly: a fresh `git init` repo
// with zero remotes fetches cleanly). What actually fails is a remote that is
// configured but unreachable, so that is the fixture used here and below.
func TestFetchOnBrokenRemoteIsPartialFailure(t *testing.T) {
	root := t.TempDir()
	dir := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	testutil.Run(t, dir, "remote", "add", "origin", "/nonexistent-grove-fetch-target")
	isolate(t)

	out, code := run(t, "fetch", "--root", root)
	if code != ExitPartial {
		t.Errorf("exit = %d, want %d — the remote cannot be reached\n%s", code, ExitPartial, out)
	}
}

// addRemote points dir's "origin" at a bare repository outside the workspace
// root, so `git fetch` in dir succeeds without any network access.
func addRemote(t *testing.T, dir string) {
	t.Helper()
	remote := t.TempDir()
	testutil.NewRepo(t, remote, testutil.Bare())
	testutil.Run(t, dir, "remote", "add", "origin", remote)
}

// addBrokenRemote points dir's "origin" at a path that is not a repository,
// so `git fetch` fails immediately with no network access required.
func addBrokenRemote(t *testing.T, dir string) {
	t.Helper()
	testutil.Run(t, dir, "remote", "add", "origin", "/nonexistent-grove-fetch-target")
}

// TestFetchMixedResultsCountsAndReportsEachRepo pins the per-repo error
// accumulation: one repo can succeed while another fails in the same run,
// the summary line must count them correctly, and only the failing repo's
// path may appear beside an error.
func TestFetchMixedResultsCountsAndReportsEachRepo(t *testing.T) {
	root := t.TempDir()
	ok := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	broken := testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())
	addRemote(t, ok)
	addBrokenRemote(t, broken)
	isolate(t)

	out, code := run(t, "fetch", "--root", root)
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitPartial, out)
	}
	if !strings.Contains(out, "fetched 1 of 2") {
		t.Errorf("output = %q, want a summary counting exactly one success", out)
	}
	if strings.Contains(out, "api/gateway:") {
		t.Errorf("the successful repo should not carry an error line\n%s", out)
	}
	if !strings.Contains(out, "web/dashboard:") {
		t.Errorf("the failing repo should be named beside its error\n%s", out)
	}
}

// TestFetchAllSucceedIsExitOK is the other half of the exit-code path: when
// nothing fails, markPartialFailure must never fire.
func TestFetchAllSucceedIsExitOK(t *testing.T) {
	root := t.TempDir()
	a := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	b := testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())
	addRemote(t, a)
	addRemote(t, b)
	isolate(t)

	out, code := run(t, "fetch", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, out)
	}
	if !strings.Contains(out, "fetched 2 of 2") {
		t.Errorf("output = %q, want both repos counted as fetched", out)
	}
}

// TestFetchPruneRemovesStaleRemoteTrackingRefs confirms --prune actually
// reaches git: without it a deleted remote branch's tracking ref lingers,
// with it the ref is gone after the next fetch.
func TestFetchPruneRemovesStaleRemoteTrackingRefs(t *testing.T) {
	root := t.TempDir()
	remote := t.TempDir()
	testutil.NewRepo(t, remote, testutil.Bare())
	local := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	testutil.Run(t, local, "remote", "add", "origin", remote)
	testutil.Run(t, local, "push", "-q", "origin", "main:doomed")
	testutil.Run(t, local, "fetch", "-q", "origin")
	if !strings.Contains(testutil.Run(t, local, "branch", "-r"), "origin/doomed") {
		t.Fatalf("setup failed: origin/doomed should exist before the branch is deleted")
	}
	testutil.Run(t, remote, "update-ref", "-d", "refs/heads/doomed")
	isolate(t)

	out, code := run(t, "fetch", "--root", root, "--prune")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(testutil.Run(t, local, "branch", "-r"), "origin/doomed") {
		t.Errorf("--prune should have removed the stale origin/doomed tracking ref")
	}
}
