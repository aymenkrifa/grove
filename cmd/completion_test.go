package cmd

import (
	"strings"
	"testing"
)

// TestCompletionGeneratesPerShell checks each shell's marker line, not just
// "some non-empty output": cobra's four generators are similar enough
// (shared __grove_debug helper, shared header shape) that a mutant routing
// one shell's case to another generator would still produce non-empty,
// plausible-looking output.
func TestCompletionGeneratesPerShell(t *testing.T) {
	for shell, marker := range map[string]string{
		"bash":       "bash completion V2 for grove",
		"zsh":        "#compdef grove",
		"fish":       "fish completion for grove",
		"powershell": "powershell completion for grove",
	} {
		t.Run(shell, func(t *testing.T) {
			out, code := run(t, "completion", shell)
			if code != ExitOK {
				t.Fatalf("exit = %d\n%s", code, out)
			}
			if !strings.Contains(out, marker) {
				t.Errorf("completion %s missing %q\n%s", shell, marker, firstLines(out, 3))
			}
		})
	}
}

func TestCompletionRejectsUnknownShell(t *testing.T) {
	out, code := run(t, "completion", "tcsh")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, out)
	}
}

func TestCompletionRejectsNoArgs(t *testing.T) {
	_, code := run(t, "completion")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
