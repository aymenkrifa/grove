package render

import (
	"encoding/json"
	"io"

	"github.com/aymenkrifa/grove/internal/git"
)

// document is the machine-readable form described in the design, §7. The field
// names are a compatibility surface: fields may be added later, but the ones
// here keep their names and their meaning.
type document struct {
	Root      string     `json:"root"`
	Workspace string     `json:"workspace,omitempty"`
	Repos     []git.Repo `json:"repos"`
	Summary   summary    `json:"summary"`
}

type summary struct {
	Repos  int `json:"repos"`
	Dirty  int `json:"dirty"`
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
	Errors int `json:"errors"`
}

// JSON writes the document for repos, indented so it stays readable when a
// human is the one reading it.
func JSON(w io.Writer, root, workspace string, repos []git.Repo) error {
	if repos == nil {
		// An empty selection is an empty list, never null: a consumer should
		// not have to special-case "no repos" before iterating.
		repos = []git.Repo{}
	}
	doc := document{Root: root, Workspace: workspace, Repos: repos}
	doc.Summary.Repos = len(repos)
	for _, r := range repos {
		if r.Dirty() {
			doc.Summary.Dirty++
		}
		if r.Ahead > 0 {
			doc.Summary.Ahead++
		}
		if r.Behind > 0 {
			doc.Summary.Behind++
		}
		if r.Error != "" {
			doc.Summary.Errors++
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
