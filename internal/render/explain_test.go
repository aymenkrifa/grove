package render

import (
	"testing"

	"github.com/aymenkrifa/grove/internal/git"
)

func TestExplain(t *testing.T) {
	tests := []struct {
		name string
		repo git.Repo
		want string
	}{
		{"clean and in sync says nothing",
			git.Repo{Branch: "main", Upstream: "origin/main"}, ""},
		{"modified only",
			git.Repo{Unstaged: 5, Upstream: "origin/main"}, "5 modified"},
		{"asymmetric counts keep their own labels",
			git.Repo{Unstaged: 4, Staged: 2, Untracked: 3, Upstream: "origin/main"},
			"4 modified, 2 staged, 3 untracked"},
		{"behind only",
			git.Repo{Upstream: "origin/main", Behind: 3}, "3 to pull"},
		{"ahead only",
			git.Repo{Upstream: "origin/main", Ahead: 2}, "2 to push"},
		{"ahead and behind both appear, push before pull",
			git.Repo{Upstream: "origin/main", Ahead: 2, Behind: 7}, "2 to push, 7 to pull"},
		{"work and divergence combine",
			git.Repo{Unstaged: 4, Untracked: 3, Upstream: "origin/main", Behind: 1},
			"4 modified, 3 untracked, 1 to pull"},
		{"conflicts and stashes are named",
			git.Repo{Conflicted: 2, Stashes: 1, Upstream: "origin/main"},
			"2 conflicted, 1 stashed"},
		{"no upstream is worth saying when there is other work",
			git.Repo{Unstaged: 2}, "2 modified, no upstream"},
		{"a clean repo with no upstream still says nothing",
			git.Repo{Branch: "main"}, ""},
		{"an errored repo defers to the state column",
			git.Repo{Error: "not a git repository"}, ""},
		{"a bare repo has no working tree to describe",
			git.Repo{Bare: true, Branch: "main"}, ""},
		{"an unborn repo is not scolded for having no upstream",
			git.Repo{Unborn: true, Branch: "main", Untracked: 1}, "1 untracked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := explain(tt.repo); got != tt.want {
				t.Errorf("explain() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A zero count must never be reported. "0 modified" would be worse than the
// symbols it replaces, and the mutation that produces it (dropping the guard)
// is otherwise invisible: every other test uses non-zero counts.
func TestExplainNeverReportsAZeroCount(t *testing.T) {
	got := explain(git.Repo{Unstaged: 3, Staged: 0, Untracked: 0, Upstream: "origin/main"})
	if got != "3 modified" {
		t.Errorf("explain() = %q, want %q — zero counts must be omitted entirely", got, "3 modified")
	}
}
