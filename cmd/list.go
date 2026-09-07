package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/git"
	"github.com/aymenkrifa/grove/internal/render"
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
				repos := make([]git.Repo, len(found))
				for i, f := range found {
					repos[i] = git.Repo{Path: f.RelPath, AbsPath: f.AbsPath, Group: f.Group}
				}
				return render.JSON(out, res.Root, res.Name, repos)
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
