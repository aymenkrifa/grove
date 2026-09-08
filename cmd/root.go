// Package cmd wires grove's command-line interface.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/config"
	"github.com/aymenkrifa/grove/internal/discover"
	"github.com/aymenkrifa/grove/internal/git"
	"github.com/aymenkrifa/grove/internal/render"
)

// Exit codes, per the spec §5.7. Anything grove refuses to do is 1; a run that
// finished but had at least one repository fail is 2, so a script can tell
// "grove could not run" from "grove ran, and something in your workspace is
// broken".
const (
	ExitOK      = 0
	ExitError   = 1
	ExitPartial = 2
)

var (
	flagRoot      string
	flagWorkspace string
	flagColor     string
	flagJobs      int
	flagASCII     bool
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "grove",
		Short:         "Status, diffs and logs across every git repo under one directory",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	pf := root.PersistentFlags()
	pf.StringVar(&flagRoot, "root", "", "directory to search for repositories")
	pf.StringVarP(&flagWorkspace, "workspace", "w", "", "named workspace from the config file")
	pf.StringVar(&flagColor, "color", "", "auto|always|never")
	pf.IntVar(&flagJobs, "jobs", 0, "maximum concurrent git processes")
	pf.BoolVar(&flagASCII, "ascii", false, "use ASCII symbols instead of unicode")

	// On the root rather than on each command, for the same reason --color is
	// a persistent flag in the first place: seven commands inherit it, and a
	// check written seven times is a check one command eventually lacks.
	// Cobra runs the nearest PersistentPreRunE up the chain and no subcommand
	// defines one, so this is that check for every one of them.
	root.PersistentPreRunE = func(*cobra.Command, []string) error {
		return config.ValidateColor("--color", flagColor)
	}

	root.AddCommand(
		newStatusCmd(),
		newListCmd(),
		newBranchCmd(),
		newDiffCmd(),
		newFetchCmd(),
		newLogCmd(),
		newExecCmd(),
		newInitCmd(),
		newConfigCmd(),
		newResolveCmd(),
		newShellInitCmd(),
		newCompletionCmd(),
		newVersionCmd(),
	)
	return root
}

// Execute runs grove against the real standard streams.
func Execute() int { return ExecuteWith(os.Stdout, os.Stderr, os.Args[1:]) }

// signalContext returns the context every command runs under: one that is
// cancelled when grove is interrupted or terminated.
//
// Every command reaches it through cmd.Context(), which is context.Background()
// unless a context is supplied here, and Background is never cancelled. That is
// invisible while grove is the foreground process of an interactive shell,
// because Ctrl-C goes to the whole foreground process group and the git
// children get it directly. It stops being invisible the moment grove is not
// the group leader — run from a script, or as the `git grove` shim — where the
// signal arrives at grove alone: grove dies on the spot and its git children
// keep running, orphaned, fetching in repositories nobody is waiting on any
// more. With a cancellable context, exec.CommandContext takes every one of
// them down on the way out.
//
// SIGTERM as well as interrupt, because a script's `kill` sends the former and
// has exactly the same problem.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// ExecuteWith runs grove against the given streams, which is what the tests use.
func ExecuteWith(stdout, stderr io.Writer, args []string) int {
	// exitCode and errOut are package-level, so they carry over from whatever
	// ran last. In the binary that is nothing; under `go test` every case
	// shares one process, and a single command that reported a per-repo
	// failure would otherwise hand ExitPartial to every run after it. Reset
	// both here, where a run begins, rather than trusting each command to
	// leave them tidy.
	exitCode = ExitOK
	errOut = stderr

	root := newRootCmd()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	ctx, stop := signalContext()
	defer stop()

	// The flag variables are reset for free: pflag writes the default value
	// through the pointer at registration, and newRootCmd registers afresh
	// on every call.
	err := root.ExecuteContext(ctx)
	if err == nil {
		return exitCode
	}
	fmt.Fprintf(stderr, "grove: %v\n", err)
	return ExitError
}

// ExecuteAsGitSubcommand runs grove against the real standard streams and
// os.Args, under its git-subcommand identity: help text reads "git grove ..."
// rather than "grove ...", which is how the user actually invoked it — git
// treats any git-grove executable on PATH as the subcommand `git grove`, and
// strips "grove" itself before invoking it, so args here start after it.
func ExecuteAsGitSubcommand() int {
	return executeAsGitSubcommandWith(os.Stdout, os.Stderr, os.Args[1:])
}

