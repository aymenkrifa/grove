package discover

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// mustWalk runs a walk that is expected to be both error-free and warning-free.
// Asserting on the warnings here means an accidental unreadable directory in
// any fixture surfaces as a failure rather than as a quietly missing repo.
func mustWalk(t *testing.T, root string, depth int, ignore []string) []Found {
	t.Helper()
	got, warns, err := Walk(root, depth, ignore)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("Walk() warnings = %v, want none", warns)
	}
	return got
}

func wantRels(t *testing.T, got []Found, why string, want ...string) {
	t.Helper()
	g := rels(got)
	if len(g) != len(want) {
		t.Fatalf("got %v, want %v — %s", g, want, why)
	}
	for i := range want {
		if g[i] != want[i] {
			t.Errorf("repo[%d] = %q, want %q — %s", i, g[i], want[i], why)
		}
	}
}

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

	// Depth 5, not 3: the inner repo sits four levels down, so at depth 3 the
	// depth limit alone would exclude it and stop-at-repo would never be the
	// reason it is absent. The limit has to be out of the way for this test to
	// be testing what it claims.
	got := mustWalk(t, root, 5, nil)
	wantRels(t, got, "descent stops at api/gateway, so vendor/inner is never seen",
		"api/gateway", "web/dashboard")
	if got[0].Group != "api" {
		t.Errorf("Group = %q, want %q", got[0].Group, "api")
	}
}

func TestWalkRespectsDepthAndIgnore(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "a", "b", "c", "deep"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "node_modules", "pkg"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "top"), testutil.WithCommit())

	got := mustWalk(t, root, 2, []string{"**/node_modules/**"})
	wantRels(t, got, "depth 2 excludes the deep repo, the glob excludes node_modules", "top")
}

// The depth limit is inclusive: a repository exactly depth levels below the
// root is found, one level further down is not. Both edges are asserted,
// because off-by-one in either direction is the likely bug.
func TestWalkDepthBoundaryIsInclusive(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "atRoot"), testutil.WithCommit())                // level 1
	testutil.NewRepo(t, filepath.Join(root, "group", "repo"), testutil.WithCommit())         // level 2
	testutil.NewRepo(t, filepath.Join(root, "deep", "group", "repo"), testutil.WithCommit()) // level 3

	got := mustWalk(t, root, 2, nil)
	wantRels(t, got, "depth 2 reaches level 2 and no further", "atRoot", "group/repo")
}

// A slash-free ignore pattern matches its segment at any depth, not only at the
// root — the .gitignore rule.
func TestWalkIgnoreMatchesAtAnyDepth(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "node_modules", "pkg"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "web", "app", "node_modules", "pkg"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "web", "app", "dashboard"), testutil.WithCommit())

	got := mustWalk(t, root, 5, []string{"**/node_modules/**"})
	wantRels(t, got, "node_modules is excluded at the root and three levels down",
		"web/app/dashboard")
}

// Symlinked directories are not descended into and not reported. A symlink can
// point back up its own tree, and even when it does not it double-reports one
// repository under two names.
func TestWalkDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	real := testutil.NewRepo(t, filepath.Join(root, "real", "repo"), testutil.WithCommit())
	if err := os.Symlink(real, filepath.Join(root, "link")); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}

	got := mustWalk(t, root, 3, nil)
	wantRels(t, got, "the symlink at root/link must not become a second repo", "real/repo")
}

// Results are sorted by RelPath, which is not the order discovery produces.
// "-" (0x2D) sorts before "/" (0x2F), so "api-legacy" must come out ahead of
// "api/gateway" even though the walk descends into "api" first.
func TestWalkSortsByRelPath(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "api-legacy"), testutil.WithCommit())

	got := mustWalk(t, root, 3, nil)
	wantRels(t, got, "discovery order is api/gateway then api-legacy; the sort reverses it",
		"api-legacy", "api/gateway")
}

