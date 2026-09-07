package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// diffWorkspace has a dirty repo, a clean repo, and a bare repo (no working
// tree, so `git diff` fails in it). The three exercise the omit-when-clean
// path, the stat path, and the per-repo error path all in one fixture.
func diffWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit(), testutil.Dirty())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "tools", "mirror"), testutil.Bare())
	isolate(t)
	return root
}

func TestDiffShowsPerRepoStat(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "api/gateway") {
		t.Errorf("the dirty repo should appear\n%s", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("the changed file should appear in the stat\n%s", out)
	}
	if strings.Contains(out, "web/dashboard") {
		t.Errorf("a repo with no changes should be omitted\n%s", out)
	}
}

func TestDiffSingleRepoShowsFullDiff(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root, "gateway")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "-hello") || !strings.Contains(out, "+changed") {
		t.Errorf("expected a real unified diff\n%s", out)
	}
}

// TestDiffStatFlagForcesStatEvenForASingleRepo pins the branch condition
// `len(found) == 1 && !statOnly`: dropping the statOnly check would make
// --stat a no-op for a single-repo selection.
func TestDiffStatFlagForcesStatEvenForASingleRepo(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root, "gateway", "--stat")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "-hello") || strings.Contains(out, "+changed") {
		t.Errorf("--stat should suppress the unified diff body\n%s", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("--stat should still name the changed file\n%s", out)
	}
}

// TestDiffPassesThroughArgsAfterDash confirms arguments after -- reach git
// diff verbatim: --name-only produces a bare filename with no +/- hunk, which
// only happens if the passthrough args actually arrived at git.
func TestDiffPassesThroughArgsAfterDash(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root, "gateway", "--", "--name-only")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("expected the changed filename\n%s", out)
	}
	if strings.Contains(out, "-hello") || strings.Contains(out, "+changed") {
		t.Errorf("--name-only should suppress the hunk body; passthrough args were not forwarded\n%s", out)
	}
}

// TestDiffAccumulatesPerRepoErrorsAndExitsPartial covers the multi-repo path:
// a bare repository has no working tree, so `git diff --stat` fails inside
// it. That failure must not stop the run — the healthy dirty repo's stat
// still has to appear — and it must downgrade the exit code.
func TestDiffAccumulatesPerRepoErrorsAndExitsPartial(t *testing.T) {
	root := diffWorkspace(t)
	out, code := run(t, "diff", "--root", root)
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitPartial, out)
	}
	if !strings.Contains(out, "api/gateway") || !strings.Contains(out, "README.md") {
		t.Errorf("the healthy dirty repo should still be reported\n%s", out)
	}
	if !strings.Contains(out, "tools/mirror") {
		t.Errorf("the failing bare repo should be named in the output\n%s", out)
	}
	if strings.Contains(out, "web/dashboard") {
		t.Errorf("the clean repo should still be omitted\n%s", out)
	}
}

func TestDiffUnknownSelectorExitsError(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "diff", "--root", root, "nosuchrepo")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, out)
	}
}
