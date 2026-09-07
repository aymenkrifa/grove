package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestShellInitEmitsAFunction(t *testing.T) {
	out, code := run(t, "shell-init", "zsh")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "git() {") {
		t.Errorf("zsh hook should define a git function\n%s", out)
	}
}

// TestShellInitEmitsEachShellsOwnScript pins that shell-init actually plumbs
// through to the right embedded file per shell name, not just "some script"
// for all three: bash and fish have textually distinct definitions of the
// git function ("git() {" vs "function git"), and only fish uses "argv".
func TestShellInitEmitsEachShellsOwnScript(t *testing.T) {
	for name, want := range map[string]string{
		"zsh":  "emulate -L zsh",
		"bash": `git() {`,
		"fish": "function git",
	} {
		t.Run(name, func(t *testing.T) {
			out, code := run(t, "shell-init", name)
			if code != ExitOK {
				t.Fatalf("exit = %d\n%s", code, out)
			}
			if !strings.Contains(out, want) {
				t.Errorf("shell-init %s missing %q\n%s", name, want, out)
			}
		})
	}
}

func TestShellInitRejectsUnknownShell(t *testing.T) {
	_, code := run(t, "shell-init", "tcsh")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
}

// TestExecuteAsGitSubcommandSetsUse pins the one thing ExecuteAsGitSubcommand
// exists for: cobra's usage text must read "git grove", not the plain "grove"
// that newRootCmd sets by default. Without the display-name override, --help
// would still pass every other test in this package — this is the only test
// that reads the command's displayed name at all.
func TestExecuteAsGitSubcommandSetsUse(t *testing.T) {
	isolate(t)
	var buf bytes.Buffer
	code := executeAsGitSubcommandWith(&buf, &buf, []string{"--help"})
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "git grove") {
		t.Errorf("help text does not read as a git subcommand:\n%s", buf.String())
	}
}

// TestExecuteAsGitSubcommandNamesTheSubcommandToo goes one step further than
// TestExecuteAsGitSubcommandSetsUse: cobra's Name() truncates Use at its
// first space, so a naive `root.Use = "git grove"` renames only the root to
// "git" and silently drops "grove" from every subcommand's own usage line —
// "grove status --help" would then read "git status --help", the name of an
// unrelated, real git command. Checking a subcommand's help is the only way
// to catch that regression; the root-only check above cannot see it.
func TestExecuteAsGitSubcommandNamesTheSubcommandToo(t *testing.T) {
	isolate(t)
	var buf bytes.Buffer
	code := executeAsGitSubcommandWith(&buf, &buf, []string{"status", "--help"})
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "git grove status") {
		t.Errorf("subcommand help does not read as a git subcommand:\n%s", buf.String())
	}
}

// TestExecuteAsGitSubcommandResetsState guards the same class of bug
// TestExitCodeIsResetBetweenRuns guards for ExecuteWith: exitCode is package
// state, and a prior partial failure elsewhere in the process must not leak
// into a later call through this entry point either.
func TestExecuteAsGitSubcommandResetsState(t *testing.T) {
	broken := brokenWorkspace(t)
	var buf bytes.Buffer
	if code := executeAsGitSubcommandWith(&buf, &buf, []string{"status", "--root", broken}); code != ExitPartial {
		t.Fatalf("first run exit = %d, want %d", code, ExitPartial)
	}
	clean := workspace(t)
	buf.Reset()
	if code := executeAsGitSubcommandWith(&buf, &buf, []string{"status", "--root", clean}); code != ExitOK {
		t.Errorf("second run exit = %d, want %d — the partial failure leaked\n%s", code, ExitOK, buf.String())
	}
}
