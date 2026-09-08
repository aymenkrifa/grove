package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	var install, alsoRC bool
	c := &cobra.Command{
		Use:   "completion <bash|zsh|fish|powershell>",
		Short: "Generate a shell completion script",
		Long: "Generate a shell completion script.\n\n" +
			"With --install, write it where the shell will find it:\n\n" +
			"  grove completion zsh --install\n\n" +
			"That needs no plugin framework and edits no startup file. It prints\n" +
			"the path it wrote, and for zsh it checks whether that directory is\n" +
			"already on your fpath — telling you the one line to add only if it\n" +
			"is not.\n\n" +
			"Without --install the script goes to stdout, which is what you want\n" +
			"for a package build or an unusual location:\n\n" +
			"  grove completion zsh > /usr/share/zsh/site-functions/_grove\n\n" +
			"Or skip the file entirely and load it afresh in every shell, at the\n" +
			"cost of running grove once per shell start:\n\n" +
			"  source <(grove completion zsh)",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := args[0]
			if !install {
				return writeCompletion(cmd.Root(), shell, cmd.OutOrStdout())
			}
			return installCompletion(cmd, shell, alsoRC)
		},
	}
	c.Flags().BoolVar(&install, "install", false, "write the script where the shell will find it, instead of to stdout")
	c.Flags().BoolVar(&alsoRC, "rc", false, "with --install, also add the line zsh needs to your ~/.zshrc")
	return c
}

// writeCompletion emits the script for shell into w.
//
// The default arm is not decoration: ValidArgs only drives shell-completion
// suggestions, and cobra does not reject an argument outside that list on its
// own. Without it, a typo like "tcsh" fell through to whichever generator came
// last and printed PowerShell's script at exit 0.
func writeCompletion(root *cobra.Command, shell string, w io.Writer) error {
	switch shell {
	case "bash":
		return root.GenBashCompletionV2(w, true)
	case "zsh":
		return root.GenZshCompletion(w)
	case "fish":
		return root.GenFishCompletion(w, true)
	case "powershell":
		return root.GenPowerShellCompletionWithDesc(w)
	default:
		return fmt.Errorf("unsupported shell %q; choose bash, zsh, fish or powershell", shell)
	}
}

// completionTarget is where each shell looks for a completion of its own
// accord, under the directories XDG gives the user.
//
// These are the paths that need no plugin framework and no root: bash's
// completion loader and fish both read their directory automatically, and zsh
// reads any directory on its fpath — which is the one case that may need a
// line in a startup file, so installCompletion checks and says so.
func completionTarget(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}

	switch shell {
	case "zsh":
		// The filename is the contract: zsh autoloads a completion by the
		// name _<command>, wherever on the fpath it sits.
		return filepath.Join(dataHome, "zsh", "site-functions", "_grove"), nil
	case "bash":
		return filepath.Join(dataHome, "bash-completion", "completions", "grove"), nil
	case "fish":
		return filepath.Join(configHome, "fish", "completions", "grove.fish"), nil
	case "powershell":
		return "", fmt.Errorf("powershell has no standard completion directory to install into; " +
			"redirect the script into your profile instead: grove completion powershell >> $PROFILE")
	default:
		return "", fmt.Errorf("unsupported shell %q; choose bash, zsh, fish or powershell", shell)
	}
}

// installCompletion writes the script to completionTarget and reports what, if
// anything, is left for the user to do.
func installCompletion(cmd *cobra.Command, shell string, alsoRC bool) error {
	target, err := completionTarget(shell)
	if err != nil {
		return err
	}

	// Generate first, write second: a failed generator should not leave a
	// half-written completion in a directory the shell is about to read.
	var buf bytes.Buffer
	if err := writeCompletion(cmd.Root(), shell, &buf); err != nil {
		return err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := os.WriteFile(target, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", target, err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "wrote %s\n", target)

	switch shell {
	case "zsh":
		if onFpath(cmd.Context(), dir) {
			fmt.Fprintln(out, "that directory is already on your fpath — open a new shell and press tab")
			fmt.Fprintln(out, "(if nothing happens, a stale completion cache is likely: rm -f ~/.zcompdump*)")
			return nil
		}
		if alsoRC {
			return appendFpathLine(out, dir)
		}
		fmt.Fprintf(out, "\nthat directory is not on your fpath yet. Either add this to ~/.zshrc\n"+
			"above the line that runs compinit:\n\n  fpath=(%s $fpath)\n\n"+
			"or let grove do it:  grove completion zsh --install --rc\n", dir)
	case "bash":
		fmt.Fprintln(out, "bash-completion reads that directory on its own — open a new shell and press tab")
	case "fish":
		fmt.Fprintln(out, "fish reads that directory on its own — open a new shell and press tab")
	}
	return nil
}

// onFpath asks zsh itself whether dir is on the fpath, because nothing else
// can answer it: fpath is assembled by the user's own startup files, and a
// framework, a distribution or a hand-written line may each have added to it.
// Being unable to ask — no zsh installed, or it fails — is reported as "not on
// the fpath", so the user is told how to add it rather than wrongly assured.
func onFpath(ctx context.Context, dir string) bool {
	out, err := exec.CommandContext(ctx, "zsh", "-i", "-c", "print -l -- $fpath").Output()
	if err != nil {
		return false
	}
	want := filepath.Clean(dir)
	for _, line := range strings.Split(string(out), "\n") {
		if filepath.Clean(strings.TrimSpace(line)) == want {
			return true
		}
	}
	return false
}

// zshrcPath is where zsh reads user configuration from, honouring ZDOTDIR.
func zshrcPath() (string, error) {
	if zdot := os.Getenv("ZDOTDIR"); zdot != "" {
		return filepath.Join(zdot, ".zshrc"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".zshrc"), nil
}

// appendFpathLine adds the fpath entry to the user's .zshrc, and re-runs
// compinit immediately after it.
//
// The second line is not redundant. fpath must be set before compinit builds
// its table, and an appended block runs after whatever already called compinit
// — a framework, or the user's own line — so without re-running it the new
// directory would be on the fpath and still ignored. Re-running costs a beat
// at shell start; a completion that silently never loads costs more.
//
// The write is idempotent: the block is skipped if the directory already
// appears anywhere in the file, so running --rc twice does not stack.
func appendFpathLine(out io.Writer, dir string) error {
	rc, err := zshrcPath()
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(rc)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", rc, err)
	}
	if strings.Contains(string(existing), dir) {
		fmt.Fprintf(out, "%s already mentions that directory — nothing to add\n", rc)
		return nil
	}

	block := fmt.Sprintf("\n# grove completions (added by `grove completion zsh --install --rc`)\n"+
		"fpath=(%s $fpath)\nautoload -Uz compinit && compinit -u\n", dir)

	f, err := os.OpenFile(rc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", rc, err)
	}
	defer f.Close()
	if _, err := f.WriteString(block); err != nil {
		return fmt.Errorf("appending to %s: %w", rc, err)
	}
	fmt.Fprintf(out, "added to %s:\n%s\nopen a new shell and press tab.\n", rc, block)
	return nil
}
