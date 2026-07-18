package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var errPruneNeedsYes = errors.New("non-interactive prune needs --yes")

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove stale worktree entries (directories deleted manually)",
	Long: `Prune stale worktree administrative entries.

If a worktree's directory was deleted outside of wt (e.g. rm -rf, or a
tmp dir getting cleared), git still tracks it until pruned. This runs
` + "`git worktree prune`" + ` to drop those stale entries. Branches are
never affected.

This is also offered as part of ` + "`wt organize`" + `.`,
	Args: cobra.NoArgs,
	RunE: runPrune,
}

func runPrune(_ *cobra.Command, _ []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return fmt.Errorf("listing worktrees: %w", err)
	}

	prunable := prunableWorktrees(wts)
	if len(prunable) == 0 {
		fmt.Printf("%s no stale worktree entries\n", stGood.Render("✓"))
		return nil
	}

	fmt.Fprintf(os.Stderr, "%d stale worktree %s (directory deleted manually):\n",
		len(prunable), plural(len(prunable)))
	for _, w := range prunable {
		fmt.Fprintf(os.Stderr, "    %s\n", w.Path)
	}

	if !yes {
		switch {
		case interactive():
			ok, err := confirm("Prune stale entries?", "Runs `git worktree prune`. Branches are not affected.", true)
			if err != nil {
				return err
			}
			if !ok {
				return errAborted
			}
		default:
			return errPruneNeedsYes
		}
	}

	if err := repo.PruneWorktrees(); err != nil {
		return fmt.Errorf("pruning worktrees: %w", err)
	}
	fmt.Fprintln(os.Stderr, "  pruned")
	return nil
}

func prunableWorktrees(wts []core.Worktree) []core.Worktree {
	var out []core.Worktree
	for _, w := range wts {
		if w.Prunable {
			out = append(out, w)
		}
	}
	return out
}
