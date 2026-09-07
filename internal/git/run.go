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
func DefaultJobs() int {
	n := runtime.NumCPU() * 2
	if n > 16 {
		n = 16
	}
	if n < 1 {
		n = 1
	}
	return n
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

	out, err := Run(ctx, f.AbsPath, "status", "--porcelain=v2", "--branch", "--untracked-files=normal", "-z")
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