func TestWalkHandlesRootThatIsItselfARepo(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, root, testutil.WithCommit())

	got := mustWalk(t, root, 3, nil)
	wantRels(t, got, "the root itself is the only repository", ".")
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

	got := mustWalk(t, root, 3, nil)
	wantRels(t, got, "a bare repo is discovered like any other", "infra/mirror", "web/dashboard")
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

	got := mustWalk(t, root, 5, nil)
	wantRels(t, got, "the .git directory must not surface as a second repo", "api/gateway")
}

// Dot-prefixed directories are not descended into.
//
// The fixture uses a hidden directory that is NOT inside a repository, because
// the .git of an ordinary repo is shielded by stop-at-repo as well — asserting
// against that one would pass for the wrong reason and keep passing if the
// dot-skip were deleted.
//
// Whether hidden directories should be skipped at all is an open question for
// the spec, deliberately left as it is. This pins today's answer so that
// changing it later has to be a decision rather than an accident, and because
// isRepo's own doc comment leans on the property.
func TestWalkSkipsHiddenDirectories(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, ".cache", "repo"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())

	got := mustWalk(t, root, 3, nil)
	wantRels(t, got, "a repo inside a dot-directory is not reported", "api/gateway")
}

// Bare detection requires objects to be a DIRECTORY. A stray pair of plain
// files named HEAD and objects is not a git directory and must not be reported
// as one — the looser "both exist" test would claim it.
func TestWalkDoesNotMistakeLooseFilesForABareRepo(t *testing.T) {
	root := t.TempDir()
	decoy := filepath.Join(root, "decoy")
	if err := os.MkdirAll(decoy, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"HEAD", "objects"} {
		if err := os.WriteFile(filepath.Join(decoy, name), []byte("not a repo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())

	if isRepo(decoy) {
		t.Errorf("isRepo(%s) = true; HEAD and objects as plain files are not a repository", decoy)
	}
	got := mustWalk(t, root, 3, nil)
	wantRels(t, got, "the decoy directory is not a bare repo", "api/gateway")
}

// The caller named this directory specifically, so "I cannot read it" must not
// be reported as "it holds no repositories".
func TestWalkUnreadableRootIsAnError(t *testing.T) {
	requireNonRoot(t)
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	lockDir(t, locked)

	got, warns, err := Walk(locked, 3, nil)
	if err == nil {
		t.Fatalf("Walk() on an unreadable root should error; got %v", rels(got))
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error = %v, want a permission error", err)
	}
	if got != nil || warns != nil {
		t.Errorf("got (%v, %v), want (nil, nil) alongside the error", rels(got), warns)
	}
}

// One locked subtree must not cost the caller the rest of the walk, but it must
// not vanish silently either.
func TestWalkUnreadableSubdirectoryWarnsAndContinues(t *testing.T) {
	requireNonRoot(t)
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	locked := filepath.Join(root, "web")
	if err := os.MkdirAll(filepath.Join(locked, "dashboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	lockDir(t, locked)

	got, warns, err := Walk(root, 3, nil)
	if err != nil {
		t.Fatalf("Walk() error = %v; an unreadable subdirectory is not fatal", err)
	}
	wantRels(t, got, "the readable half of the tree still comes back", "api/gateway")
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warns)
	}
	if !strings.Contains(warns[0], "web") {
		t.Errorf("warning %q should name the directory it skipped", warns[0])
	}
	if !strings.Contains(warns[0], fs.ErrPermission.Error()) {
		t.Errorf("warning %q should say why it was skipped", warns[0])
	}
}

func requireNonRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: the permission bits this test relies on do not apply")
	}
}

// lockDir makes dir unreadable, restoring it before TempDir's own cleanup runs
// (cleanups are LIFO) so the temporary tree can still be removed.
func lockDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

func rels(f []Found) []string {
	out := make([]string, len(f))
	for i := range f {
		out[i] = f[i].RelPath
	}
	return out
}
