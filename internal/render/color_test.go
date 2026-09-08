package render

import (
	"strings"
	"testing"
)

func TestUseColor(t *testing.T) {
	tests := []struct {
		name, mode string
		tty        bool
		noColorEnv string
		want       bool
	}{
		{"auto on a tty", "auto", true, "", true},
		{"auto off a tty", "auto", false, "", false},
		{"always, even off a tty", "always", false, "", true},
		{"never, even on a tty", "never", true, "", false},
		{"NO_COLOR beats always", "always", true, "1", false},
		{"NO_COLOR beats auto on a tty", "auto", true, "1", false},
		// no-color.org: the variable is honoured when it is present and not
		// empty, whatever it is set to. "0" is not an opt-out.
		{"NO_COLOR=0 still disables", "always", true, "0", false},
		{"NO_COLOR=false still disables", "always", true, "false", false},
		// An unset mode means auto: it is how both the flag and a config
		// file that never mentions colour spell "no preference".
		{"an unset mode follows the tty, on", "", true, "", true},
		{"an unset mode follows the tty, off", "", false, "", false},
		// A misspelling never gets this far — config.ValidateColor rejects it
		// at the edge — so this case pins the arm's totality, not a mode grove
		// accepts. It used to be the behaviour for "--color=alwyas".
		{"an unreachable mode still falls back to the tty", "sometimes", false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set unconditionally: the ambient environment may already have
			// NO_COLOR set, and t.Setenv restores it after the test.
			t.Setenv("NO_COLOR", tt.noColorEnv)
			if got := UseColor(tt.mode, tt.tty); got != tt.want {
				t.Errorf("UseColor(%q, %v) with NO_COLOR=%q = %v, want %v",
					tt.mode, tt.tty, tt.noColorEnv, got, tt.want)
			}
		})
	}
}

func TestPainter(t *testing.T) {
	on := painter{on: true}
	off := painter{on: false}

	if got, want := on.paint(red, "boom"), "\x1b[31mboom\x1b[0m"; got != want {
		t.Errorf("paint on = %q, want %q", got, want)
	}
	if got := off.paint(red, "boom"); got != "boom" {
		t.Errorf("paint off = %q, want the string untouched", got)
	}
	// An empty cell must stay empty: wrapping "" would emit escapes that pad
	// nothing and make an empty column look occupied.
	if got := on.paint(red, ""); got != "" {
		t.Errorf("paint on of an empty string = %q, want %q", got, "")
	}
}

func TestPaintedCellsKeepPlainWidth(t *testing.T) {
	on := painter{on: true}
	c := on.cellOf(green, "clean")
	if c.plain != "clean" {
		t.Errorf("plain = %q, want %q", c.plain, "clean")
	}
	if !strings.Contains(c.painted, green) || !strings.Contains(c.painted, reset) {
		t.Errorf("painted = %q, want it wrapped in %q...%q", c.painted, green, reset)
	}

	j := joinCells(" ", []cell{on.cellOf(yellow, "~3"), on.cellOf(red, "!2")})
	if j.plain != "~3 !2" {
		t.Errorf("joined plain = %q, want %q", j.plain, "~3 !2")
	}
	if j.painted != "\x1b[33m~3\x1b[0m \x1b[31m!2\x1b[0m" {
		t.Errorf("joined painted = %q", j.painted)
	}
}

// The escape constants are load-bearing: a wrong code paints the wrong colour
// and nothing else in the suite would notice.
func TestEscapeConstants(t *testing.T) {
	for _, tt := range []struct{ name, got, want string }{
		{"reset", reset, "\x1b[0m"},
		{"dim", dim, "\x1b[2m"},
		{"red", red, "\x1b[31m"},
		{"green", green, "\x1b[32m"},
		{"yellow", yellow, "\x1b[33m"},
		{"blue", blue, "\x1b[34m"},
		{"cyan", cyan, "\x1b[36m"},
	} {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