// executeAsGitSubcommandWith is ExecuteAsGitSubcommand with its streams and
// arguments parameterised, mirroring the Execute/ExecuteWith split above and
// for the same two reasons: it is what the tests use, since os.Args and
// os.Stdout cannot be swapped out from inside one, and exitCode/errOut are
// reset here rather than trusted to start clean, for the reason ExecuteWith
// already documents below.
//
// Renaming the command is done through cobra's CommandDisplayNameAnnotation,
// not by overwriting root.Use. Cobra's Name() — which CommandPath() and
// UseLine() build on for every subcommand — takes only the first
// space-separated word of Use, so Use = "git grove" actually displays as
// just "git" everywhere but the root's own usage line, dropping "grove"
// entirely from "grove status --help" and printing "git status" (the name of
// a real, different git command) instead of "git grove status". The
// annotation is what cobra 1.10 added for exactly this — a command invoked
// under another program's name — and it correctly reaches every subcommand:
// "git grove status --help" rather than "git status --help".
func executeAsGitSubcommandWith(stdout, stderr io.Writer, args []string) int {
	exitCode = ExitOK
	errOut = stderr

	root := newRootCmd()
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations[cobra.CommandDisplayNameAnnotation] = "git grove"
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	// The shim needs this more than ExecuteWith does, not less: `git grove
	// fetch` runs grove as git's own child, so grove is never the process
	// group leader there.
	ctx, stop := signalContext()
	defer stop()

	err := root.ExecuteContext(ctx)
	if err == nil {
		return exitCode
	}
	fmt.Fprintf(stderr, "grove: %v\n", err)
	return ExitError
}

// exitCode is set to ExitPartial by commands that saw a per-repo failure.
var exitCode = ExitOK

func markPartialFailure() { exitCode = ExitPartial }

// errOut is where diagnostics that are not the command's return value go —
// today the walk's warnings about directories it could not read. It is a
// package variable rather than a parameter so that resolveAndFind keeps the
// one-argument signature every command shares: a warning has to reach stderr
// whichever command triggered the walk, and threading a writer through all of
// them invites the one command that forgets. ExecuteWith sets it; the default
// keeps a direct caller of resolveAndFind from writing to a nil writer.
var errOut io.Writer = os.Stderr

// resolveAndFind performs the root resolution, the walk, and the selector
// narrowing that nearly every command needs.
//
// Every caller goes on to run git, so the git preflight lives here too: one
// clear error instead of the same message repeated once per repository.
func resolveAndFind(selector string) (*config.Resolved, []discover.Found, error) {
	if err := git.Available(); err != nil {
		return nil, nil, err
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", config.Path(), err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	res, err := config.Resolve(cfg, config.Opts{Root: flagRoot, Workspace: flagWorkspace, Cwd: cwd})
	if err != nil {
		return nil, nil, err
	}
	found, warnings, err := discover.Walk(res.Root, res.Depth, res.Ignore)
	// Warnings describe subtrees the walk could not read. They are not fatal —
	// the rest of the workspace was still searched — but they are the only
	// notice the user gets that the listing below is incomplete, so they are
	// printed before anything else, and before the empty-workspace error that
	// an unreadable root-level directory may well be the cause of.
	for _, w := range warnings {
		fmt.Fprintf(errOut, "grove: warning: %s\n", w)
	}
	if err != nil {
		return nil, nil, err
	}
	if len(found) == 0 {
		return nil, nil, fmt.Errorf("no git repositories under %s", res.Root)
	}
	sel, err := discover.Select(found, selector)
	if err != nil {
		return nil, nil, err
	}
	return res, sel, nil
}

// collectOpts merges the config with the flags that govern collection rather
// than presentation. It is a separate function from renderOptions because the
// two answer different questions — what git work to do, and how to print it —
// and because a value built inline inside RunE can only be checked through
// whatever the table happens to show, which for --jobs is nothing at all.
func collectOpts(res *config.Resolved, noStash bool) git.CollectOpts {
	return git.CollectOpts{
		Jobs: flagJobs,
		// Counting stashes costs an extra git call per repository, so it is
		// decided before collection rather than at render time: switching it
		// off has to save the work, not just hide the number. Either source
		// can switch it off; neither can override the other into on.
		Stash: res.Display.ShowStash && !noStash,
	}
}

// renderOptions merges config display settings with the global flags.
//
// It takes the writer because the colour decision depends on it: "auto" means
// "colour when a human is watching", and only the writer knows whether it is a
// terminal or a pipe.
func renderOptions(res *config.Resolved, out io.Writer) render.Options {
	mode := res.Display.Color
	if mode == "" {
		mode = "auto"
	}
	if flagColor != "" {
		mode = flagColor
	}
	return render.Options{
		Display:   res.Display,
		Color:     render.UseColor(mode, isTerminal(out)),
		ASCII:     flagASCII || res.Display.ASCII,
		ShowClean: res.Display.ShowClean,
	}
}

// isTerminal reports whether w is an interactive terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// firstArg returns the optional selector argument.
func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
