package discover

import (
	"strings"
	"testing"
)

// The fixture is deliberately adversarial about the stage order:
//
//   - "api/gateway" and "api/gateway-v2" share a prefix, so an exact path and
//     an exact basename are both substrings of two entries. Without stages 1
//     and 3 the substring fallback would call them ambiguous.
//   - "api/billing" and "web/billing" share a basename, so stage 3 has a real
//     ambiguity to report.
//   - "notes" sits at the root with an empty Group, which is what an
//     un-normalised empty selector would wrongly match in stage 2.
func repos() []Found {
	return []Found{
		{RelPath: "api/gateway", Group: "api"},
		{RelPath: "api/gateway-v2", Group: "api"},
		{RelPath: "api/billing", Group: "api"},
		{RelPath: "web/dashboard", Group: "web"},
		{RelPath: "web/billing", Group: "web"},
		{RelPath: "infra/k8s", Group: "infra"},
		{RelPath: "notes", Group: ""},
	}
}

func TestSelectStages(t *testing.T) {
	all := []string{"api/gateway", "api/gateway-v2", "api/billing", "web/dashboard", "web/billing", "infra/k8s", "notes"}
	tests := []struct {
		name, selector string
		want           []string
	}{
		{"empty selects everything", "", all},
		{"exact path", "web/dashboard", []string{"web/dashboard"}},
		// Stage 1 must win: "api/gateway" is a substring of "api/gateway-v2"
		// too, so the fallback alone would report an ambiguity.
		{"exact path beats the substring fallback", "api/gateway", []string{"api/gateway"}},
		{"group expands", "api", []string{"api/gateway", "api/gateway-v2", "api/billing"}},
		// Stage 3 must win, for the same reason.
		{"basename beats the substring fallback", "gateway", []string{"api/gateway"}},
		{"unique substring", "dash", []string{"web/dashboard"}},
		{"trailing slash is trimmed", "web/dashboard/", []string{"web/dashboard"}},
		{"leading ./ is trimmed", "./web/dashboard", []string{"web/dashboard"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Select(repos(), tt.selector)
			if err != nil {
				t.Fatalf("Select(%q) error = %v", tt.selector, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Select(%q) = %v, want %v", tt.selector, rels(got), tt.want)
			}
			for i := range tt.want {
				if got[i].RelPath != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i].RelPath, tt.want[i])
				}
			}
		})
	}
}

// "./" and "/" normalise to nothing, which names the root — every repository.
// The bug this pins is subtle and silent: checking for an empty selector before
// trimming lets "./" reach stage 2, where it matches the empty Group of every
// root-level repo and returns that subset with no error at all.
func TestSelectRootSelectorsSelectEverything(t *testing.T) {
	for _, sel := range []string{"", "./", "/"} {
		got, err := Select(repos(), sel)
		if err != nil {
			t.Fatalf("Select(%q) error = %v", sel, err)
		}
		if len(got) != len(repos()) {
			t.Errorf("Select(%q) = %v, want all %d repositories — a root selector must not "+
				"silently narrow to the root-level ones", sel, rels(got), len(repos()))
		}
	}
}

func TestSelectAmbiguousListsCandidates(t *testing.T) {
	_, err := Select(repos(), "a")
	if err == nil {
		t.Fatal("Select(\"a\") should be ambiguous")
	}
	for _, want := range []string{"api/gateway", "api/billing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name candidate %q", err, want)
		}
	}
}

// A basename shared by two repositories is an ambiguity, not a pick-the-first.
func TestSelectAmbiguousBasenameIsAnError(t *testing.T) {
	got, err := Select(repos(), "billing")
	if err == nil {
		t.Fatalf("Select(\"billing\") = %v, want an ambiguity error — two repos share that basename", rels(got))
	}
	for _, want := range []string{"api/billing", "web/billing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name candidate %q", err, want)
		}
	}
}

func TestSelectNoMatch(t *testing.T) {
	_, err := Select(repos(), "nothing")
	if err == nil {
		t.Fatal("Select() with no match should error")
	}
	if !strings.Contains(err.Error(), "grove list") {
		t.Errorf("error %q should suggest `grove list`", err)
	}
}
