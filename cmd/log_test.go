package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// commitAs makes an empty commit with an explicit author and an explicit,
// distinct timestamp. testutil.Run cannot do this — it fixes the author and
// leaves the timestamp to the wall clock — and two commits created within the
// same wall-clock second tie on git's %ct, which would make the ordering
// assertions below depend on input order rather than on the sort actually
// being tested.
func commitAs(t *testing.T, dir, msg, author string, when time.Time) {
	t.Helper()
	ts := when.Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", msg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+author, "GIT_AUTHOR_EMAIL="+author+"@example.com",
		"GIT_COMMITTER_NAME="+author, "GIT_COMMITTER_EMAIL="+author+"@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_DATE="+ts, "GIT_COMMITTER_DATE="+ts,
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
// gateway, dashboard, oldest to newest — so a correct global sort produces an
// order that alternates between repositories. A mutation that concatenates
// per-repo results instead of truly merging them, or that sorts ascending
// instead of descending, produces a visibly different sequence rather than
// one that merely looks plausible.
func logWorkspace(t *testing.T) (root, gateway, dashboard string) {
	t.Helper()
	root = t.TempDir()
	gateway = testutil.NewRepo(t, filepath.Join(root, "api", "gateway"))
	dashboard = testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"))
	commitAs(t, gateway, "gateway alpha", "alice", t10)
	commitAs(t, dashboard, "dashboard bravo", "bob", t11)
	commitAs(t, gateway, "gateway charlie", "alice", t12)
	commitAs(t, dashboard, "dashboard delta", "bob", t13)
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

// TestLogUnbornRepoIsPartialFailure covers the git.Run error path inside the
// per-repo loop: a freshly initialised repo with no commits yet makes `git
// log` fail. That must downgrade the exit code (it is a real failure to
// produce a log for that repository) without stopping the rest of the run or
// losing the other repository's commits.
func TestLogUnbornRepoIsPartialFailure(t *testing.T) {
	root := t.TempDir()
	healthy := testutil.NewRepo(t, filepath.Join(root, "api", "gateway"))
	commitAs(t, healthy, "only commit", "alice", t10)
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard")) // unborn: no commits
	isolate(t)

	out, code := run(t, "log", "--root", root, "-n", "10")
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitPartial, out)
	}
	if !strings.Contains(out, "only commit") {
		t.Errorf("the healthy repo's commit should still be reported\n%s", out)
	}
}

func TestLogUnknownSelectorExitsError(t *testing.T) {
	root, _, _ := logWorkspace(t)
	out, code := run(t, "log", "--root", root, "nosuchrepo")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, out)
	}
}
