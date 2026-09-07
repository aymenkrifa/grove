package render

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/aymenkrifa/grove/internal/git"
)

func encode(t *testing.T, root, workspace string, repos []git.Repo) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := JSON(&buf, root, workspace, repos); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	return buf.Bytes()
}

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, b)
	}
	return doc
}

func keys(t *testing.T, m map[string]any) []string {
	t.Helper()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// jsonRepo gives every field a value nothing else has: a distinct number for
// each counter and a distinct string for each name. A field encoded under the
// wrong key then shows up as the wrong value, which a fixture full of zeroes
// and repeated ones would hide. It is not a state a real repo can be in.
func jsonRepo() git.Repo {
	return git.Repo{
		Path: "api/gateway", AbsPath: "/home/user/work/api/gateway", Group: "api",
		Branch: "main", Detached: true, Unborn: true, Bare: true,
		Upstream: "origin/main", Ahead: 2, Behind: 7,
		Staged: 1, Unstaged: 3, Untracked: 4, Conflicted: 6, Stashes: 5,
		Clean: false, Error: "boom",
	}
}

// The document shape is a compatibility surface: fields may be added later,
// but these names and meanings are fixed. The assertion is on the exact key
// set, so a rename fails here rather than in somebody's script.
func TestJSONDocumentKeys(t *testing.T) {
	doc := decode(t, encode(t, "/home/user/work", "work", sample()))
	want := []string{"repos", "root", "summary", "workspace"}
	if got := keys(t, doc); !equal(got, want) {
		t.Errorf("document keys = %q, want %q", got, want)
	}

	sum, ok := doc["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary = %T, want an object", doc["summary"])
	}
	wantSummary := []string{"ahead", "behind", "dirty", "errors", "repos"}
	if got := keys(t, sum); !equal(got, wantSummary) {
		t.Errorf("summary keys = %q, want %q", got, wantSummary)
	}
}

func TestJSONRepoKeys(t *testing.T) {
	doc := decode(t, encode(t, "/root", "", []git.Repo{
		{Path: "api/gateway", Group: "api", Branch: "main", Clean: true},
		{Path: "web/broken", Group: "web", Error: "not a git repository"},
	}))
	repos, ok := doc["repos"].([]any)
	if !ok || len(repos) != 2 {
		t.Fatalf("repos = %#v, want two objects", doc["repos"])
	}

	base := []string{
		"abs_path", "ahead", "bare", "behind", "branch", "clean", "conflicted",
		"detached", "group", "path", "staged", "stashes", "unborn", "unstaged",
		"untracked", "upstream",
	}
	clean := repos[0].(map[string]any)
	if got := keys(t, clean); !equal(got, base) {
		t.Errorf("repo keys = %q, want %q", got, base)
	}
	// "error" is omitted rather than sent as an empty string, so its presence
	// is itself the signal that a repo could not be read.
	if _, present := clean["error"]; present {
		t.Errorf("a repo with no error should not carry an error key: %#v", clean)
	}
	failed := repos[1].(map[string]any)
	withError := append(append([]string{}, base...), "error")
	sort.Strings(withError)
	if got := keys(t, failed); !equal(got, withError) {
		t.Errorf("errored repo keys = %q, want %q", got, withError)
	}
	if failed["error"] != "not a git repository" {
		t.Errorf("error = %#v", failed["error"])
	}
}

func TestJSONRepoFieldValues(t *testing.T) {
	doc := decode(t, encode(t, "/root", "", []git.Repo{jsonRepo()}))
	repo := doc["repos"].([]any)[0].(map[string]any)
	tests := []struct {
		key  string
		want any
	}{
		{"path", "api/gateway"},
		{"abs_path", "/home/user/work/api/gateway"},
		{"group", "api"},
		{"branch", "main"},
		{"detached", true},
		{"unborn", true},
		{"bare", true},
		{"upstream", "origin/main"},
		{"ahead", 2.0},
		{"behind", 7.0},
		{"staged", 1.0},
		{"unstaged", 3.0},
		{"untracked", 4.0},
		{"conflicted", 6.0},
		{"stashes", 5.0},
		{"clean", false},
		{"error", "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := repo[tt.key]; got != tt.want {
				t.Errorf("%q = %#v, want %#v", tt.key, got, tt.want)
			}
		})
	}
}

