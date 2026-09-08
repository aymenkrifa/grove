package cmd

import (
	"fmt"
	"io"
	"strings"
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
			// done carries one token per finished repository. Workers must not
			// print: they would race on the writer, and the rule everywhere
			// else in grove is that only this goroutine writes diagnostics.
			// Sending a token instead keeps the counter here, where the
			// summary line is already written.
			done := make(chan struct{}, len(found))
			var wg sync.WaitGroup
			for i, f := range found {
				wg.Add(1)
				go func(i int, dir string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					_, errs[i] = git.Run(cmd.Context(), dir, gitArgs...)
					done <- struct{}{}
				}(i, f.AbsPath)
			}
			go func() {
				wg.Wait()
				close(done)
			}()

			prog := newProgress(cmd.ErrOrStderr(), len(found))
			finished := 0
			for range done {
				finished++
				prog.update(finished)
			}
			prog.clear()

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

// progress reports how many repositories have been fetched so far, rewriting
// one line in place.
//
// It writes to stderr and only when stderr is a terminal: a fetch whose output
// is redirected or read by CI should produce the summary line and nothing
// else, and \r into a log file is noise that no one ever wants. That also
// keeps it consistent with every other diagnostic grove emits.
type progress struct {
	w     io.Writer
	total int
	on    bool
	width int
}

func newProgress(w io.Writer, total int) *progress {
	return &progress{w: w, total: total, on: isTerminal(w)}
}

// update rewrites the counter. The carriage return returns to the start of the
// line rather than starting a new one, so the count advances in place.
func (p *progress) update(n int) {
	if !p.on {
		return
	}
	line := fmt.Sprintf("fetching… %d/%d", n, p.total)
	if len(line) > p.width {
		p.width = len(line)
	}
	fmt.Fprintf(p.w, "\r%s", line)
}

// clear wipes the counter so the summary line does not land on top of it.
// Blanking the widest line it ever drew is what makes that reliable: a shorter
// final line would otherwise leave the tail of a longer one behind.
func (p *progress) clear() {
	if !p.on || p.width == 0 {
		return
	}
	fmt.Fprintf(p.w, "\r%s\r", strings.Repeat(" ", p.width))
}
