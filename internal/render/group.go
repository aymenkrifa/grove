package render

import (
	"regexp"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/git"
)

// noGroup is the heading for a repo that matches no branch prefix.
const noGroup = "(none)"

// groupOf is the heading a repo files under.
func groupOf(r git.Repo, d config.Display) string {
	switch d.GroupBy {
	case "none":
		return ""
	case "branch-prefix":
		return branchPrefix(r.Branch, d.BranchPrefix)
	default:
		return r.Group
	}
}

// branchPrefix returns the leading match of pattern against branch, or
// "(none)" when there is no match. An invalid pattern groups everything
// together rather than failing the command: a typo in a config file should not
// stop grove from reporting status.
func branchPrefix(branch, pattern string) string {
	if pattern == "" {
		return noGroup
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return noGroup
	}
	if m := re.FindString(branch); m != "" {
		return m
	}
	return noGroup
}

// grouped buckets repos under their headings, preserving the order in which
// both the headings and the repos within them first appear.
//
// Bucketing rather than watching for the heading to change matters for
// branch-prefix grouping, where repos sharing a heading are not adjacent in
// discovery order — a repo in api/ and one in web/ can be on the same ticket.
// Without it the same heading is printed once per run of repos.
func grouped(repos []git.Repo, d config.Display) ([]string, map[string][]git.Repo) {
	order := make([]string, 0, len(repos))
	buckets := make(map[string][]git.Repo, len(repos))
	for _, r := range repos {
		g := groupOf(r, d)
		if _, seen := buckets[g]; !seen {
			order = append(order, g)
		}
		buckets[g] = append(buckets[g], r)
	}
	return order, buckets
}
