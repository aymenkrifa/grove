// Package git runs the git binary and turns its porcelain output into values.
package git

// Repo is one repository's state. The JSON tags are a compatibility surface:
// fields may be added, but existing ones keep their meaning.
type Repo struct {
	Path       string `json:"path"` // relative to root
	AbsPath    string `json:"abs_path"`
	Group      string `json:"group"`
	Branch     string `json:"branch"`
	Detached   bool   `json:"detached"`
	Unborn     bool   `json:"unborn"`
	Bare       bool   `json:"bare"`
	Upstream   string `json:"upstream"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
	Staged     int    `json:"staged"`
	Unstaged   int    `json:"unstaged"`
	Untracked  int    `json:"untracked"`
	Conflicted int    `json:"conflicted"`
	Stashes    int    `json:"stashes"`
	Clean      bool   `json:"clean"`
	Error      string `json:"error,omitempty"`
}

// HasUpstream reports whether divergence numbers are meaningful.
func (r Repo) HasUpstream() bool { return r.Upstream != "" }

// Dirty reports whether the working tree has any change at all.
func (r Repo) Dirty() bool {
	return r.Staged+r.Unstaged+r.Untracked+r.Conflicted > 0
}

// NeedsAttention is the filter behind `grove status -d`.
func (r Repo) NeedsAttention() bool {
	return r.Dirty() || r.Ahead > 0 || r.Behind > 0 || r.Error != ""
}
