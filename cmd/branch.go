package cmd

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/git"
	"github.com/aymenkrifa/grove/internal/render"
)

func newBranchCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:     "branch [selector]",
		Aliases: []string{"br"},
		Short:   "Show which branch each repository is on",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, found, err := resolveAndFind(firstArg(args))
			if err != nil {
				return err
			}
			repos := git.Collect(cmd.Context(), found, git.CollectOpts{Jobs: flagJobs})
			for _, r := range repos {
				if r.Error != "" {
					markPartialFailure()
				}
			}
			out := cmd.OutOrStdout()
			if asJSON {
				return render.JSON(out, res.Root, res.Name, repos)
			}
			// Group repositories by the branch they are on, so a ticket branch
			// spanning several repositories is visible at a glance.
			byBranch := map[string][]string{}
			for _, r := range repos {
				key := r.Branch
				if r.Error != "" {
					key = "(error)"
				}
				byBranch[key] = append(byBranch[key], r.Path)
			}
			keys := make([]string, 0, len(byBranch))
			for k := range byBranch {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			for _, k := range keys {
				for i, p := range byBranch[k] {
					label := k
					if i > 0 {
						label = ""
					}
					fmt.Fprintf(tw, "%s\t%s\n", label, p)
				}
			}
			return tw.Flush()
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return c
}