func TestJSONRootAndWorkspace(t *testing.T) {
	doc := decode(t, encode(t, "/home/user/work", "work", sample()))
	if doc["root"] != "/home/user/work" {
		t.Errorf("root = %#v", doc["root"])
	}
	if doc["workspace"] != "work" {
		t.Errorf("workspace = %#v", doc["workspace"])
	}
}

// An unnamed workspace is left out rather than sent as "": a consumer reading
// the key can then tell "no workspace" from a workspace called "".
func TestJSONOmitsAnEmptyWorkspace(t *testing.T) {
	doc := decode(t, encode(t, "/root", "", nil))
	if _, present := doc["workspace"]; present {
		t.Errorf("workspace should be omitted when empty: %#v", doc)
	}
	if doc["root"] != "/root" {
		t.Errorf("root = %#v", doc["root"])
	}
}

// An empty selection must encode as [], never null, so a consumer can iterate
// without a nil check.
func TestJSONEmptyReposIsAnArray(t *testing.T) {
	for _, tt := range []struct {
		name  string
		repos []git.Repo
	}{
		{"nil", nil},
		{"empty", []git.Repo{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := encode(t, "/root", "work", tt.repos)
			if !bytes.Contains(out, []byte(`"repos": []`)) {
				t.Errorf("repos should encode as an empty array, got\n%s", out)
			}
			doc := decode(t, out)
			if repos, ok := doc["repos"].([]any); !ok || len(repos) != 0 {
				t.Errorf("repos = %#v", doc["repos"])
			}
		})
	}
}

func TestJSONSummaryCounts(t *testing.T) {
	tests := []struct {
		name                                string
		repos                               []git.Repo
		repoN, dirty, ahead, behind, errors float64
	}{
		{"the sample", sample(), 9, 3, 2, 1, 1},
		{"nothing", nil, 0, 0, 0, 0, 0},
		{
			"divergence is not dirt and an error is not divergence",
			[]git.Repo{
				{Ahead: 1, Upstream: "u", Clean: true},
				{Behind: 1, Upstream: "u", Clean: true},
				{Behind: 4, Upstream: "u", Clean: true},
				{Error: "boom"},
				{Untracked: 1},
			},
			5, 1, 1, 2, 1,
		},
		{
			"a stash alone is not dirt",
			[]git.Repo{{Stashes: 3, Clean: true}},
			1, 0, 0, 0, 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sum := decode(t, encode(t, "/root", "work", tt.repos))["summary"].(map[string]any)
			for _, c := range []struct {
				key  string
				want float64
			}{
				{"repos", tt.repoN},
				{"dirty", tt.dirty},
				{"ahead", tt.ahead},
				{"behind", tt.behind},
				{"errors", tt.errors},
			} {
				if got := sum[c.key]; got != c.want {
					t.Errorf("summary.%s = %#v, want %v", c.key, got, c.want)
				}
			}
		})
	}
}

// The summary counts repos, not events: one repo that is ahead and behind at
// once adds one to each total, never two to either.
func TestJSONSummaryCountsReposNotCommits(t *testing.T) {
	sum := decode(t, encode(t, "/root", "", []git.Repo{
		{Upstream: "u", Ahead: 9, Behind: 9, Unstaged: 9, Untracked: 9},
	}))["summary"].(map[string]any)
	for k, want := range map[string]float64{"repos": 1, "dirty": 1, "ahead": 1, "behind": 1} {
		if sum[k] != want {
			t.Errorf("summary.%s = %#v, want %v", k, sum[k], want)
		}
	}
}

func TestJSONIsIndentedAndNewlineTerminated(t *testing.T) {
	out := string(encode(t, "/root", "work", sample()))
	if !strings.HasSuffix(out, "}\n") {
		t.Errorf("output should end with a newline: %q", out[len(out)-10:])
	}
	if !strings.Contains(out, "\n  \"root\": \"/root\"") {
		t.Errorf("output should be indented by two spaces:\n%s", out)
	}
}

func TestJSONReturnsWriteErrors(t *testing.T) {
	err := JSON(failingWriter{}, "/root", "work", sample())
	if err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("JSON() error = %v, want the writer's error", err)
	}
}
