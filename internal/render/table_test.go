package render

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/git"
)

// sample is deliberately lopsided: every count is a different number, every
// summary total is a different number, and each state (dirty, clean, errored,
// no upstream, ahead, behind, root-level) appears on a different repo. A
// fixture with one of everything cannot tell two swapped axes apart.
func sample() []git.Repo {
	return []git.Repo{
		{Path: "api/gateway", Group: "api", Branch: "feat/ABC-123-token", Upstream: "origin/feat/ABC-123-token",
			Unstaged: 3, Staged: 1, Untracked: 4, Conflicted: 2, Stashes: 5},
		{Path: "api/billing", Group: "api", Branch: "develop", Upstream: "origin/develop", Ahead: 2, Clean: true},
		{Path: "api/search", Group: "api", Branch: "feat/ABC-123-search", Upstream: "origin/feat/ABC-123-search",
			Unstaged: 1, Ahead: 3},
		{Path: "web/dashboard", Group: "web", Branch: "main", Clean: true},
		{Path: "web/broken", Group: "web", Error: "not a git repository"},
		{Path: "web/landing", Group: "web", Branch: "fix/XY-9-typo", Untracked: 2},
		{Path: "infra/k8s", Group: "infra", Branch: "fix/XY-9-retry", Upstream: "origin/fix/XY-9-retry",
			Behind: 7, Clean: true},
		{Path: "infra/terraform", Group: "infra", Branch: "main", Upstream: "origin/main", Clean: true},
		{Path: "notes", Branch: "main", Upstream: "origin/main", Clean: true},
	}
}

func opts() Options {
	return Options{Display: config.Display{GroupBy: "dir"}, ShowClean: true}
}

func render(t *testing.T, repos []git.Repo, o Options) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Table(&buf, repos, o); err != nil {
		t.Fatalf("Table() error = %v", err)
	}
	return buf.String()
}

