package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// commitAs makes an empty commit with an explicit author, an explicit
// committer timestamp, and — separately — an explicit author timestamp.
// testutil.Run cannot do any of this: it fixes the author and leaves both
// timestamps to the wall clock, and two commits created within the same
// wall-clock second tie on git's %ct, which would make the ordering
// assertions below depend on input order rather than on the sort actually
// being tested. Taking the two timestamps separately (rather than one
// "when" reused for both) also lets a fixture pin %ct specifically: the spec
// (§5.4) sorts by committer date, and a fixture where author and committer
// dates always match cannot tell that apart from sorting by author date.
func commitAs(t *testing.T, dir, msg, author string, authorTime, committerTime time.Time) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", msg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+author, "GIT_AUTHOR_EMAIL="+author+"@example.com",
		"GIT_COMMITTER_NAME="+author, "GIT_COMMITTER_EMAIL="+author+"@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_DATE="+authorTime.Format(time.RFC3339),
		"GIT_COMMITTER_DATE="+committerTime.Format(time.RFC3339),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit in %s: %v\n%s", dir, err, out)
	}
}

var (
	t10 = time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t11 = time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	t12 = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t13 = time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)
)

// logWorkspace interleaves two repos' commits in time — gateway, dashboard,
// gateway, dashboard, oldest to newest by COMMITTER date — so a correct
// global sort produces an order that alternates between repositories. A
// mutation that concatenates per-repo results instead of truly merging them,
// or that sorts ascending instead of descending, produces a visibly
// different sequence rather than one that merely looks plausible.
//
// Author dates are deliberately the exact reverse of committer dates (alpha's
// author time is delta's committer time, and so on): if the implementation
// ever sorted by %at instead of the spec's %ct, the observed order would be
// completely inverted rather than coincidentally correct.
func logWorkspace(t *testing.T) (root, gateway, dashboard string) {
	t.Helper()
	root = t.TempDir()
	gateway = testutil.NewRepo(t, filepath.Join(root, "api", "gateway"))
	dashboard = testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"))
	commitAs(t, gateway, "gateway alpha", "alice", t13, t10)
	commitAs(t, dashboard, "dashboard bravo", "bob", t12, t11)
	commitAs(t, gateway, "gateway charlie", "alice", t11, t12)
	commitAs(t, dashboard, "dashboard delta", "bob", t10, t13)
	isolate(t)
	return root, gateway, dashboard
}

func TestLogMergesAndSortsAcrossRepos(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "-n", "10")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d commit lines, want 4\n%s", len(lines), out)
	}
	// Strict, fully-interleaved order: newest first, alternating repos.
	wantSubject := []string{"dashboard delta", "gateway charlie", "dashboard bravo", "gateway alpha"}
	wantRepo := []string{"web/dashboard", "api/gateway", "web/dashboard", "api/gateway"}
	for i, line := range lines {
		if !strings.Contains(line, wantSubject[i]) {
			t.Errorf("line %d = %q, want subject %q", i, line, wantSubject[i])
		}
		if !strings.Contains(line, wantRepo[i]) {
			t.Errorf("line %d = %q, want repo %q", i, line, wantRepo[i])
		}
	}
}

// TestLogLimitTruncatesAfterTheGlobalMerge catches a limit applied only
// per-repository: each repo here has two commits, so -n 2 requested per-repo
// would still surface all four after merging unless the result is also cut
// down globally, to the two newest overall.
func TestLogLimitTruncatesAfterTheGlobalMerge(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "-n", "2")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (global limit, not per-repo)\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "dashboard delta") {
		t.Errorf("line 0 = %q, want the newest commit overall", lines[0])
	}
	if !strings.Contains(lines[1], "gateway charlie") {
		t.Errorf("line 1 = %q, want the second-newest commit overall", lines[1])
	}
}

func TestLogSelectorNarrowsToOneRepo(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "gateway", "-n", "10")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "dashboard") {
		t.Errorf("selector 'gateway' should exclude web/dashboard's commits\n%s", out)
	}
	for _, want := range []string{"gateway alpha", "gateway charlie"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n%s", want, out)
		}
	}
}

// TestLogAuthorFlagForwardedVerbatim confirms --author actually reaches git
// log rather than being parsed and discarded.
func TestLogAuthorFlagForwardedVerbatim(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "--author", "alice", "-n", "10")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "bob") || strings.Contains(out, "dashboard bravo") || strings.Contains(out, "dashboard delta") {
		t.Errorf("--author alice should exclude bob's commits\n%s", out)
	}
	for _, want := range []string{"gateway alpha", "gateway charlie"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n%s", want, out)
		}
	}
}

