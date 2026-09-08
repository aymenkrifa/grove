package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// The hooks in this package are the only code grove ships that can make git
// answer about a repository the user is not in, and until these tests existed
// nothing executed them: the rest of shell_test.go greps the embedded text,
// which cannot tell a working hook from a broken one. A wrapper that forwards
// when it should not is this project's worst outcome, so the forwarding rules
// are pinned here by running the real hook, in a real shell, against the real
// binary, and reading back the exact argument vectors grove and git received.
//
// Two shims on PATH do the reading: each logs its own argv to a file and then
// execs the real program, so the assertions are about what was actually
// invoked rather than about what the output happened to look like.

var (
	buildOnce sync.Once
	buildDir  string
	buildBin  string
	buildErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
	os.Exit(code)
}

// groveBinary builds grove once per package run. os.MkdirTemp rather than
// t.TempDir because the binary outlives the test that first asked for it.
func groveBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		if _, err := exec.LookPath("go"); err != nil {
			buildErr = fmt.Errorf("no go toolchain on PATH: %w", err)
			return
		}
		dir, err := os.MkdirTemp("", "grove-hook-build")
		if err != nil {
			buildErr = err
			return
		}
		bin := filepath.Join(dir, "grove")
		build := exec.Command("go", "build", "-o", bin, ".")
		build.Dir = filepath.Join("..", "..") // the module root
		if out, err := build.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build: %w\n%s", err, out)
			_ = os.RemoveAll(dir)
			return
		}
		buildDir, buildBin = dir, bin
	})
	if buildErr != nil {
		t.Skipf("cannot build grove for the shell tests: %v", buildErr)
	}
	return buildBin
}

// hookFixture is a grove with three groups and a directory outside it,
// everything under a path containing a space.
type hookFixture struct {
	tree    string // the grove root
	outside string // a directory in no grove at all
	// sibling is a directory beside the grove root whose absolute path
	// string-prefixes it — ".../work tree shop" next to ".../work tree". It
	// is in no grove either, but only a containment test that respects path
	// separators can tell: strings.HasPrefix(cwd, root) calls it a child.
	sibling string
	env     []string // base environment: PATH, HOME and config, all isolated
}

