package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion <bash|zsh|fish|powershell>",
		Short: "Generate a shell completion script",
		Long: "  zsh:   grove completion zsh > \"${fpath[1]}/_grove\"\n" +
			"  bash:  grove completion bash > /etc/bash_completion.d/grove\n" +
			"  fish:  grove completion fish > ~/.config/fish/completions/grove.fish",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(out)
			default:
				// ValidArgs only drives shell-completion suggestions; cobra
				// does not reject an argument outside that list on its own
				// (that needs Args: cobra.OnlyValidArgs, which this command
				// does not set). Without this case an unrecognised shell name
				// would silently fall through to whichever generator
				// happened to be last, printing PowerShell's script at exit
				// 0 for a typo like "tcsh" instead of failing loudly.
				return fmt.Errorf("unsupported shell %q; choose bash, zsh, fish or powershell", args[0])
			}
		},
	}
}
