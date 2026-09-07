# grove shell hook for fish. Installed with:  grove shell-init fish | source
# See the zsh hook for the forwarding rules. Disable with GROVE_NO_GIT_HOOK=1.
#
# Every expansion below is double-quoted. Fish does not word-split a quoted
# expansion (each list element still becomes its own argument, as with
# bash's "$@"), but it DOES glob an unquoted one — so an unquoted $argv would
# let a path or branch name containing "*" or "?" expand against the
# filesystem instead of being passed through literally.
function git
    if set -q GROVE_NO_GIT_HOOK
        command git "$argv"
        return
    end
    if not contains -- "$argv[1]" status diff fetch log branch
        command git "$argv"
        return
    end
    if command git rev-parse --git-dir >/dev/null 2>&1
        command git "$argv"
        return
    end
    set -l scope (command grove __resolve --scope 2>/dev/null)
    if test $status -ne 0
        command git "$argv"
        return
    end
    set -l sub $argv[1]
    set -e argv[1]
    if test -n "$scope"
        printf '\033[2m→ grove %s %s\033[0m\n' "$sub" "$scope" >&2
        command grove "$sub" "$scope" "$argv"
    else
        printf '\033[2m→ grove %s\033[0m\n' "$sub" >&2
        command grove "$sub" "$argv"
    end
end
