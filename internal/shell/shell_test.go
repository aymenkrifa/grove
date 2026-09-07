package shell

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestHookForEachShell(t *testing.T) {
	for _, name := range []string{"zsh", "bash", "fish"} {
		t.Run(name, func(t *testing.T) {
			body, err := Hook(name)
			if err != nil {
				t.Fatalf("Hook(%q) error = %v", name, err)
			}
			for _, want := range []string{"GROVE_NO_GIT_HOOK", "__resolve", "command git", "→ grove"} {
				if !strings.Contains(body, want) {
					t.Errorf("%s hook is missing %q", name, want)
				}
			}
		})
	}
}

func TestHookUnknownShell(t *testing.T) {
	if _, err := Hook("tcsh"); err == nil {
		t.Fatal("Hook(\"tcsh\") should error")
	}
}

// allowlistIn extracts the subcommands a hook forwards, from the one line in
// each script that lists them: `status|diff|fetch|log|branch)` in the POSIX
// case statement, and `contains -- "$argv[1]" status diff ...` in fish.
//
// It parses rather than searches on purpose. A strings.Contains over the
// script can only prove the five are present; it is structurally blind to a
// sixth being added, which is the dangerous direction — an added "push" makes
// grove answer for a subcommand that writes.
func allowlistIn(t *testing.T, name string) []string {
	t.Helper()
	body, err := Hook(name)
	if err != nil {
		t.Fatal(err)
	}
	patterns := map[string]*regexp.Regexp{
		"zsh":  regexp.MustCompile(`(?m)^\s*([a-z|]+)\)\s*;;\s*$`),
		"bash": regexp.MustCompile(`(?m)^\s*([a-z|]+)\)\s*;;\s*$`),
		"fish": regexp.MustCompile(`(?m)^\s*if not contains -- "\$argv\[1\]"([a-z ]+)$`),
	}
	m := patterns[name].FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no allowlist line found in the %s hook — the test cannot see what it forwards\n%s", name, body)
	}
	return strings.FieldsFunc(m[1], func(r rune) bool { return r == '|' || r == ' ' })
}

// TestHookAllowlistIsTheSameAcrossShells pins the allowlist exactly, in every
// shell. Both directions matter: a subcommand dropped from one script makes
// the hook inconsistent between two of the user's terminals, and one added to
// any of them hands git's own subcommand to grove — for a writing subcommand
// such as push that is the worst thing the wrapper can do.
//
// This is the only executable check fish gets on a machine without fish
// installed, which is why it parses the script rather than grepping it.
func TestHookAllowlistIsTheSameAcrossShells(t *testing.T) {
	want := []string{"status", "diff", "fetch", "log", "branch"}
	for _, name := range []string{"zsh", "bash", "fish"} {
		got := allowlistIn(t, name)
		if !slices.Equal(got, want) {
			t.Errorf("%s hook forwards %q, want exactly %q", name, got, want)
		}
	}
}

// TestHookContentIsShellSpecific catches the map in Hook being wired to the
// wrong file for a given name — e.g. "zsh" serving the fish script — which
// TestHookForEachShell's shared-substring check would not notice, since all
// three scripts contain the same shared substrings.
func TestHookContentIsShellSpecific(t *testing.T) {
	zsh, err := Hook("zsh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(zsh, "emulate -L zsh") {
		t.Errorf("zsh hook does not look like the zsh script\n%s", zsh)
	}

	bash, err := Hook("bash")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bash, "emulate -L zsh") {
		t.Errorf("bash hook looks like the zsh script\n%s", bash)
	}
	if !strings.Contains(bash, `git() {`) {
		t.Errorf("bash hook does not define a git() function\n%s", bash)
	}

	fish, err := Hook("fish")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fish, "function git") {
		t.Errorf("fish hook does not look like the fish script\n%s", fish)
	}
	if strings.Contains(fish, `git() {`) {
		t.Errorf("fish hook looks like a POSIX-syntax script\n%s", fish)
	}
}

// TestHookDisablesWithEnvVar and TestHookBypassesWithCommandGit pin the two
// documented escape hatches textually, in every shell, since neither has any
// other test coverage once the hook itself only exists as embedded text.
func TestHookDisablesWithEnvVar(t *testing.T) {
	for _, name := range []string{"zsh", "bash", "fish"} {
		body, err := Hook(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, "GROVE_NO_GIT_HOOK") {
			t.Errorf("%s hook does not mention the disable variable", name)
		}
	}
}

// TestFishHookPassesArgumentsAsAList is the one shell rule this package can
// only assert textually: fish is not installed on every machine, and the
// executable matrix skips it when it is absent.
//
// In fish a double-quoted list expansion joins its elements with spaces into
// exactly ONE argument — it behaves like POSIX "$*", not "$@". So
// `command git "$argv"` turns `git commit -m "hello world"` into the single
// argument `commit -m hello world` ("git: 'commit -m hello world' is not a
// git command"), a bare `git` into `git ""`, and a forwarded `git status` in
// a group directory into `grove status api ""`, which the CLI rejects. The
// quoting buys nothing in return: fish expands wildcards only in literal
// tokens, never in the value of a variable.
//
// `test -n "$scope"` and `contains -- "$argv[1]"` keep their quotes and are
// therefore not matched here: both want exactly one argument, including when
// the value is empty.
func TestFishHookPassesArgumentsAsAList(t *testing.T) {
	body, err := Hook("fish")
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "command ") {
			continue
		}
		if strings.Contains(trimmed, `"$`) {
			t.Errorf("fish.fish:%d quotes an expansion it passes to a command, which joins it into one argument:\n\t%s", i+1, trimmed)
		}
	}
	for _, want := range []string{
		"command git $argv",
		"command grove $sub $scope $argv",
		"command grove $sub $argv",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("fish hook does not forward with %q\n%s", want, body)
		}
	}
}

// TestFishHookDoesNotReadTheStatusOfSet pins the other half of the same
// defect class. `set -l scope (command grove __resolve --scope)` followed by
// `test $status -ne 0` reads set's own exit status rather than the command
// substitution's, so a __resolve that failed — "you are not in a grove" —
// would read as success and the hook would forward anyway. The in-grove test
// therefore has to be a plain command in an `if` condition, whose status fish
// reads directly.
//
// Unverified by execution: fish is not installed here, and this reasoning is
// from fish's documented semantics, not from a run.
func TestFishHookDoesNotReadTheStatusOfSet(t *testing.T) {
	body, err := Hook("fish")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "$status") {
		t.Errorf("the fish hook branches on $status; after `set` that is set's own status, not the command substitution's\n%s", body)
	}
	if !strings.Contains(body, "if not command grove __resolve") {
		t.Errorf("the fish hook does not test the resolver's own exit status\n%s", body)
	}
}
