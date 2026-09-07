package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/config"
)

func newConfigCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Inspect grove's configuration"}

	c.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print where the config file is looked for",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), config.Path())
			return nil
		},
	})

	c.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration and why this root was chosen",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			res, err := config.Resolve(cfg, config.Opts{Root: flagRoot, Workspace: flagWorkspace, Cwd: cwd})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "config file:  %s\n", config.Path())
			fmt.Fprintf(out, "root:         %s\n", res.Root)
			fmt.Fprintf(out, "chosen by:    %s\n", res.Source)
			fmt.Fprintf(out, "workspace:    %s\n", orNone(res.Name))
			fmt.Fprintf(out, "depth:        %d\n", res.Depth)
			fmt.Fprintf(out, "ignore:       %v\n", res.Ignore)
			fmt.Fprintf(out, "group_by:     %s\n", res.Display.GroupBy)
			fmt.Fprintf(out, "show_clean:   %v\n", res.Display.ShowClean)
			if len(cfg.Workspaces) > 0 {
				fmt.Fprintln(out, "\nconfigured workspaces:")
				for _, w := range cfg.Workspaces {
					fmt.Fprintf(out, "  %-12s %s\n", w.Name, w.Root)
				}
			}
			return nil
		},
	})
	return c
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
