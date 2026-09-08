package cmd

import (
	"os"
	"path/filepath"
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

// --install writes the script where the shell looks for it, so nobody has to
// copy a screenful of generated code. The paths are XDG's, which means no
// plugin framework is required and no startup file is edited.
func TestCompletionInstallWritesToTheShellsOwnDirectory(t *testing.T) {
	data := t.TempDir()
	config := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("NO_COLOR", "1")

	tests := []struct {
		shell string
		want  string
	}{
		{"zsh", filepath.Join(data, "zsh", "site-functions", "_grove")},
		{"bash", filepath.Join(data, "bash-completion", "completions", "grove")},
		{"fish", filepath.Join(config, "fish", "completions", "grove.fish")},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			out, code := run(t, "completion", tt.shell, "--install")
			if code != ExitOK {
				t.Fatalf("exit = %d\n%s", code, out)
			}
			body, err := os.ReadFile(tt.want)
			if err != nil {
				t.Fatalf("nothing written to %s: %v", tt.want, err)
			}
			if len(body) < 200 {
				t.Errorf("wrote only %d bytes to %s; that is not a completion script", len(body), tt.want)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("output should name the path it wrote, got %q", out)
			}
		})
	}
}

// The zsh file must be named _grove: zsh autoloads a completion by filename,
// so the name is the contract, not a convention.
func TestCompletionInstallNamesTheZshFileForAutoloading(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("NO_COLOR", "1")

	if _, code := run(t, "completion", "zsh", "--install"); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(filepath.Join(data, "zsh", "site-functions", "_grove")); err != nil {
		t.Errorf("zsh completion must be named _grove to be autoloaded: %v", err)
	}
}

// Installing must be opt-in: the bare command stays a generator whose output
// can be redirected, which is what a package build needs.
func TestCompletionWithoutInstallWritesNoFile(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("NO_COLOR", "1")

	out, code := run(t, "completion", "zsh")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "#compdef grove") {
		t.Errorf("the script should still go to stdout, got %q", out[:min(80, len(out))])
	}
	if _, err := os.Stat(filepath.Join(data, "zsh")); err == nil {
		t.Error("plain `completion zsh` must not write anything to disk")
	}
}

// powershell has no standard directory, so --install must say so rather than
// invent one.
func TestCompletionInstallRejectsPowershell(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")

	out, code := run(t, "completion", "powershell", "--install")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, out)
	}
	if !strings.Contains(out, "$PROFILE") {
		t.Errorf("the error should point at the profile instead, got %q", out)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --rc closes the last manual step for zsh: the fpath line, added for you.
func TestCompletionInstallRCAppendsAndIsIdempotent(t *testing.T) {
	data := t.TempDir()
	zdot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("ZDOTDIR", zdot)
	t.Setenv("NO_COLOR", "1")
	rc := filepath.Join(zdot, ".zshrc")
	if err := os.WriteFile(rc, []byte("# my existing config\nalias ll='ls -la'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := run(t, "completion", "zsh", "--install", "--rc")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	body, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)

	if !strings.Contains(got, "# my existing config") {
		t.Error("appending must not disturb what was already in the file")
	}
	dir := filepath.Join(data, "zsh", "site-functions")
	if !strings.Contains(got, "fpath=("+dir+" $fpath)") {
		t.Errorf(".zshrc missing the fpath line:\n%s", got)
	}
	// compinit must be re-run: the block lands after whatever already called
	// it, so without this the directory is on the fpath and still ignored.
	if !strings.Contains(got, "compinit") {
		t.Errorf(".zshrc has the fpath line but never re-runs compinit, so it will never load:\n%s", got)
	}

	// Running it again must not stack a second block.
	if _, code := run(t, "completion", "zsh", "--install", "--rc"); code != ExitOK {
		t.Fatalf("second run exit = %d", code)
	}
	again, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(again), "fpath=("+dir); n != 1 {
		t.Errorf("fpath line appears %d times after two runs, want 1", n)
	}
}

// --rc without --install should not silently do nothing useful; and --install
// alone must never touch a startup file.
func TestCompletionInstallAloneDoesNotTouchZshrc(t *testing.T) {
	data := t.TempDir()
	zdot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("ZDOTDIR", zdot)
	t.Setenv("NO_COLOR", "1")
	rc := filepath.Join(zdot, ".zshrc")
	if err := os.WriteFile(rc, []byte("# untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := run(t, "completion", "zsh", "--install"); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	body, _ := os.ReadFile(rc)
	if string(body) != "# untouched\n" {
		t.Errorf("--install must not edit a startup file on its own, got:\n%s", body)
	}
}
