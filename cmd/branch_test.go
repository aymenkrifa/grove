package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// branchWorkspace has two repos on "main" and one on "feature/login", so the
// grouping and the alphabetical ordering of the group headings ("feature/..."
// sorts before "main") are both exercised, and a repo landing under the wrong
// branch heading is visible.
func branchWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "auth"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"),
		testutil.WithCommit(), testutil.WithBranch("feature/login"))
	isolate(t)
	return root
}

func nonEmptyLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestBranchGroupsAndSortsHeadings(t *testing.T) {
	root := branchWorkspace(t)
	out, code := run(t, "branch", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	lines := nonEmptyLines(out)
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3\n%s", len(lines), out)
	}
	// feature/login sorts before main.
	if f := strings.Fields(lines[0]); len(f) != 2 || f[0] != "feature/login" || f[1] != "web/dashboard" {
		t.Errorf("line 0 = %q, want [feature/login web/dashboard]", lines[0])
	}
	if f := strings.Fields(lines[1]); len(f) != 2 || f[0] != "main" || f[1] != "api/auth" {
		t.Errorf("line 1 = %q, want [main api/auth]", lines[1])
	}
	// The second repo on "main" repeats no label: only the path.
	if f := strings.Fields(lines[2]); len(f) != 1 || f[0] != "api/gateway" {
		t.Errorf("line 2 = %q, want just [api/gateway] (no repeated label)", lines[2])
	}
}

func TestBranchSelectorNarrowsResults(t *testing.T) {
	root := branchWorkspace(t)
	out, code := run(t, "branch", "--root", root, "web")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "main") || strings.Contains(out, "auth") || strings.Contains(out, "gateway") {
		t.Errorf("selector 'web' should exclude the api/* repos\n%s", out)
	}
	if !strings.Contains(out, "feature/login") || !strings.Contains(out, "web/dashboard") {
		t.Errorf("selector 'web' should include web/dashboard on feature/login\n%s", out)
	}
}

func TestBranchJSON(t *testing.T) {
	root := branchWorkspace(t)
	out, code := run(t, "branch", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	var doc struct {
		Repos []struct {
			Path   string `json:"path"`
			Branch string `json:"branch"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(doc.Repos) != 3 {
		t.Fatalf("got %d repos, want 3", len(doc.Repos))
	}
	want := map[string]string{"api/auth": "main", "api/gateway": "main", "web/dashboard": "feature/login"}
	for _, r := range doc.Repos {
		if want[r.Path] != r.Branch {
			t.Errorf("repo %s: branch = %q, want %q", r.Path, r.Branch, want[r.Path])
		}
	}
}

// TestBranchErroredRepoGroupsUnderErrorAndExitsPartial covers the "(error)"
// bucket and the partial-failure exit code together: a repo git cannot open
// still gets a row, and its presence downgrades the exit code without
// stopping the rest of the report.
func TestBranchErroredRepoGroupsUnderErrorAndExitsPartial(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	broken := filepath.Join(root, "tools", "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, ".git"),
		[]byte("gitdir: /nonexistent-grove-target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	isolate(t)

	out, code := run(t, "branch", "--root", root)
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitPartial, out)
	}
	found := false
	for _, l := range nonEmptyLines(out) {
		f := strings.Fields(l)
		if len(f) == 2 && f[0] == "(error)" && f[1] == "tools/broken" {
			found = true
		}
	}
	if !found {
		t.Errorf("the broken repo should appear on a line naming (error)\n%s", out)
	}
	if !strings.Contains(out, "api/gateway") {
		t.Errorf("the healthy repo should still be reported\n%s", out)
	}
}

// TestBranchReportsTheErrorItGroupsUnder covers what the "(error)" bucket
// cannot say. The bucket names the repository; git's own message — the part
// that tells the user whether the clone is corrupt, unreadable or something
// else — was collected and then dropped, so `grove branch` handed back exit 2
// and no explanation on either stream. It belongs on stderr, where every other
// command puts per-repo failures and where it stays out of the listing.
func TestBranchReportsTheErrorOnStderr(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())
	broken := filepath.Join(root, "tools", "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, ".git"),
		[]byte("gitdir: /nonexistent-grove-target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	isolate(t)

	stdout, stderr, code := runSplit(t, "branch", "--root", root)
	if code != ExitPartial {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, ExitPartial, stdout, stderr)
	}
	if !strings.Contains(stderr, "tools/broken") {
		t.Errorf("stderr does not name the repository that failed:\n%s", stderr)
	}
	// git's own words, not just the path: "(error)" already carries as much
	// as a bare path would.
	//
	// Assert on the part of the message that is stable across git versions.
	// git 2.43 echoes the missing gitdir ("not a git repository:
	// /nonexistent-grove-target") while newer versions print "(null)" in its
	// place, so asserting on the path passed locally and failed in CI.
	const gitsWords = "not a git repository"
	if !strings.Contains(stderr, gitsWords) {
		t.Errorf("stderr does not carry git's own message, so the user still cannot "+
			"tell what went wrong:\n%s", stderr)
	}
	if strings.Contains(stdout, gitsWords) {
		t.Errorf("the message must not land in the listing:\n%s", stdout)
	}
	if !strings.Contains(stdout, "api/gateway") {
		t.Errorf("the healthy repo should still be listed\n%s", stdout)
	}
}
