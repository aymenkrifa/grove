package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadMalformedConfigIsAnError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))

	path := filepath.Join(dir, "cfg", "grove", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// An unterminated table header: valid-looking, definitely not valid TOML.
	if err := os.WriteFile(path, []byte("default = \"work\"\n[[workspace\nname = \"work\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() with a malformed config should error, not fall back to defaults")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q should name the offending file %q", err, path)
	}
}

// TestValidateColor pins the enumeration in spec §4.2 and, just as much, the
// two values that are not in it: the empty string, which both the flag and an
// unwritten config key use for "unset", and everything else, which is a typo
// the user needs told about rather than quietly resolved to auto.
func TestValidateColor(t *testing.T) {
	for _, mode := range []string{"", "auto", "always", "never"} {
		if err := ValidateColor("--color", mode); err != nil {
			t.Errorf("ValidateColor(%q) = %v, want nil", mode, err)
		}
	}
	for _, mode := range []string{"alwyas", "Always", "yes", "true", "auto ", "none"} {
		err := ValidateColor("--color", mode)
		if err == nil {
			t.Errorf("ValidateColor(%q) = nil, want an error", mode)
			continue
		}
		msg := err.Error()
		// The message has one job beyond saying no: telling the user what to
		// type instead, and which setting to type it in.
		for _, want := range []string{"--color", mode, "auto", "always", "never"} {
			if !strings.Contains(msg, want) {
				t.Errorf("ValidateColor(%q) message %q does not mention %q", mode, msg, want)
			}
		}
	}
}

// TestLoadRejectsAnInvalidColor covers the config file half: the same typo in
// display.color that --color now refuses must not be accepted just because it
// arrived in TOML.
func TestLoadRejectsAnInvalidColor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "grove"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "grove", "config.toml"),
		[]byte("[display]\ncolor = \"alwyas\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err == nil {
		t.Fatalf("Load() = %+v, nil; want an error naming the bad colour mode", cfg)
	}
	if !strings.Contains(err.Error(), "alwyas") || !strings.Contains(err.Error(), "display.color") {
		t.Errorf("error = %q, want it to name display.color and the value", err)
	}
}
