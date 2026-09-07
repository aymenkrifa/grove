package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/git"
	"github.com/aymenkrifa/grove/internal/testutil"
)

// runSplit is run() with the two streams kept apart, so a test can say which
// stream something arrived on. The merged run() cannot tell a warning printed
// to stderr from one wrongly printed into the table.
func runSplit(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = ExecuteWith(&out, &errb, args)
	return out.String(), errb.String(), code
}

// isolate cuts a test off from the ambient environment: the developer's own
// config file, their NO_COLOR preference and any GROVE_ROOT they have exported
// would otherwise decide the result.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	t.Setenv("GROVE_ROOT", "")
}

// writeConfig plants a config file where config.Path will look for it.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	path := config.Path()
	if path == "" {
		t.Fatal("config.Path() is empty; isolate(t) must run first")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// richWorkspace is deliberately lopsided — two repos in one group and one in
// another, three distinct branch names, three distinct working-tree states. A
// workspace with one of everything cannot tell a swapped pair of columns, or a
// swapped pair of groups, from a correct one.
func richWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"),
		testutil.WithCommit(), testutil.Dirty())
	testutil.NewRepo(t, filepath.Join(root, "api", "auth"),
		testutil.WithCommit(), testutil.WithBranch("feature/login"),
		testutil.Staged(), testutil.Untracked())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"),
		testutil.WithCommit(), testutil.WithBranch("release/2.0"))
	isolate(t)
	return root
}

// brokenWorkspace holds one healthy repo and one that git cannot open: a .git
// file pointing at a directory that is not there. The walk still finds it —
// the .git entry exists — and git fails on it, which is exactly the per-repo
// failure the exit code has to survive.
func brokenWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	broken := filepath.Join(root, "tools", "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, ".git"),
		[]byte("gitdir: /nonexistent-grove-target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	isolate(t)
	return root
}

// lineWith returns the first output line containing want, split into fields.
// Asserting on the fields of one row catches a pair of columns being swapped,
// which a strings.Contains over the whole table never can.
func lineWith(t *testing.T, out, want string) []string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, want) {
			return strings.Fields(line)
		}
	}
	t.Fatalf("no line containing %q in:\n%s", want, out)
	return nil
}

