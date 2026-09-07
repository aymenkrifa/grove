package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/discover"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [dir]",
		Short: "Mark a directory as a grove",
		Long:  "Writes a " + config.MarkerName + " marker so grove commands run from anywhere beneath it.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := firstArg(args)
			if dir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				dir = cwd
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			path := filepath.Join(abs, config.MarkerName)
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists", path)
			} else if !os.IsNotExist(err) {
				// Something other than "not there" — e.g. an unreadable
				// parent directory — is a real problem to surface, not a
				// green light to write the marker.
				return err
			}
			found, warnings, err := discover.Walk(abs, config.DefaultDepth, nil)
			// Same treatment as resolveAndFind: an unreadable subtree does
			// not stop init from writing the marker, but the user should
			// still hear about it.
			for _, w := range warnings {
				fmt.Fprintf(errOut, "grove: warning: %s\n", w)
			}
			if err != nil {
				return err
			}
			body := fmt.Sprintf(`# grove workspace marker
# %d repositories found when this file was created.
depth = %d

# Directories to skip while searching:
# ignore = ["**/vendor/**", "**/node_modules/**"]

# [display]
# group_by   = "dir"       # dir | none | branch-prefix
# show_clean = true
`, len(found), config.DefaultDepth)
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d repositories)\n", path, len(found))
			return nil
		},
	}
}
