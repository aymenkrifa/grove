package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecRunsInEveryRepo(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "exec", "--root", root, "--", "pwd")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	for _, want := range []string{"api/gateway", "web/dashboard"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

// TestExecDryRunRunsNothing checks the filesystem, not just the printed
// text: a dry run that prints the right thing but calls exec.Command anyway
// would still pass a text-only assertion.
func TestExecDryRunRunsNothing(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "exec", "--root", root, "--dry-run", "--", "touch", "SHOULD_NOT_EXIST")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "touch SHOULD_NOT_EXIST") {
		t.Errorf("dry run should print the command\n%s", out)
	}
	if !strings.Contains(out, "api/gateway") || !strings.Contains(out, "web/dashboard") {
		t.Errorf("dry run should list the repos it would run in\n%s", out)
	}
	for _, repo := range []string{"api/gateway", "web/dashboard"} {
		if _, err := os.Stat(filepath.Join(root, repo, "SHOULD_NOT_EXIST")); !os.IsNotExist(err) {
			t.Errorf("--dry-run must run nothing, but %s/SHOULD_NOT_EXIST exists (err=%v)", repo, err)
		}
	}
}

func TestExecFailureIsPartialAndStopsWithoutKeepGoing(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "exec", "--root", root, "--", "false")
	if code != ExitPartial {
		t.Errorf("exit = %d, want %d\n%s", code, ExitPartial, out)
	}
}

// execOrderWorkspace is api/gateway (sorts first) and web/dashboard (sorts
// second); the command below fails in gateway and would leave a marker file
// in dashboard if it got to run there.
func execOrderScript(marker string) []string {
	// A literal shell script the *test* is choosing to hand to exec, exactly
	// as a user would type `grove exec -- sh -c '...'`. It contains no grove
	// data — only the fixed marker filename below — so this does not build a
	// shell string out of anything grove collected.
	return []string{"sh", "-c",
		`case "$(pwd)" in *gateway) exit 9;; *) touch ` + marker + `;; esac`}
}

// TestExecStopsBeforeTheSecondRepoWithoutKeepGoing pins that a failure
// actually halts the fan-out, not merely that the exit code ends up
// ExitPartial: a mutation that keeps looping regardless of --keep-going would
// still leave the marker file behind.
func TestExecStopsBeforeTheSecondRepoWithoutKeepGoing(t *testing.T) {
	root := workspace(t)
	argv := append([]string{"exec", "--root", root, "--"}, execOrderScript("ran.marker")...)
	out, code := run(t, argv...)
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitPartial, out)
	}
	if _, err := os.Stat(filepath.Join(root, "web", "dashboard", "ran.marker")); !os.IsNotExist(err) {
		t.Errorf("without --keep-going, exec must stop before web/dashboard runs (err=%v)", err)
	}
}

// TestExecKeepGoingRunsTheRemainingRepos is the other half: with
// --keep-going, the repository after the failing one must still run.
func TestExecKeepGoingRunsTheRemainingRepos(t *testing.T) {
	root := workspace(t)
	argv := append([]string{"exec", "--root", root, "--keep-going", "--"}, execOrderScript("ran.marker")...)
	out, code := run(t, argv...)
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitPartial, out)
	}
	if _, err := os.Stat(filepath.Join(root, "web", "dashboard", "ran.marker")); err != nil {
		t.Errorf("--keep-going should have let web/dashboard run too (err=%v)", err)
	}
}

func TestExecRequiresACommand(t *testing.T) {
	root := workspace(t)
	_, code := run(t, "exec", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d when no command is given", code, ExitError)
	}
}

// TestExecRequiresACommandAfterADanglingDash covers the boundary distinct
// from "no -- at all": the user typed --, but nothing follows it.
func TestExecRequiresACommandAfterADanglingDash(t *testing.T) {
	root := workspace(t)
	_, code := run(t, "exec", "--root", root, "--")
	if code != ExitError {
		t.Errorf("exit = %d, want %d for `exec --` with no command after it", code, ExitError)
	}
}

// TestExecSelectorNarrowsRepos confirms the selector before -- is forwarded,
// not discarded or replaced with everything.
func TestExecSelectorNarrowsRepos(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "exec", "--root", root, "web", "--", "pwd")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "gateway") {
		t.Errorf("selector 'web' should exclude api/gateway\n%s", out)
	}
	if !strings.Contains(out, "dashboard") {
		t.Errorf("selector 'web' should include web/dashboard\n%s", out)
	}
}

// TestExecRejectsMultipleSelectors matches status/list/branch/fetch/log,
// which all reject a second positional argument before -- rather than
// silently using only the first.
func TestExecRejectsMultipleSelectors(t *testing.T) {
	root := workspace(t)
	_, code := run(t, "exec", "--root", root, "web", "api", "--", "pwd")
	if code != ExitError {
		t.Errorf("exit = %d, want %d for two selector arguments", code, ExitError)
	}
}

// TestExecPrintsARepoHeaderBeforeRunningTheCommand deliberately runs a
// command ("echo ran") whose own output never contains the repository path,
// so the header line is the *only* possible source of "api/gateway" and
// "web/dashboard" in the output. The other exec tests all run "pwd", whose
// own stdout already contains the path — deleting the header line at
// cmd/exec.go would not fail any of them.
func TestExecPrintsARepoHeaderBeforeRunningTheCommand(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "exec", "--root", root, "--", "echo", "ran")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	for _, want := range []string{"api/gateway", "web/dashboard"} {
		if !hasLine(out, want) {
			t.Errorf("missing a header line naming %q\n%s", want, out)
		}
	}
	if got := strings.Count(out, "ran"); got != 2 {
		t.Errorf("expected the command's own output (\"ran\") twice, got %d\n%s", got, out)
	}
}

// TestExecErrorsGoToStderr pins the stream-placement rule: a per-repo
// failure message must land on stderr, never mixed into the command's own
// stdout, so `grove exec -- <cmd> 2>/dev/null` shows only real output.
func TestExecErrorsGoToStderr(t *testing.T) {
	root := workspace(t)
	_, stderr, code := runSplit(t, "exec", "--root", root, "--keep-going", "--", "false")
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\nstderr: %s", code, ExitPartial, stderr)
	}
	if !strings.Contains(stderr, "api/gateway") && !strings.Contains(stderr, "web/dashboard") {
		t.Errorf("expected at least one per-repo error on stderr\n%s", stderr)
	}
}

// TestExecChildsOwnStderrGoesToStderr closes a real gap found by mutating
// past the tests above: TestExecErrorsGoToStderr only exercises grove's OWN
// diagnostic line (written via cmd.ErrOrStderr() directly), not the
// sub.Stderr wiring for the child command's own stderr output. Routing
// sub.Stderr to stdout instead survived every existing test, because "false"
// writes nothing of its own to either stream. A command that writes a
// distinct, recognisable line to its own stderr makes the wiring observable.
func TestExecChildsOwnStderrGoesToStderr(t *testing.T) {
	root := workspace(t)
	stdout, stderr, code := runSplit(t, "exec", "--root", root, "--", "sh", "-c", "echo to-stdout; echo to-stderr 1>&2")
	if code != ExitOK {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "to-stdout") {
		t.Errorf("the child's stdout should reach grove's stdout\n%s", stdout)
	}
	if strings.Contains(stdout, "to-stderr") {
		t.Errorf("the child's OWN stderr must not leak into grove's stdout\n%s", stdout)
	}
	if !strings.Contains(stderr, "to-stderr") {
		t.Errorf("the child's own stderr should reach grove's stderr\n%s", stderr)
	}
}
