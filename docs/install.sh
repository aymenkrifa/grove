#!/bin/sh
# grove installer — https://grove.aymenkrifa.com
#   curl -LsSf https://grove.aymenkrifa.com/install.sh | sh
#
# Downloads the release tarball for this machine from the latest GitHub
# release, verifies it against the published checksums, and drops both the
# grove and git-grove binaries in ~/.local/bin. Re-run any time to update —
# it tells you which version you came from and landed on. Override the
# destination with GROVE_BIN_DIR=/somewhere, or silence the progress lines
# with GROVE_QUIET=1 (errors still print).
set -eu

# Everything below is function definitions only; nothing executes until the
# `main "$@"` on the last line. If the download is cut off mid-transfer, a
# truncated script is a syntax error or a no-op — it can never run half an
# install.

quiet() { [ -n "${GROVE_QUIET:-}" ]; }
line() { quiet || printf '%s\n' "$*"; }
ok()   { quiet || printf '  %s✓%s %-10s %s\n' "$grn" "$rst" "$1" "$2"; }
warn() { quiet || printf '  %s!%s %s\n' "$ylw" "$rst" "$*"; }
die()  { printf '\n  %serror%s %s\n' "${red:-}" "${rst:-}" "$*" >&2; exit 1; }

fetch() { curl --proto '=https' --tlsv1.2 -fsSL "$1" -o "$2"; }

# sha256 of a file, with whichever tool this system has.
hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# Print the version an installed grove reports, or nothing. grove's version
# subcommand just prints a line and exits, so — unlike reaper's TUI — there is
# nothing here that can hijack the terminal and no setsid guard is needed.
probe_version() {
  [ -x "$1" ] || return 0
  "$1" version 2>/dev/null || true
}

# Closing guidance — shared by the early "up to date" exit and the full
# install path. Warns when this grove (or git-grove) isn't the one PATH
# resolves to.
closing() {
  line ""
  case ":$PATH:" in
    *":$BIN_DIR:"*)
      for bin in grove git-grove; do
        shadow="$(command -v "$bin" 2>/dev/null || true)"
        if [ -n "$shadow" ] && [ "$shadow" != "$BIN_DIR/$bin" ]; then
          warn "another $bin at ${b}$shadow${rst} comes first on your PATH and will shadow this one."
        fi
      done
      line "  ${b}grove${rst} is ready — run ${b}grove status${rst} in a directory of clones, or ${b}grove init .${rst} to mark one."
      line "  ${b}git grove status${rst} works too, now that ${b}git-grove${rst} is on your PATH."
      ;;
    *)
      warn "${b}$BIN_DIR${rst} is not on your PATH yet."
      line "     this session:  ${b}export PATH=\"$BIN_DIR:\$PATH\"${rst}"
      line "     to keep it:     append that line to ~/.bashrc or ~/.zshrc"
      line "  then run ${b}grove status${rst} in a directory of clones, or ${b}grove init .${rst} to mark one."
      ;;
  esac
}