func TestFirstArg(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"web"}, "web"},
		{[]string{"web", "api"}, "web"},
	} {
		if got := firstArg(tc.args); got != tc.want {
			t.Errorf("firstArg(%q) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	regular, err := os.CreateTemp(t.TempDir(), "grove-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()

	// /dev/null is a character device, which is the same class of file a
	// terminal is: without it this test could only ever prove the false side.
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()

	closed, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()

	for name, tc := range map[string]struct {
		w    io.Writer
		want bool
	}{
		"buffer":       {&bytes.Buffer{}, false},
		"regular file": {regular, false},
		"char device":  {devnull, true},
		"closed file":  {closed, false},
	} {
		if got := isTerminal(tc.w); got != tc.want {
			t.Errorf("isTerminal(%s) = %v, want %v", name, got, tc.want)
		}
	}
}

func TestRenderOptionsColorDecision(t *testing.T) {
	// NO_COLOR is emptied rather than left alone: set, it short-circuits every
	// case below to false and the test would pass against any implementation.
	t.Setenv("NO_COLOR", "")
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	buf := &bytes.Buffer{}

	for name, tc := range map[string]struct {
		cfgMode  string
		flagMode string
		tty      bool
		want     bool
	}{
		"auto off a pipe":              {"auto", "", false, false},
		"auto on a terminal":           {"auto", "", true, true},
		"unset config means auto":      {"", "", true, true},
		"unset config off a pipe":      {"", "", false, false},
		"config always beats the pipe": {"always", "", false, true},
		"config never beats the tty":   {"never", "", true, false},
		"flag always beats config":     {"never", "always", false, true},
		"flag never beats config":      {"always", "never", true, false},
		"flag auto beats config":       {"always", "auto", false, false},
	} {
		t.Run(name, func(t *testing.T) {
			flagColor = tc.flagMode
			t.Cleanup(func() { flagColor = "" })
			var w io.Writer = buf
			if tc.tty {
				w = devnull
			}
			res := &config.Resolved{Display: config.Display{Color: tc.cfgMode}}
			if got := renderOptions(res, w).Color; got != tc.want {
				t.Errorf("Color = %v, want %v (config %q, flag %q, tty %v)",
					got, tc.want, tc.cfgMode, tc.flagMode, tc.tty)
			}
		})
	}
}

func TestRenderOptionsCarriesDisplaySettings(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	res := &config.Resolved{Display: config.Display{
		GroupBy:      "branch-prefix",
		BranchPrefix: `[A-Z]+-\d+`,
		ShowClean:    true,
		ASCII:        false,
	}}
	o := renderOptions(res, &bytes.Buffer{})
	if o.Display.GroupBy != "branch-prefix" || o.Display.BranchPrefix != `[A-Z]+-\d+` {
		t.Errorf("Display not passed through: %+v", o.Display)
	}
	if !o.ShowClean {
		t.Error("ShowClean = false, want true from the config")
	}
	if o.ASCII {
		t.Error("ASCII = true, want false")
	}

	// show_clean = false in the config must reach Options, not be assumed.
	res.Display.ShowClean = false
	if renderOptions(res, &bytes.Buffer{}).ShowClean {
		t.Error("ShowClean = true, want false from the config")
	}

	// The flag adds to the config setting rather than replacing it: either
	// source alone is enough to switch ASCII on.
	res.Display.ASCII = true
	if !renderOptions(res, &bytes.Buffer{}).ASCII {
		t.Error("ASCII = false, want true from the config")
	}
	res.Display.ASCII = false
	flagASCII = true
	t.Cleanup(func() { flagASCII = false })
	if !renderOptions(res, &bytes.Buffer{}).ASCII {
		t.Error("ASCII = false, want true from --ascii")
	}
}

// TestExitCodeIsResetBetweenRuns is the reason ExecuteWith clears exitCode.
// exitCode is package state; without the reset the first run that meets a
// broken repository makes every later run in the same process exit 2, which
// under `go test` means every subsequent test case.
func TestExitCodeIsResetBetweenRuns(t *testing.T) {
	broken := brokenWorkspace(t)
	if _, code := run(t, "status", "--root", broken); code != ExitPartial {
		t.Fatalf("first run exit = %d, want %d", code, ExitPartial)
	}
	clean := workspace(t)
	if out, code := run(t, "status", "--root", clean); code != ExitOK {
		t.Errorf("second run exit = %d, want %d — the partial failure leaked\n%s",
			code, ExitOK, out)
	}
}

func TestStatusBrokenRepoIsPartialFailure(t *testing.T) {
	root := brokenWorkspace(t)
	out, code := run(t, "status", "--root", root)
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitPartial, out)
	}
	// The run continues: the healthy repo is still reported.
	if !strings.Contains(out, "gateway") {
		t.Errorf("a broken repo aborted the run\n%s", out)
	}
	if !strings.Contains(out, "not a git repository") {
		t.Errorf("the failure is not shown in the table\n%s", out)
	}
	if !strings.Contains(out, "1 errored") {
		t.Errorf("summary does not count the failure\n%s", out)
	}
}

