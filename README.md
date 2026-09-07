# grove

Status, diffs and logs across every git repository under one directory.

`git` answers questions about one repository at a time. In a directory holding
twenty clones, the useful questions — which have uncommitted work, what branch
is each on, what did I touch today — have no answer at all. `grove` answers
them.

```
$ grove status
api
  auth-service  feat/ABC-123-token-refresh  clean  ↑0 ↓0
  billing       develop                     ~4     ↑0 ↓0
  gateway       main                        ~2 ?1  ↑1 ↓0
web
  dashboard     develop                     clean  ↑0 ↓0
  landing       develop                     ~2     ↑0 ↓0

5 repos · 3 dirty · 1 ahead · 0 behind
```

## Install

```bash
go install github.com/aymenkrifa/grove/...@latest
```

The `...` installs both `grove` and `git-grove`. The second is what makes
`git grove status` work as a native git subcommand, from any directory — see
below.

## Use

```bash
grove status                  # every repository under the current root
grove status -d               # only what needs attention
grove status --json           # machine-readable
grove list                    # every repository grove can see
grove branch                  # who is on which branch
grove diff                    # per-repository diffstat
grove diff gateway            # the full diff for one repository
grove fetch                   # refresh divergence numbers, concurrently
grove log --since yesterday   # commits across every repository, merged
grove exec -- git log -1 --oneline
```

A **selector** narrows any command: a group (`api`), a path (`web/dashboard`),
or a unique substring (`dash`). Ambiguous selectors list the candidates instead
of guessing:

```
$ grove status e
grove: "e" is ambiguous — it matches api/auth-service, api/gateway, web/dashboard, web/landing
```

Every command accepts `--root`, `-w`/`--workspace`, `--color auto|always|never`,
`--jobs`, and `--ascii` (plain-text symbols in place of the unicode ones). Most
also take `--json` for scripting; run `grove <command> --help` for the exact
flags — `status`, `list` and `branch` share little beyond the selector, and
`diff`, `fetch`, `log` and `exec` each add flags of their own (`diff --stat`,
`fetch --prune`, `log --since`/`--author`/`-n`, `exec --dry-run`/`--keep-going`).

## Configuration

grove needs none: run it anywhere and it reports on the git repositories
beneath the current directory (up to 3 levels deep, by default). Configuration
adds names, depth and defaults.

```bash
grove init .   # marks a directory as a grove
```

```
$ grove init .
wrote /tmp/work/.grove.toml (5 repositories)
```

That marker is enough on its own — depth, ignore patterns and a per-directory
display override, commented out and ready to edit:

```toml
# grove workspace marker
# 5 repositories found when this file was created.
depth = 3

# Directories to skip while searching:
# ignore = ["**/vendor/**", "**/node_modules/**"]

# [display]
# group_by   = "dir"       # dir | none | branch-prefix
# show_clean = true
```

For more than one workspace, use `~/.config/grove/config.toml`
(`$XDG_CONFIG_HOME/grove/config.toml` if set):

```toml
default = "work"

[[workspace]]
name   = "work"
root   = "~/work"
depth  = 3
ignore = ["node_modules"]

[display]
group_by   = "dir"     # dir | none | branch-prefix
show_clean = true
color      = "auto"
```

Then `grove -w work status` works from anywhere. An ignore pattern follows
gitignore's own dialect: a pattern with no slash, like `node_modules` above,
matches that segment at any depth (`node_modules` and `web/app/node_modules`
alike); a pattern containing a slash is anchored to the root instead.

The root is chosen by the first of these that applies: `--root`, `$GROVE_ROOT`,
a `.grove.toml` marker at or above the working directory, a configured
workspace containing it, the default workspace, the working directory.
`grove config show` explains which one won:

```
$ grove config show
config file:  /home/user/.config/grove/config.toml
root:         /tmp/work
chosen by:    .grove.toml
workspace:    (none)
depth:        3
ignore:       []
group_by:     dir
show_clean:   true
```

## Making `git status` do it

Two independent options, both optional.

**As a git subcommand**, needing no shell changes at all — this works as soon
as `git-grove` is on your `PATH`:

```bash
git grove status
```

**As a `git` wrapper**, so plain `git status` works from a workspace directory:

```bash
eval "$(grove shell-init zsh)"     # or bash; for fish: grove shell-init fish | source
```

```
~/work $ git status
→ grove status
api
  auth-service  feat/ABC-123-token-refresh  clean  ↑0 ↓0
  billing       develop                     ~4     ↑0 ↓0
  gateway       main                        ~2 ?1  ↑1 ↓0
web
  dashboard     develop                     clean  ↑0 ↓0
  landing       develop                     ~2     ↑0 ↓0

5 repos · 3 dirty · 1 ahead · 0 behind
```

The wrapper forwards a git invocation to grove only when all three hold: the
subcommand is one of `status`, `diff`, `fetch`, `log` or `branch`; the working
directory is outside any git repository; and it is inside a configured grove.
Everywhere else — including plain `git status` inside a single clone — `git`
is untouched. It prints the grove command it runs, so nothing happens
invisibly. Bypass it once with `command git`, or turn it off for the shell
with `GROVE_NO_GIT_HOOK=1`.

## What grove will not do

grove never changes a working tree. There is no `pull`, `push`, `checkout` or
`commit`; `fetch` is the only network command, and it touches no files. This is
deliberate, not a missing feature: anything that mutates goes through
`grove exec`, where you type the command yourself and grove only fans it out.

```bash
grove exec --dry-run -- git pull --ff-only   # see what would run, and where
grove exec -- git pull --ff-only
grove exec --keep-going -- go build ./...    # keep going even if one repo fails
```

A detached HEAD renders as its short SHA in parentheses, e.g. `(abc1234)` —
parentheses being git's own convention for the same thing. As with git itself,
a branch literally *named* `(abc1234)` would look identical; grove doesn't
try to disambiguate what git can't either.

## License

MIT