func lines(out string) []string {
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

// lineStartingWith finds the one line whose first non-space token is want.
// Matching the token rather than looking for the name anywhere in the output
// keeps a repo name from being found inside a branch or a group heading.
func lineStartingWith(t *testing.T, out, want string) string {
	t.Helper()
	var found string
	for _, l := range lines(out) {
		if fields := strings.Fields(l); len(fields) > 0 && fields[0] == want {
			if found != "" {
				t.Fatalf("more than one line starts with %q\n%s", want, out)
			}
			found = l
		}
	}
	if found == "" {
		t.Fatalf("no line starts with %q\n%s", want, out)
	}
	return found
}

func hasLineStartingWith(out, want string) bool {
	for _, l := range lines(out) {
		if fields := strings.Fields(l); len(fields) > 0 && fields[0] == want {
			return true
		}
	}
	return false
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

// --- layout ---------------------------------------------------------------

// goldenSample is small enough that the expected table can be worked out by
// hand, which is the point: the assertion below pins every column position, so
// a value rendered into the wrong column fails even though the same characters
// are still somewhere in the output.
func goldenSample() []git.Repo {
	return []git.Repo{
		{Path: "api/gateway", Group: "api", Branch: "main", Upstream: "origin/main",
			Unstaged: 3, Staged: 1, Untracked: 4, Conflicted: 2, Stashes: 5},
		{Path: "api/billing", Group: "api", Branch: "develop", Upstream: "origin/develop", Ahead: 2, Clean: true},
		{Path: "web/dashboard", Group: "web", Branch: "main", Clean: true},
	}
}

func TestTableGolden(t *testing.T) {
	want := "api\n" +
		"  gateway    main     ~3 +1 ?4 !2 $5  ↑0 ↓0\n" +
		"  billing    develop  clean           ↑2 ↓0\n" +
		"web\n" +
		"  dashboard  main     clean           -\n" +
		"\n" +
		"3 repos · 1 dirty · 1 ahead · 0 behind\n"
	if got := render(t, goldenSample(), opts()); got != want {
		t.Errorf("Table() =\n%q\nwant\n%q", got, want)
	}
}

func TestTableGoldenASCII(t *testing.T) {
	o := opts()
	o.ASCII = true
	want := "api\n" +
		"  gateway    main     ~3 +1 ?4 !2 *5  ^0 v0\n" +
		"  billing    develop  clean           ^2 v0\n" +
		"web\n" +
		"  dashboard  main     clean           -\n" +
		"\n" +
		"3 repos | 1 dirty | 1 ahead | 0 behind\n"
	if got := render(t, goldenSample(), o); got != want {
		t.Errorf("Table() =\n%q\nwant\n%q", got, want)
	}
}

// The colour golden. Written from the constants rather than from raw bytes so
// that it says which colour goes where, and paired with TestEscapeConstants,
// which pins what each constant is. Every other colour test here is indirect —
// Contains, or stripping the escapes back off — and none of them would notice
// an escape emitted in the wrong place on the line.
func TestTableGoldenColor(t *testing.T) {
	o := opts()
	o.Color = true
	want := blue + "api" + reset + "\n" +
		"  gateway    " + dim + "main" + reset + "     " +
		yellow + "~3" + reset + " " + yellow + "+1" + reset + " " + yellow + "?4" + reset + " " +
		red + "!2" + reset + " " + dim + "$5" + reset + "  " +
		dim + "↑0 ↓0" + reset + "\n" +
		"  billing    " + dim + "develop" + reset + "  " +
		green + "clean" + reset + "           " + cyan + "↑2 ↓0" + reset + "\n" +
		blue + "web" + reset + "\n" +
		"  dashboard  " + dim + "main" + reset + "     " +
		green + "clean" + reset + "           " + dim + "-" + reset + "\n" +
		"\n" +
		dim + "3 repos · 1 dirty · 1 ahead · 0 behind" + reset + "\n"
	if got := render(t, goldenSample(), o); got != want {
		t.Errorf("Table() =\n%q\nwant\n%q", got, want)
	}
}

// A group heading is a row of one cell, so it must not widen the name column:
// under a heading longer than any of its members, every row would be pushed
// right by the difference. Every other fixture here has a member name longer
// than its heading, which cannot show the difference.
func TestTableGoldenHeadingLongerThanItsMembers(t *testing.T) {
	repos := []git.Repo{
		{Path: "infrastructure/k8s", Group: "infrastructure", Branch: "main", Upstream: "origin/main", Clean: true},
	}
	want := "infrastructure\n" +
		"  k8s  main  clean  ↑0 ↓0\n" +
		"\n" +
		"1 repos · 0 dirty · 0 ahead · 0 behind\n"
	if got := render(t, repos, opts()); got != want {
		t.Errorf("Table() =\n%q\nwant\n%q", got, want)
	}
}

// Columns are counted in characters, not bytes. Nothing else in the suite puts
// a multi-byte string in a column that gets padded — the arrows live in the
// last column, which never does — so nothing else would notice a repo whose
// name is five bytes longer than it is wide.
func TestTableGoldenNonASCII(t *testing.T) {
	repos := []git.Repo{
		{Path: "web/crème-brûlée", Group: "web", Branch: "feat/café", Upstream: "origin/feat/café", Untracked: 2},
		{Path: "web/api", Group: "web", Branch: "main", Upstream: "origin/main", Ahead: 1, Clean: true},
	}
	want := "web\n" +
		"  crème-brûlée  feat/café  ?2     ↑0 ↓0\n" +
		"  api           main       clean  ↑1 ↓0\n" +
		"\n" +
		"2 repos · 1 dirty · 1 ahead · 0 behind\n"
	if got := render(t, repos, opts()); got != want {
		t.Errorf("Table() =\n%q\nwant\n%q", got, want)
	}
}

// No line may end in whitespace: the last column is unpadded, but the padding
// of an empty cell before it would otherwise trail off the end of the line.
func TestTableHasNoTrailingWhitespace(t *testing.T) {
	for _, l := range lines(render(t, sample(), opts())) {
		if l != strings.TrimRight(l, " \t") {
			t.Errorf("line has trailing whitespace: %q", l)
		}
	}
}

// Columns line up across group headings, not only within a group.
func TestTableColumnsAlignAcrossGroups(t *testing.T) {
	out := render(t, sample(), opts())
	first := strings.Index(lineStartingWith(t, out, "gateway"), "feat/ABC-123-token")
	second := strings.Index(lineStartingWith(t, out, "terraform"), "main")
	if first != second {
		t.Errorf("branch column starts at %d in the api group and %d in the infra group\n%s", first, second, out)
	}
}

// --- content --------------------------------------------------------------

func TestTableRendersEveryRepoOnce(t *testing.T) {
	out := render(t, sample(), opts())
	for _, want := range []string{"gateway", "billing", "search", "dashboard", "broken", "landing", "k8s", "terraform", "notes"} {
		lineStartingWith(t, out, want) // fatals on zero or more than one match
	}
	for _, want := range []string{"api", "web", "infra"} {
		if !hasLineStartingWith(out, want) {
			t.Errorf("missing group heading %q\n%s", want, out)
		}
	}
}

func TestTableRowContents(t *testing.T) {
	out := render(t, sample(), opts())
	tests := []struct {
		name string
		want []string // the whole row, split on runs of two or more spaces
	}{
		{"gateway", []string{"gateway", "feat/ABC-123-token", "~3 +1 ?4 !2 $5", "↑0 ↓0"}},
		{"billing", []string{"billing", "develop", "clean", "↑2 ↓0"}},
		{"search", []string{"search", "feat/ABC-123-search", "~1", "↑3 ↓0"}},
		{"dashboard", []string{"dashboard", "main", "clean", "-"}},
		{"landing", []string{"landing", "fix/XY-9-typo", "?2", "-"}},
		{"k8s", []string{"k8s", "fix/XY-9-retry", "clean", "↑0 ↓7"}},
		{"notes", []string{"notes", "main", "clean", "↑0 ↓0"}},
	}
	gap := regexp.MustCompile(` {2,}`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gap.Split(strings.TrimSpace(lineStartingWith(t, out, tt.name)), -1)
			want := tt.want
			if len(got) != len(want) {
				t.Fatalf("row = %q, want %q", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("column %d = %q, want %q (row %q)", i, got[i], want[i], got)
				}
			}
		})
	}
}

