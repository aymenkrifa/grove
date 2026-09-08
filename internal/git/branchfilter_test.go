package git

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aymenkrifa/grove/internal/discover"
	"github.com/aymenkrifa/grove/internal/testutil"
)

func branchFixture(t *testing.T) (string, []discover.Found) {
	t.Helper()
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit(), testutil.WithBranch("bugfix/ABC-123-token"))
	testutil.NewRepo(t, filepath.Join(root, "api", "billing"), testutil.WithCommit(), testutil.WithBranch("develop"))
	testutil.NewRepo(t, filepath.Join(root, "web", "dash"), testutil.WithCommit(), testutil.WithBranch("feat/ABC-123-ui"))
	found, _, err := discover.Walk(root, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	return root, found
}

func rels(f []discover.Found) []string {
	out := make([]string, len(f))
	for i := range f {
		out[i] = f[i].RelPath
	}
	return out
}

func TestFilterByBranchKeepsOnlyMatchingRepos(t *testing.T) {
	_, found := branchFixture(t)
	got := FilterByBranch(context.Background(), found, "ABC-123", 4)
	want := []string{"api/gateway", "web/dash"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", rels(got), want)
	}
	for i := range want {
		if got[i].RelPath != want[i] {
			t.Errorf("[%d] = %q, want %q — input order must be preserved", i, got[i].RelPath, want[i])
		}
	}
}

func TestFilterByBranchIsCaseInsensitive(t *testing.T) {
	_, found := branchFixture(t)
	if got := FilterByBranch(context.Background(), found, "abc-123", 4); len(got) != 2 {
		t.Errorf("got %v, want both repos — a ticket key typed in lower case must still match", rels(got))
	}
}

func TestFilterByBranchEmptyPatternIsANoOp(t *testing.T) {
	_, found := branchFixture(t)
	if got := FilterByBranch(context.Background(), found, "", 4); len(got) != len(found) {
		t.Errorf("got %d repos, want all %d — an empty pattern must not filter", len(got), len(found))
	}
}

func TestFilterByBranchNoMatchIsEmpty(t *testing.T) {
	_, found := branchFixture(t)
	if got := FilterByBranch(context.Background(), found, "ZZZ-999", 4); len(got) != 0 {
		t.Errorf("got %v, want none", rels(got))
	}
}

// A detached HEAD is not "on" a branch in any sense, so it cannot match — and
// the probe must not error the whole filter when it happens.
func TestFilterByBranchDropsADetachedHead(t *testing.T) {
	root := t.TempDir()
	dir := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit(), testutil.WithBranch("bugfix/ABC-123"))
	sha := testutil.Run(t, dir, "rev-parse", "HEAD")
	testutil.Run(t, dir, "checkout", "-q", sha[:40])

	found, _, err := discover.Walk(root, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := FilterByBranch(context.Background(), found, "ABC-123", 1); len(got) != 0 {
		t.Errorf("got %v, want none: a detached checkout is on no branch", rels(got))
	}
}

// symbolic-ref is used precisely because rev-parse HEAD fails in a repository
// with no commits, which would otherwise drop a freshly branched repo.
func TestFilterByBranchMatchesAnUnbornRepo(t *testing.T) {
	root := t.TempDir()
	dir := testutil.NewRepo(t, filepath.Join(root, "api", "fresh"))
	testutil.Run(t, dir, "checkout", "-qb", "bugfix/ABC-123-new")

	found, _, err := discover.Walk(root, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := FilterByBranch(context.Background(), found, "ABC-123", 1); len(got) != 1 {
		t.Errorf("got %v, want the unborn repo: it is on the branch even with no commits", rels(got))
	}
}
