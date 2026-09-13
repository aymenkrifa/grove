# Contributing Guidelines

Thanks for your interest in contributing to grove.

## Code of Conduct

This project follows the [Contributor Covenant](https://www.contributor-covenant.org/version/2/0/code_of_conduct.html). Please report unacceptable behaviour to the maintainer.

## Getting Started

1. Fork the repository and clone your fork.
2. Create a branch with a descriptive name (e.g. `fix/fetch-jobs-flag`, `feature/log-author-filter`).
3. Make your changes.
4. Push and open a pull request against `main`.

grove requires **Go 1.26 or newer**.

**Linux and macOS only** — this isn't a gap, it's what the test suite can actually stand behind: it builds fixture repositories with `?` and `"` in filenames, relies on symlinks, and sends real signals, none of which behave the same way (or at all) on Windows. A Windows binary would be a promise nothing here can back, so there's no CI leg for it and none is built.

```sh
git clone https://github.com/aymenkrifa/grove && cd grove
go build -o ~/.local/bin/grove . && go build -o ~/.local/bin/git-grove ./cmd/git-grove
```

## Bug Reports and Feature Requests

Open a GitHub issue. Search existing issues first to avoid duplicates. For a bug, include the exact `grove` (or `git grove`) invocation and enough of your directory layout — or `.grove.toml` / `~/.config/grove/config.toml` — to reproduce the root and selector resolution grove used. For a feature request, describe the use case.

## Code Style

`go vet ./...` must be clean and `gofmt -l .` must print nothing. Both are enforced in CI on every push and pull request — see [`.github/workflows/ci.yml`](.github/workflows/ci.yml). Run them locally before opening a PR:

```sh
gofmt -l .
go vet ./...
```

## Tests

```sh
go test -race ./...
```

Most of the suite is ordinary Go tests against fixture repositories, but the shell-hook wrappers get their own end-to-end treatment: rather than merely checking the embedded script text, the suite loads the hook into a real `zsh`, `bash` and `fish`, drives it through `git status` and friends, and reads back the exact argument vectors `git` and `grove` were invoked with — including the cases that must *not* forward. `fish` runs on the Linux CI leg only, since the macOS runner has no `fish` installed; a shell missing from `PATH` is skipped rather than failing the build, so if you change the fish wrapper, the Linux leg is where you'll see it exercised.

## Pull Requests

Keep PRs focused on a single change and describe the motivation in the PR body. If the change touches the shell hooks or `git-grove`, say which shells (and, for the hook, which forwarding rule) you tested — that's the part a reviewer can't infer from a diff alone.

## Contact

For questions, email the maintainer at [aymenkrifa@gmail.com](mailto:aymenkrifa@gmail.com).
