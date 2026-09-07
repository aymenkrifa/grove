# grove shell hook for zsh. Installed with:  eval "$(grove shell-init zsh)"
#
# Forwards to grove only when all three hold:
#   1. the working directory is not inside a git repository, and
#   2. it is inside a configured grove, and
#   3. the subcommand is one grove implements.
# Every other invocation reaches git untouched.
#
# Disable for a shell with:  export GROVE_NO_GIT_HOOK=1
# Bypass for one command with:  command git ...
git() {
  emulate -L zsh
  if [[ -n ${GROVE_NO_GIT_HOOK:-} ]]; then
    command git "$@"
    return
  fi
  case ${1:-} in
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
  local sub=$1
  shift
  if [[ -n $scope ]]; then
    print -u2 -- $'\e[2m'"→ grove $sub $scope"$'\e[0m'
    command grove "$sub" "$scope" "$@"
  else
    print -u2 -- $'\e[2m'"→ grove $sub"$'\e[0m'
    command grove "$sub" "$@"
  fi
}
