// Package config loads grove's optional TOML configuration and resolves which
// directory a command should operate on.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// DefaultDepth is how many directory levels below the root are searched for
// repositories when the config does not say otherwise.
const DefaultDepth = 3

type Config struct {
	Default    string      `toml:"default"`
	Workspaces []Workspace `toml:"workspace"`
	Display    Display     `toml:"display"`
}

type Workspace struct {
	Name   string   `toml:"name"`
	Root   string   `toml:"root"`
	Depth  int      `toml:"depth"`
	Ignore []string `toml:"ignore"`
}

type Display struct {
	GroupBy      string `toml:"group_by"`
	BranchPrefix string `toml:"branch_prefix"`
	ShowClean    bool   `toml:"show_clean"`
	ShowStash    bool   `toml:"show_stash"`
	Color        string `toml:"color"`
	ASCII        bool   `toml:"ascii"`
}

// MarkerName is the per-directory workspace marker file.
const MarkerName = ".grove.toml"

func defaults() Config {
	return Config{Display: Display{
		GroupBy:   "dir",
		ShowClean: true,
		ShowStash: true,
		Color:     "auto",
	}}
}

// Path returns where the config file is expected to live, whether or not it exists.
func Path() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "grove", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "grove", "config.toml")
}

// Load reads the config file. A missing file yields defaults and no error.
func Load() (*Config, error) {
	cfg := defaults()
	path := Path()
	if path == "" {
		return &cfg, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for i := range cfg.Workspaces {
		cfg.Workspaces[i].Root = ExpandPath(cfg.Workspaces[i].Root, filepath.Dir(path))
		if cfg.Workspaces[i].Depth == 0 {
			cfg.Workspaces[i].Depth = DefaultDepth
		}
	}
	return &cfg, nil
}

// ExpandPath expands a leading ~, expands environment variables, and resolves a
// relative path against base.
func ExpandPath(p, base string) string {
	if p == "" {
		return ""
	}
	p = os.ExpandEnv(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	if !filepath.IsAbs(p) && base != "" {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}
