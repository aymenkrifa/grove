package render

import (
	"regexp"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/git"
)

// noGroup is the heading for a repo that matches no branch prefix.
const noGroup = "(none)"

// prefixMatcher compiles the branch-prefix pattern. A nil matcher means no
// heading can be taken from a branch at all — the pattern is empty, or it does
// not compile — and everything files under "(none)" rather than the command
// failing: a typo in a config file should not stop grove reporting status.
func prefixMatcher(pattern string) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}

// branchPrefix returns the leading match of re against branch, or "(none)".
// An empty match is not a heading — it would print as a blank line.
func branchPrefix(re *regexp.Regexp, branch string) string {
	if re == nil {
		return noGroup
	}
	if m := re.FindString(branch); m != "" {
		return m
	}
	return noGroup
}

// heading is the group a repo files under.
func heading(r git.Repo, d config.Display, re *regexp.Regexp) string {
	switch d.GroupBy {
	case "none":
		return ""
	case "branch-prefix":
		return branchPrefix(re, r.Branch)
	default:
		return r.Group
	}
}

// grouped buckets repos under their headings, preserving the order in which
// both the headings and the repos within them first appear.
//
// Bucketing rather than watching for the heading to change matters for
// branch-prefix grouping, where repos sharing a heading are not adjacent in
// discovery order — a repo in api/ and one in web/ can be on the same ticket.
// Without it the same heading is printed once per run of repos.
//
// The pattern is compiled here, once for the whole table rather than once per
// repository.
func grouped(repos []git.Repo, d config.Display) ([]string, map[string][]git.Repo) {
	var re *regexp.Regexp
	if d.GroupBy == "branch-prefix" {
		re = prefixMatcher(d.BranchPrefix)
	}
	order := make([]string, 0, len(repos))
	buckets := make(map[string][]git.Repo, len(repos))
	for _, r := range repos {
		g := heading(r, d, re)
		if _, seen := buckets[g]; !seen {
			order = append(order, g)
		}
		buckets[g] = append(buckets[g], r)
	}
	return order, buckets
}