main() {
  REPO="aymenkrifa/grove"

  # Readable output: colour only on a real terminal (honours NO_COLOR), plain
  # when piped to a file. GROVE_QUIET=1 hushes everything but errors, which
  # always go to stderr.
  if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    b=$(printf '\033[1m');   dim=$(printf '\033[2m'); grn=$(printf '\033[32m')
    ylw=$(printf '\033[33m'); red=$(printf '\033[31m'); rst=$(printf '\033[0m')
  else
    b=; dim=; grn=; ylw=; red=; rst=
  fi

  # Where to put the binaries: an explicit override wins; root gets a system
  # dir that's already on PATH (so 'grove' just works, no profile edits);
  # everyone else gets a no-sudo user dir.
  if [ -n "${GROVE_BIN_DIR:-}" ]; then
    BIN_DIR="$GROVE_BIN_DIR"
  elif [ "$(id -u)" = 0 ]; then
    BIN_DIR="/usr/local/bin"
  else
    BIN_DIR="$HOME/.local/bin"
  fi

  # Platform. grove supports linux and macos only — that's not "untested
  # elsewhere", it's the honest state of things: release binaries are
  # published for linux and darwin alone, and the test suite is POSIX-only
  # (it builds fixtures with '?' and '"' in filenames, uses symlinks, and
  # sends real signals). A source build would hit the same wall, so this
  # doesn't suggest one.
  case "$(uname -s)" in
    Linux)  OS=linux ;;
    Darwin) OS=darwin ;;
    *) die "grove supports linux and macos only; windows isn't supported — release binaries aren't published for it and the test suite is POSIX-only, so a source build wouldn't work either." ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64)  ARCH=amd64 ;;
    aarch64 | arm64) ARCH=arm64 ;;
    *) die "unsupported architecture: $(uname -m)" ;;
  esac

  command -v curl >/dev/null 2>&1 || die "curl is required."
  command -v tar  >/dev/null 2>&1 || die "tar is required."
  command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 \
    || die "need sha256sum or shasum to verify the download."

  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT

  line ""
  line "  ${b}grove${rst}${dim} · status, diffs and logs across every git repository under one directory${rst}"
  line ""
  ok "target" "${OS}_${ARCH}"

  # What's already installed, so the end of the run can say whether this was
  # a fresh install, an update, or a no-op. Both binaries are installed
  # together, so "up to date" only holds when git-grove is still there too —
  # otherwise a prior partial install would be reported as fine.
  old_ver=""
  [ -x "$BIN_DIR/grove" ] && old_ver="$(probe_version "$BIN_DIR/grove")"

  # The asset names carry the tag, so the tag has to be resolved before any
  # download URL exists. One redirect, no body: /releases/latest lands on
  # /releases/tag/vX.Y.Z.
  latest_url="$(curl --proto '=https' --tlsv1.2 -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/$REPO/releases/latest" 2>/dev/null || true)"
  case "$latest_url" in
    */releases/tag/*) TAG="${latest_url##*/tag/}" ;;
    *) die "could not determine the latest release — is there one yet?" ;;
  esac

  # Version reporting is a straight string compare against the tag: release
  # binaries are built with -ldflags -X cmd.version=$TAG, so an installed
  # grove's own "version" output already matches the tag verbatim.
  if [ -n "$old_ver" ] && [ "$old_ver" = "$TAG" ] && [ -x "$BIN_DIR/git-grove" ]; then
    ok "up to date" "already on the latest release ($TAG)"
    closing
    return 0
  fi

  STEM="grove_${TAG}_${OS}_${ARCH}"
  TARBALL="$STEM.tar.gz"
  BASE="https://github.com/$REPO/releases/download/$TAG"

  fetch "$BASE/$TARBALL" "$TMP/$TARBALL" || die "could not download $TARBALL — is there a release for $OS/$ARCH?"
  fetch "$BASE/checksums.txt" "$TMP/checksums.txt" || die "could not download checksums.txt."
  ok "downloaded" "$TARBALL"

  # One checksums.txt covers every asset in the release, so the expected hash
  # is picked out by filename rather than assumed to be the file's only line.
  expected="$(awk -v f="$TARBALL" '$2 == f { print $1 }' "$TMP/checksums.txt")"
  [ -n "$expected" ] || die "checksums.txt has no entry for $TARBALL."
  actual="$(hash_file "$TMP/$TARBALL")"
  [ "$expected" = "$actual" ] || die "checksum mismatch — refusing to install."
  ok "verified" "sha256 checksum"

  # The binaries sit one directory deep, inside a folder named after the
  # stem. grove ships two binaries per tarball — grove and git-grove — and
  # both are required: an archive with grove but no git-grove would install
  # a working grove and a silently broken `git grove status`. So both are
  # checked for before either is installed.
  tar -xzf "$TMP/$TARBALL" -C "$TMP"
  for bin in grove git-grove; do
    [ -f "$TMP/$STEM/$bin" ] || die "archive did not contain $bin."
  done
  mkdir -p "$BIN_DIR"
  for bin in grove git-grove; do
    install -m 755 "$TMP/$STEM/$bin" "$BIN_DIR/$bin"
  done

  # Reaching here with old_ver already at the tag means the "up to date" exit
  # above was skipped because git-grove had gone missing. That is a repair,
  # not an update — "updated v0.1.0 → v0.1.0" would read as a no-op.
  if [ -z "$old_ver" ]; then
    ok "installed" "$BIN_DIR/grove, $BIN_DIR/git-grove  ($TAG)"
  elif [ "$old_ver" = "$TAG" ]; then
    ok "repaired" "reinstalled $TAG (git-grove was missing)"
  else
    ok "updated" "$old_ver → $TAG"
  fi

  closing
}

main "$@"
