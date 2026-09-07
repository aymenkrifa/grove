package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aymenkrifa/grove/internal/testutil"
)

func TestWalkFindsReposAndStopsDescending(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())
	// A nested repo inside another repo must NOT be reported separately.
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway", "vendor", "inner"), testutil.WithCommit())
	// A plain directory with no repo in it.
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Walk(root, 3, nil)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	want := []string{"api/gateway", "web/dashboard"}
	if len(got) != len(want) {
		t.Fatalf("got %d repos %v, want %d %v", len(got), rels(got), len(want), want)
	}
	for i := range want {
		if got[i].RelPath != want[i] {
			t.Errorf("repo[%d] = %q, want %q", i, got[i].RelPath, want[i])
		}
	}
	if got[0].Group != "api" {
		t.Errorf("Group = %q, want %q", got[0].Group, "api")
	}
}

func TestWalkRespectsDepthAndIgnore(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "a", "b", "c", "deep"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "node_modules", "pkg"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "top"), testutil.WithCommit())

	got, err := Walk(root, 2, []string{"**/node_modules/**"})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if len(got) != 1 || got[0].RelPath != "top" {
		t.Fatalf("got %v, want [top] — depth 2 excludes the deep repo, the glob excludes node_modules", rels(got))
	}
}

func TestWalkHandlesRootThatIsItselfARepo(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, root, testutil.WithCommit())

	got, err := Walk(root, 3, nil)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if len(got) != 1 || got[0].RelPath != "." {
		t.Fatalf("got %v, want [.]", rels(got))
	}
	if got[0].Group != "" {
		t.Errorf("Group = %q, want \"\" for a repo at the root", got[0].Group)
	}
}

// A bare repository has no .git entry at all — the directory *is* the git
// directory. Discovery must still find it, because the spec asks for bare
// repos to be listed and marked, and a status collector cannot mark what
// discovery never handed it.
func TestWalkFindsBareRepo(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "infra", "mirror"), testutil.Bare())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())

	if _, err := os.Lstat(filepath.Join(root, "infra", "mirror", ".git")); !os.IsNotExist(err) {
		t.Fatalf("fixture is not bare: .git exists (err = %v)", err)
	}

	got, err := Walk(root, 3, nil)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	want := []string{"infra/mirror", "web/dashboard"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", rels(got), want)
	}
	for i := range want {
		if got[i].RelPath != want[i] {
			t.Errorf("repo[%d] = %q, want %q", i, got[i].RelPath, want[i])
		}
	}
	if got[0].Group != "infra" {
		t.Errorf("Group = %q, want %q", got[0].Group, "infra")
	}
}

// Bare detection keys off HEAD + objects/, which every .git directory also
// has. The walk must not therefore report a normal repository twice — once for
// the working tree and once for its own .git. It does not, because dot-prefixed
// directories are skipped and descent stops at the first repo, but the
// interaction is worth pinning.
func TestWalkDoesNotReportGitDirOfNormalRepo(t *testing.T) {
	root := t.TempDir()
	repo := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())

	gitDir := filepath.Join(repo, ".git")
	if !isRepo(gitDir) {
		t.Fatalf("precondition: %s should look like a git directory to isRepo", gitDir)
	}

	got, err := Walk(root, 5, nil)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if len(got) != 1 || got[0].RelPath != "api/gateway" {
		t.Fatalf("got %v, want [api/gateway]", rels(got))
	}
}

func rels(f []Found) []string {
	out := make([]string, len(f))
	for i := range f {
		out[i] = f[i].RelPath
	}
	return out
}
