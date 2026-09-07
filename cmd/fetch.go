package cmd

import (
	"fmt"
	"sync"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/git"
)

func newFetchCmd() *cobra.Command {
	var prune bool
	c := &cobra.Command{
		Use:   "fetch [selector]",
		Short: "Fetch every repository, concurrently",
		Long:  "Updates remote-tracking refs so divergence numbers are accurate. Modifies no working tree.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, found, err := resolveAndFind(firstArg(args))
			if err != nil {
				return err
			}
			jobs := fetchJobs(flagJobs)
			gitArgs := []string{"fetch", "--quiet"}
			if prune {
				gitArgs = append(gitArgs, "--prune")
			}

			// errs is written one slot per goroutine (indexed by i, never
			// shared), so it needs no lock. markPartialFailure is called only
			// after wg.Wait() below, on this, the main goroutine: it and the
			// package-level exit code it sets are not safe to touch from a
			// worker.
			errs := make([]error, len(found))
			sem := make(chan struct{}, jobs)
			var wg sync.WaitGroup
			for i, f := range found {
				wg.Add(1)
				go func(i int, dir string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					_, errs[i] = git.Run(cmd.Context(), dir, gitArgs...)
				}(i, f.AbsPath)
			}
			wg.Wait()

			out := cmd.OutOrStdout()
			ok := 0
			for i, e := range errs {
				if e != nil {
					markPartialFailure()
					// Diagnostics to stderr, the result to stdout, so
					// `grove fetch 2>/dev/null` sees only the summary line.
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", found[i].RelPath, e)
					continue
				}
				ok++
			}
			fmt.Fprintf(out, "fetched %d of %d\n", ok, len(found))
			return nil
		},
	}
	c.Flags().BoolVar(&prune, "prune", false, "remove remote-tracking refs that no longer exist")
	return c
}

// fetchJobs decides the concurrency cap: --jobs when positive, DefaultJobs()
// otherwise. It is a separate function, not an inline literal, because
// concurrency level is not something a black-box test can observe from
// output alone (a mutation ignoring --jobs entirely was found to survive the
// whole suite) — pulling the decision out lets it be checked directly,
// mirroring collectOpts's role for the same problem in status/branch.
func fetchJobs(flagJobs int) int {
	if flagJobs > 0 {
		return flagJobs
	}
	return git.DefaultJobs()
}
