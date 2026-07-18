package cli

import (
	"errors"
	"fmt"

	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var lockReason string

var lockCmd = &cobra.Command{
	Use:   "lock [worktree]",
	Short: "Lock a worktree to protect it from removal and pruning",
	Long: `Lock a worktree so ` + "`wt remove`" + ` and ` + "`wt prune`" + ` (and their
git equivalents) refuse to touch it without an explicit override. Locking
never affects the branch or its commits.

Use --reason to record why; it shows up in ` + "`wt list`" + ` and
` + "`git worktree list`" + `.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLock,
}

var unlockCmd = &cobra.Command{
	Use:   "unlock [worktree]",
	Short: "Unlock a previously locked worktree",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUnlock,
}

func init() {
	lockCmd.Flags().StringVar(&lockReason, "reason", "", "why the worktree is locked")
}

func runLock(cmd *cobra.Command, args []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return err
	}
	target, err := pickTarget(repo, linkedWorktrees(wts), args, "Lock which worktree?")
	if err != nil {
		return err
	}
	if target.Locked {
		return fmt.Errorf("%q is already locked%s", repo.WorktreeName(target), reasonSuffix(target.LockReason))
	}
	if err := repo.LockWorktree(target.Path, lockReason); err != nil {
		return err
	}
	name := repo.WorktreeName(target)
	if lockReason != "" {
		fmt.Printf("Locked worktree %q (%s).\n", name, lockReason)
	} else {
		fmt.Printf("Locked worktree %q.\n", name)
	}
	return nil
}

func runUnlock(cmd *cobra.Command, args []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return err
	}
	locked := lockedWorktrees(wts)
	if len(locked) == 0 {
		return errors.New("no worktrees are locked")
	}
	target, err := pickTarget(repo, locked, args, "Unlock which worktree?")
	if err != nil {
		return err
	}
	if !target.Locked {
		return fmt.Errorf("%q is not locked", repo.WorktreeName(target))
	}
	if err := repo.UnlockWorktree(target.Path); err != nil {
		return err
	}
	fmt.Printf("Unlocked worktree %q.\n", repo.WorktreeName(target))
	return nil
}

// pickTarget resolves a single worktree from args if given, otherwise
// prompts interactively among candidates.
func pickTarget(repo *core.Repo, candidates []core.Worktree, args []string, title string) (core.Worktree, error) {
	if len(candidates) == 0 {
		return core.Worktree{}, errors.New("no worktrees to choose from (the main checkout is not eligible)")
	}
	if len(args) > 0 {
		return resolveWorktree(repo, candidates, args[0])
	}
	if !interactive() {
		return core.Worktree{}, errors.New("worktree name required when not running interactively")
	}
	return pickWorktree(repo, candidates, title)
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return fmt.Sprintf(" (%s)", reason)
}

func lockedWorktrees(wts []core.Worktree) []core.Worktree {
	var out []core.Worktree
	for _, w := range wts {
		if w.Locked {
			out = append(out, w)
		}
	}
	return out
}
