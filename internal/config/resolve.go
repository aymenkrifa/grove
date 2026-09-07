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

// Opts are the command-line inputs to root resolution.
type Opts struct {
	Root      string // --root
	Workspace string // -w/--workspace
	Cwd       string
}

// Resolved is the effective workspace a command operates on.
type Resolved struct {
	Name    string
	Root    string
	Depth   int
	Ignore  []string
	Display Display
	Source  string // which precedence rule matched, for `grove config show`
}

// Resolve implements the precedence chain in the spec, §4.1.
func Resolve(cfg *Config, o Opts) (*Resolved, error) {
	base := &Resolved{Depth: DefaultDepth, Display: cfg.Display}

	// 1. --root
	if o.Root != "" {
		base.Root = ExpandPath(o.Root, o.Cwd)
		base.Source = "--root flag"
		return base, nil
	}
	// 2. $GROVE_ROOT
	if env := os.Getenv("GROVE_ROOT"); env != "" && o.Workspace == "" {
		base.Root = ExpandPath(env, o.Cwd)
		base.Source = "GROVE_ROOT"
		return base, nil
	}
	// A named workspace, when asked for by name, outranks the marker file.
	if o.Workspace != "" {
		for _, w := range cfg.Workspaces {
			if w.Name == o.Workspace {
				return fromWorkspace(w, cfg.Display, "workspace "+w.Name), nil
			}
		}
		return nil, fmt.Errorf("unknown workspace %q; `grove config show` lists the configured ones", o.Workspace)
	}
	// 3. marker file at cwd or an ancestor
	if dir, mark, ok := findMarker(o.Cwd); ok {
		r := &Resolved{Root: dir, Depth: DefaultDepth, Display: cfg.Display, Source: MarkerName}
		if mark.Depth > 0 {
			r.Depth = mark.Depth
		}
		r.Ignore = mark.Ignore
		if mark.hasDisplay {
			r.Display = mark.Display
		}
		return r, nil
	}
	// 4. a configured workspace containing cwd
	if w, ok := containingWorkspace(cfg, o.Cwd); ok {
		return fromWorkspace(w, cfg.Display, "workspace "+w.Name+" contains the working directory"), nil
	}
	// 5. the default workspace
	if cfg.Default != "" {
		for _, w := range cfg.Workspaces {
			if w.Name == cfg.Default {
				return fromWorkspace(w, cfg.Display, "default workspace "+w.Name), nil
			}
		}
	}
	// 6. the working directory
	base.Root = o.Cwd
	base.Source = "working directory"
	return base, nil
}

func fromWorkspace(w Workspace, d Display, source string) *Resolved {
	depth := w.Depth
	if depth == 0 {
		depth = DefaultDepth
	}
	return &Resolved{Name: w.Name, Root: w.Root, Depth: depth, Ignore: w.Ignore, Display: d, Source: source}
}

type marker struct {
	Depth      int      `toml:"depth"`
	Ignore     []string `toml:"ignore"`
	Display    Display  `toml:"display"`
	hasDisplay bool
}

func findMarker(start string) (string, marker, bool) {
	dir := start
	for {
		path := filepath.Join(dir, MarkerName)
		data, err := os.ReadFile(path)
		if err == nil {
			m := marker{Display: defaults().Display}
			if e := toml.Unmarshal(data, &m); e == nil {
				m.hasDisplay = strings.Contains(string(data), "[display]")
				return dir, m, true
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			// Unreadable marker: treat as absent rather than failing the run.
			_ = err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", marker{}, false
		}
		dir = parent
	}
}

func containingWorkspace(cfg *Config, cwd string) (Workspace, bool) {
	best := Workspace{}
	found := false
	for _, w := range cfg.Workspaces {
		if w.Root != "" && isWithin(cwd, w.Root) && len(w.Root) > len(best.Root) {
			best, found = w, true
		}
	}
	return best, found
}

// isWithin reports whether path is root or lives beneath it.
func isWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}
