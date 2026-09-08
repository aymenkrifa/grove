package git

import (
	"context"
	"strings"
	"sync"

	"github.com/aymenkrifa/grove/internal/discover"
)

// FilterByBranch keeps the repositories whose current branch contains substr,
// compared case-insensitively.
//
// This is how a ticket key selects repositories. Nothing in git associates an
// uncommitted change with a ticket — the working tree carries no such metadata
// — so the branch name is the only signal available, and it is a good one
// wherever branches are named after the ticket they serve. It is worth being
// clear about the limit: this finds work you are ON the ticket's branch for,
// not every edit that belongs to the ticket.
//
// The probe is symbolic-ref rather than rev-parse because it answers correctly
// for a repository with no commits yet, where rev-parse HEAD fails outright. A
// detached HEAD has no branch to match, so it drops out — which is right: a
// detached checkout is not "on" a ticket's branch in any sense.
//
// Order is preserved, and a repository whose branch cannot be read is dropped
// rather than reported: it is a filter, and the commands that care about
// broken repositories say so through their own collection.
func FilterByBranch(ctx context.Context, found []discover.Found, substr string, jobs int) []discover.Found {
	if substr == "" {
		return found
	}
	if jobs <= 0 {
		jobs = DefaultJobs()
	}
	want := strings.ToLower(substr)

	keep := make([]bool, len(found))
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	for i, f := range found {
		wg.Add(1)
		go func(i int, dir string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out, err := Run(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
			if err != nil {
				return // detached, or unreadable: no branch to match
			}
			keep[i] = strings.Contains(strings.ToLower(strings.TrimSpace(out)), want)
		}(i, f.AbsPath)
	}
	wg.Wait()

	out := make([]discover.Found, 0, len(found))
	for i, f := range found {
		if keep[i] {
			out = append(out, f)
		}
	}
	return out
}