func TestStatusBrokenRepoIsPartialFailureInJSON(t *testing.T) {
	root := brokenWorkspace(t)
	out, code := run(t, "status", "--root", root, "--json")
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitPartial, out)
	}
	var doc struct {
		Summary struct {
			Repos  int `json:"repos"`
			Errors int `json:"errors"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if doc.Summary.Repos != 2 || doc.Summary.Errors != 1 {
		t.Errorf("summary = %+v, want 2 repos and 1 error", doc.Summary)
	}
}

// TestStatusWithoutGitOnPath covers spec §10: a missing git is one clear error
// and exit 1, not one error per repository and exit 2.
func TestStatusWithoutGitOnPath(t *testing.T) {
	root := workspace(t) // built while git is still on PATH
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, code := runSplit(t, "status", "--root", root)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, ExitError, stdout, stderr)
	}
	if n := strings.Count(stderr, "git was not found on PATH"); n != 1 {
		t.Errorf("the missing-git message appears %d times, want exactly 1:\n%s", n, stderr)
	}
	if stdout != "" {
		t.Errorf("a failed run wrote a table to stdout:\n%s", stdout)
	}
}

// TestStatusWarnsAboutUnreadableDirectories covers the other half of spec §10:
// a subtree that cannot be read is skipped with a warning, and the rest of the
// workspace is still reported.
func TestStatusWarnsAboutUnreadableDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a 0000 directory, so there is nothing to warn about")
	}
	root := workspace(t)
	locked := filepath.Join(root, "vault")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	// Restore before TempDir's own cleanup, which cannot remove it otherwise.
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	stdout, stderr, code := runSplit(t, "status", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d — an unreadable subtree is not a failure\nstderr: %s",
			code, ExitOK, stderr)
	}
	if !strings.Contains(stderr, "vault") || !strings.Contains(stderr, "permission denied") {
		t.Errorf("no warning about the unreadable directory on stderr:\n%s", stderr)
	}
	if strings.Contains(stdout, "vault") {
		t.Errorf("the warning was written to stdout, where it corrupts the table:\n%s", stdout)
	}
	if !strings.Contains(stdout, "gateway") || !strings.Contains(stdout, "dashboard") {
		t.Errorf("the readable repos were not reported:\n%s", stdout)
	}
}

// The warning must not reach stdout even when stdout is a JSON document a
// script is parsing.
func TestStatusJSONStaysCleanWhenWarningsAreEmitted(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a 0000 directory")
	}
	root := workspace(t)
	locked := filepath.Join(root, "vault")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	stdout, stderr, code := runSplit(t, "status", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, stderr)
	}
	if !strings.Contains(stderr, "vault") {
		t.Errorf("warning missing from stderr:\n%s", stderr)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
}

func TestStatusNonexistentRootExitsOne(t *testing.T) {
	isolate(t)
	missing := filepath.Join(t.TempDir(), "not-here")
	stdout, stderr, code := runSplit(t, "status", "--root", missing)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, ExitError, stdout, stderr)
	}
	if !strings.Contains(stderr, "not-here") {
		t.Errorf("the error does not name the root:\n%s", stderr)
	}
	// A root that is not there is not a root that holds no repositories: the
	// walk's own error has to reach the user, not be replaced by the
	// empty-workspace message that follows it.
	if strings.Contains(stderr, "no git repositories") {
		t.Errorf("a missing root was misreported as an empty one:\n%s", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty:\n%s", stdout)
	}
}

func TestStatusEmptyRootExitsOne(t *testing.T) {
	isolate(t)
	empty := t.TempDir()
	_, stderr, code := runSplit(t, "status", "--root", empty)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitError, stderr)
	}
	if !strings.Contains(stderr, "no git repositories") {
		t.Errorf("unexpected error:\n%s", stderr)
	}
}

func TestStatusUnknownWorkspaceExitsOne(t *testing.T) {
	isolate(t)
	_, stderr, code := runSplit(t, "status", "-w", "nosuchworkspace")
	if code != ExitError {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitError, stderr)
	}
	if !strings.Contains(stderr, "nosuchworkspace") {
		t.Errorf("the error does not name the workspace:\n%s", stderr)
	}
}

// TestStatusNamedWorkspace checks the two values that reach render.JSON as
// separate arguments: the root and the workspace name. Swapping them is the
// kind of defect a single-field assertion cannot see.
func TestStatusNamedWorkspace(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	writeConfig(t, fmt.Sprintf("[[workspace]]\nname = \"demo\"\nroot = %q\n", root))

	stdout, stderr, code := runSplit(t, "status", "-w", "demo", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, stderr)
	}
	var doc struct {
		Root      string `json:"root"`
		Workspace string `json:"workspace"`
		Repos     []struct {
			Path string `json:"path"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, stdout)
	}
	if doc.Workspace != "demo" {
		t.Errorf("workspace = %q, want %q", doc.Workspace, "demo")
	}
	if doc.Root != root {
		t.Errorf("root = %q, want %q", doc.Root, root)
	}
	if len(doc.Repos) != 1 || doc.Repos[0].Path != "api/gateway" {
		t.Errorf("repos = %+v, want one api/gateway", doc.Repos)
	}
}

