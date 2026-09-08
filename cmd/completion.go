package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion <bash|zsh|fish|powershell>",
		Short: "Generate a shell completion script",
		Long: "Generate a shell completion script.\n\n" +
			"zsh loads a file named _grove from any directory on its fpath:\n\n" +
			"  mkdir -p ~/.zfunc && grove completion zsh > ~/.zfunc/_grove\n\n" +
			"then once, in ~/.zshrc above compinit:  fpath=(~/.zfunc $fpath)\n" +
			"oh-my-zsh users can skip that line and write to\n" +
			"~/.oh-my-zsh/completions/_grove, which is already on the fpath.\n\n" +
			"  bash:  grove completion bash > ~/.local/share/bash-completion/completions/grove\n" +
			"  fish:  grove completion fish > ~/.config/fish/completions/grove.fish\n\n" +
			"Or skip the file and load it afresh in every shell, at the cost of\n" +
			"running grove once per shell start:\n\n" +
			"  source <(grove completion zsh)\n\n" +
			"Do not follow the ${fpath[1]} advice found in many tools' docs: the\n" +
			"first fpath entry is often a plugin directory, not a completions one.",
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
