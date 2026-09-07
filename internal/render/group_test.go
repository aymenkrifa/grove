package render

import (
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/git"
)

func TestBranchPrefix(t *testing.T) {
	const ticket = `[A-Z]+-[0-9]+`
	tests := []struct {
		name, branch, pattern, want string
	}{
		{"a leading ticket", "ABC-123-token-refresh", ticket, "ABC-123"},
		{"a ticket after a type prefix", "feat/ABC-123-token", ticket, "ABC-123"},
		{"the leftmost match wins", "feat/ABC-123/XY-9", ticket, "ABC-123"},
		{"no match at all", "develop", ticket, noGroup},
		{"an empty branch", "", ticket, noGroup},
		// A repo whose branch does not match is filed under "(none)" rather
		// than under its own branch name, which would give one heading per
		// repo and no grouping at all.
		{"the branch is never the heading", "main", ticket, noGroup},
		// Configuring no pattern is not an error; it just groups nothing.
		{"no pattern", "feat/ABC-123", "", noGroup},
		// A typo in a config file must not stop grove reporting status.
		{"an unparseable pattern", "feat/ABC-123", "[A-Z", noGroup},
		{"another unparseable pattern", "feat/ABC-123", "a(b", noGroup},
		// A pattern that can match nothing at all matches emptily everywhere;
		// an empty heading would print as a blank line, so it is "(none)".
		{"a pattern that matches the empty string", "develop", "[A-Z]*", noGroup},
		{"a slash-terminated prefix", "feature/login", `^[a-z]+/`, "feature/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := branchPrefix(tt.branch, tt.pattern); got != tt.want {
				t.Errorf("branchPrefix(%q, %q) = %q, want %q", tt.branch, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestGroupOf(t *testing.T) {
	r := git.Repo{Path: "api/gateway", Group: "api", Branch: "feat/ABC-123-token"}
	tests := []struct {
		name    string
		display config.Display
		want    string
	}{
		{"dir uses the first path segment", config.Display{GroupBy: "dir"}, "api"},
		{"none groups nothing", config.Display{GroupBy: "none"}, ""},
		{"branch-prefix uses the branch", config.Display{GroupBy: "branch-prefix", BranchPrefix: `[A-Z]+-[0-9]+`}, "ABC-123"},
		// An unrecognised mode behaves like the documented default rather than
		// producing an empty heading.
		{"an unknown mode falls back to dir", config.Display{GroupBy: "sideways"}, "api"},
		{"an empty mode falls back to dir", config.Display{}, "api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupOf(r, tt.display); got != tt.want {
				t.Errorf("groupOf() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The bucketing is what makes branch-prefix grouping work: two repos on the
// same ticket sit in different directories, so they are not adjacent in
// discovery order. Walking the list and starting a new heading whenever the
// key changes would print the same heading several times.
func TestGroupedBucketsNonAdjacentRepos(t *testing.T) {
	repos := []git.Repo{
		{Path: "api/gateway", Group: "api", Branch: "feat/ABC-123-token"},
		{Path: "api/billing", Group: "api", Branch: "develop"},
		{Path: "web/landing", Group: "web", Branch: "feat/ABC-123-form"},
		{Path: "web/dashboard", Group: "web", Branch: "develop"},
	}
	d := config.Display{GroupBy: "branch-prefix", BranchPrefix: `[A-Z]+-[0-9]+`}
	order, buckets := grouped(repos, d)

	if strings.Join(order, ",") != "ABC-123,(none)" {
		t.Errorf("order = %q, want first-appearance order [ABC-123 (none)]", order)
	}
	if len(buckets) != 2 {
		t.Fatalf("buckets = %d, want 2", len(buckets))
	}
	want := map[string][]string{
		"ABC-123": {"api/gateway", "web/landing"},
		"(none)":  {"api/billing", "web/dashboard"},
	}
	for g, paths := range want {
		var got []string
		for _, r := range buckets[g] {
			got = append(got, r.Path)
		}
		if strings.Join(got, ",") != strings.Join(paths, ",") {
			t.Errorf("bucket %q = %q, want %q", g, got, paths)
		}
	}
}

func TestGroupedKeepsInputOrder(t *testing.T) {
	repos := []git.Repo{
		{Path: "web/dashboard", Group: "web"},
		{Path: "api/gateway", Group: "api"},
		{Path: "web/landing", Group: "web"},
		{Path: "notes"},
	}
	order, buckets := grouped(repos, config.Display{GroupBy: "dir"})
	if strings.Join(order, ",") != "web,api," {
		t.Errorf("order = %q, want the order the groups first appear in", order)
	}
	if len(buckets["web"]) != 2 || buckets["web"][0].Path != "web/dashboard" {
		t.Errorf("web bucket = %+v", buckets["web"])
	}
	// A repo at the root of the workspace has no group; the empty heading is a
	// real bucket, it just prints without a heading line.
	if len(buckets[""]) != 1 || buckets[""][0].Path != "notes" {
		t.Errorf("root bucket = %+v", buckets[""])
	}
}

func TestGroupedWithNoGrouping(t *testing.T) {
	order, buckets := grouped(sample(), config.Display{GroupBy: "none"})
	if len(order) != 1 || order[0] != "" {
		t.Fatalf("order = %q, want a single unnamed bucket", order)
	}
	if len(buckets[""]) != len(sample()) {
		t.Errorf("bucket holds %d repos, want %d", len(buckets[""]), len(sample()))
	}
}