// The walk's depth and ignore list come from the resolved workspace. Neither
// is visible in the output directly, so each is checked by what it removes.
func TestStatusHonoursWorkspaceDepthAndIgnore(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())

	writeConfig(t, fmt.Sprintf("[[workspace]]\nname = \"shallow\"\nroot = %q\ndepth = 1\n", root))
	_, stderr, code := runSplit(t, "status", "-w", "shallow")
	if code != ExitError || !strings.Contains(stderr, "no git repositories") {
		t.Errorf("depth = 1 should find nothing two levels down: exit %d\n%s", code, stderr)
	}

	writeConfig(t, fmt.Sprintf(
		"[[workspace]]\nname = \"filtered\"\nroot = %q\ndepth = 3\nignore = [\"web\"]\n", root))
	stdout, stderr, code := runSplit(t, "status", "-w", "filtered")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, stderr)
	}
	if strings.Contains(stdout, "dashboard") {
		t.Errorf("ignore = [\"web\"] did not exclude web/dashboard\n%s", stdout)
	}
	if !strings.Contains(stdout, "gateway") {
		t.Errorf("ignore = [\"web\"] excluded too much\n%s", stdout)
	}
}

// TestStatusRowColumnOrder guards the column axis. Every field differs between
// the two rows, so a swapped pair of columns cannot look correct in both.
func TestStatusRowColumnOrder(t *testing.T) {
	root := richWorkspace(t)
	out, code := run(t, "status", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	for _, tc := range []struct {
		find string
		want []string
	}{
		{"auth", []string{"auth", "feature/login", "+1", "?1", "-"}},
		{"gateway", []string{"gateway", "main", "~1", "-"}},
		{"dashboard", []string{"dashboard", "release/2.0", "clean", "-"}},
	} {
		if got := lineWith(t, out, tc.find); !equalFields(got, tc.want) {
			t.Errorf("row for %s = %q, want %q", tc.find, got, tc.want)
		}
	}
	// Grouping by directory: the heading carries the group, the row does not
	// repeat it.
	if !hasLine(out, "api") || !hasLine(out, "web") {
		t.Errorf("group headings missing\n%s", out)
	}
	if strings.Contains(out, "api/gateway") {
		t.Errorf("the group is repeated in the row\n%s", out)
	}
	if !strings.Contains(out, "3 repos") || !strings.Contains(out, "2 dirty") {
		t.Errorf("summary wrong\n%s", out)
	}
}

// hasLine reports whether out contains a line that is exactly want. A group
// heading is a whole line, and the first one has no newline before it, so
// searching for "\napi\n" would miss it.
func hasLine(out, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

func equalFields(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestStatusDirtyOnlyKeepsTheSummaryWhole is the other half of -d: the filter
// hides rows, and only rows. It also pins the ruling that -d sets
// Options.ShowClean — writing Display.ShowClean instead changes nothing,
// because Table reads Options.
func TestStatusDirtyOnlyKeepsTheSummaryWhole(t *testing.T) {
	root := richWorkspace(t)
	out, code := run(t, "status", "--root", root, "-d")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "dashboard") {
		t.Errorf("-d should hide the clean repo\n%s", out)
	}
	if hasLine(out, "web") {
		t.Errorf("-d should drop a heading whose repos are all hidden\n%s", out)
	}
	if !strings.Contains(out, "gateway") || !strings.Contains(out, "auth") {
		t.Errorf("-d hid a repo that needs attention\n%s", out)
	}
	if !strings.Contains(out, "3 repos") {
		t.Errorf("the summary counts the selection, not the visible rows\n%s", out)
	}
}

// Without -d the clean repo is present: the filter is off by default rather
// than the table happening never to show it.
func TestStatusShowsCleanReposByDefault(t *testing.T) {
	root := richWorkspace(t)
	out, code := run(t, "status", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "dashboard") {
		t.Errorf("the clean repo is missing without -d\n%s", out)
	}
}

// show_clean = false in the config is the same filter as -d, arriving by the
// other route.
func TestStatusConfigShowCleanFalse(t *testing.T) {
	root := richWorkspace(t)
	writeConfig(t, "[display]\nshow_clean = false\n")
	out, code := run(t, "status", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "dashboard") {
		t.Errorf("show_clean = false did not hide the clean repo\n%s", out)
	}
}

// stashWorkspace has one repo carrying a stash and nothing else, so "$1" in
// the output can only have come from the stash count.
func stashWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := testutil.NewRepo(t, filepath.Join(root, "lib", "stashed"),
		testutil.WithCommit(), testutil.Dirty())
	testutil.Run(t, dir, "stash", "push", "-q", "-m", "wip")
	isolate(t)
	return root
}

func TestStatusCountsStashesByDefault(t *testing.T) {
	root := stashWorkspace(t)
	out, code := run(t, "status", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "$1") {
		t.Errorf("stash count missing\n%s", out)
	}
}

func TestStatusNoStashFlagSkipsTheCount(t *testing.T) {
	root := stashWorkspace(t)
	out, code := run(t, "status", "--root", root, "--no-stash")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "$1") {
		t.Errorf("--no-stash still counted the stash\n%s", out)
	}
	if !strings.Contains(out, "clean") {
		t.Errorf("the repo should read clean once the stash is not counted\n%s", out)
	}
}

