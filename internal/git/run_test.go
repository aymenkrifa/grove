package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/discover"
	"github.com/aymenkrifa/grove/internal/testutil"
)

// walk is discover.Walk with the warnings dropped: no test here builds an
// unreadable directory, so a warning would mean the fixture is wrong.
func walk(t *testing.T, root string) []discover.Found {
	t.Helper()
	found, warnings, err := discover.Walk(root, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected walk warnings: %v", warnings)
	}
	return found
}

func removeAllAndWrite(path, body string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func TestCollectFillsEveryRepoAndPreservesOrder(t *testing.T) {
	root := t.TempDir()
	api := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit(), testutil.Dirty())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())

	got := Collect(context.Background(), walk(t, root), CollectOpts{Jobs: 4, Stash: true})

	if len(got) != 2 {
		t.Fatalf("got %d repos, want 2", len(got))
	}
	if got[0].Path != "api/gateway" || got[1].Path != "web/dashboard" {
		t.Errorf("order = %q,%q — results must follow the input, not arrival", got[0].Path, got[1].Path)
	}
	if got[0].AbsPath != api {
		t.Errorf("AbsPath = %q, want %q", got[0].AbsPath, api)
	}
	if !got[0].Dirty() {
		t.Error("api/gateway should be dirty")
	}
	if got[1].Dirty() {
		t.Error("web/dashboard should be clean")
	}
	if got[0].Group != "api" {
		t.Errorf("Group = %q, want api", got[0].Group)
	}
	if got[1].Branch != "main" {
		t.Errorf("Branch = %q, want main", got[1].Branch)
	}
	if got[0].Error != "" || got[1].Error != "" {
		t.Errorf("unexpected errors: %q %q", got[0].Error, got[1].Error)
	}
}

// TestCollectKeepsResultsInInputOrder feeds deliberately unsorted input to a
// pool wide enough that the goroutines finish in an unpredictable order. Two
// repositories cannot tell "in input order" from "in arrival order" — half the
// time arrival order is input order by luck — so this uses ten.
func TestCollectKeepsResultsInInputOrder(t *testing.T) {
	root := t.TempDir()
	var found []discover.Found
	for i := 9; i >= 0; i-- { // reverse of sorted order, on purpose
		name := fmt.Sprintf("repo%02d", i)
		opts := []testutil.RepoOpt{testutil.WithCommit()}
		if i%2 == 0 {
			opts = append(opts, testutil.Dirty())
		}
		dir := testutil.NewRepo(t, filepath.Join(root, name), opts...)
		found = append(found, discover.Found{AbsPath: dir, RelPath: name})
	}

	got := Collect(context.Background(), found, CollectOpts{Jobs: 8})

	if len(got) != len(found) {
		t.Fatalf("got %d repos, want %d", len(got), len(found))
	}
	for i, f := range found {
		if got[i].Path != f.RelPath {
			t.Fatalf("got[%d].Path = %q, want %q — results are not in input order", i, got[i].Path, f.RelPath)
		}
		wantDirty := strings.HasSuffix(f.RelPath, "0") || strings.HasSuffix(f.RelPath, "2") ||
			strings.HasSuffix(f.RelPath, "4") || strings.HasSuffix(f.RelPath, "6") ||
			strings.HasSuffix(f.RelPath, "8")
		if got[i].Dirty() != wantDirty {
			t.Errorf("got[%d] (%s) Dirty = %v, want %v", i, f.RelPath, got[i].Dirty(), wantDirty)
		}
	}
}

func TestCollectDetachedHeadGetsShortSHA(t *testing.T) {
	root := t.TempDir()
	dir := testutil.NewRepo(t, filepath.Join(root, "r"), testutil.WithCommit())
	sha := strings.TrimSpace(testutil.Run(t, dir, "rev-parse", "HEAD"))
	testutil.Run(t, dir, "checkout", "-q", sha)

	got := Collect(context.Background(), walk(t, root), CollectOpts{Jobs: 1})

	if !got[0].Detached {
		t.Fatal("Detached = false, want true")
	}
	if got[0].Branch != sha[:7] {
		t.Errorf("Branch = %q, want short SHA %q", got[0].Branch, sha[:7])
	}
}

