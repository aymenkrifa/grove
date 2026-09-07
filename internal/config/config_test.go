package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExpandsTildeAndDefaults(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(filepath.Join(home, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))

	path := filepath.Join(dir, "cfg", "grove", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "default = \"work\"\n\n[[workspace]]\nname = \"work\"\nroot = \"~/work\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Workspaces) != 1 {
		t.Fatalf("got %d workspaces, want 1", len(cfg.Workspaces))
	}
	if got, want := cfg.Workspaces[0].Root, filepath.Join(home, "work"); got != want {
		t.Errorf("Root = %q, want %q", got, want)
	}
	if cfg.Workspaces[0].Depth != DefaultDepth {
		t.Errorf("Depth = %d, want default %d", cfg.Workspaces[0].Depth, DefaultDepth)
	}
	if !cfg.Display.ShowClean {
		t.Error("Display.ShowClean should default to true")
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with no config file error = %v", err)
	}
	if len(cfg.Workspaces) != 0 {
		t.Errorf("got %d workspaces, want 0", len(cfg.Workspaces))
	}
}
