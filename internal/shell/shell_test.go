package shell

import (
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

// TestHookAllowlistIsTheSameAcrossShells guards against the allowlist
// silently drifting between the three scripts — e.g. a copy-paste that
// leaves "checkout" or drops "branch" in only one of them. Behaviourally
// this matters more than the shared-substring check in
// TestHookForEachShell, which cannot see the allowlist's actual membership.
func TestHookAllowlistIsTheSameAcrossShells(t *testing.T) {
	const allowed = "status|diff|fetch|log|branch"
	for _, name := range []string{"zsh", "bash"} {
		body, err := Hook(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, allowed) {
			t.Errorf("%s hook's allowlist is not exactly %q\n%s", name, allowed, body)
		}
	}
	body, err := Hook("fish")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `status diff fetch log branch`) {
		t.Errorf("fish hook's allowlist is not the expected five subcommands\n%s", body)
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