// TestLogSinceFlagForwardedVerbatim confirms --since actually reaches git
// log. t11:30 falls strictly between "dashboard bravo" (t11) and "gateway
// charlie" (t12), so only the two newest commits should survive.
func TestLogSinceFlagForwardedVerbatim(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "--since", "2024-01-01T11:30:00Z", "-n", "10")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "gateway alpha") || strings.Contains(out, "dashboard bravo") {
		t.Errorf("--since should exclude commits before the cutoff\n%s", out)
	}
	for _, want := range []string{"gateway charlie", "dashboard delta"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n%s", want, out)
		}
	}
}

func TestLogJSON(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "--json", "-n", "10")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	var doc struct {
		Commits []struct {
			Timestamp int64  `json:"timestamp"`
			Repo      string `json:"repo"`
			Hash      string `json:"hash"`
			Author    string `json:"author"`
			Subject   string `json:"subject"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(doc.Commits) != 4 {
		t.Fatalf("got %d commits, want 4", len(doc.Commits))
	}
	first := doc.Commits[0]
	if first.Subject != "dashboard delta" || first.Repo != "web/dashboard" || first.Author != "bob" || first.Hash == "" {
		t.Errorf("commits[0] = %+v, want the newest commit with its own fields, not swapped", first)
	}
	last := doc.Commits[3]
	if last.Subject != "gateway alpha" || last.Repo != "api/gateway" || last.Author != "alice" {
		t.Errorf("commits[3] = %+v, want the oldest commit", last)
	}
	for i := 1; i < len(doc.Commits); i++ {
		if doc.Commits[i].Timestamp > doc.Commits[i-1].Timestamp {
			t.Errorf("commits are not sorted newest-first: %+v then %+v", doc.Commits[i-1], doc.Commits[i])
		}
	}
}

// TestLogAllUnbornWorkspaceIsExitOKAndSilent pins the (coordinator-ruled)
// ruling: `git log` fails identically for an unborn repo (no commits yet) and
// a genuinely broken one, so log.go probes `rev-parse --verify HEAD` to tell
// them apart. An unborn repo is common and not a failure at all — it must
// cost nothing: exit 0, and no output, since there is nothing to log anywhere
// in the workspace.
func TestLogAllUnbornWorkspaceIsExitOKAndSilent(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway")) // unborn: no commits
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"))
	isolate(t)

	out, code := run(t, "log", "--root", root, "-n", "10")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d (an unborn repo is not a failure)\n%s", code, ExitOK, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected no output for an all-unborn workspace, got %q", out)
	}
}

// TestLogBrokenRepoIsPartialFailureAndReported is the other half: a repo
// whose HEAD resolves fine (so it is not unborn) but whose history cannot be
// walked — here, an ancestor commit's object has been deleted — must still
// be reported, on stderr, and must still downgrade the exit code, while the
// healthy repository's commits are unaffected.
func TestLogBrokenRepoIsPartialFailureAndReported(t *testing.T) {
	root := t.TempDir()
	healthy := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"))
	commitAs(t, healthy, "only commit", "alice", t10, t10)

	broken := testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"))
	commitAs(t, broken, "c1", "bob", t10, t10)
	c1 := strings.TrimSpace(testutil.Run(t, broken, "rev-parse", "HEAD"))
	commitAs(t, broken, "c2", "bob", t11, t11)
	objPath := filepath.Join(broken, ".git", "objects", c1[:2], c1[2:])
	if err := os.Remove(objPath); err != nil {
		t.Fatalf("removing %s: %v", objPath, err)
	}
	isolate(t)

	stdout, stderr, code := runSplit(t, "log", "--root", root, "-n", "10")
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, ExitPartial, stdout, stderr)
	}
	if !strings.Contains(stdout, "only commit") {
		t.Errorf("the healthy repo's commit should still be reported\n%s", stdout)
	}
	if !strings.Contains(stderr, "web/dashboard") {
		t.Errorf("the broken repo should be named on stderr\n%s", stderr)
	}
	if strings.Contains(stdout, "web/dashboard") {
		t.Errorf("the broken repo's error should not leak into stdout\n%s", stdout)
	}
}

// TestLogNegativeLimitIsUnlimited is the CRITICAL regression test: `-n -1`
// is valid git meaning "no limit", and all[:limit] with a negative limit
// panics. A Go panic exits the process with status 2, indistinguishable from
// ExitPartial to a script, which is exactly the wrong signal for "grove log
// crashed."
func TestLogNegativeLimitIsUnlimited(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "--max-count", "-1")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d (must not panic)\n%s", code, ExitOK, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want all 4 commits\n%s", len(lines), out)
	}
}

func TestLogNegativeLimitIsUnlimitedJSON(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "--max-count", "-1", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d (must not panic)\n%s", code, ExitOK, out)
	}
	var doc struct {
		Commits []struct{} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(doc.Commits) != 4 {
		t.Errorf("got %d commits, want all 4", len(doc.Commits))
	}
}

// TestLogZeroLimitShowsNothing pins git's own convention for the other edge:
// `-n 0` means zero commits, not "the default".
func TestLogZeroLimitShowsNothing(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "--max-count", "0")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected no output for -n 0, got %q", out)
	}
}

func TestLogZeroLimitShowsNothingJSON(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "--max-count", "0", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, out)
	}
	var doc struct {
		Commits []struct{} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(doc.Commits) != 0 {
		t.Errorf("got %d commits, want 0", len(doc.Commits))
	}
}

// TestLogTiedTimestampsKeepAStableOrder gives four commits, two at each of
// two identical committer timestamps, spread across both repos. A
// sort.Slice (unstable) in place of sort.SliceStable is free to reorder
// commits that compare equal; a stable sort must keep them in the order they
// were appended, which is repo-discovery order (api/gateway before
// web/dashboard) followed by each repo's own git-log order (newest first).
func TestLogTiedTimestampsKeepAStableOrder(t *testing.T) {
	root := t.TempDir()
	gateway := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"))
	dashboard := testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"))
	tie1 := time.Date(2024, 6, 1, 9, 0, 0, 0, time.UTC)
	tie2 := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	commitAs(t, gateway, "gateway tie1", "alice", tie1, tie1)
	commitAs(t, gateway, "gateway tie2", "alice", tie2, tie2)
	commitAs(t, dashboard, "dashboard tie1", "bob", tie1, tie1)
	commitAs(t, dashboard, "dashboard tie2", "bob", tie2, tie2)
	isolate(t)

	out, code := run(t, "log", "--root", root, "-n", "10")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4\n%s", len(lines), out)
	}
	// tie2 (newer) before tie1 (older); within a tie, gateway (discovered
	// first) before dashboard.
	want := []string{"gateway tie2", "dashboard tie2", "gateway tie1", "dashboard tie1"}
	for i, w := range want {
		if !strings.Contains(lines[i], w) {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

// TestLogExcludesMergeCommits justifies --no-merges with a test: a merge
// commit's subject is a generated line that repeats no real change, so it is
// excluded from the merged timeline while its parents' own commits still
// appear.
func TestLogExcludesMergeCommits(t *testing.T) {
	root := t.TempDir()
	repo := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	testutil.Run(t, repo, "checkout", "-qb", "feature")
	testutil.Run(t, repo, "commit", "-q", "--allow-empty", "-m", "feature work")
	testutil.Run(t, repo, "checkout", "-q", "main")
	testutil.Run(t, repo, "commit", "-q", "--allow-empty", "-m", "main work")
	testutil.Run(t, repo, "merge", "-q", "--no-ff", "-m", "merge feature into main", "feature")
	isolate(t)

	out, code := run(t, "log", "--root", root, "-n", "10")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "merge feature into main") {
		t.Errorf("merge commits should be excluded by --no-merges\n%s", out)
	}
	for _, want := range []string{"feature work", "main work", "initial"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing non-merge commit %q\n%s", want, out)
		}
	}
}

// TestLogPlainTextColumnOrder closes a real gap found by mutating past the
// existing tests: every assertion above uses strings.Contains, which does
// not care WHERE a substring appears on the line, so swapping the hash and
// repo columns in the tabwriter format string survived the whole suite. A
// regex anchored to the line's full shape pins the order: hash (hex, no
// slash) first, then repo, then author, then subject.
func TestLogPlainTextColumnOrder(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "-n", "1")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	line := strings.TrimRight(out, "\n")
	re := regexp.MustCompile(`^[0-9a-f]{4,40}\s+web/dashboard\s+bob\s+dashboard delta\s*$`)
	if !re.MatchString(line) {
		t.Errorf("line = %q, want columns in order: hash, repo, author, subject", line)
	}
}

func TestLogUnknownSelectorExitsError(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "nosuchrepo")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, out)
	}
}
