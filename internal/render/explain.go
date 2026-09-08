package render

import (
	"fmt"
	"strings"

	"github.com/aymenkrifa/grove/internal/git"
)

// explain renders the symbol columns as words: what "~4 ?3" and "↑0 ↓3" mean
// for someone who does not carry the legend in their head.
//
// It is empty for a repository that needs no attention, which is the whole
// point — a column that says "nothing to do" on every clean row is noise, and
// the eye should land only where there is something to land on. NeedsAttention
// is the same predicate `-d` filters on, so the two flags agree by
// construction: everything `-d` keeps has a description, everything it hides
// has none.
//
// An errored repository is also empty: the state column already carries git's
// own message, and repeating it in different words would only invite the two
// to drift apart.
func explain(r git.Repo) string {
	if r.Error != "" || r.Bare || !r.NeedsAttention() {
		return ""
	}

	var parts []string
	parts = countPhrase(parts, r.Unstaged, "modified")
	parts = countPhrase(parts, r.Staged, "staged")
	parts = countPhrase(parts, r.Untracked, "untracked")
	parts = countPhrase(parts, r.Conflicted, "conflicted")
	parts = countPhrase(parts, r.Stashes, "stashed")

	// Divergence only means something against an upstream. Saying so is worth
	// a phrase here: it is the reason the arrows read "↑0 ↓0" forever no
	// matter how far the branch has actually drifted.
	if r.HasUpstream() {
		parts = countPhrase(parts, r.Ahead, "to push")
		parts = countPhrase(parts, r.Behind, "to pull")
	} else if !r.Unborn {
		parts = append(parts, "no upstream")
	}

	return strings.Join(parts, ", ")
}

// countPhrase appends "<n> <label>" unless n is zero, so a phrase never says
// "0 modified" — an absent count is absent, not reported as nothing.
func countPhrase(parts []string, n int, label string) []string {
	if n == 0 {
		return parts
	}
	return append(parts, fmt.Sprintf("%d %s", n, label))
}
