package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/aymenkrifa/grove/internal/git"
)

type commit struct {
	when    int64
	repo    string
	hash    string
	author  string
	subject string
}

func newLogCmd() *cobra.Command {
	var (
		limit  int
		since  string
		author string
		asJSON bool
	)
	c := &cobra.Command{
		Use:   "log [selector]",
		Short: "Commits from every repository, merged and sorted by date",
		Long:  "--since and --author are forwarded to git log verbatim and take git's own syntax.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, found, err := resolveAndFind(firstArg(args))
			if err != nil {
				return err
			}
			// \x1f separates fields; it cannot occur in a commit subject.
			//
			// --no-merges: a merge commit carries no diff of its own and its
			// subject is usually a generated "Merge branch ..." line that says
			// nothing about what changed, so it would just add noise to a
			// timeline that is already mixing many repositories together. No
			// other command in this batch takes a flag to toggle its git
			// arguments on or off, so this is a fixed choice rather than a new
			// bit of surface area to maintain; TestLogExcludesMergeCommits
			// pins it.
			gitArgs := []string{"log", "--no-merges", "--pretty=format:%ct\x1f%h\x1f%an\x1f%s",
				"-n", strconv.Itoa(limit)}
			if since != "" {
				gitArgs = append(gitArgs, "--since", since)
			}
			if author != "" {
				gitArgs = append(gitArgs, "--author", author)
			}

			var all []commit
			for _, f := range found {
				body, err := git.Run(cmd.Context(), f.AbsPath, gitArgs...)
				if err != nil {
					// git log fails identically for two very different
					// situations: a repository with no commits yet (unborn)
					// and one that is genuinely broken. Only the second is
					// worth a message and a partial-failure exit, so probe
					// HEAD directly. `rev-parse --verify` also fails when HEAD
					// resolves to a commit whose object is missing from a
					// corrupt repository — that case is silently skipped here
					// too, which is an accepted tradeoff (unborn is by far the
					// more common cause, and telling the two apart cheaply
					// would need more than one extra git call per repo).
					if _, verr := git.Run(cmd.Context(), f.AbsPath, "rev-parse", "--quiet", "--verify", "HEAD"); verr != nil {
						continue // unborn (or corrupt): nothing to log, not worth reporting
					}
					markPartialFailure()
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", f.RelPath, err)
					continue
				}
				for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
					if line == "" {
						continue
					}
					parts := strings.SplitN(line, "\x1f", 4)
					if len(parts) != 4 {
						continue
					}
					ts, _ := strconv.ParseInt(parts[0], 10, 64)
					all = append(all, commit{when: ts, repo: f.RelPath, hash: parts[1], author: parts[2], subject: parts[3]})
				}
			}
			sort.SliceStable(all, func(i, j int) bool { return all[i].when > all[j].when })
			// limit <= 0 means "unlimited" — git's own convention (`git log -n
			// -1` shows everything; `-n 0` shows nothing, and by then all is
			// already empty because every per-repo call was given the same
			// -n). Slicing all[:limit] with a negative limit panics, and that
			// panic's exit status is indistinguishable from ExitPartial to a
			// caller, so the guard has to come first.
			if limit > 0 && len(all) > limit {
				all = all[:limit]
			}

			out := cmd.OutOrStdout()
			if asJSON {
				type jsonCommit struct {
					Timestamp int64  `json:"timestamp"`
					Repo      string `json:"repo"`
					Hash      string `json:"hash"`
					Author    string `json:"author"`
					Subject   string `json:"subject"`
				}
				docs := make([]jsonCommit, len(all))
				for i, cm := range all {
					docs[i] = jsonCommit{cm.when, cm.repo, cm.hash, cm.author, cm.subject}
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"commits": docs})
			}

			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			for _, cm := range all {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", cm.hash, cm.repo, cm.author, cm.subject)
			}
			return tw.Flush()
		},
	}
	c.Flags().IntVarP(&limit, "max-count", "n", 20, "maximum commits to show")
	c.Flags().StringVar(&since, "since", "", "only commits more recent than this date")
	c.Flags().StringVar(&author, "author", "", "only commits by this author")
	c.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return c
}
