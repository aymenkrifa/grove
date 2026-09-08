package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// diffWorkspace has a dirty repo, a clean repo, and a bare repo (no working
// tree, so `git diff` fails in it). The three exercise the omit-when-clean
// path, the stat path, and the per-repo error path all in one fixture.
func diffWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit(), testutil.Dirty())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "tools", "mirror"), testutil.Bare())
	isolate(t)
	return root
}

func TestDiffShowsPerRepoStat(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "api/gateway") {
		t.Errorf("the dirty repo should appear\n%s", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("the changed file should appear in the stat\n%s", out)
	}
	if strings.Contains(out, "web/dashboard") {
		t.Errorf("a repo with no changes should be omitted\n%s", out)
	}
}

func TestDiffSingleRepoShowsFullDiff(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root, "gateway")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "-hello") || !strings.Contains(out, "+changed") {
		t.Errorf("expected a real unified diff\n%s", out)
	}
}

// TestDiffNoSelectorInASingleRepoWorkspaceShowsStat pins that the mode
// decision is "was a selector given", not merely "did the selection resolve
// to one repository": a workspace that happens to contain only one
// repository must still get the --stat form when the user gave no selector
// at all (spec §5.2), and the full diff only when they explicitly named that
// repo.
func TestDiffNoSelectorInASingleRepoWorkspaceShowsStat(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit(), testutil.Dirty())
	isolate(t)

	out, code := run(t, "diff", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "-hello") || strings.Contains(out, "+changed") {
		t.Errorf("no selector, even in a single-repo workspace, should give the --stat form, not the full diff\n%s", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("the stat form should still name the changed file\n%s", out)
	}

	out, code = run(t, "diff", "--root", root, "gateway")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "-hello") || !strings.Contains(out, "+changed") {
		t.Errorf("naming the repo explicitly should give the full diff\n%s", out)
	}
}

// TestDiffRejectsMultipleSelectors matches status/list/branch/fetch/log,
// which all reject a second positional argument rather than silently using
// only the first and dropping the rest.
func TestDiffRejectsMultipleSelectors(t *testing.T) {
	root := workspace(t)
	_, code := run(t, "diff", "--root", root, "web", "api")
	if code != ExitError {
		t.Errorf("exit = %d, want %d for two selector arguments", code, ExitError)
	}
}

// TestDiffStatFlagForcesStatEvenForASingleRepo pins the branch condition
// `len(found) == 1 && !statOnly`: dropping the statOnly check would make
// --stat a no-op for a single-repo selection.
func TestDiffStatFlagForcesStatEvenForASingleRepo(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root, "gateway", "--stat")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "-hello") || strings.Contains(out, "+changed") {
		t.Errorf("--stat should suppress the unified diff body\n%s", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("--stat should still name the changed file\n%s", out)
	}
}

// TestDiffPassesThroughArgsAfterDash confirms arguments after -- reach git
// diff verbatim: --name-only produces a bare filename with no +/- hunk, which
// only happens if the passthrough args actually arrived at git.
func TestDiffPassesThroughArgsAfterDash(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root, "gateway", "--", "--name-only")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("expected the changed filename\n%s", out)
	}
	if strings.Contains(out, "-hello") || strings.Contains(out, "+changed") {
		t.Errorf("--name-only should suppress the hunk body; passthrough args were not forwarded\n%s", out)
	}
}

// TestDiffAccumulatesPerRepoErrorsAndExitsPartial covers the multi-repo path:
// a bare repository has no working tree, so `git diff --stat` fails inside
// it. That failure must not stop the run — the healthy dirty repo's stat
// still has to appear — and it must downgrade the exit code.
func TestDiffAccumulatesPerRepoErrorsAndExitsPartial(t *testing.T) {
	root := diffWorkspace(t)
	stdout, stderr, code := runSplit(t, "diff", "--root", root)
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, ExitPartial, stdout, stderr)
	}
	if !strings.Contains(stdout, "api/gateway") || !strings.Contains(stdout, "README.md") {
		t.Errorf("the healthy dirty repo should still be reported\n%s", stdout)
	}
	if strings.Contains(stdout, "web/dashboard") {
		t.Errorf("the clean repo should still be omitted\n%s", stdout)
	}
	// Diagnostics go to stderr, never into the (potentially piped) stdout
	// body: `grove diff 2>/dev/null` must still show a clean stat summary.
	if !strings.Contains(stderr, "tools/mirror") {
		t.Errorf("the failing bare repo should be named on stderr\n%s", stderr)
	}
	if strings.Contains(stdout, "tools/mirror") {
		t.Errorf("the failing repo's error should not leak into stdout\n%s", stdout)
	}
}

