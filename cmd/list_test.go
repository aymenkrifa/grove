package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestListPrintsRelativePaths(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "list", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	lines := strings.Fields(strings.TrimSpace(out))
	if len(lines) != 2 || lines[0] != "api/gateway" || lines[1] != "web/dashboard" {
		t.Errorf("list output = %q, want the two relative paths", out)
	}
}

func TestListJSON(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "list", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	var doc struct {
		Root      string `json:"root"`
		Workspace string `json:"workspace"`
		Repos     []struct {
			Path    string `json:"path"`
			AbsPath string `json:"abs_path"`
			Group   string `json:"group"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	// Root and Workspace are both plain strings passed into listJSON in that
	// order; a swap of the two call-site arguments is otherwise invisible
	// (workspace() uses --root directly, so Workspace is empty either way,
	// but a swap would still put the root path where "" belongs).
	if doc.Root != root {
		t.Errorf("doc.Root = %q, want %q", doc.Root, root)
	}
	if doc.Workspace != "" {
		t.Errorf("doc.Workspace = %q, want empty (no named workspace here)", doc.Workspace)
	}
	if len(doc.Repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(doc.Repos))
	}
	// Field values, not just the count: a swap between Path and Group (both
	// strings, both plausible-looking) would pass a count-only assertion.
	if doc.Repos[0].Path != "api/gateway" {
		t.Errorf("repo[0].Path = %q, want api/gateway", doc.Repos[0].Path)
	}
	if doc.Repos[0].Group != "api" {
		t.Errorf("repo[0].Group = %q, want api", doc.Repos[0].Group)
	}
	if doc.Repos[1].Path != "web/dashboard" || doc.Repos[1].Group != "web" {
		t.Errorf("repo[1] = %+v, want web/dashboard in group web", doc.Repos[1])
	}
	// HasSuffix(AbsPath, "api/gateway") would also be satisfied by AbsPath
	// having been set to RelPath itself ("api/gateway" ends in "api/gateway"
	// trivially), so pin absoluteness explicitly and check the exact value.
	wantAbs := filepath.Join(root, "api", "gateway")
	if !filepath.IsAbs(doc.Repos[0].AbsPath) {
		t.Errorf("repo[0].AbsPath = %q, want an absolute path", doc.Repos[0].AbsPath)
	}
	if doc.Repos[0].AbsPath != wantAbs {
		t.Errorf("repo[0].AbsPath = %q, want %q", doc.Repos[0].AbsPath, wantAbs)
	}
}

// TestListJSONIsIndented closes a real gap found by mutating past the
// existing tests: json.Unmarshal does not care about formatting, so removing
// enc.SetIndent("", "  ") from listJSON survived every other test here.
func TestListJSONIsIndented(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "list", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.HasSuffix(out, "}\n") {
		t.Errorf("output should end with a newline: %q", out[max(0, len(out)-10):])
	}
	if !strings.Contains(out, "\n  \"root\": ") {
		t.Errorf("output should be indented by two spaces:\n%s", out)
	}
}

// TestListJSONOmitsUncomputedStatusFields pins that list's JSON output only
// ever publishes what list actually knows (path, abs_path, group). list
// never runs git, so a "clean", "branch", "ahead"/"behind" or "error" field —
// or a status "summary" — would necessarily be a fabricated zero value, on a
// compatibility surface the spec says keeps its meaning.
func TestListJSONOmitsUncomputedStatusFields(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "list", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	var generic map[string]any
	if err := json.Unmarshal([]byte(out), &generic); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if _, ok := generic["summary"]; ok {
		t.Errorf("list --json should not publish a status summary it never computed: %v", generic)
	}
	repos, ok := generic["repos"].([]any)
	if !ok || len(repos) == 0 {
		t.Fatalf("expected a non-empty repos array, got %v", generic["repos"])
	}
	for _, r := range repos {
		entry, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("repo entry is not an object: %v", r)
		}
		for _, forbidden := range []string{"clean", "branch", "ahead", "behind", "staged", "unstaged", "untracked", "conflicted", "stashes", "error", "detached", "unborn", "bare", "upstream"} {
			if _, present := entry[forbidden]; present {
				t.Errorf("list --json repo entry should not publish uncomputed field %q: %v", forbidden, entry)
			}
		}
	}
}

// TestListSelectorNarrowsResults pins that list actually forwards its
// selector argument to resolveAndFind rather than always passing "". Both
// repo names are asymmetric (gateway vs dashboard), so a mutation that
// ignores the selector, or one that inverts the filter, is equally visible.
func TestListSelectorNarrowsResults(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "list", "--root", root, "web")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "gateway") {
		t.Errorf("selector 'web' should exclude api/gateway\n%s", out)
	}
	lines := strings.Fields(strings.TrimSpace(out))
	if len(lines) != 1 || lines[0] != "web/dashboard" {
		t.Errorf("list web output = %q, want just web/dashboard", out)
	}
}

func TestListUnknownSelectorExitsError(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "list", "--root", root, "nosuchrepo")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, out)
	}
}