// show_stash is a collector switch, not a render one: internal/render ignores
// Display.ShowStash entirely, so the config value has to be honoured here or
// nowhere.
func TestStatusConfigShowStashFalse(t *testing.T) {
	root := stashWorkspace(t)
	writeConfig(t, "[display]\nshow_stash = false\n")
	out, code := run(t, "status", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "$1") {
		t.Errorf("show_stash = false still counted the stash\n%s", out)
	}
}

func TestStatusColorFlag(t *testing.T) {
	root := richWorkspace(t)
	t.Setenv("NO_COLOR", "") // isolate() sets it; colour cannot be tested with it on

	out, code := run(t, "status", "--root", root, "--color", "always")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	// The group heading is blue. Asserting the exact escape, rather than any
	// escape anywhere, ties the colour to the thing being coloured.
	if !strings.Contains(out, "\x1b[34mapi\x1b[0m") {
		t.Errorf("--color always produced no coloured heading\n%q", out)
	}

	out, code = run(t, "status", "--root", root, "--color", "never")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("--color never emitted escapes\n%q", out)
	}

	// Writing to a buffer is not a terminal, so auto means no colour.
	out, code = run(t, "status", "--root", root, "--color", "auto")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("--color auto coloured a non-terminal\n%q", out)
	}
}

func TestStatusASCIIFlag(t *testing.T) {
	root := richWorkspace(t)
	out, code := run(t, "status", "--root", root, "--ascii")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "3 repos | 2 dirty") {
		t.Errorf("--ascii did not switch the summary separator\n%s", out)
	}
	if strings.Contains(out, "·") {
		t.Errorf("--ascii left a unicode separator behind\n%s", out)
	}

	out, code = run(t, "status", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "3 repos · 2 dirty") {
		t.Errorf("the default should use the unicode separator\n%s", out)
	}
}

