package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkdirs creates every directory in ds, failing the test if any cannot be made.
func mkdirs(t *testing.T, ds ...string) {
	t.Helper()
	for _, d := range ds {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// writeMarker drops a .grove.toml with the given body into dir.
func writeMarker(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, MarkerName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Resolve reads $GROVE_ROOT from the process environment, so every test that
// calls it pins the variable. Without this the fixtures only steer the outcome
// when the developer running the suite happens not to have GROVE_ROOT exported.

func TestResolvePrecedence(t *testing.T) {
	t.Setenv("GROVE_ROOT", "")

	dir := t.TempDir()
	marked := filepath.Join(dir, "marked")
	nested := filepath.Join(marked, "a", "b")
	plain := filepath.Join(dir, "plain")
	ws := filepath.Join(dir, "ws")
	for _, d := range []string{nested, plain, filepath.Join(ws, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(marked, MarkerName), []byte("depth = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := defaults()
	cfg.Default = "work"
	cfg.Workspaces = []Workspace{{Name: "work", Root: ws, Depth: DefaultDepth}}

	tests := []struct {
		name string
		opts Opts
		want string
	}{
		{"flag wins over everything", Opts{Root: plain, Cwd: nested}, plain},
		{"named workspace wins over marker", Opts{Workspace: "work", Cwd: nested}, ws},
		{"marker found from a nested cwd", Opts{Cwd: nested}, marked},
		{"marker found when cwd is the marked dir", Opts{Cwd: marked}, marked},
		{"configured workspace containing cwd", Opts{Cwd: filepath.Join(ws, "sub")}, ws},
		{"falls back to the default workspace", Opts{Cwd: plain}, ws},
		{"falls back to cwd with no config", Opts{Cwd: plain}, plain},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cfg
			if tt.name == "falls back to cwd with no config" {
				c = defaults()
			}
			got, err := Resolve(&c, tt.opts)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.Root != tt.want {
				t.Errorf("Root = %q, want %q", got.Root, tt.want)
			}
		})
	}
}

func TestResolveUnknownWorkspaceIsAnError(t *testing.T) {
	t.Setenv("GROVE_ROOT", "")

	cfg := defaults()
	if _, err := Resolve(&cfg, Opts{Workspace: "nope", Cwd: t.TempDir()}); err == nil {
		t.Fatal("Resolve() with an unknown workspace should error")
	}
}

func TestResolveMarkerOverridesDepth(t *testing.T) {
	t.Setenv("GROVE_ROOT", "")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, MarkerName), []byte("depth = 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaults()
	got, err := Resolve(&cfg, Opts{Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got.Depth != 7 {
		t.Errorf("Depth = %d, want 7 from the marker file", got.Depth)
	}
}

// --- rule 2, $GROVE_ROOT -------------------------------------------------
//
// Every test above pins GROVE_ROOT to "", which means none of them ever
// executes rule 2. These three do, so that deleting the rule or reordering it
// against rule 1 is a test failure rather than a silent behaviour change.

func TestResolveEnvRootBeatsMarker(t *testing.T) {
	dir := t.TempDir()
	marked := filepath.Join(dir, "marked")
	nested := filepath.Join(marked, "a", "b")
	envRoot := filepath.Join(dir, "from-env")
	mkdirs(t, nested, envRoot)
	writeMarker(t, marked, "depth = 2\n")
	t.Setenv("GROVE_ROOT", envRoot)

	cfg := defaults()
	got, err := Resolve(&cfg, Opts{Cwd: nested})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Root != envRoot {
		t.Errorf("Root = %q, want %q from $GROVE_ROOT", got.Root, envRoot)
	}
	if got.Source != "GROVE_ROOT" {
		t.Errorf("Source = %q, want %q", got.Source, "GROVE_ROOT")
	}
}

func TestResolveRootFlagBeatsEnvRoot(t *testing.T) {
	dir := t.TempDir()
	flagRoot := filepath.Join(dir, "from-flag")
	envRoot := filepath.Join(dir, "from-env")
	mkdirs(t, flagRoot, envRoot)
	t.Setenv("GROVE_ROOT", envRoot)

	cfg := defaults()
	got, err := Resolve(&cfg, Opts{Root: flagRoot, Cwd: dir})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Root != flagRoot {
		t.Errorf("Root = %q, want %q: --root outranks $GROVE_ROOT", got.Root, flagRoot)
	}
	if got.Source != "--root flag" {
		t.Errorf("Source = %q, want %q", got.Source, "--root flag")
	}
}

func TestResolveNamedWorkspaceBeatsEnvRoot(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	envRoot := filepath.Join(dir, "from-env")
	mkdirs(t, ws, envRoot)
	t.Setenv("GROVE_ROOT", envRoot)

	cfg := defaults()
	cfg.Workspaces = []Workspace{{Name: "work", Root: ws, Depth: DefaultDepth}}
	got, err := Resolve(&cfg, Opts{Workspace: "work", Cwd: dir})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Root != ws {
		t.Errorf("Root = %q, want %q: an explicitly named workspace outranks $GROVE_ROOT", got.Root, ws)
	}
}

// --- rules 3, 4 and 5 held apart -----------------------------------------
//
// Two workspaces, and the one that is Default is deliberately NOT the one that
// contains the working directory, nor the first in the slice. That separates
// "a workspace contains cwd" from "the default workspace" from "the first
// workspace in the list", which a single-workspace fixture cannot do.

func TestResolveWorkspacePrecedence(t *testing.T) {
	t.Setenv("GROVE_ROOT", "")

	dir := t.TempDir()
	wsA := filepath.Join(dir, "wsA") // contains cwd, listed first, NOT default
	wsB := filepath.Join(dir, "wsB") // the default, listed second
	inA := filepath.Join(wsA, "sub")
	markedInA := filepath.Join(wsA, "marked") // a marker *inside* a workspace
	outside := filepath.Join(dir, "outside")
	mkdirs(t, inA, markedInA, filepath.Join(wsB, "sub"), outside)
	writeMarker(t, markedInA, "depth = 2\n")

	cfg := defaults()
	cfg.Default = "beta"
	cfg.Workspaces = []Workspace{
		{Name: "alpha", Root: wsA, Depth: DefaultDepth},
		{Name: "beta", Root: wsB, Depth: DefaultDepth},
	}

	tests := []struct {
		name     string
		cwd      string
		wantRoot string
		wantName string
	}{
		{"containing workspace beats the default workspace", inA, wsA, "alpha"},
		{"marker beats the workspace that contains it", markedInA, markedInA, ""},
		{"default workspace when no workspace contains cwd", outside, wsB, "beta"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(&cfg, Opts{Cwd: tt.cwd})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.Root != tt.wantRoot {
				t.Errorf("Root = %q, want %q (source %q)", got.Root, tt.wantRoot, got.Source)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

// --- a marker that does not parse ----------------------------------------

func TestResolveMalformedMarkerIsAnError(t *testing.T) {
	t.Setenv("GROVE_ROOT", "")

	dir := t.TempDir()
	inner := filepath.Join(dir, "inner")
	mkdirs(t, inner)
	// An ancestor marker that WOULD resolve, to prove we do not walk past the
	// broken one and quietly answer with a different directory.
	writeMarker(t, dir, "depth = 9\n")
	writeMarker(t, inner, "depth = \n")

	cfg := defaults()
	got, err := Resolve(&cfg, Opts{Cwd: inner})
	if err == nil {
		t.Fatalf("Resolve() with a malformed marker should error, got root %q", got.Root)
	}
	if !strings.Contains(err.Error(), filepath.Join(inner, MarkerName)) {
		t.Errorf("error %q should name the offending marker file", err)
	}
}

// --- the marker's [display] table overrides, it does not replace ----------

func customDisplay() Display {
	return Display{
		GroupBy:      "flat",
		BranchPrefix: "feat/",
		ShowClean:    false,
		ShowStash:    true,
		Color:        "never",
		ASCII:        false,
	}
}

func TestResolveMarkerDisplayMergesOverGlobal(t *testing.T) {
	t.Setenv("GROVE_ROOT", "")

	dir := t.TempDir()
	writeMarker(t, dir, "[display]\nascii = true\n")

	cfg := defaults()
	cfg.Display = customDisplay()
	got, err := Resolve(&cfg, Opts{Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}

	want := customDisplay()
	want.ASCII = true // the only key the marker sets
	if got.Display != want {
		t.Errorf("Display = %+v, want %+v: the marker overrides single keys, it does not replace the whole table", got.Display, want)
	}
}

func TestResolveMarkerDisplayAcceptsAnInlineTable(t *testing.T) {
	t.Setenv("GROVE_ROOT", "")

	dir := t.TempDir()
	// Legal TOML for the same thing, without the literal "[display]" text.
	writeMarker(t, dir, "depth = 5\ndisplay = { ascii = true }\n")

	cfg := defaults()
	cfg.Display = customDisplay()
	got, err := Resolve(&cfg, Opts{Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Display.ASCII {
		t.Error("Display.ASCII should be true: an inline display table is still a display table")
	}
	if got.Display.Color != "never" {
		t.Errorf("Display.Color = %q, want %q carried over from the global config", got.Display.Color, "never")
	}
}

func TestResolveMarkerCommentMentioningDisplayChangesNothing(t *testing.T) {
	t.Setenv("GROVE_ROOT", "")

	dir := t.TempDir()
	// No display table at all - just a comment that happens to mention one.
	writeMarker(t, dir, "# tweak rendering under [display]; see the docs\ndepth = 4\n")

	cfg := defaults()
	cfg.Display = customDisplay()
	got, err := Resolve(&cfg, Opts{Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got.Display != customDisplay() {
		t.Errorf("Display = %+v, want the global %+v untouched", got.Display, customDisplay())
	}
	if got.Depth != 4 {
		t.Errorf("Depth = %d, want 4", got.Depth)
	}
}

// --- isWithin ------------------------------------------------------------

func TestIsWithin(t *testing.T) {
	const root = "/w/root"
	tests := []struct {
		path string
		want bool
	}{
		{"/w/root", true},
		{"/w/root/api", true},
		{"/w/root/api/web", true},
		{"/w/root/..cache", true}, // a child whose name merely starts with ".."
		{"/w/root/..", false},     // the parent itself
		{"/w/other", false},       // a sibling
		{"/w/rootsuffix", false},  // a sibling sharing a string prefix
		{"/", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isWithin(tt.path, root); got != tt.want {
				t.Errorf("isWithin(%q, %q) = %v, want %v", tt.path, root, got, tt.want)
			}
		})
	}
}
