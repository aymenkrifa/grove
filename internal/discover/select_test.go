package discover

import (
	"strings"
	"testing"
)

func repos() []Found {
	return []Found{
		{RelPath: "api/gateway", Group: "api"},
		{RelPath: "api/billing", Group: "api"},
		{RelPath: "web/dashboard", Group: "web"},
		{RelPath: "infra/k8s", Group: "infra"},
	}
}

func TestSelectStages(t *testing.T) {
	tests := []struct {
		name, selector string
		want           []string
	}{
		{"empty selects everything", "", []string{"api/gateway", "api/billing", "web/dashboard", "infra/k8s"}},
		{"exact path", "web/dashboard", []string{"web/dashboard"}},
		{"group expands", "api", []string{"api/gateway", "api/billing"}},
		{"basename", "gateway", []string{"api/gateway"}},
		{"unique substring", "dash", []string{"web/dashboard"}},
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

func TestSelectNoMatch(t *testing.T) {
	_, err := Select(repos(), "nothing")
	if err == nil {
		t.Fatal("Select() with no match should error")
	}
	if !strings.Contains(err.Error(), "grove list") {
		t.Errorf("error %q should suggest `grove list`", err)
	}
}