// TestCollectAttachedHeadKeepsItsBranchName guards the other side of the
// detached branch: the extra rev-parse must not run for an ordinary checkout.
func TestCollectAttachedHeadKeepsItsBranchName(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "r"), testutil.WithCommit(), testutil.WithBranch("feature/x"))

	got := Collect(context.Background(), walk(t, root), CollectOpts{Jobs: 1})

	if got[0].Detached {
		t.Error("Detached = true, want false")
	}
	if got[0].Branch != "feature/x" {
		t.Errorf("Branch = %q, want feature/x", got[0].Branch)
	}
}

func TestCollectBareRepoIsMarkedNotFailed(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "mirror"), testutil.Bare())

	got := Collect(context.Background(), walk(t, root), CollectOpts{Jobs: 1})

	if len(got) != 1 {
		t.Fatalf("got %d repos, want 1", len(got))
	}
	if !got[0].Bare {
		t.Errorf("Bare = false, want true (got error %q)", got[0].Error)
	}
	if got[0].Error != "" {
		t.Errorf("a bare repo should not be an error, got %q", got[0].Error)
	}
	if !got[0].Clean {
		t.Error("Clean = false, want true: a bare repo has no working tree to be dirty")
	}
	// The branch name comes from HEAD rather than from status, which cannot run.
	if got[0].Branch == "" {
		t.Error("Branch = empty, want the branch HEAD points at")
	}
	if got[0].Path != "mirror" {
		t.Errorf("Path = %q, want mirror", got[0].Path)
	}
}

func TestCollectCountsStashes(t *testing.T) {
	root := t.TempDir()
	dir := testutil.NewRepo(t, filepath.Join(root, "r"), testutil.WithCommit(), testutil.Dirty())
	testutil.Run(t, dir, "stash", "push", "-q", "-m", "wip")

	got := Collect(context.Background(), walk(t, root), CollectOpts{Jobs: 1, Stash: true})

	if got[0].Stashes != 1 {
		t.Errorf("Stashes = %d, want 1", got[0].Stashes)
	}
}

// TestCollectStashCounting pins the three answers stash counting has to give:
// the real number, zero when nothing is stashed (no refs/stash is not a
// failure), and zero when the caller did not ask, because each stash costs an
// extra git process per repository.
func TestCollectStashCounting(t *testing.T) {
	root := t.TempDir()
	dir := testutil.NewRepo(t, filepath.Join(root, "r"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "none"), testutil.WithCommit())
	for _, body := range []string{"one\n", "two\n"} {
		writeFile(t, filepath.Join(dir, "README.md"), body)
		testutil.Run(t, dir, "stash", "push", "-q", "-m", "wip")
	}
	found := walk(t, root)

	asked := Collect(context.Background(), found, CollectOpts{Jobs: 2, Stash: true})
	byPath := map[string]Repo{}
	for _, r := range asked {
		byPath[r.Path] = r
	}
	if got := byPath["r"].Stashes; got != 2 {
		t.Errorf("Stashes = %d, want 2", got)
	}
	if got := byPath["none"].Stashes; got != 0 {
		t.Errorf("Stashes = %d, want 0 for a repo that has never stashed", got)
	}
	if byPath["none"].Error != "" {
		t.Errorf("a missing refs/stash is not an error, got %q", byPath["none"].Error)
	}

	unasked := Collect(context.Background(), found, CollectOpts{Jobs: 2})
	for _, r := range unasked {
		if r.Stashes != 0 {
			t.Errorf("%s: Stashes = %d with Stash unset, want 0", r.Path, r.Stashes)
		}
	}
}

func TestCollectBrokenRepoGetsAnErrorNotAPanic(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "broken")
	testutil.NewRepo(t, dir, testutil.WithCommit())
	// Corrupt it: a .git that is a file pointing nowhere.
	if err := removeAllAndWrite(filepath.Join(dir, ".git"), "gitdir: /nonexistent\n"); err != nil {
		t.Fatal(err)
	}

	got := Collect(context.Background(), walk(t, root), CollectOpts{Jobs: 1})

	if len(got) != 1 {
		t.Fatalf("got %d repos, want 1", len(got))
	}
	if got[0].Error == "" {
		t.Fatal("a broken repo should carry an error string")
	}
	if strings.Contains(got[0].Error, "\n") {
		t.Errorf("Error = %q, want a single line fit for a table cell", got[0].Error)
	}
	if strings.HasPrefix(got[0].Error, "exit status") {
		t.Errorf("Error = %q, want git's own message rather than the exit code", got[0].Error)
	}
	if got[0].Bare {
		t.Error("Bare = true for a broken repo, want false")
	}
	// A failure still identifies which repository failed.
	if got[0].Path != "broken" || got[0].AbsPath != dir {
		t.Errorf("Path/AbsPath = %q/%q, want broken/%s", got[0].Path, got[0].AbsPath, dir)
	}
}

