package cmd

import (
	"encoding/json"
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
		Root  string `json:"root"`
		Repos []struct {
			Path    string `json:"path"`
			AbsPath string `json:"abs_path"`
			Group   string `json:"group"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
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
	if !strings.HasSuffix(doc.Repos[0].AbsPath, "api/gateway") {
		t.Errorf("repo[0].AbsPath = %q, should end in api/gateway", doc.Repos[0].AbsPath)
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
