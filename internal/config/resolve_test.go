package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