// TestCollectSurvivesOneBrokenRepoAmongGoodOnes is the run-level promise: one
// bad repository costs its own row, not the run.
func TestCollectSurvivesOneBrokenRepoAmongGoodOnes(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "a-good"), testutil.WithCommit())
	broken := testutil.NewRepo(t, filepath.Join(root, "b-broken"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "c-good"), testutil.WithCommit(), testutil.Untracked())
	if err := removeAllAndWrite(filepath.Join(broken, ".git"), "gitdir: /nonexistent\n"); err != nil {
		t.Fatal(err)
	}

	got := Collect(context.Background(), walk(t, root), CollectOpts{Jobs: 3})

	if len(got) != 3 {
		t.Fatalf("got %d repos, want 3", len(got))
	}
	if got[0].Error != "" || got[2].Error != "" {
		t.Errorf("healthy repos carried errors: %q %q", got[0].Error, got[2].Error)
	}
	if got[1].Error == "" {
		t.Error("the broken repo carried no error")
	}
	if got[0].Branch != "main" || got[2].Untracked != 1 {
		t.Errorf("healthy repos were not collected: %+v %+v", got[0], got[2])
	}
}

func TestCollectEmptyInput(t *testing.T) {
	got := Collect(context.Background(), nil, CollectOpts{})
	if len(got) != 0 {
		t.Errorf("got %d repos, want 0", len(got))
	}
}

// TestCollectWithZeroJobsUsesTheDefault also proves the pool cannot be built
// with a zero-width semaphore, which would block every worker forever.
func TestCollectWithZeroJobsUsesTheDefault(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "one"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "two"), testutil.WithCommit(), testutil.Untracked())

	got := Collect(context.Background(), walk(t, root), CollectOpts{}) // Jobs unset

	if len(got) != 2 {
		t.Fatalf("got %d repos, want 2", len(got))
	}
	if got[0].Branch != "main" || got[1].Untracked != 1 {
		t.Errorf("collection with default jobs produced %+v %+v", got[0], got[1])
	}
}

func TestCollectCancelledContextFailsEveryRepoWithoutPanicking(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "a"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "b"), testutil.WithCommit())
	found := walk(t, root)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := Collect(ctx, found, CollectOpts{Jobs: 2, Stash: true})

	if len(got) != 2 {
		t.Fatalf("got %d repos, want 2", len(got))
	}
	for _, r := range got {
		if r.Error == "" {
			t.Errorf("%s: Error = empty, want the cancellation reported", r.Path)
		}
	}
}

// TestCollectHandlesPathsWithSpacesAndQuotes is the standing proof that no
// shell is involved: a repository whose directory and untracked file both
// carry spaces and quotes must come back as an ordinary result.
func TestCollectHandlesPathsWithSpacesAndQuotes(t *testing.T) {
	root := t.TempDir()
	dir := testutil.NewRepo(t, filepath.Join(root, `we ird "repo" name`), testutil.WithCommit())
	writeFile(t, filepath.Join(dir, `a "quoted" file.txt`), "x\n")

	got := Collect(context.Background(), walk(t, root), CollectOpts{Jobs: 1, Stash: true})

	if len(got) != 1 {
		t.Fatalf("got %d repos, want 1", len(got))
	}
	if got[0].Error != "" {
		t.Fatalf("Error = %q, want none", got[0].Error)
	}
	if got[0].Untracked != 1 {
		t.Errorf("Untracked = %d, want 1", got[0].Untracked)
	}
}

