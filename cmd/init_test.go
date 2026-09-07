package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/testutil"
)

func TestInitWritesAMarker(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())

	out, code := run(t, "init", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data, err := os.ReadFile(filepath.Join(root, config.MarkerName))
	if err != nil {
		t.Fatalf("marker not written: %v", err)
	}
	if !strings.Contains(string(data), "depth") {
		t.Errorf("marker should be pre-filled\n%s", data)
	}
}

func TestInitRefusesToOverwrite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	if err := os.WriteFile(filepath.Join(root, config.MarkerName), []byte("depth = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, code := run(t, "init", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d — an existing marker must not be clobbered", code, ExitError)
	}
}

// TestInitDoesNotTouchAnExistingMarker pins the refusal further than the exit
// code: a mutant that still writes the fresh body after "refusing" (e.g. one
// that reports the error but forgets to return) would pass
// TestInitRefusesToOverwrite yet corrupt the file. Reading it back after the
// call is the only way to catch that.
func TestInitDoesNotTouchAnExistingMarker(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	marker := filepath.Join(root, config.MarkerName)
	const original = "depth = 1\n# do not touch\n"
	if err := os.WriteFile(marker, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := run(t, "init", root); code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("marker was modified despite the refusal:\n%s", data)
	}
}

func TestConfigPathPrintsALocation(t *testing.T) {
	cfgHome := filepath.Join(t.TempDir(), "cfg")
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	out, code := run(t, "config", "path")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, filepath.Join(cfgHome, "grove", "config.toml")) {
		t.Errorf("config path = %q", out)
	}
}

// TestConfigShowResolutionSource pins `chosen by:` to each distinct
// precedence rule so a swapped or hard-coded Source string is caught: two
// different roots that would otherwise print identically (both are the
// working directory in the bare case, both come from a marker in the marked
// case) are told apart only by this line.
func TestConfigShowResolutionSource(t *testing.T) {
	t.Run("bare working directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
		t.Setenv("GROVE_ROOT", "")
		chdir(t, dir)
		out, code := run(t, "config", "show")
		if code != ExitOK {
			t.Fatalf("exit = %d\n%s", code, out)
		}
		if !strings.Contains(out, "chosen by:    working directory") {
			t.Errorf("source not reported as the bare cwd rule:\n%s", out)
		}
	})

	t.Run("marker file", func(t *testing.T) {
		root := markedWorkspace(t)
		chdir(t, root)
		out, code := run(t, "config", "show")
		if code != ExitOK {
			t.Fatalf("exit = %d\n%s", code, out)
		}
		if !strings.Contains(out, "chosen by:    "+config.MarkerName) {
			t.Errorf("source not reported as the marker file:\n%s", out)
		}
	})

	t.Run("--root flag", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
		out, code := run(t, "config", "show", "--root", root)
		if code != ExitOK {
			t.Fatalf("exit = %d\n%s", code, out)
		}
		if !strings.Contains(out, "chosen by:    --root flag") {
			t.Errorf("source not reported as the --root flag:\n%s", out)
		}
		if !strings.Contains(out, "root:         "+root) {
			t.Errorf("root not reported:\n%s", out)
		}
	})
}
