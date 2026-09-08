package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Opts are the command-line inputs to root resolution.
type Opts struct {
	Root      string // --root
	Workspace string // -w/--workspace

	// Cwd is the directory the command was invoked from. It MUST be absolute.
	// Rule 3 walks towards the filesystem root with filepath.Dir, which for a
	// relative path bottoms out at "." instead of "/", so an ancestor marker
	// file would never be found. Callers pass os.Getwd().
	Cwd string
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
	dir, mark, ok, err := findMarker(o.Cwd, cfg.Display)
	if err != nil {
		return nil, err
	}
	if ok {
		r := &Resolved{Root: dir, Depth: DefaultDepth, Display: mark.Display, Source: MarkerName}
		if mark.Depth > 0 {
			r.Depth = mark.Depth
		}
		r.Ignore = mark.Ignore
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
		// A default naming no configured workspace is a typo, and falling
		// through to rule 6 is the worst possible answer to one: grove reports
		// on whatever directory the user is standing in, looks like it worked,
		// and never mentions the setting it ignored. `-w typo` already errors
		// on the identical mistake, and a marker file that does not parse is
		// already an error rather than something to walk past — §4.1 exists so
		// the user can always tell which rule chose the root, and a typo must
		// not quietly change the answer.
		return nil, fmt.Errorf("unknown default workspace %q; `grove config show` lists the configured ones", cfg.Default)
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
	Depth   int      `toml:"depth"`
	Ignore  []string `toml:"ignore"`
	Display Display  `toml:"display"`
}

// findMarker walks from start towards the filesystem root, returning the first
// directory holding a marker file.
//
// The marker's display table is unmarshalled over seed, so keys the marker
// omits keep the value they had globally: spec §4.2 calls this "an optional
// [display] override", which is a merge, not a wholesale replacement.
//
// A marker that does not parse is a returned error rather than something to
// walk past. Skipping it would silently resolve the root to some ancestor
// directory, and §4.1 exists so the user can always tell which rule chose the
// root — a typo in a marker must not quietly change the answer.
func findMarker(start string, seed Display) (string, marker, bool, error) {
	dir := start
	for {
		path := filepath.Join(dir, MarkerName)
		data, err := os.ReadFile(path)
		if err == nil {
			m := marker{Display: seed}
			if e := toml.Unmarshal(data, &m); e != nil {
				return "", marker{}, false, fmt.Errorf("parse %s: %w", path, e)
			}
			// The marker's [display] override gets the same reading as the
			// global one: a marker is a workspace, and a typo in its colour
			// mode is no more something to walk past than a typo in its TOML.
			if e := ValidateColor("display.color in "+path, m.Display.Color); e != nil {
				return "", marker{}, false, e
			}
			return dir, m, true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", marker{}, false, nil
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
//
// Both sides are resolved first. On macOS /var is a symlink to /private/var,
// so a working directory comes back resolved from os.Getwd() while a
// configured root keeps whatever the user wrote — and rule 4 then fails to
// recognise a workspace that plainly contains the caller. An unresolvable
// path falls back to itself: whether it exists is not this predicate's
// question.
func isWithin(path, root string) bool {
	path, root = evalSymlinks(path), evalSymlinks(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	// Testing only for a ".." prefix would also reject a legitimate child whose
	// name merely begins with two dots, such as "..cache": the escape marker is
	// the whole first segment, not the first two characters.
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// evalSymlinks follows symlinks, falling back to the path itself when it
// cannot be resolved.
func evalSymlinks(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