// The errored repo has no branch and no divergence, so its row cannot be split
// on whitespace runs without losing which column the message sits in. Its
// position is checked against the columns of a neighbouring row instead.
func TestTableErrorRowUsesTheStateColumn(t *testing.T) {
	out := render(t, sample(), opts())
	stateCol := strings.Index(lineStartingWith(t, out, "dashboard"), "clean")
	errLine := lineStartingWith(t, out, "broken")
	if got := strings.Index(errLine, "not a git repository"); got != stateCol {
		t.Errorf("the error message starts at %d, want the state column at %d\n%s", got, stateCol, out)
	}
	if strings.TrimSpace(errLine[:stateCol]) != "broken" {
		t.Errorf("the branch column of an errored repo should be blank, got %q", errLine[:stateCol])
	}
}

func TestTableSummaryLine(t *testing.T) {
	out := render(t, sample(), opts())
	want := "9 repos · 3 dirty · 2 ahead · 1 behind · 1 errored"
	last := lines(out)[len(lines(out))-1]
	if last != want {
		t.Errorf("summary = %q, want %q", last, want)
	}
	if blank := lines(out)[len(lines(out))-2]; blank != "" {
		t.Errorf("the summary should be preceded by a blank line, got %q", blank)
	}
}

// "errored" is omitted entirely when nothing failed, so a run with no errors
// does not report a zero that invites a second look.
func TestTableSummaryOmitsErroredWhenNoneFailed(t *testing.T) {
	out := render(t, goldenSample(), opts())
	if strings.Contains(out, "errored") {
		t.Errorf("no repo errored, so the summary should not mention it\n%s", out)
	}
}

