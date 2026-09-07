package git

import "testing"

// TestRepoDirty gives each counter a turn on its own: a term dropped from the
// sum is invisible while several counters are non-zero at once.
func TestRepoDirty(t *testing.T) {
	tests := []struct {
		name string
		repo Repo
		want bool
	}{
		{"nothing", Repo{}, false},
		{"staged", Repo{Staged: 1}, true},
		{"unstaged", Repo{Unstaged: 1}, true},
		{"untracked", Repo{Untracked: 1}, true},
		{"conflicted", Repo{Conflicted: 1}, true},
		{"divergence alone is not dirt", Repo{Ahead: 3, Behind: 2}, false},
		{"a stash alone is not dirt", Repo{Stashes: 2}, false},
		{"an error alone is not dirt", Repo{Error: "boom"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.repo.Dirty(); got != tt.want {
				t.Errorf("Dirty() = %v, want %v for %+v", got, tt.want, tt.repo)
			}
		})
	}
}

func TestRepoNeedsAttention(t *testing.T) {
	tests := []struct {
		name string
		repo Repo
		want bool
	}{
		{"clean and in sync", Repo{Branch: "main", Clean: true}, false},
		{"dirty", Repo{Unstaged: 1}, true},
		{"untracked only", Repo{Untracked: 1}, true},
		{"conflicted only", Repo{Conflicted: 1}, true},
		{"ahead", Repo{Ahead: 1}, true},
		{"behind", Repo{Behind: 1}, true},
		{"failed", Repo{Error: "not a git repository"}, true},
		{"stashes alone do not demand attention", Repo{Stashes: 4}, false},
		{"an upstream alone does not demand attention", Repo{Upstream: "origin/main"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.repo.NeedsAttention(); got != tt.want {
				t.Errorf("NeedsAttention() = %v, want %v for %+v", got, tt.want, tt.repo)
			}
		})
	}
}

func TestRepoHasUpstream(t *testing.T) {
	if (Repo{}).HasUpstream() {
		t.Error("HasUpstream() = true for an empty upstream, want false")
	}
	if !(Repo{Upstream: "origin/main"}).HasUpstream() {
		t.Error("HasUpstream() = false for origin/main, want true")
	}
}
