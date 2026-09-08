package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/git"
	"github.com/aymenkrifa/grove/internal/testutil"
)

// TestFetchJobs closes a real gap found by mutating past the existing tests:
// hardcoding fetch's concurrency to 1 (ignoring --jobs entirely) survived the
// whole suite, because no black-box test can observe concurrency level from
// output alone. fetchJobs is checked directly instead.
func TestFetchJobs(t *testing.T) {
	if got, want := fetchJobs(4), 4; got != want {
		t.Errorf("fetchJobs(4) = %d, want %d", got, want)
	}
	if got, want := fetchJobs(0), git.DefaultJobs(); got != want {
		t.Errorf("fetchJobs(0) = %d, want DefaultJobs() = %d", got, want)
	}
	if got, want := fetchJobs(-1), git.DefaultJobs(); got != want {
		t.Errorf("fetchJobs(-1) = %d, want DefaultJobs() = %d", got, want)
	}
}

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

	stdout, stderr, code := runSplit(t, "fetch", "--root", root)
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, ExitPartial, stdout, stderr)
	}
	if !strings.Contains(stdout, "fetched 1 of 2") {
		t.Errorf("stdout = %q, want a summary counting exactly one success", stdout)
	}
	// Stream placement: the result summary belongs on stdout, per-repo
	// diagnostics on stderr, so `grove fetch 2>/dev/null` shows only the
	// summary and `grove fetch >/dev/null` shows only the errors.
	if strings.Contains(stdout, "web/dashboard:") {
		t.Errorf("the failing repo's error should not appear on stdout\n%s", stdout)
	}
	if strings.Contains(stderr, "api/gateway:") {
		t.Errorf("the successful repo should not carry an error line\n%s", stderr)
	}
	if !strings.Contains(stderr, "web/dashboard:") {
		t.Errorf("the failing repo should be named on stderr beside its error\n%s", stderr)
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

// Progress must never reach a non-terminal writer: a redirected fetch or a CI
// log should carry the summary line and nothing else, and a carriage return
// into a file is noise no one wants.
func TestProgressIsSilentWhenNotATerminal(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(&buf, 26)
	p.update(1)
	p.update(7)
	p.clear()
	if buf.Len() != 0 {
		t.Errorf("progress wrote %q to a non-terminal writer, want nothing", buf.String())
	}
}

// The counter's shape is the requirement — "3/26", rewritten in place rather
// than appended as new lines.
func TestProgressCounterFormat(t *testing.T) {
	p := &progress{w: nil, total: 26, on: false}
	var buf bytes.Buffer
	p.w, p.on = &buf, true

	p.update(1)
	p.update(3)
	got := buf.String()

	for _, want := range []string{"1/26", "3/26"} {
		if !strings.Contains(got, want) {
			t.Errorf("progress output %q is missing %q", got, want)
		}
	}
	if strings.Contains(got, "\n") {
		t.Errorf("progress must rewrite one line, but it emitted a newline: %q", got)
	}
	if n := strings.Count(got, "\r"); n != 2 {
		t.Errorf("got %d carriage returns, want one per update: %q", n, got)
	}
}

// clear blanks the widest line drawn, not the last one: going from "10/26" to
// a shorter summary would otherwise leave the tail of the longer line behind.
func TestProgressClearWipesTheWidestLine(t *testing.T) {
	var buf bytes.Buffer
	p := &progress{w: &buf, total: 100, on: true}
	p.update(100) // the widest line this run will draw
	buf.Reset()
	p.clear()

	got := buf.String()
	wantWidth := len("fetching… 100/100")
	if spaces := strings.Count(got, " "); spaces < wantWidth {
		t.Errorf("clear wrote %d spaces, want at least %d to cover %q", spaces, wantWidth, "fetching… 100/100")
	}
}
