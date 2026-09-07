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

// noOptionalLocks is prefixed to every git invocation grove makes. It sets
// GIT_OPTIONAL_LOCKS=0 for that one process, which asks git to skip the
// sub-operations that would need to take a lock — above all, refreshing and
// rewriting the index of a repository grove is only looking at.
//
// It lives here, in the single function every git call goes through, rather
// than at the call sites. For most of this project's life it was spelled out
// on the status call alone and forgotten by diff and log, which is what a
// per-call-site rule reliably becomes: the next command forgets it too.
// `fetch` carries it as well, harmlessly — the ref updates fetch exists to
// perform are not optional operations, so the flag does not change its work.
//
// What it actually buys, measured on git 2.43.0 against a repository with one
// tracked file whose mtime was backdated before every run, so the index's
// cached stat data was stale each time and git had something to refresh:
//
//	git status --porcelain=v2 ...          20/20 runs rewrote .git/index
//	git --no-optional-locks status ...      0/20
//	git --no-optional-locks log ...         0/20   (log never reads the index)
//	git --no-optional-locks diff --stat    20/20   (as does plain git diff)
//
// So it makes status genuinely read-only and costs nothing anywhere else — but
// it does NOT make `grove diff` read-only on this git: builtin/diff.c refreshes
// the index without consulting use_optional_locks(), so the flag is ignored
// there. Leaving the index alone in diff would mean pointing GIT_INDEX_FILE at
// a throwaway copy per repository — an extra git call and a file copy on every
// repo, and the user's own index left stale for their next command. That is a
// trade for the author to make deliberately, not one to smuggle in here.
// TestReadOnlyCommandsLeaveTheIndexAlone pins the cases that do hold.
const noOptionalLocks = "--no-optional-locks"

// Run executes git in dir and returns its stdout. Arguments are passed as a
// slice: no shell is involved, so paths with spaces or quotes are safe without
// any escaping, and nothing a repository is named can turn into a command.
//
// Every call is prefixed with noOptionalLocks; see its comment for why that
// belongs here rather than at each call site.
//
// A failure carries git's own first line of stderr rather than "exit status
// 128", because that line is what ends up in the user's table.
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{noOptionalLocks}, args...)...)
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

	// Run supplies --no-optional-locks, so this status call cannot refresh and
	// rewrite the index of a repository grove is only looking at — a poor way
	// to repay a tool that fans out over a whole workspace.
	out, err := Run(ctx, f.AbsPath,
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
