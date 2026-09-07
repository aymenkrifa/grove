package render

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/git"
)

// Options controls rendering. Color and ASCII are decided by the caller, which
// is the layer that knows whether stdout is a terminal.
type Options struct {
	Display   config.Display
	Color     bool
	ASCII     bool
	ShowClean bool
}

// symbols are the glyphs that differ between a UTF-8 terminal and one that
// cannot render them.
type symbols struct{ ahead, behind, stash, sep string }

func symbolsFor(ascii bool) symbols {
	if ascii {
		return symbols{ahead: "^", behind: "v", stash: "*", sep: " | "}
	}
	return symbols{ahead: "↑", behind: "↓", stash: "$", sep: " · "}
}

// indent is how far a repo row is set in under its group heading.
const indent = "  "

// gutter is the number of spaces between two columns.
const gutter = 2

// Table writes the grouped status table followed by a summary line.
func Table(w io.Writer, repos []git.Repo, o Options) error {
	p := painter{on: o.Color}
	sym := symbolsFor(o.ASCII)

	visible := make([]git.Repo, 0, len(repos))
	for _, r := range repos {
		if o.ShowClean || r.NeedsAttention() {
			visible = append(visible, r)
		}
	}

	order, buckets := grouped(visible, o.Display)
	rows := make([][]cell, 0, len(visible)+len(order))
	for _, g := range order {
		pad := ""
		if g != "" {
			rows = append(rows, []cell{p.cellOf(blue, g)})
			pad = indent
		}
		for _, r := range buckets[g] {
			rows = append(rows, repoCells(r, o.Display, p, sym, pad))
		}
	}

	var b strings.Builder
	layout(&b, rows)
	if len(rows) > 0 {
		b.WriteString("\n")
	}
	b.WriteString(p.paint(dim, summaryLine(repos, sym)))
	b.WriteString("\n")

	_, err := io.WriteString(w, b.String())
	return err
}

// layout writes rows as aligned columns. Widths come from the plain text, so a
// coloured table lines up exactly as an uncoloured one does: escapes take no
// screen columns, which is the one thing text/tabwriter cannot be told.
//
// A group heading is a row of a single cell. The last cell of a row is never
// padded, so a heading sets no width and gets no padding, and every line is
// right-trimmed — the padding of an empty cell would otherwise trail off the
// end of the line and show up in diffs and in copied output.
func layout(b *strings.Builder, rows [][]cell) {
	var widths []int
	for _, cells := range rows {
		for i := 0; i < len(cells)-1; i++ {
			w := utf8.RuneCountInString(cells[i].plain) + gutter
			for len(widths) <= i {
				widths = append(widths, 0)
			}
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	for _, cells := range rows {
		var line strings.Builder
		for i, c := range cells {
			line.WriteString(c.painted)
			if i < len(cells)-1 && i < len(widths) {
				line.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.plain)))
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteString("\n")
	}
}

// repoCells builds one repo's row: name, branch, working-tree state, divergence.
func repoCells(r git.Repo, d config.Display, p painter, sym symbols, pad string) []cell {
	return []cell{
		plainCell(pad + name(r, d)),
		branch(r, p),
		state(r, p, sym),
		divergence(r, p, sym),
	}
}

// name is the repo's label within its group: the directory grouping already
// says which group it is in, so repeating that segment in every row is noise.
// Every other grouping mode shows the full path, because nothing else does.
func name(r git.Repo, d config.Display) string {
	if d.GroupBy == "dir" && r.Group != "" {
		return strings.TrimPrefix(r.Path, r.Group+"/")
	}
	return r.Path
}

// branch is the branch column. A detached HEAD carries its short SHA in Branch,
// which on its own is indistinguishable from a repo sitting on a branch whose
// name happens to be seven hex characters. Parenthesising it is git's own
// convention — `git branch` prints "* (HEAD detached at abc1234)" — so it reads
// correctly to anyone who uses git, and costs two columns.
func branch(r git.Repo, p painter) cell {
	if r.Detached && r.Branch != "" {
		return p.cellOf(dim, "("+r.Branch+")")
	}
	return p.cellOf(dim, r.Branch)
}

// state is the working-tree column.
func state(r git.Repo, p painter, sym symbols) cell {
	switch {
	case r.Error != "":
		return p.cellOf(red, r.Error)
	case r.Bare:
		return p.cellOf(dim, "bare")
	case r.Unborn:
		return p.cellOf(dim, "no commits")
	}
	var parts []cell
	if r.Unstaged > 0 {
		parts = append(parts, p.cellOf(yellow, fmt.Sprintf("~%d", r.Unstaged)))
	}
	if r.Staged > 0 {
		parts = append(parts, p.cellOf(yellow, fmt.Sprintf("+%d", r.Staged)))
	}
	if r.Untracked > 0 {
		parts = append(parts, p.cellOf(yellow, fmt.Sprintf("?%d", r.Untracked)))
	}
	if r.Conflicted > 0 {
		parts = append(parts, p.cellOf(red, fmt.Sprintf("!%d", r.Conflicted)))
	}
	if r.Stashes > 0 {
		parts = append(parts, p.cellOf(dim, fmt.Sprintf("%s%d", sym.stash, r.Stashes)))
	}
	if len(parts) == 0 {
		return p.cellOf(green, "clean")
	}
	// Each part carries its own colour rather than the joined string carrying
	// one, so a red conflict count does not swallow the colour of whatever
	// follows it.
	return joinCells(" ", parts)
}

// divergence is the ahead/behind column. A repo with no upstream has no
// divergence to report, which is different from being level with one.
func divergence(r git.Repo, p painter, sym symbols) cell {
	if r.Error != "" || r.Bare {
		return cell{}
	}
	if !r.HasUpstream() {
		return p.cellOf(dim, "-")
	}
	s := fmt.Sprintf("%s%d %s%d", sym.ahead, r.Ahead, sym.behind, r.Behind)
	if r.Ahead > 0 || r.Behind > 0 {
		return p.cellOf(cyan, s)
	}
	return p.cellOf(dim, s)
}

// summaryLine counts the whole selection, not the rows that survived the
// filter: "3 of 40 repos are dirty" is the useful fact, and hiding the clean
// ones should not change it.
func summaryLine(repos []git.Repo, sym symbols) string {
	dirty, ahead, behind, errs := 0, 0, 0, 0
	for _, r := range repos {
		if r.Dirty() {
			dirty++
		}
		if r.Ahead > 0 {
			ahead++
		}
		if r.Behind > 0 {
			behind++
		}
		if r.Error != "" {
			errs++
		}
	}
	parts := []string{
		fmt.Sprintf("%d repos", len(repos)),
		fmt.Sprintf("%d dirty", dirty),
		fmt.Sprintf("%d ahead", ahead),
		fmt.Sprintf("%d behind", behind),
	}
	if errs > 0 {
		parts = append(parts, fmt.Sprintf("%d errored", errs))
	}
	return strings.Join(parts, sym.sep)
}
