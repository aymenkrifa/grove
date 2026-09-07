// Package render turns collected repository state into the two output forms
// grove supports: a grouped terminal table and a JSON document.
package render

import (
	"os"
	"strings"
)

// ANSI escape sequences, written out rather than pulled from a dependency —
// colour is the entire requirement, and a library for it would cost more than
// it saves.
const (
	reset  = "\x1b[0m"
	dim    = "\x1b[2m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	blue   = "\x1b[34m"
	cyan   = "\x1b[36m"
)

// UseColor decides whether to emit escapes. NO_COLOR, when set to any non-empty
// value, wins over both the config file and the --color flag — see
// https://no-color.org. Any mode other than "always" or "never" (including the
// documented "auto" and an unset value) defers to whether stdout is a terminal.
func UseColor(mode string, isTTY bool) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default:
		return isTTY
	}
}

type painter struct{ on bool }

// paint wraps s in code, or returns it untouched when colour is off.
func (p painter) paint(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return code + s + reset
}

// cell is one table cell held in both forms: the text as it appears on screen
// and the same text carrying its escapes. Column widths are measured on plain,
// because escapes occupy no screen columns. Keeping the two side by side is why
// this package lays its columns out itself instead of handing coloured strings
// to text/tabwriter, which counts an escape sequence as ordinary characters and
// so pads a coloured table crooked.
type cell struct{ plain, painted string }

// cellOf paints s with code and remembers what it looked like unpainted.
func (p painter) cellOf(code, s string) cell {
	return cell{plain: s, painted: p.paint(code, s)}
}

// plainCell is a cell that is never coloured.
func plainCell(s string) cell { return cell{plain: s, painted: s} }

// joinCells concatenates cells with sep, keeping both forms in step.
func joinCells(sep string, cs []cell) cell {
	var plain, painted strings.Builder
	for i, c := range cs {
		if i > 0 {
			plain.WriteString(sep)
			painted.WriteString(sep)
		}
		plain.WriteString(c.plain)
		painted.WriteString(c.painted)
	}
	return cell{plain: plain.String(), painted: painted.String()}
}