// TestCollectBoundsConcurrency puts a recording stand-in for git on PATH and
// reconstructs how many ran at once. Without it the pool's whole purpose —
// not spawning one git process per repository in a workspace of hundreds — is
// invisible to every other test in this file.
func TestCollectBoundsConcurrency(t *testing.T) {
	const (
		repos = 12
		jobs  = 3
	)
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls.log")
	script := "#!/bin/sh\n" +
		"printf 'start %s\\n' \"$(date +%s%N)\" >> \"" + log + "\"\n" +
		"sleep 0.05\n" +
		"printf 'end %s\\n' \"$(date +%s%N)\" >> \"" + log + "\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	var found []discover.Found
	for i := range repos {
		name := fmt.Sprintf("r%02d", i)
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		found = append(found, discover.Found{AbsPath: dir, RelPath: name})
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	Collect(context.Background(), found, CollectOpts{Jobs: jobs})

	if peak := peakOverlap(t, log); peak > jobs {
		t.Errorf("peak concurrent git processes = %d, want at most Jobs = %d", peak, jobs)
	} else if peak < 2 {
		t.Errorf("peak concurrent git processes = %d — the fixture never overlapped, so it proves nothing", peak)
	}
}

// peakOverlap replays the stand-in's start/end stamps and reports the largest
// number of processes alive at once.
func peakOverlap(t *testing.T, log string) int {
	t.Helper()
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("reading %s: %v", log, err)
	}
	type event struct {
		at    int64
		delta int
	}
	var events []event
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		kind, stamp, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("unreadable log line %q", line)
		}
		at, err := strconv.ParseInt(stamp, 10, 64)
		if err != nil {
			t.Fatalf("unreadable timestamp in %q: %v", line, err)
		}
		delta := 1
		if kind == "end" {
			delta = -1
		}
		events = append(events, event{at, delta})
	}
	if len(events) == 0 {
		t.Fatal("the stand-in git was never called")
	}
	// Ends sort before starts at the same instant, so a process that has just
	// finished is not counted alongside the one replacing it.
	sort.Slice(events, func(i, j int) bool {
		if events[i].at != events[j].at {
			return events[i].at < events[j].at
		}
		return events[i].delta < events[j].delta
	})
	live, peak := 0, 0
	for _, e := range events {
		live += e.delta
		if live > peak {
			peak = live
		}
	}
	return peak
}

func TestDefaultJobs(t *testing.T) {
	n := DefaultJobs()
	if n < 1 {
		t.Errorf("DefaultJobs() = %d, want at least 1", n)
	}
	if n > 16 {
		t.Errorf("DefaultJobs() = %d, want at most 16 — the cap is what keeps a big "+
			"workspace from spawning a git process per repo", n)
	}
	if want := min(runtime.NumCPU()*2, 16); n != want {
		t.Errorf("DefaultJobs() = %d, want %d on a %d-CPU machine", n, want, runtime.NumCPU())
	}
}

func TestRunReturnsStdoutFromTheRepoItWasGiven(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	want := strings.TrimSpace(testutil.Run(t, dir, "rev-parse", "HEAD"))

	out, err := Run(context.Background(), dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.TrimSpace(out) != want {
		t.Errorf("Run() = %q, want %q — git ran somewhere other than dir", strings.TrimSpace(out), want)
	}
}

// TestRunPassesArgumentsWithoutAShell hands git an argument that no shell
// would survive intact.
func TestRunPassesArgumentsWithoutAShell(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	name := `a "quoted" file with spaces.txt`
	writeFile(t, filepath.Join(dir, name), "x\n")
	testutil.Run(t, dir, "add", "-A")

	out, err := Run(context.Background(), dir, "ls-files", "-z", "--", name)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := strings.TrimRight(out, "\x00"); got != name {
		t.Errorf("Run() = %q, want %q", got, name)
	}
}

func TestRunReportsGitsOwnMessageOnOneLine(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())

	// A remote that does not exist: git fails with several lines of advice.
	_, err := Run(context.Background(), dir, "fetch", filepath.Join(dir, "no-such-remote"))
	if err == nil {
		t.Fatal("Run() error = nil, want a failure")
	}
	msg := err.Error()
	if strings.Contains(msg, "\n") {
		t.Errorf("error = %q, want a single line", msg)
	}
	if msg == "" || strings.HasPrefix(msg, "exit status") {
		t.Errorf("error = %q, want git's own words rather than the exit code", msg)
	}
}

func TestRunReturnsAnErrorForAnUnknownCommand(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	if _, err := Run(context.Background(), dir, "no-such-subcommand"); err == nil {
		t.Error("Run() error = nil, want a failure")
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"single line", "fatal: nope", "fatal: nope"},
		{"first of many", "fatal: nope\nhint: try harder\n", "fatal: nope"},
		{"trailing newline only", "fatal: nope\n", "fatal: nope"},
		{"leading newline", "\nfatal: nope", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstLine(tt.in); got != tt.want {
				t.Errorf("firstLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
