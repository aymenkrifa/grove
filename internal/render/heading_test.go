package render

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const body = " src/auth.go  | 12 ++++++---\n 1 file changed, 9 insertions(+), 3 deletions(-)\n"

func TestRepoHeadingCarriesTheNameAndARule(t *testing.T) {
	got := RepoHeading("api/gateway", body, Options{})
	if !strings.Contains(got, "api/gateway") {
		t.Fatalf("heading lost the name: %q", got)
	}
	if !strings.HasPrefix(got, "── api/gateway ─") {
		t.Errorf("heading = %q, want it to open with the name set into a rule", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("heading must end its own line: %q", got)
	}
}

// The rule is what separates repositories when colour is off — piped to a
// file, a bare name is indistinguishable from diff content. So an uncoloured
// heading must still be visibly a heading.
func TestRepoHeadingWorksWithoutColour(t *testing.T) {
	got := RepoHeading("api/gateway", body, Options{})
	if strings.Contains(got, "\x1b[") {
		t.Errorf("colour is off, but the heading emitted escapes: %q", got)
	}
	if strings.Count(got, "─") < 3 {
		t.Errorf("uncoloured heading needs its rule to read as a heading: %q", got)
	}
}

func TestRepoHeadingColoursNameAndRuleDifferently(t *testing.T) {
	got := RepoHeading("api/gateway", body, Options{Color: true})
	if !strings.Contains(got, bold+blue+"api/gateway") {
		t.Errorf("name should be bold blue, matching the status table's group headings: %q", got)
	}
	if !strings.Contains(got, dim) {
		t.Errorf("rule should be dim, so it reads as scaffolding: %q", got)
	}
}

func TestRepoHeadingASCIIFallback(t *testing.T) {
	got := RepoHeading("api/gateway", body, Options{ASCII: true})
	if strings.Contains(got, "─") {
		t.Errorf("--ascii must not emit box-drawing characters: %q", got)
	}
	if !strings.HasPrefix(got, "-- api/gateway -") {
		t.Errorf("ascii heading = %q, want a dashed rule", got)
	}
}

// The rule ends where the content does, so it needs the body's width — not a
// terminal query, which would be wrong the moment output is piped.
func TestRepoHeadingWidthFollowsTheBody(t *testing.T) {
	wide := strings.Repeat("x", 70) + "\n"
	narrow := "y\n"

	w := utf8.RuneCountInString(strings.TrimRight(RepoHeading("r", wide, Options{}), "\n"))
	n := utf8.RuneCountInString(strings.TrimRight(RepoHeading("r", narrow, Options{}), "\n"))
	if w <= n {
		t.Errorf("a wider body should give a wider rule, got %d vs %d", w, n)
	}
	if n < headingMin {
		t.Errorf("a one-character body gave a %d-wide rule, want at least %d", n, headingMin)
	}

	huge := strings.Repeat("x", 500) + "\n"
	if h := utf8.RuneCountInString(strings.TrimRight(RepoHeading("r", huge, Options{}), "\n")); h > headingMax {
		t.Errorf("a %d-wide rule would run off the terminal; cap is %d", h, headingMax)
	}
}

// A name longer than the cap must not produce a negative repeat count, which
// panics.
func TestRepoHeadingSurvivesAnOverlongName(t *testing.T) {
	long := strings.Repeat("deeply/nested/", 12) + "repo"
	got := RepoHeading(long, body, Options{})
	if !strings.Contains(got, long) {
		t.Errorf("an overlong name must still be shown in full: %q", got)
	}
}

// The body arrives already coloured by git, so measuring it byte-for-byte
// counts escape sequences as screen columns and draws a rule far too long.
//
// The fixture has to be WIDER than headingMin: at 15 visible columns both the
// correct and the naive measurement clamp to the same floor, and the test
// passes against a broken implementation. That is how this test first failed
// to catch the bug it was written for.
func TestRepoHeadingIgnoresAnsiWhenMeasuring(t *testing.T) {
	wide := strings.Repeat("x", 60)
	if len(wide) <= headingMin {
		t.Fatalf("fixture must exceed headingMin (%d) or the clamp hides the bug", headingMin)
	}
	plain := " " + wide + "\n"
	coloured := " \x1b[32m" + wide + "\x1b[m\n"

	p := utf8.RuneCountInString(strings.TrimRight(RepoHeading("r", plain, Options{}), "\n"))
	c := utf8.RuneCountInString(strings.TrimRight(RepoHeading("r", coloured, Options{}), "\n"))
	if p != c {
		t.Errorf("colouring the body changed the rule width (%d vs %d): escapes are being counted as columns", p, c)
	}
}

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"\x1b[32mabc\x1b[m", 3},
		{"\x1b[1m\x1b[34mapi/gateway\x1b[0m", 11},
		{" file.go | 2 \x1b[32m+\x1b[m\x1b[31m-\x1b[m", len(" file.go | 2 +-")},
		{"héllo", 5}, // multi-byte runes are one column each
	}
	for _, tt := range tests {
		if got := visibleWidth(tt.in); got != tt.want {
			t.Errorf("visibleWidth(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