// TestDiffMultiRepoPassthroughAppliesThePathspec closes a real coverage gap
// found by mutating past the tests already written: the multi-repo --stat
// loop already appends passthrough args in the implementation, but no
// existing test exercises passthrough together with more than one repo —
// TestDiffPassesThroughArgsAfterDash only covers the single-repo full-diff
// branch. Deleting `passthrough...` from the --stat loop's gitArgs survived
// the whole suite before this test existed. An exclude pathspec makes the
// difference observable: with it correctly forwarded, the only changed file
// is excluded, the stat is empty, and the dirty repo is omitted entirely
// (same as a clean repo); if the pathspec is dropped, the repo still appears.
func TestDiffMultiRepoPassthroughAppliesThePathspec(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root, "--", "--", ":!README.md")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "api/gateway") {
		t.Errorf("excluding README.md should leave api/gateway with an empty stat, so it should be omitted\n%s", out)
	}
}

func TestDiffUnknownSelectorExitsError(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root, "nosuchrepo")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, out)
	}
}

// The three tests below call page() directly, because it is the one place
// grove ever builds a shell command line and, before this, had zero
// coverage: neither `run()` nor `runSplit()` ever gives page() a writer that
// isTerminal reports true for, since a bytes.Buffer is not an *os.File.
// isTerminal only checks for a character device, not an actual pty, and
// /dev/null satisfies that check — verified directly below — which is what
// makes the pager branch reachable at all without a real terminal.

func devNull(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { f.Close() })
	if !isTerminal(f) {
		t.Fatalf("%s should satisfy isTerminal's character-device check", os.DevNull)
	}
	return f
}

func TestPageUsesThePagerWhenOutIsATerminal(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "pager-output.txt")
	t.Setenv("GIT_PAGER", "")
	t.Setenv("PAGER", "cat > "+dest)

	if err := page(devNull(t), "hello from the pager\n"); err != nil {
		t.Fatalf("page() error = %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("the pager should have written %s: %v", dest, err)
	}
	if string(got) != "hello from the pager\n" {
		t.Errorf("pager received %q, want the body", got)
	}
}

func TestPageFallsBackToPlainOutputWhenThePagerFails(t *testing.T) {
	t.Setenv("GIT_PAGER", "")
	t.Setenv("PAGER", "no-such-pager-binary-grove-test")

	if err := page(devNull(t), "body\n"); err != nil {
		t.Errorf("page() should fall back to a plain write when the pager fails to run, got error %v", err)
	}
}

func TestPageDoesNotInvokeThePagerWhenOutIsNotATerminal(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "should-not-exist.txt")
	t.Setenv("GIT_PAGER", "")
	t.Setenv("PAGER", "cat > "+dest)

	var buf bytes.Buffer
	if err := page(&buf, "body\n"); err != nil {
		t.Fatalf("page() error = %v", err)
	}
	if buf.String() != "body\n" {
		t.Errorf("expected the body written directly to a non-terminal writer, got %q", buf.String())
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("the pager should not run when out is not a terminal (err=%v)", err)
	}
}

// git disables colour whenever its stdout is not a terminal, and grove's is
// always a buffer — so a diff arrives plain unless grove asks for colour
// explicitly. These pin that grove asks, and that it asks on its own terms.
func TestDiffPassesAColourModeToGit(t *testing.T) {
	root := workspace(t)
	// workspace() pins NO_COLOR for the table tests; this one is about the
	// colour decision itself, so it has to start from no preference.
	t.Setenv("NO_COLOR", "")

	// --color=always makes grove's decision "yes" regardless of the buffer it
	// is writing into, which is what a user forcing colour expects.
	out, code := run(t, "diff", "--root", root, "--color=always", "gateway")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("--color=always produced no escape sequences:\n%q", out)
	}

	// The default path writes to a buffer, so the decision is "no" and the
	// output must stay a clean, applyable patch.
	plain, code := run(t, "diff", "--root", root, "gateway")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, plain)
	}
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("redirected diff must stay plain, got escapes:\n%q", plain)
	}
}

// grove's colour flag goes before the passthrough args precisely so the user's
// own choice after -- still wins: git honours the last one it is given.
func TestDiffUserColourOverridesGroves(t *testing.T) {
	root := workspace(t)
	t.Setenv("NO_COLOR", "")
	out, code := run(t, "diff", "--root", root, "--color=always", "gateway", "--", "--no-color")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("`-- --no-color` must override grove's --color=always, got escapes:\n%q", out)
	}
}

// NO_COLOR is the standard escape hatch and outranks everything, including an
// explicit --color=always. The status table already honours it; the diff must
// not be the one command that ignores it.
func TestDiffHonoursNoColor(t *testing.T) {
	root := workspace(t)
	t.Setenv("NO_COLOR", "1")
	out, code := run(t, "diff", "--root", root, "--color=always", "gateway")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("NO_COLOR must beat --color=always, got escapes:\n%q", out)
	}
}