func newHookFixture(t *testing.T) *hookFixture {
	t.Helper()
	grove := groveBinary(t)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git is not on PATH: %v", err)
	}

	// Spaces throughout, in the root, in the group and in the repository
	// name. A fixture without them cannot tell a quoted expansion from an
	// unquoted one, and the unquoted version passes every test.
	base := t.TempDir()
	f := &hookFixture{
		tree:    filepath.Join(base, "my groves", "work tree"),
		outside: filepath.Join(base, "not a grove", "some dir"),
		sibling: filepath.Join(base, "my groves", "work tree shop"),
	}
	binDir := filepath.Join(base, "bin dir")
	home := filepath.Join(base, "home dir")
	cfgHome := filepath.Join(base, "config home")
	for _, dir := range []string{binDir, home, f.outside, f.sibling, filepath.Join(cfgHome, "grove")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	testutil.NewRepo(t, filepath.Join(f.tree, "api", "gate way"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(f.tree, "web", "dash board"), testutil.WithCommit())
	testutil.NewRepo(t, filepath.Join(f.tree, "mono repo", "the thing"), testutil.WithCommit())
	if err := os.WriteFile(filepath.Join(f.tree, ".grove.toml"), []byte("depth = 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A default workspace pointing at the tree. It is what made the hook fire
	// outside any grove: rule 5 resolves a root without ever looking at the
	// working directory, so the "outside" cases below are only meaningful
	// with it configured.
	cfg := fmt.Sprintf("default = \"work\"\n\n[[workspace]]\nname = \"work\"\nroot = %q\n", f.tree)
	if err := os.WriteFile(filepath.Join(cfgHome, "grove", "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	writeShim(t, filepath.Join(binDir, "grove"), grove)
	writeShim(t, filepath.Join(binDir, "git"), realGit)

	f.env = envWithout(os.Environ(),
		// GROVE_NO_GIT_HOOK must be absent rather than empty: fish tests it
		// with `set -q`, which is true for a variable that is set to "".
		"GROVE_NO_GIT_HOOK", "GROVE_ROOT", "GROVE_HOOK_LOG",
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE",
		"PATH", "HOME", "XDG_CONFIG_HOME", "NO_COLOR",
		"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM",
	)
	f.env = append(f.env,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+home,
		"XDG_CONFIG_HOME="+cfgHome,
		"NO_COLOR=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return f
}

// writeShim installs a wrapper that records its own argv, one tab-separated
// line per invocation, and then becomes the real program.
func writeShim(t *testing.T, path, real string) {
	t.Helper()
	body := "#!/bin/sh\n" +
		"{ printf '%s' " + shquote(t, filepath.Base(path)) + "; " +
		"for a in \"$@\"; do printf '\\t%s' \"$a\"; done; printf '\\n'; } >> \"$GROVE_HOOK_LOG\"\n" +
		"exec " + shquote(t, real) + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func envWithout(env []string, keys ...string) []string {
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if name, _, ok := strings.Cut(kv, "="); !ok || !drop[name] {
			out = append(out, kv)
		}
	}
	return out
}

// shquote wraps s for a shell. Single quotes mean the same thing in POSIX
// shells and in fish, which is why every fixture path must be free of them.
func shquote(t *testing.T, s string) string {
	t.Helper()
	if strings.Contains(s, "'") {
		t.Fatalf("fixture path %q contains a single quote; the shell scripts here cannot express it", s)
	}
	return "'" + s + "'"
}

type shellUnderTest struct {
	name    string
	bin     string
	args    []string // everything before the script
	install string   // the line that loads the hook
}

var shellsUnderTest = []shellUnderTest{
	{"zsh", "zsh", []string{"-f", "-c"}, `eval "$(grove shell-init zsh)"`},
	{"bash", "bash", []string{"--norc", "--noprofile", "-c"}, `eval "$(grove shell-init bash)"`},
	{"fish", "fish", []string{"--no-config", "-c"}, `grove shell-init fish | source`},
}

// invocation is one logged call: the program, and the argv it was given.
type invocation struct {
	cmd  string
	argv []string
}

type hookResult struct {
	stdout, stderr string
	code           int
	calls          []invocation
}

// grove returns the calls the hook made to grove, minus the two it makes for
// its own sake — loading itself with `shell-init`, and asking `__resolve`
// where it is. What is left is the command the user's git was rewritten into,
// if any.
func (r hookResult) grove() []invocation {
	var out []invocation
	for _, c := range r.calls {
		if c.cmd != "grove" || len(c.argv) == 0 {
			continue
		}
		if c.argv[0] == "__resolve" || c.argv[0] == "shell-init" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// git returns the calls to git, minus the hook's own `rev-parse --git-dir`
// probe: what is left is the command git actually saw.
func (r hookResult) git() []invocation {
	var out []invocation
	for _, c := range r.calls {
		if c.cmd == "git" && !(len(c.argv) == 2 && c.argv[0] == "rev-parse" && c.argv[1] == "--git-dir") {
			out = append(out, c)
		}
	}
	return out
}

func (f *hookFixture) run(t *testing.T, sh shellUnderTest, dir, cmdline string, extraEnv ...string) hookResult {
	t.Helper()
	log := filepath.Join(t.TempDir(), "calls.log")
	script := sh.install + "\n" + "cd " + shquote(t, dir) + "\n" + cmdline + "\n"

	cmd := exec.Command(sh.bin, append(sh.args, script)...)
	cmd.Env = append(append([]string{}, f.env...), append([]string{"GROVE_HOOK_LOG=" + log}, extraEnv...)...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if _, ok := err.(*exec.ExitError); err != nil && !ok {
		t.Fatalf("%s: %v\nscript:\n%s", sh.bin, err, script)
	}

	res := hookResult{stdout: stdout.String(), stderr: stderr.String(), code: cmd.ProcessState.ExitCode()}
	data, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		res.calls = append(res.calls, invocation{cmd: fields[0], argv: fields[1:]})
	}
	return res
}

// forwardCase is one row of the matrix. A nil grove field means "this must
// reach git untouched", which is the answer for everything but the five
// allowlisted subcommands run inside a grove but outside a repository.
type forwardCase struct {
	name     string
	dir      func(f *hookFixture) string
	cmdline  string
	env      []string
	announce string    // the notice on stderr, when it differs from the argv
	grove    []string  // argv grove must be invoked with, nil for "do not forward"
	git      *[]string // argv git must have received, nil for "do not check"
}

func argv(a ...string) *[]string { return &a }

var forwardCases = []forwardCase{
	{
		name:    "grove root reports the whole grove",
		dir:     func(f *hookFixture) string { return f.tree },
		cmdline: "git status",
		grove:   []string{"status"},
	},
	{
		name:    "a group directory becomes a selector",
		dir:     func(f *hookFixture) string { return filepath.Join(f.tree, "api") },
		cmdline: "git status",
		grove:   []string{"status", "api"},
	},
	{
		// The selector is one argument even though it contains a space. An
		// unquoted expansion in bash splits it into "mono" and "repo", which
		// grove rejects as two selectors.
		name:    "a group directory with a space stays one argument",
		dir:     func(f *hookFixture) string { return filepath.Join(f.tree, "mono repo") },
		cmdline: "git status",
		grove:   []string{"status", "mono repo"},
	},
	{
		// A second allowlisted subcommand: a hook that hard-codes "status"
		// passes every case above.
		name:    "branch forwards as well as status",
		dir:     func(f *hookFixture) string { return f.tree },
		cmdline: "git branch",
		grove:   []string{"branch"},
	},
	{
		name:    "the user's own arguments follow the selector",
		dir:     func(f *hookFixture) string { return filepath.Join(f.tree, "web") },
		cmdline: "git log -n 1",
		grove:   []string{"log", "web", "-n", "1"},
		// The notice names the rewrite the hook performed — the subcommand
		// and the selector it added — not the user's own arguments, which it
		// passed through untouched.
		announce: "→ grove log web",
	},
	{
		name:    "inside a repository git answers",
		dir:     func(f *hookFixture) string { return filepath.Join(f.tree, "api", "gate way") },
		cmdline: "git status",
		git:     argv("status"),
	},
	{
		// The regression test for the hook firing outside any grove: the
		// fixture's config names a default workspace, which resolves to the
		// tree from anywhere on the machine.
		name:    "outside any grove git answers",
		dir:     func(f *hookFixture) string { return f.outside },
		cmdline: "git status",
		git:     argv("status"),
	},
	{
		// The mirror of the "we"/"web" case one level down, at the level that
		// matters most: a sibling of the grove ROOT whose path merely starts
		// with it. ".../work tree shop" is not in the grove, and the fixture's
		// default workspace resolves to ".../work tree" from anywhere, so a
		// containment test written as strings.HasPrefix(cwd, root) answers
		// happily here and the wrapper reports a tree the user is not in. That
		// mutant passed the entire suite until this row existed.
		name:    "a sibling whose path prefixes the grove root is outside it",
		dir:     func(f *hookFixture) string { return f.sibling },
		cmdline: "git status",
		git:     argv("status"),
	},
	{
		name:    "outside any grove with GROVE_ROOT set git still answers",
		dir:     func(f *hookFixture) string { return f.outside },
		cmdline: "git status",
		env:     []string{"GROVE_ROOT=@tree@"},
		git:     argv("status"),
	},
	{
		name:    "GROVE_NO_GIT_HOOK switches the hook off",
		dir:     func(f *hookFixture) string { return f.tree },
		cmdline: "git status",
		env:     []string{"GROVE_NO_GIT_HOOK=1"},
		git:     argv("status"),
	},
	{
		name:    "command git bypasses the hook",
		dir:     func(f *hookFixture) string { return f.tree },
		cmdline: "command git status",
		git:     argv("status"),
	},
	{
		// The one that matters most: a subcommand that writes must never be
		// answered by grove.
		name:    "push is never forwarded",
		dir:     func(f *hookFixture) string { return f.tree },
		cmdline: "git push",
		git:     argv("push"),
	},
	{
		name:    "checkout is never forwarded",
		dir:     func(f *hookFixture) string { return f.tree },
		cmdline: "git checkout -b topic",
		git:     argv("checkout", "-b", "topic"),
	},
	{
		// An argument containing a space must arrive as one argument. Joining
		// the vector makes git report:
		//   git: 'commit -m hello world' is not a git command
		name:    "commit keeps its arguments apart",
		dir:     func(f *hookFixture) string { return f.tree },
		cmdline: "git commit -m 'hello world'",
		git:     argv("commit", "-m", "hello world"),
	},
	{
		// A bare `git` must stay bare. A hook that passes its argument list
		// as one joined string turns it into `git ""`, which is a different
		// command with a different error message.
		name:    "a bare git stays bare",
		dir:     func(f *hookFixture) string { return f.tree },
		cmdline: "git",
		git:     argv(),
	},
}

func TestGitHookForwardingMatrix(t *testing.T) {
	f := newHookFixture(t)
	for _, sh := range shellsUnderTest {
		t.Run(sh.name, func(t *testing.T) {
			if _, err := exec.LookPath(sh.bin); err != nil {
				t.Skipf("%s is not installed: %v", sh.bin, err)
			}
			for _, tc := range forwardCases {
				t.Run(tc.name, func(t *testing.T) {
					env := make([]string, len(tc.env))
					for i, kv := range tc.env {
						env[i] = strings.ReplaceAll(kv, "@tree@", f.tree)
					}
					got := f.run(t, sh, tc.dir(f), tc.cmdline, env...)

					if tc.grove == nil {
						if forwarded := got.grove(); len(forwarded) != 0 {
							t.Errorf("git was rewritten into grove %q — it must reach git untouched\n%s",
								forwarded[0].argv, got.stderr)
						}
						if strings.Contains(got.stderr, "→ grove") {
							t.Errorf("the hook announced a forward it must not make:\n%s", got.stderr)
						}
					} else {
						forwarded := got.grove()
						if len(forwarded) != 1 {
							t.Fatalf("grove was invoked %d times, want once: %v\nstderr:\n%s",
								len(forwarded), forwarded, got.stderr)
						}
						if !equalArgv(forwarded[0].argv, tc.grove) {
							t.Errorf("grove argv = %q, want %q", forwarded[0].argv, tc.grove)
						}
						want := tc.announce
						if want == "" {
							want = "→ grove " + strings.Join(tc.grove, " ")
						}
						if !strings.Contains(got.stderr, want) {
							t.Errorf("stderr does not announce %q:\n%s", want, got.stderr)
						}
						if got.code != 0 {
							t.Errorf("exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
						}
					}

					if tc.git != nil {
						calls := got.git()
						if len(calls) != 1 {
							t.Fatalf("git was invoked %d times, want once: %v", len(calls), calls)
						}
						if !equalArgv(calls[0].argv, *tc.git) {
							t.Errorf("git argv = %q, want %q", calls[0].argv, *tc.git)
						}
					}
				})
			}
		})
	}
}

// TestGitHookAnnouncesOnStderr pins which stream the "→ grove ..." notice
// goes to. On stdout it would land in the middle of the piped output of
// `git status | ...`, corrupting it.
func TestGitHookAnnouncesOnStderr(t *testing.T) {
	f := newHookFixture(t)
	for _, sh := range shellsUnderTest {
		t.Run(sh.name, func(t *testing.T) {
			if _, err := exec.LookPath(sh.bin); err != nil {
				t.Skipf("%s is not installed: %v", sh.bin, err)
			}
			got := f.run(t, sh, filepath.Join(f.tree, "api"), "git status")
			if strings.Contains(got.stdout, "→ grove") {
				t.Errorf("the notice is on stdout, where it corrupts a pipe:\n%s", got.stdout)
			}
			if !strings.Contains(got.stderr, "→ grove status api") {
				t.Errorf("the notice is not on stderr:\n%s", got.stderr)
			}
			if !strings.Contains(got.stdout, "gate way") {
				t.Errorf("stdout is not the grove's own report:\n%s", got.stdout)
			}
		})
	}
}

func equalArgv(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