// A run with no selector reports everything, which is what makes the selector
// tests mean something.
func TestStatusEmptySelectorSelectsEverything(t *testing.T) {
	root := richWorkspace(t)
	out, code := run(t, "status", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	for _, want := range []string{"gateway", "auth", "dashboard", "3 repos"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

// The selector reaches discover.Select intact, including its exact-path and
// ambiguity behaviour.
func TestStatusSelectorForms(t *testing.T) {
	root := richWorkspace(t)
	for name, tc := range map[string]struct {
		selector string
		want     []string
		absent   []string
	}{
		"group":          {"api", []string{"gateway", "auth", "2 repos"}, []string{"dashboard"}},
		"exact path":     {"api/auth", []string{"auth", "feature/login", "1 repos"}, []string{"gateway", "dashboard"}},
		"basename":       {"dashboard", []string{"dashboard", "release/2.0", "1 repos"}, []string{"gateway", "auth"}},
		"trailing slash": {"web/", []string{"dashboard", "1 repos"}, []string{"gateway", "auth"}},
	} {
		t.Run(name, func(t *testing.T) {
			out, code := run(t, "status", "--root", root, tc.selector)
			if code != ExitOK {
				t.Fatalf("exit = %d\n%s", code, out)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output missing %q\n%s", w, out)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(out, a) {
					t.Errorf("selector %q should have excluded %q\n%s", tc.selector, a, out)
				}
			}
		})
	}
}

func TestStatusAlias(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "st", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "2 repos") {
		t.Errorf("`grove st` did not run status\n%s", out)
	}
}

func TestStatusRejectsASecondArgument(t *testing.T) {
	root := workspace(t)
	_, stderr, code := runSplit(t, "status", "--root", root, "web", "api")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, stderr)
	}
}

func TestUnknownCommandExitsOne(t *testing.T) {
	isolate(t)
	_, stderr, code := runSplit(t, "nosuchcommand")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, stderr)
	}
}

func TestHelpExitsZero(t *testing.T) {
	isolate(t)
	stdout, _, code := runSplit(t, "--help")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	for _, want := range []string{"grove", "status", "--root", "--workspace", "--color", "--jobs", "--ascii"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help does not mention %q\n%s", want, stdout)
		}
	}
}

func TestGlobalFlagsAreBound(t *testing.T) {
	root := workspace(t)
	if _, code := run(t, "status", "--root", root, "--jobs", "1"); code != ExitOK {
		t.Errorf("--jobs 1 failed with exit %d", code)
	}
	if flagJobs != 1 {
		t.Errorf("flagJobs = %d, want 1", flagJobs)
	}
	// Each run re-registers the flags, so the previous run's values do not
	// carry over into the next one.
	if _, code := run(t, "status", "--root", root); code != ExitOK {
		t.Errorf("second run failed with exit %d", code)
	}
	if flagJobs != 0 {
		t.Errorf("flagJobs = %d after a run without --jobs, want 0", flagJobs)
	}
	if _, _, code := runSplit(t, "status", "--root", root, "--jobs", "notanumber"); code != ExitError {
		t.Errorf("a bad --jobs value should exit %d, got %d", ExitError, code)
	}
}

// The exit codes are a contract with shell scripts, which see the numbers and
// not the constants. Every other test compares against the constant, so only
// this one can tell that ExitPartial is still 2.
func TestExitCodeValues(t *testing.T) {
	if ExitOK != 0 || ExitError != 1 || ExitPartial != 2 {
		t.Errorf("exit codes = %d/%d/%d, want 0/1/2 per spec §5.7",
			ExitOK, ExitError, ExitPartial)
	}
}

