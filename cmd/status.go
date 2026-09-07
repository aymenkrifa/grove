package cmd

import (
	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/git"
	"github.com/aymenkrifa/grove/internal/render"
)

func newStatusCmd() *cobra.Command {
	var (
		dirtyOnly bool
		asJSON    bool
		noStash   bool
	)
	c := &cobra.Command{
		Use:     "status [selector]",
		Aliases: []string{"st"},
		Short:   "Show branch and working-tree state for every repository",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, found, err := resolveAndFind(firstArg(args))
			if err != nil {
				return err
			}
			repos := git.Collect(cmd.Context(), found, git.CollectOpts{
				Jobs: flagJobs,
				// Counting stashes costs an extra git call per repository, so
				// it is decided before collection rather than at render time:
				// switching it off has to save the work, not just hide it.
				Stash: res.Display.ShowStash && !noStash,
			})
			for _, r := range repos {
				if r.Error != "" {
					// One broken repository still gets its row; it only
					// changes the exit code.
					markPartialFailure()
				}
			}
			out := cmd.OutOrStdout()
			if asJSON {
				return render.JSON(out, res.Root, res.Name, repos)
			}
			o := renderOptions(res, out)
			if dirtyOnly {
				// Options.ShowClean, not Display.ShowClean: the flag is this
				// run's choice, and writing it into the config's display
				// settings would leak into anything else reading them.
				o.ShowClean = false
			}
			return render.Table(out, repos, o)
		},
	}
	c.Flags().BoolVarP(&dirtyOnly, "dirty", "d", false, "only repositories needing attention")
	c.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	c.Flags().BoolVar(&noStash, "no-stash", false, "skip counting stash entries")
	return c
}
