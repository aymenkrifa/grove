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

// unmarkedTree is markedWorkspace without the marker file and without a
// config: the root is discoverable only through whatever rule the test itself
// arms ($GROVE_ROOT, a configured workspace), which is what the rules that do
// not consult the working directory need in order to be tested at all.
func unmarkedTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())
	return root
}

// TestResolveRefusesWhenTheRootIsNotAnAncestorOfCwd is the regression test for
// the hook firing outside any grove.
//
// Rejecting only Source == "working directory" is not enough. $GROVE_ROOT
// (rule 2) and the configured default workspace (rule 5) resolve to a root
// without ever looking at the working directory, so both answer happily from
// an unrelated directory — and the shell hook, reading exit code 0 as "you
// are in a grove", then rewrites `git status` into a report about a tree the
// user is not in. Verified as a live defect in a real zsh before this test
// existed: with `default = "work"` configured, `git status` in an unrelated
// non-repo directory printed the grove's table.
//
// Both flag forms are checked in each case: the hook uses the plain form as a
// cheap in-grove probe, so it has to be exactly as strict as --scope.
func TestResolveRefusesWhenTheRootIsNotAnAncestorOfCwd(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, tree string)
	}{
		{
			// Rule 2.
			name: "GROVE_ROOT points elsewhere",
			setup: func(t *testing.T, tree string) {
				t.Setenv("GROVE_ROOT", tree)
			},
		},
		{
			// Rule 5.
			name: "default workspace is elsewhere",
			setup: func(t *testing.T, tree string) {
				writeConfig(t, "default = \"work\"\n\n[[workspace]]\nname = \"work\"\nroot = \""+tree+"\"\n")
			},
		},
		{
			// Rule 6: a workspace exists but is neither the default nor a
			// container of cwd, so nothing claims this directory.
			name: "a configured workspace that claims nothing",
			setup: func(t *testing.T, tree string) {
				writeConfig(t, "[[workspace]]\nname = \"work\"\nroot = \""+tree+"\"\n")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			tree := unmarkedTree(t)
			outside := t.TempDir() // a plain directory, nowhere near the tree
			tc.setup(t, tree)
			chdir(t, outside)

			for _, args := range [][]string{{"__resolve", "--scope"}, {"__resolve"}} {
				out, code := run(t, args...)
				if code != ExitError {
					t.Errorf("grove %v: exit = %d, want %d — cwd is not inside %s\n%s",
						args, code, ExitError, tree, out)
				}
				if strings.Contains(out, tree) {
					t.Errorf("grove %v named a grove the user is not in:\n%s", args, out)
				}
			}
		})
	}
}