func TestCollectOpts(t *testing.T) {
	for name, tc := range map[string]struct {
		jobs      int
		showStash bool
		noStash   bool
		want      git.CollectOpts
	}{
		"defaults":                {0, true, false, git.CollectOpts{Jobs: 0, Stash: true}},
		"--jobs reaches the pool": {4, true, false, git.CollectOpts{Jobs: 4, Stash: true}},
		"--no-stash wins":         {0, true, true, git.CollectOpts{Jobs: 0, Stash: false}},
		"config off":              {0, false, false, git.CollectOpts{Jobs: 0, Stash: false}},
		"both off":                {2, false, true, git.CollectOpts{Jobs: 2, Stash: false}},
	} {
		t.Run(name, func(t *testing.T) {
			flagJobs = tc.jobs
			t.Cleanup(func() { flagJobs = 0 })
			res := &config.Resolved{Display: config.Display{ShowStash: tc.showStash}}
			if got := collectOpts(res, tc.noStash); got != tc.want {
				t.Errorf("collectOpts = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// An unreadable root is a different answer from an empty one. Reporting "no
// git repositories under X" for a directory nobody could read would send the
// user looking for the wrong problem, so the walk's error has to survive.
func TestStatusUnreadableRootReportsTheRealCause(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a 0000 directory")
	}
	isolate(t)
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	_, stderr, code := runSplit(t, "status", "--root", root)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitError, stderr)
	}
	if !strings.Contains(stderr, "permission denied") {
		t.Errorf("the error does not give the cause:\n%s", stderr)
	}
	if strings.Contains(stderr, "no git repositories") {
		t.Errorf("an unreadable root was misreported as an empty one:\n%s", stderr)
	}
}

func TestStatusBadConfigExitsOne(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	writeConfig(t, "this is not = = toml\n")

	stdout, stderr, code := runSplit(t, "status", "--root", root)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, ExitError, stdout, stderr)
	}
	if !strings.Contains(stderr, "config.toml") {
		t.Errorf("the error does not name the file to fix:\n%s", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty:\n%s", stdout)
	}
}

// backdate rewinds path's access and modification times, so the stat data the
// index has cached for it is stale and git has something it would like to
// refresh. Each call uses a different instant: once git has written the
// refreshed stat data, a second backdate to the *same* time matches what is
// already recorded and asks git to do nothing at all — which is how a loop
// measuring this can report "1 rewrite in 20 runs" for a command that in fact
// rewrites the index every single time it is given something to refresh.
func backdate(t *testing.T, path string) {
	t.Helper()
	backdateSeq++
	when := time.Date(2001, time.March, 4, 5, 6, backdateSeq, 0, time.UTC)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

var backdateSeq int

func hashFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(body))
}

// TestReadOnlyCommandsLeaveTheIndexAlone measures the read-only guarantee
// instead of asserting it: a tracked file is backdated so git has an index
// refresh to perform, and .git/index is hashed either side of the command. A
// report grove only reads must leave the repository byte for byte as it found
// it.
//
// Each case first proves its own fixture is live by running a plain `git
// status` — no --no-optional-locks — and requiring that it *does* rewrite the
// index. Without that control the test passes on a grove that never passed the
// flag at all, because a repository whose stat data is already fresh gives git
// nothing to refresh and every command looks read-only.
//
// diff is deliberately absent, and it is the command that most needs saying
// out loud: on git 2.43.0 builtin/diff.c refreshes the index without
// consulting use_optional_locks(), so `grove diff` still rewrites it — 20 runs
// out of 20, with the flag and without it — and no flag grove can pass changes
// that. See noOptionalLocks in internal/git/run.go for the measurements and
// for what fixing it would cost; TestRunAlwaysPassesNoOptionalLocks pins the
// part grove does control, which is the argv it hands git.
func TestReadOnlyCommandsLeaveTheIndexAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"status", []string{"status"}},
		{"branch", []string{"branch"}},
		{"log", []string{"log", "-n", "10"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			repo := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
			isolate(t)
			readme := filepath.Join(repo, "README.md")
			index := filepath.Join(repo, ".git", "index")

			backdate(t, readme)
			control := hashFile(t, index)
			testutil.Run(t, repo, "status", "--porcelain")
			if hashFile(t, index) == control {
				t.Fatalf("the fixture proves nothing: plain git status left %s untouched, "+
					"so this repository has no index refresh for the flag to prevent", index)
			}

			backdate(t, readme)
			before := hashFile(t, index)
			if out, code := run(t, append(tc.args, "--root", root)...); code != ExitOK {
				t.Fatalf("exit = %d\n%s", code, out)
			}
			if hashFile(t, index) != before {
				t.Errorf("grove %s rewrote %s — a command that only reads must leave the "+
					"repositories it reads exactly as it found them", tc.name, index)
			}
		})
	}
}
