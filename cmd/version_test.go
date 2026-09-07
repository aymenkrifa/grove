package cmd

import (
	"strings"
	"testing"
)

func TestVersionPrintsSomething(t *testing.T) {
	out, code := run(t, "version")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("version output is empty")
	}
}

// TestResolveVersionPrefersTheLdflagsOverride pins the precedence
// resolveVersion documents: a release build's -ldflags value must win over
// whatever runtime/debug.ReadBuildInfo reports, not the other way around.
func TestResolveVersionPrefersTheLdflagsOverride(t *testing.T) {
	old := version
	version = "v1.2.3"
	t.Cleanup(func() { version = old })
	if got := resolveVersion(); got != "v1.2.3" {
		t.Errorf("resolveVersion() = %q, want %q", got, "v1.2.3")
	}
}

// TestVersionCommandPrintsTheResolvedValue closes the gap between "version
// prints something" and "version prints resolveVersion()'s value" — a
// version command that printed a hard-coded placeholder would still pass
// TestVersionPrintsSomething.
func TestVersionCommandPrintsTheResolvedValue(t *testing.T) {
	old := version
	version = "v9.9.9-test"
	t.Cleanup(func() { version = old })
	out, code := run(t, "version")
	if code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.TrimSpace(out) != "v9.9.9-test" {
		t.Errorf("version = %q, want %q", strings.TrimSpace(out), "v9.9.9-test")
	}
}
