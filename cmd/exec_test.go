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
