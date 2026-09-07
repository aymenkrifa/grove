package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/discover"
)

func newListCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:     "list [selector]",
		Aliases: []string{"ls"},
		Short:   "List the repositories grove can see",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, found, err := resolveAndFind(firstArg(args))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				return listJSON(out, res.Root, res.Name, found)
			}
			for _, f := range found {
				fmt.Fprintln(out, f.RelPath)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return c
}

// listRepo is list's own JSON shape, deliberately narrower than git.Repo.
// list never runs git — it only walks the filesystem — so it can only ever
// know where a repository is, never its status. Routing this through
// render.JSON (which expects a fully collected git.Repo) would publish
// "clean": false, an empty branch, and a summary of zero dirty/ahead/behind
// repos for every entry: fields that look like real status but were never
// computed, on a compatibility surface the spec says keeps its meaning.
type listRepo struct {
	Path    string `json:"path"`
	AbsPath string `json:"abs_path"`
	Group   string `json:"group"`
}

type listDocument struct {
	Root      string     `json:"root"`
	Workspace string     `json:"workspace,omitempty"`
	Repos     []listRepo `json:"repos"`
}

func listJSON(w io.Writer, root, workspace string, found []discover.Found) error {
	repos := make([]listRepo, len(found))
	for i, f := range found {
		repos[i] = listRepo{Path: f.RelPath, AbsPath: f.AbsPath, Group: f.Group}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(listDocument{Root: root, Workspace: workspace, Repos: repos})
}
