# grove shell hook for fish. Installed with:  grove shell-init fish | source
# See the zsh hook for the forwarding rules. Disable with GROVE_NO_GIT_HOOK=1.
#
# Expansions used as arguments are bare, deliberately. In fish a double-quoted
# list expansion joins its elements with a space into exactly ONE argument —
# it is POSIX "$*", not "$@" — so "$argv" would turn
#   git commit -m "hello world"
# into the single argument `commit -m hello world`, and a bare `git` into the
# one empty argument `git ""`. Bare $argv needs no defending against globbing
# either: fish expands wildcards only in literal tokens, never in the value of
# a variable. "$scope" inside `test -n` is the one exception, and is correct
# there precisely because it joins: test wants one argument.
function git
    if set -q GROVE_NO_GIT_HOOK
        command git $argv
        return
    end
    if not contains -- "$argv[1]" status diff fetch log branch
        command git $argv
        return
    end
    if command git rev-parse --git-dir >/dev/null 2>&1
        command git $argv
        return
    end
    # The in-grove test is a plain command in the `if` condition, whose exit
    # status fish reads directly. `set -l scope (command ...)` would report
    # set's own status, not the command substitution's, so a __resolve that
    # failed — i.e. "you are not in a grove" — could read as success and
    # forward anyway. Without --scope the resolver answers the same question
    # without walking the tree, so this second call costs a process, not a
    # directory scan; it runs only on the path that is about to forward.
    if not command grove __resolve >/dev/null 2>&1
        command git $argv
        return
    end
    set -l scope (command grove __resolve --scope 2>/dev/null)
    set -l sub $argv[1]
    set -e argv[1]
    if test -n "$scope"
        printf '\033[2m→ grove %s %s\033[0m\n' $sub $scope >&2
        command grove $sub $scope $argv
    else
        printf '\033[2m→ grove %s\033[0m\n' $sub >&2
        command grove $sub $argv
    end
end