func TestSummaryLineCounts(t *testing.T) {
	tests := []struct {
		name  string
		repos []git.Repo
		want  string
	}{
		{"nothing at all", nil, "0 repos · 0 dirty · 0 ahead · 0 behind"},
		{"one of each", sample(), "9 repos · 3 dirty · 2 ahead · 1 behind · 1 errored"},
		{
			"divergence alone is not dirt",
			[]git.Repo{{Ahead: 1, Upstream: "u", Clean: true}, {Behind: 1, Upstream: "u", Clean: true}},
			"2 repos · 0 dirty · 1 ahead · 1 behind",
		},
		{
			"a repo both ahead and behind counts in both",
			[]git.Repo{{Ahead: 1, Behind: 1, Upstream: "u", Clean: true}},
			"1 repos · 0 dirty · 1 ahead · 1 behind",
		},
		{
			"an errored repo is still a repo",
			[]git.Repo{{Error: "boom"}, {Error: "bang"}, {Clean: true}},
			"3 repos · 0 dirty · 0 ahead · 0 behind · 2 errored",
		},
		{
			"a stash alone is not dirt",
			[]git.Repo{{Stashes: 4, Clean: true}},
			"1 repos · 0 dirty · 0 ahead · 0 behind",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summaryLine(tt.repos, symbolsFor(false)); got != tt.want {
				t.Errorf("summaryLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- the branch column ----------------------------------------------------

func TestBranchColumn(t *testing.T) {
	tests := []struct {
		name string
		repo git.Repo
		want string
	}{
		{"a branch", git.Repo{Branch: "main"}, "main"},
		// The two rows that would otherwise be identical: the same seven
		// characters, once as a detached SHA and once as a branch name.
		{"a detached head", git.Repo{Branch: "a1b2c3d", Detached: true}, "(a1b2c3d)"},
		{"a branch that looks like a sha", git.Repo{Branch: "a1b2c3d"}, "a1b2c3d"},
		{"a long sha is not shortened here", git.Repo{Branch: "a1b2c3d4e5f6", Detached: true}, "(a1b2c3d4e5f6)"},
		// An errored repo has no head at all; empty parentheses would be worse
		// than nothing.
		{"detached with nothing to show", git.Repo{Detached: true}, ""},
		{"no branch", git.Repo{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := branch(tt.repo, painter{}).plain; got != tt.want {
				t.Errorf("branch() = %q, want %q", got, tt.want)
			}
			// Both forms of the cell are dim; a detached head is marked by
			// the parentheses, not by dropping out of the column's colour.
			wantPainted := tt.want
			if wantPainted != "" {
				wantPainted = dim + wantPainted + reset
			}
			if got := branch(tt.repo, painter{on: true}).painted; got != wantPainted {
				t.Errorf("painted branch() = %q, want %q", got, wantPainted)
			}
		})
	}
}

// The parenthesised form is two columns wider than the SHA it wraps, so the
// column has to be measured after the parentheses are added, not before.
func TestTableGoldenDetached(t *testing.T) {
	repos := []git.Repo{
		{Path: "api/gateway", Group: "api", Branch: "a1b2c3d", Detached: true, Clean: true},
		{Path: "api/billing", Group: "api", Branch: "develop", Upstream: "origin/develop", Clean: true},
	}
	want := "api\n" +
		"  gateway  (a1b2c3d)  clean  -\n" +
		"  billing  develop    clean  ↑0 ↓0\n" +
		"\n" +
		"2 repos · 0 dirty · 0 ahead · 0 behind\n"
	if got := render(t, repos, opts()); got != want {
		t.Errorf("Table() =\n%q\nwant\n%q", got, want)
	}
}

// --- the state column -----------------------------------------------------

func TestStateColumn(t *testing.T) {
	tests := []struct {
		name string
		repo git.Repo
		want string
	}{
		{"clean", git.Repo{Clean: true}, "clean"},
		{"unstaged only", git.Repo{Unstaged: 3}, "~3"},
		{"staged only", git.Repo{Staged: 1}, "+1"},
		{"untracked only", git.Repo{Untracked: 4}, "?4"},
		{"conflicted only", git.Repo{Conflicted: 2}, "!2"},
		{"stashed only", git.Repo{Stashes: 5}, "$5"},
		// Distinct counts throughout, so a symbol paired with the wrong
		// counter shows up.
		{"all of them, in order", git.Repo{Unstaged: 3, Staged: 1, Untracked: 4, Conflicted: 2, Stashes: 5}, "~3 +1 ?4 !2 $5"},
		{"a stash on an otherwise clean tree", git.Repo{Stashes: 2, Clean: true}, "$2"},
		{"an error replaces the state", git.Repo{Error: "not a git repository", Unstaged: 9}, "not a git repository"},
		{"bare", git.Repo{Bare: true}, "bare"},
		{"unborn", git.Repo{Unborn: true, Branch: "main"}, "no commits"},
		{"an error outranks bare", git.Repo{Bare: true, Error: "boom"}, "boom"},
		{"bare outranks unborn", git.Repo{Bare: true, Unborn: true}, "bare"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := state(tt.repo, painter{}, symbolsFor(false)).plain; got != tt.want {
				t.Errorf("state() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateColumnASCII(t *testing.T) {
	r := git.Repo{Unstaged: 3, Stashes: 5}
	if got := state(r, painter{}, symbolsFor(true)).plain; got != "~3 *5" {
		t.Errorf("ascii state() = %q, want %q", got, "~3 *5")
	}
	if got := state(r, painter{}, symbolsFor(false)).plain; got != "~3 $5" {
		t.Errorf("unicode state() = %q, want %q", got, "~3 $5")
	}
}

// --- the divergence column ------------------------------------------------

func TestDivergenceColumn(t *testing.T) {
	tests := []struct {
		name          string
		repo          git.Repo
		unicode, asci string
	}{
		{"level with upstream", git.Repo{Upstream: "origin/main"}, "↑0 ↓0", "^0 v0"},
		{"ahead", git.Repo{Upstream: "origin/main", Ahead: 2}, "↑2 ↓0", "^2 v0"},
		{"behind", git.Repo{Upstream: "origin/main", Behind: 7}, "↑0 ↓7", "^0 v7"},
		{"both, and not the same number", git.Repo{Upstream: "origin/main", Ahead: 2, Behind: 7}, "↑2 ↓7", "^2 v7"},
		// No upstream is not the same as level with one, and the difference
		// matters: nothing has been pushed anywhere.
		{"no upstream", git.Repo{Branch: "wip", Ahead: 2, Behind: 7}, "-", "-"},
		{"errored", git.Repo{Error: "boom", Upstream: "origin/main", Ahead: 2}, "", ""},
		{"bare", git.Repo{Bare: true, Upstream: "origin/main", Ahead: 2}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := divergence(tt.repo, painter{}, symbolsFor(false)).plain; got != tt.unicode {
				t.Errorf("divergence() = %q, want %q", got, tt.unicode)
			}
			if got := divergence(tt.repo, painter{}, symbolsFor(true)).plain; got != tt.asci {
				t.Errorf("ascii divergence() = %q, want %q", got, tt.asci)
			}
		})
	}
}

// --- symbols --------------------------------------------------------------

func TestSymbolsFor(t *testing.T) {
	u, a := symbolsFor(false), symbolsFor(true)
	if u != (symbols{ahead: "↑", behind: "↓", stash: "$", sep: " · "}) {
		t.Errorf("unicode symbols = %+v", u)
	}
	if a != (symbols{ahead: "^", behind: "v", stash: "*", sep: " | "}) {
		t.Errorf("ascii symbols = %+v", a)
	}
}

func TestTableASCIIEmitsNoUnicode(t *testing.T) {
	o := opts()
	o.ASCII = true
	out := render(t, sample(), o)
	if strings.ContainsAny(out, "↑↓·$") {
		t.Errorf("ASCII mode must not emit unicode symbols\n%s", out)
	}
	for _, want := range []string{"^2 v0", "^0 v7", "*5", " | "} {
		if !strings.Contains(out, want) {
			t.Errorf("ASCII output is missing %q\n%s", want, out)
		}
	}
}

func TestTableUnicodeByDefault(t *testing.T) {
	out := render(t, sample(), opts())
	if strings.ContainsAny(out, "^*|") {
		t.Errorf("unicode mode must not emit the ASCII fallbacks\n%s", out)
	}
	for _, want := range []string{"↑2 ↓0", "↑0 ↓7", "$5", " · "} {
		if !strings.Contains(out, want) {
			t.Errorf("unicode output is missing %q\n%s", want, out)
		}
	}
}

// --- colour ---------------------------------------------------------------

func TestTableEmitsNoEscapesWhenColorIsOff(t *testing.T) {
	if out := render(t, sample(), opts()); strings.Contains(out, "\x1b[") {
		t.Errorf("Options.Color is false, so no escape sequences should be emitted\n%q", out)
	}
}

func TestTableColorPaintsEachColumn(t *testing.T) {
	o := opts()
	o.Color = true
	out := render(t, sample(), o)
	tests := []struct {
		name, want string
	}{
		{"the group heading is blue", blue + "api" + reset},
		{"a clean tree is green", green + "clean" + reset},
		{"an error is red", red + "not a git repository" + reset},
		{"a conflict count is red", red + "!2" + reset},
		{"the other counts are yellow", yellow + "~3" + reset},
		{"a stash count is dim", dim + "$5" + reset},
		{"the branch is dim", dim + "develop" + reset},
		{"divergence is cyan when a repo is ahead", cyan + "↑2 ↓0" + reset},
		{"divergence is cyan when a repo is only behind", cyan + "↑0 ↓7" + reset},
		{"divergence is dim when it is level", dim + "↑0 ↓0" + reset},
		{"a missing upstream is dim", dim + "-" + reset},
		{"the summary is dim", dim + "9 repos · 3 dirty · 2 ahead · 1 behind · 1 errored" + reset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(out, tt.want) {
				t.Errorf("output is missing %q\n%q", tt.want, out)
			}
		})
	}
}

// Colour must not move anything: escapes take no screen columns, so stripping
// them from a coloured table has to give back the uncoloured one exactly. This
// is what text/tabwriter cannot do — it counts an escape sequence as
// characters and pads the row short by their length.
func TestTableColorDoesNotDisturbAlignment(t *testing.T) {
	o := opts()
	plain := render(t, sample(), o)
	o.Color = true
	coloured := render(t, sample(), o)
	if coloured == plain {
		t.Fatal("colour was requested but nothing was painted")
	}
	if got := stripANSI(coloured); got != plain {
		t.Errorf("stripped colour output does not match the plain one:\n%q\nwant\n%q", got, plain)
	}
}

// --- filtering ------------------------------------------------------------

func TestTableHidesCleanWhenAsked(t *testing.T) {
	o := opts()
	o.ShowClean = false
	out := render(t, sample(), o)

	for _, hidden := range []string{"dashboard", "terraform", "notes"} {
		if hasLineStartingWith(out, hidden) {
			t.Errorf("%q is clean and level, so it should be hidden\n%s", hidden, out)
		}
	}
	for _, shown := range []string{"gateway", "billing", "search", "broken", "landing", "k8s"} {
		if !hasLineStartingWith(out, shown) {
			t.Errorf("%q needs attention, so it should be shown\n%s", shown, out)
		}
	}
	// A group whose repos were all hidden loses its heading with them.
	if hasLineStartingWith(out, "notes") {
		t.Error("a hidden root-level repo should leave nothing behind")
	}
	// The summary still counts the whole selection, filtered or not.
	if last := lines(out)[len(lines(out))-1]; last != "9 repos · 3 dirty · 2 ahead · 1 behind · 1 errored" {
		t.Errorf("summary = %q, want the counts for all nine repos", last)
	}
}

func TestTableShowsCleanByDefaultOfTheOption(t *testing.T) {
	out := render(t, sample(), opts())
	for _, shown := range []string{"dashboard", "terraform", "notes"} {
		if !hasLineStartingWith(out, shown) {
			t.Errorf("ShowClean is on, so %q should be shown\n%s", shown, out)
		}
	}
}

func TestTableDropsAnEmptyGroupHeading(t *testing.T) {
	o := opts()
	o.ShowClean = false
	repos := []git.Repo{
		{Path: "api/gateway", Group: "api", Branch: "main", Clean: true, Upstream: "origin/main"},
		{Path: "web/landing", Group: "web", Branch: "main", Untracked: 1},
	}
	out := render(t, repos, o)
	if hasLineStartingWith(out, "api") {
		t.Errorf("the api group has no visible repo, so its heading should be gone\n%s", out)
	}
	if !hasLineStartingWith(out, "web") {
		t.Errorf("the web group still has a repo\n%s", out)
	}
}

// --- grouping -------------------------------------------------------------

func TestTableGroupByNone(t *testing.T) {
	o := opts()
	o.Display.GroupBy = "none"
	out := render(t, sample(), o)

	for _, heading := range []string{"api", "web", "infra"} {
		if hasLineStartingWith(out, heading) {
			t.Errorf("group_by = none, so %q should not head a line\n%s", heading, out)
		}
	}
	// Without a heading the group is no longer implied, so the full path is
	// shown and nothing is indented.
	if got := lineStartingWith(t, out, "api/gateway"); strings.HasPrefix(got, " ") {
		t.Errorf("rows should not be indented without a heading: %q", got)
	}
	for _, want := range []string{"api/billing", "web/dashboard", "infra/k8s", "notes"} {
		if !hasLineStartingWith(out, want) {
			t.Errorf("missing full path %q\n%s", want, out)
		}
	}
}

func TestTableGroupByDirUsesTheRelativeName(t *testing.T) {
	out := render(t, sample(), opts())
	if hasLineStartingWith(out, "api/gateway") {
		t.Errorf("under a dir heading the group segment is dropped from the name\n%s", out)
	}
	if got := lineStartingWith(t, out, "gateway"); !strings.HasPrefix(got, indent+"gateway") {
		t.Errorf("row = %q, want it indented under its heading", got)
	}
	// A repo at the root of the workspace has no group, so it keeps its path
	// and stays flush left.
	if got := lineStartingWith(t, out, "notes"); strings.HasPrefix(got, " ") {
		t.Errorf("a root-level repo should not be indented: %q", got)
	}
}

func TestTableGroupByBranchPrefix(t *testing.T) {
	o := opts()
	o.Display.GroupBy = "branch-prefix"
	o.Display.BranchPrefix = `[A-Z]+-[0-9]+`
	out := render(t, sample(), o)

	// Headings appear once each, in first-appearance order, even though the
	// repos sharing one are not adjacent in the input.
	var headings []string
	for _, l := range lines(out) {
		if l != "" && !strings.HasPrefix(l, indent) && !strings.Contains(l, " repos ") {
			headings = append(headings, strings.TrimSpace(l))
		}
	}
	want := []string{"ABC-123", "(none)", "XY-9"}
	if len(headings) != len(want) {
		t.Fatalf("headings = %q, want %q\n%s", headings, want, out)
	}
	for i := range want {
		if headings[i] != want[i] {
			t.Errorf("heading %d = %q, want %q\n%s", i, headings[i], want[i], out)
		}
	}

	// Membership: the repo rows between one heading and the next.
	members := map[string][]string{}
	current := ""
	for _, l := range lines(out) {
		switch {
		case l == "" || strings.Contains(l, " repos "):
		case !strings.HasPrefix(l, indent):
			current = strings.TrimSpace(l)
		default:
			members[current] = append(members[current], strings.Fields(l)[0])
		}
	}
	wantMembers := map[string][]string{
		"ABC-123": {"api/gateway", "api/search"},
		"(none)":  {"api/billing", "web/dashboard", "web/broken", "infra/terraform", "notes"},
		"XY-9":    {"web/landing", "infra/k8s"},
	}
	for g, w := range wantMembers {
		got := members[g]
		if strings.Join(got, ",") != strings.Join(w, ",") {
			t.Errorf("group %q holds %q, want %q\n%s", g, got, w, out)
		}
	}
}

func TestTableBranchPrefixWithABadPatternStillRenders(t *testing.T) {
	o := opts()
	o.Display.GroupBy = "branch-prefix"
	o.Display.BranchPrefix = `[A-Z` // never compiles
	out := render(t, sample(), o)
	if !hasLineStartingWith(out, "(none)") {
		t.Errorf("an invalid pattern should group everything under %q\n%s", noGroup, out)
	}
	for _, l := range lines(out) {
		if l != "" && !strings.HasPrefix(l, indent) && !strings.Contains(l, " repos ") && strings.TrimSpace(l) != noGroup {
			t.Errorf("unexpected second heading %q\n%s", l, out)
		}
	}
	if !hasLineStartingWith(out, "api/gateway") {
		t.Errorf("every repo should still be listed\n%s", out)
	}
}

func TestNameColumn(t *testing.T) {
	tests := []struct {
		name    string
		repo    git.Repo
		groupBy string
		want    string
	}{
		{"dir grouping trims the group segment", git.Repo{Path: "api/gateway", Group: "api"}, "dir", "gateway"},
		{"only the leading segment is trimmed", git.Repo{Path: "api/team/api", Group: "api"}, "dir", "team/api"},
		{"a root repo keeps its path", git.Repo{Path: "notes"}, "dir", "notes"},
		{"no grouping keeps the path", git.Repo{Path: "api/gateway", Group: "api"}, "none", "api/gateway"},
		{"branch grouping keeps the path", git.Repo{Path: "api/gateway", Group: "api"}, "branch-prefix", "api/gateway"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := name(tt.repo, config.Display{GroupBy: tt.groupBy})
			if got != tt.want {
				t.Errorf("name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- edges ----------------------------------------------------------------

func TestTableWithNoRepos(t *testing.T) {
	out := render(t, nil, opts())
	if out != "0 repos · 0 dirty · 0 ahead · 0 behind\n" {
		t.Errorf("Table() with no repos = %q", out)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk on fire") }

func TestTableReturnsWriteErrors(t *testing.T) {
	err := Table(failingWriter{}, sample(), opts())
	if err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("Table() error = %v, want the writer's error", err)
	}
}

// The explain column is off unless asked for, and when asked for it must
// occupy a real column: the mutation that appends it to the *state* cell
// instead of as its own passes any test that only greps for the words.
func TestTableExplainColumn(t *testing.T) {
	repos := []git.Repo{
		{Path: "api/gateway", Group: "api", Branch: "main", Unstaged: 4, Untracked: 3, Upstream: "origin/main"},
		{Path: "api/billing", Group: "api", Branch: "develop", Clean: true, Upstream: "origin/develop"},
		{Path: "web/search", Group: "web", Branch: "main", Upstream: "origin/main", Behind: 3},
	}

	off := opts()
	var plain bytes.Buffer
	if err := Table(&plain, repos, off); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "modified") {
		t.Errorf("explain is off by default, but the table describes state in words:\n%s", plain.String())
	}

	on := opts()
	on.Explain = true
	var buf bytes.Buffer
	if err := Table(&buf, repos, on); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	gateway := lineStartingWith(t, out, "gateway")
	if !strings.HasSuffix(gateway, "4 modified, 3 untracked") {
		t.Errorf("description must be the last column of its row, got %q", gateway)
	}
	// The divergence cell still precedes it, so the description did not
	// swallow or replace a column.
	if !strings.Contains(gateway, "↑0 ↓0") {
		t.Errorf("divergence column lost when explain is on: %q", gateway)
	}

	if got := lineStartingWith(t, out, "search"); !strings.HasSuffix(got, "3 to pull") {
		t.Errorf("behind-only repo = %q, want it to end with %q", got, "3 to pull")
	}

	// A clean repo keeps an empty cell, and trailing padding must not survive
	// into the line — the same right-trim rule every other row obeys.
	billing := lineStartingWith(t, out, "billing")
	if billing != strings.TrimRight(billing, " ") {
		t.Errorf("clean row carries trailing padding from the empty description: %q", billing)
	}
	if strings.Contains(billing, "modified") || strings.Contains(billing, "pull") {
		t.Errorf("clean repo should have an empty description, got %q", billing)
	}
}
