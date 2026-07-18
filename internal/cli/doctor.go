package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var doctorFix bool

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	Aliases: []string{"organize"},
	Short:   "Check that every worktree follows the .worktrees convention",
	Long: `Check the repository's worktrees (also available as ` + "`wt organize`" + `):

  - worktrees outside <repo>.worktrees/ (e.g. created with raw
    ` + "`git worktree add`" + `) are reported and can be moved into place
  - stale entries whose directories were deleted manually are pruned
    (also available standalone as ` + "`wt prune`" + `)

Interactively each fix is confirmed; --fix applies everything.`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "apply all fixes without prompting")
}

func runDoctor(cmd *cobra.Command, args []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return err
	}

	fix := doctorFix || yes
	vs := repo.Violations(wts)
	prunable := prunableWorktrees(wts)

	if len(vs) == 0 && len(prunable) == 0 {
		fmt.Printf("%s all worktrees live inside %s\n", stGood.Render("✓"), repo.WorktreesDir())
		return nil
	}

	if len(vs) > 0 {
		fmt.Fprintf(os.Stderr, "%d worktree(s) live outside %s:\n", len(vs), repo.WorktreesDir())
		for _, v := range vs {
			fmt.Fprintf(os.Stderr, "    %s\n    -> %s\n", v.Worktree.Path, v.Target)
		}
		switch {
		case fix:
			moveViolations(repo, vs, false)
		case interactive():
			moveViolations(repo, vs, true)
		default:
			warnf("re-run with --fix to move them")
		}
	}

	if len(prunable) > 0 {
		fmt.Fprintf(os.Stderr, "%d stale worktree entr%s (directory deleted manually):\n", len(prunable), plural(len(prunable), "y", "ies"))
		for _, w := range prunable {
			fmt.Fprintf(os.Stderr, "    %s\n", w.Path)
		}
		doPrune := fix
		if !fix && interactive() {
			doPrune, err = confirm("Prune stale entries?", "Runs `git worktree prune`. Branches are not affected.", true)
			if err != nil {
				return err
			}
		}
		if doPrune {
			if err := repo.PruneWorktrees(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "  pruned")
		} else if !interactive() {
			warnf("re-run with --fix to prune them")
		}
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
