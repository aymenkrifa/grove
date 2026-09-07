package git

import (
	"strconv"
	"strings"
)

// ParseStatus fills r from `git status --porcelain=v2 --branch -z` output.
//
// The -z form is NUL-separated, which sidesteps every quoting problem: a path
// is never escaped, so it never has to be unescaped. It costs one wrinkle.
// Exactly one record type is variable-length — a rename ("2 ") is followed by
// a second NUL-terminated field holding the original path — and that field
// must be consumed here, because it is an ordinary path and would otherwise be
// read as the next record. An original path such as "? old name.txt" would
// then be counted as an untracked file, and every count after it would be
// wrong in a way nothing in the output announces.
func ParseStatus(out []byte, r *Repo) error {
	records := strings.Split(string(out), "\x00")
	for i := 0; i < len(records); i++ {
		rec := records[i]
		if rec == "" {
			continue
		}
		switch {
		case strings.HasPrefix(rec, "# branch.oid "):
			if strings.TrimPrefix(rec, "# branch.oid ") == "(initial)" {
				r.Unborn = true
			}
		case strings.HasPrefix(rec, "# branch.head "):
			head := strings.TrimPrefix(rec, "# branch.head ")
			if head == "(detached)" {
				// Porcelain v2 says only that HEAD is detached; the commit it
				// points at takes a second git call, so Branch is left to the
				// collector to fill.
				r.Detached = true
			} else {
				r.Branch = head
			}
		case strings.HasPrefix(rec, "# branch.upstream "):
			r.Upstream = strings.TrimPrefix(rec, "# branch.upstream ")
		case strings.HasPrefix(rec, "# branch.ab "):
			r.Ahead, r.Behind = parseAheadBehind(strings.TrimPrefix(rec, "# branch.ab "))
		case strings.HasPrefix(rec, "1 "), strings.HasPrefix(rec, "2 "):
			countXY(rec[2:], r)
			if rec[0] == '2' {
				i++ // a rename record is followed by its original path
			}
		case strings.HasPrefix(rec, "u "):
			r.Conflicted++
		case strings.HasPrefix(rec, "? "):
			r.Untracked++
		}
	}
	r.Clean = !r.Dirty()
	return nil
}

// countXY reads the two-character staged/unstaged field at the head of a
// changed-entry record. '.' means unmodified in that axis, so a single record
// can count towards both ("MM": staged edit, then edited again).
func countXY(rest string, r *Repo) {
	if len(rest) < 2 {
		return
	}
	if rest[0] != '.' {
		r.Staged++
	}
	if rest[1] != '.' {
		r.Unstaged++
	}
}

// parseAheadBehind reads the "+N -M" field.
func parseAheadBehind(s string) (ahead, behind int) {
	for _, f := range strings.Fields(s) {
		if len(f) < 2 {
			continue
		}
		n, err := strconv.Atoi(f[1:])
		if err != nil {
			continue
		}
		switch f[0] {
		case '+':
			ahead = n
		case '-':
			behind = n
		}
	}
	return ahead, behind
}
