package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// run executes the root command with args and returns stdout and the exit code.
func run(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var buf bytes.Buffer
	code := ExecuteWith(&buf, &buf, args)
	return buf.String(), code
}

func workspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit(), testutil.Dirty())
	testutil.NewRepo(t, filepath.Join(root, "web", "dashboard"), testutil.WithCommit())
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	return root
}

func TestStatusRendersEveryRepo(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "status", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, out)
	}
	for _, want := range []string{"api", "gateway", "web", "dashboard", "2 repos"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestStatusJSON(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "status", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	var doc struct {
		Repos []struct {
			Path  string `json:"path"`
			Clean bool   `json:"clean"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(doc.Repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(doc.Repos))
	}
	if doc.Repos[0].Path != "api/gateway" || doc.Repos[0].Clean {
		t.Errorf("repo[0] = %+v, want api/gateway dirty", doc.Repos[0])
	}
}

func TestStatusSelector(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "status", "--root", root, "web")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "gateway") {
		t.Errorf("selector 'web' should exclude api/gateway\n%s", out)
	}
	if !strings.Contains(out, "dashboard") {
		t.Errorf("selector 'web' should include web/dashboard\n%s", out)
	}
}

func TestStatusDirtyOnly(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "status", "--root", root, "-d")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "dashboard") {
		t.Errorf("-d should hide the clean repo\n%s", out)
	}
}

func TestStatusUnknownSelectorExitsOne(t *testing.T) {
	root := workspace(t)
	out, code := run(t, "status", "--root", root, "nosuchrepo")
	if code != ExitError {
		t.Errorf("exit = %d, want %d\n%s", code, ExitError, out)
	}
}
