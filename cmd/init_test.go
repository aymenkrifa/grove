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

// lockDir makes dir unreadable and restores it before TempDir's own cleanup
// runs — cleanups are LIFO, and a 0000 directory cannot be removed.
func lockDir(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root can read a 0000 directory, so there is nothing to warn about")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// TestInitWarnsAboutUnreadableSubtrees covers init's own warning routing,
// which nothing else reaches: init does not go through resolveAndFind, so it
// carries a second copy of the "print the walk's warnings, then carry on"
// rule. The repository count it writes into the marker comes from that same
// walk, so a subtree it could not read is exactly the case where the count
// silently under-reports — the warning is the only notice the user gets.
func TestInitWarnsAboutUnreadableSubtrees(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	lockDir(t, filepath.Join(root, "vault"))

	stdout, stderr, code := runSplit(t, "init", root)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d — an unreadable subtree does not stop init\nstderr: %s",
			code, ExitOK, stderr)
	}
	if !strings.Contains(stderr, "grove: warning:") || !strings.Contains(stderr, "vault") {
		t.Errorf("no warning about the unreadable directory on stderr:\n%s", stderr)
	}
	if strings.Contains(stdout, "vault") {
		t.Errorf("the warning was written to stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "wrote ") {
		t.Errorf("init did not report writing the marker:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, config.MarkerName)); err != nil {
		t.Errorf("marker not written despite the warning: %v", err)
	}
}
