package render

import (
	"strings"
	"unicode/utf8"
)

// headingMin and headingMax bound the banner's width. A one-line diffstat
// would otherwise produce a stub too short to read as a separator, and a
// pathological long line would draw a rule off the edge of the terminal.
const (
	headingMin = 40
	headingMax = 80
)

// RepoHeading is the banner that separates one repository's diff from the
// next: the name set into a rule, with a blank line's worth of air around it
// supplied by the caller.
//
// The name is bold blue — the same blue the status table gives a group
// heading, so the two views agree about what a heading looks like — and the
// rule is dim, because it is scaffolding rather than content. Neither collides
// with git's own diff palette underneath (bold for its file header, cyan for
// hunks, green and red for the lines themselves), which matters because the
// two are printed into the same stream and read as one document.
//
// The rule carries the separation on its own when colour is off, which is the
// case that motivated the banner: piped to a file, a bare repository name is
// indistinguishable from the diff content around it.
func RepoHeading(name, body string, o Options) string {
	p := painter{on: o.Color}
	dash := "─"
	if o.ASCII {
		dash = "-"
	}

	// "── name " — two dashes, a space, the name, a space.
	const leadDashes = 2
	used := leadDashes + 1 + utf8.RuneCountInString(name) + 1
	tail := headingWidth(body) - used
	if tail < 0 {
		tail = 0
	}

	var b strings.Builder
	b.WriteString(p.paint(dim, strings.Repeat(dash, leadDashes)))
	b.WriteString(" ")
	b.WriteString(p.paint(bold+blue, name))
	b.WriteString(" ")
	b.WriteString(p.paint(dim, strings.Repeat(dash, tail)))
	b.WriteString("\n")
	return b.String()
}

// headingWidth measures the block the banner sits above, so the rule ends
// where the content does rather than at some arbitrary column. Measuring the
// body avoids asking the terminal its size, which would cost a dependency and
// be wrong the moment the output is piped anyway.
//
// The body arrives already coloured by git, so it must be measured by what is
// visible: counting the escape bytes drew a rule half again too long, the same
// mistake that makes text/tabwriter unusable for a coloured table.
func headingWidth(body string) int {
	w := 0
	for _, line := range strings.Split(body, "\n") {
		if n := visibleWidth(line); n > w {
			w = n
		}
	}
	if w < headingMin {
		return headingMin
	}
	if w > headingMax {
		return headingMax
	}
	return w
}

// visibleWidth counts the columns a line occupies on screen, skipping ANSI
// escape sequences, which occupy none.
//
// It recognises the CSI form — ESC [ ... final-byte — which is all git and
// grove emit between them: colour, bold, dim and reset are every one of them.
func visibleWidth(s string) int {
	const (
		text   = iota // counting characters
		escape        // just saw ESC, expecting '['
		csi           // inside ESC[ ... , consuming until the final byte
	)
	w, state := 0, text
	for _, r := range s {
		switch state {
		case escape:
			// Only ESC[ starts a CSI sequence. Anything else was a lone ESC,
			// and the character after it is ordinary text.
			if r == '[' {
				state = csi
			} else {
				state = text
				w++
			}
		case csi:
			// Parameter and intermediate bytes run 0x20-0x3F; the sequence
			// ends at the first byte from 0x40 to 0x7E. '[' itself falls in
			// that range, which is why it has to be consumed above rather
			// than here — treating it as a terminator ends every sequence
			// immediately and counts the whole escape as visible text.
			if r >= '@' && r <= '~' {
				state = text
			}
		default:
			if r == 0x1b {
				state = escape
			} else {
				w++
			}
		}
	}
	return w
}
