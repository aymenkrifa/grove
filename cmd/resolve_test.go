package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/testutil"
)

// chdir moves into dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func markedWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())
	if err := os.WriteFile(filepath.Join(root, config.MarkerName), []byte("depth = 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestResolveScopeAtRootIsEmpty(t *testing.T) {
	root := markedWorkspace(t)
	chdir(t, root)
	out, code := run(t, "__resolve", "--scope")
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 inside a marked grove\n%s", code, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("scope = %q, want empty at the root", out)
	}
}

func TestResolveScopeInAGroupDir(t *testing.T) {
	root := markedWorkspace(t)
	chdir(t, filepath.Join(root, "api"))
	out, code := run(t, "__resolve", "--scope")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.TrimSpace(out) != "api" {
		t.Errorf("scope = %q, want %q", strings.TrimSpace(out), "api")
	}
}

// TestResolveScopeInTheOtherGroupDir is the twin of
// TestResolveScopeInAGroupDir with the other group. A mapping that always
// returns the first path segment it happens to see, or a hard-coded "api",
// passes the single-group test but fails this one.
func TestResolveScopeInTheOtherGroupDir(t *testing.T) {
	root := markedWorkspace(t)
	chdir(t, filepath.Join(root, "web"))
	out, code := run(t, "__resolve", "--scope")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.TrimSpace(out) != "web" {
		t.Errorf("scope = %q, want %q", strings.TrimSpace(out), "web")
	}
}

// TestResolveScopeInADirWithNoRepos covers the degrade-to-everything case:
// scopeFor must yield "" rather than a false selector for a subdirectory of
// the grove that holds no repository at all.
func TestResolveScopeInADirWithNoRepos(t *testing.T) {
	root := markedWorkspace(t)
	empty := filepath.Join(root, "docs")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, empty)
	out, code := run(t, "__resolve", "--scope")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("scope = %q, want empty — docs holds no repository", strings.TrimSpace(out))
	}
}

// TestResolveScopeAtARepoItself exercises the exact-match branch of
// scopeFor, distinct from the group-prefix branch: cwd here IS a repository's
// own RelPath, not an ancestor directory of one. A mutant that keeps only the
// prefix check (dropping "f.RelPath == rel") still passes every group-level
// scope test but fails this one.
func TestResolveScopeAtARepoItself(t *testing.T) {
	root := markedWorkspace(t)
	chdir(t, filepath.Join(root, "api", "gateway"))
	out, code := run(t, "__resolve", "--scope")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.TrimSpace(out) != "api/gateway" {
		t.Errorf("scope = %q, want %q", strings.TrimSpace(out), "api/gateway")
	}
}

func TestResolveFailsOutsideAnyGrove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	t.Setenv("GROVE_ROOT", "")
	chdir(t, dir)
	_, code := run(t, "__resolve", "--scope")
	if code != ExitError {
		t.Errorf("exit = %d, want %d — a plain directory is not a configured grove", code, ExitError)
	}
}

// TestResolveWithoutScopePrintsTheRoot covers the other flag state: without
// --scope the whole resolved root is printed, not a selector.
func TestResolveWithoutScopePrintsTheRoot(t *testing.T) {
	root := markedWorkspace(t)
	chdir(t, filepath.Join(root, "api"))
	out, code := run(t, "__resolve")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.TrimSpace(out) != root {
		t.Errorf("root = %q, want %q", strings.TrimSpace(out), root)
	}
}

// TestResolveDoesNotRequireGit pins ruling 3: __resolve must work even when
// git is not on PATH, because it never runs git — unlike every other command,
// which goes through resolveAndFind and its git.Available() preflight.
func TestResolveDoesNotRequireGit(t *testing.T) {
	root := markedWorkspace(t)
	chdir(t, root)
	t.Setenv("PATH", t.TempDir())
	out, code := run(t, "__resolve", "--scope")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d — __resolve must not need git\n%s", code, ExitOK, out)
	}
}
