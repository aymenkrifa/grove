package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/aymenkrifa/grove/internal/discover"
)

// CollectOpts tunes a collection run.
type CollectOpts struct {
	Jobs  int  // 0 means DefaultJobs()
	Stash bool // count stash entries (one extra git call per repo)
}

// DefaultJobs bounds concurrency so a workspace of hundreds of repos does not
// spawn hundreds of git processes at once.
func DefaultJobs() int { return jobsFor(runtime.NumCPU()) }

// jobsFor holds DefaultJobs's arithmetic away from the machine it runs on, so
// the rule can be checked at CPU counts this host does not have. Asking the
// host how many CPUs it has and then asserting the same calculation back is
// not a test of anything.
//
// Twice the CPU count, because a git process spends most of its life waiting
// on the disk rather than computing. Capped, because the pool exists to stay
// well clear of the process and descriptor limits. Floored at one, because a
// pool of zero workers never finishes: runtime.NumCPU returns 1 on a system
// whose CPU count cannot be read today, but that is an implementation detail
// rather than a documented promise, and this line costs nothing.
func jobsFor(ncpu int) int {
	n := ncpu * 2
	if n > 16 {
		n = 16
	}
	if n < 1 {
		n = 1
	}
	return n
}

// Available reports whether the git binary can be found, so a caller can fail
// once with a clear message instead of once per repository. Without it a
// missing git is discovered separately by every worker, and a workspace of
// twenty repositories answers a single mistake twenty times.
func Available() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git was not found on PATH: grove runs the git binary, "+
			"so install git and make sure it is on your PATH (%v)", err)
	}
	return nil
}

// Run executes git in dir and returns its stdout. Arguments are passed as a
// slice: no shell is involved, so paths with spaces or quotes are safe without
// any escaping, and nothing a repository is named can turn into a command.
//
// A failure carries git's own first line of stderr rather than "exit status
// 128", because that line is what ends up in the user's table.
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s", firstLine(msg))
	}
	return stdout.String(), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Collect gathers the state of every repo, concurrently, returning results in
// the same order as the input. A failing repo yields a Repo carrying Error:
// one unreadable repository costs its own row, never the run.
func Collect(ctx context.Context, found []discover.Found, opts CollectOpts) []Repo {
	jobs := opts.Jobs
	if jobs <= 0 {
		jobs = DefaultJobs()
	}
	out := make([]Repo, len(found))
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	for i, f := range found {
		wg.Add(1)
		go func(i int, f discover.Found) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// Each worker owns one slot, so the results need no lock and no
			// sort: writing by index is what keeps the output in input order
			// rather than in the order the git processes happened to finish.
			out[i] = collectOne(ctx, f, opts)
		}(i, f)
	}
	wg.Wait()
	return out
}

func collectOne(ctx context.Context, f discover.Found, opts CollectOpts) Repo {
	r := Repo{Path: f.RelPath, AbsPath: f.AbsPath, Group: f.Group}

	// --no-optional-locks keeps a read-only report read-only: without it git
	// may refresh and rewrite the index of a repository grove is only looking
	// at, which is a poor way to repay a tool that fans out over a whole
	// workspace.
	out, err := Run(ctx, f.AbsPath, "--no-optional-locks",
		"status", "--porcelain=v2", "--branch", "--untracked-files=normal", "-z")
	if err != nil {
		// A bare repository has no work tree, so status cannot run in it at
		// all. That is a state to report, not a failure, and the only way to
		// tell the two apart is to ask.
		if bare, berr := Run(ctx, f.AbsPath, "rev-parse", "--is-bare-repository"); berr == nil &&
			strings.TrimSpace(bare) == "true" {
			r.Bare = true
			if head, herr := Run(ctx, f.AbsPath, "symbolic-ref", "--short", "HEAD"); herr == nil {
				r.Branch = strings.TrimSpace(head)
			}
			r.Clean = true
			return r
		}
		r.Error = err.Error()
		return r
	}
	if perr := ParseStatus([]byte(out), &r); perr != nil {
		r.Error = perr.Error()
		return r
	}
	if r.Detached {
		// Porcelain v2 will not name the commit, so this is the second call
		// that fills Branch for a detached HEAD.
		if sha, serr := Run(ctx, f.AbsPath, "rev-parse", "--short=7", "HEAD"); serr == nil {
			r.Branch = strings.TrimSpace(sha)
		}
	}
	if opts.Stash {
		r.Stashes = stashCount(ctx, f.AbsPath)
	}
	return r
}

// stashCount reads the stash reflog. No stash means no ref, which is not an
// error: the repository simply has nothing stashed.
func stashCount(ctx context.Context, dir string) int {
	out, err := Run(ctx, dir, "rev-list", "--walk-reflogs", "--count", "refs/stash")
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0
	}
	return n
}
