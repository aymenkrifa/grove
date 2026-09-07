# grove shell hook for bash. Installed with:  eval "$(grove shell-init bash)"
# See the zsh hook for the forwarding rules. Disable with GROVE_NO_GIT_HOOK=1.
git() {
  if [ -n "${GROVE_NO_GIT_HOOK:-}" ]; then
    command git "$@"
    return
  fi
  case "${1:-}" in
    status|diff|fetch|log|branch) ;;
    *) command git "$@"; return ;;
  esac
  if command git rev-parse --git-dir >/dev/null 2>&1; then
    command git "$@"
    return
  fi
  local scope
  if ! scope=$(command grove __resolve --scope 2>/dev/null); then
    command git "$@"
    return
  fi
  local sub="$1"
  shift
  if [ -n "$scope" ]; then
    printf '\033[2m→ grove %s %s\033[0m\n' "$sub" "$scope" >&2
    command grove "$sub" "$scope" "$@"
  else
    printf '\033[2m→ grove %s\033[0m\n' "$sub" >&2
    command grove "$sub" "$@"
  fi
}