// TestResolveAcceptsTheRulesThatDoContainCwd is the other half of
// TestResolveRefusesWhenTheRootIsNotAnAncestorOfCwd: the new containment
// check must not break the cases the hook exists for. A mutant that simply
// refuses everything passes the refusal test above and fails here.
func TestResolveAcceptsTheRulesThatDoContainCwd(t *testing.T) {
	for _, tc := range []struct {
		name  string
		where string // subdirectory of the tree to run in
		want  string
		setup func(t *testing.T, tree string)
	}{
		{
			name:  "GROVE_ROOT at its own root",
			where: ".",
			want:  "",
			setup: func(t *testing.T, tree string) { t.Setenv("GROVE_ROOT", tree) },
		},
		{
			name:  "GROVE_ROOT from a group directory",
			where: "api",
			want:  "api",
			setup: func(t *testing.T, tree string) { t.Setenv("GROVE_ROOT", tree) },
		},
		{
			// Rule 4: a configured workspace that does contain cwd.
			name:  "workspace containing the working directory",
			where: "web",
			want:  "web",
			setup: func(t *testing.T, tree string) {
				writeConfig(t, "[[workspace]]\nname = \"work\"\nroot = \""+tree+"\"\n")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			tree := unmarkedTree(t)
			tc.setup(t, tree)
			chdir(t, filepath.Join(tree, tc.where))
			out, code := run(t, "__resolve", "--scope")
			if code != ExitOK {
				t.Fatalf("exit = %d, want 0\n%s", code, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("scope = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveScopeGoesToStdoutOnly pins the stream the hook actually reads.
// The hook captures stdout through a command substitution: a selector moved
// to stderr would leave the substitution empty, which is not an error but the
// value meaning "the whole grove" — so `git status` in a group directory
// would quietly report every repository instead of that group's. run() merges
// the two streams and cannot see the difference; only runSplit can.
func TestResolveScopeGoesToStdoutOnly(t *testing.T) {
	root := markedWorkspace(t)
	chdir(t, filepath.Join(root, "api"))

	stdout, stderr, code := runSplit(t, "__resolve", "--scope")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s%s", code, stdout, stderr)
	}
	if stdout != "api\n" {
		t.Errorf("stdout = %q, want %q", stdout, "api\n")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing — the hook reads stdout, and anything here is either lost or parsed as a selector", stderr)
	}

	chdir(t, root)
	stdout, stderr, code = runSplit(t, "__resolve")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s%s", code, stdout, stderr)
	}
	if stdout != root+"\n" {
		t.Errorf("stdout = %q, want %q", stdout, root+"\n")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

// TestResolveScopeDoesNotClaimAPrefixSibling covers scopeFor's rel+"/" guard.
// With a bare strings.HasPrefix(f.RelPath, rel) the directory "we" — which
// holds no repository at all — would match "web/dashboard" and hand the hook
// the selector "we", so `git status` in an empty directory would report the
// neighbouring group's repositories. Every other scope fixture has groups
// with no shared prefix and cannot see this.
func TestResolveScopeDoesNotClaimAPrefixSibling(t *testing.T) {
	root := markedWorkspace(t) // holds api/gateway and web/dashboard
	sibling := filepath.Join(root, "we")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, sibling)
	out, code := run(t, "__resolve", "--scope")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "" {
		t.Errorf("scope = %q, want empty — %q holds no repository; %q is a different group", got, "we", "web")
	}
}

// TestRelWithin exercises the containment test directly, including the two
// cases the command-level tests cannot reach: cwd exactly one segment above
// the root (rel == ".."), and a directory inside the grove whose name merely
// begins with two dots. Testing only for a ".." prefix would call "..cache" an
// escape and refuse to forward from a perfectly ordinary subdirectory.
func TestRelWithin(t *testing.T) {
	root := filepath.Join("/groves", "work")
	for _, tc := range []struct {
		path string
		rel  string
		ok   bool
	}{
		{root, ".", true},
		{filepath.Join(root, "api"), "api", true},
		{filepath.Join(root, "api", "gateway"), "api/gateway", true},
		{filepath.Join(root, "..cache"), "..cache", true},
		{filepath.Join(root, "..cache", "repo"), "..cache/repo", true},
		{filepath.Join(root, ".."), "", false},                    // /groves
		{filepath.Join(root, "..", "other"), "", false},           // /groves/other
		{filepath.Join(root, "..", "..", "elsewhere"), "", false}, // /elsewhere
		{"/", "", false},
	} {
		rel, ok := relWithin(root, tc.path)
		if ok != tc.ok || rel != tc.rel {
			t.Errorf("relWithin(%q, %q) = (%q, %v), want (%q, %v)", root, tc.path, rel, ok, tc.rel, tc.ok)
		}
	}
}

// TestResolveScopeAtAGroveThatIsItselfARepo pins the empty selector against
// the one shape where the walk reports a repository at the root itself: the
// contract is that "" means the whole grove, and scopeFor must not answer "."
// — which grove would then resolve back to every repository anyway, by a
// longer route and with a puzzling "→ grove status ." on screen.
func TestResolveScopeAtAGroveThatIsItselfARepo(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	t.Setenv("GROVE_ROOT", "")
	testutil.NewRepo(t, root, testutil.WithCommit())
	if err := os.WriteFile(filepath.Join(root, config.MarkerName), []byte("depth = 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)

	out, code := run(t, "__resolve", "--scope")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "" {
		t.Errorf("scope = %q, want empty — the root itself is the whole grove", got)
	}
}
