package cli

import (
	"fmt"
	"os"

	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var renameBranchToo bool

// renameArgCount is <worktree> and <new-name>.
const renameArgCount = 2

var renameCmd = &cobra.Command{
	Use:   "rename <worktree> <new-name>",
	Short: "Rename a worktree directory (branch unchanged unless --branch)",
	Args:  cobra.ExactArgs(renameArgCount),
	RunE:  runRename,
}

func init() {
	renameCmd.Flags().BoolVar(&renameBranchToo, "branch", false, "also rename the branch to <new-name>")
}

func runRename(_ *cobra.Command, args []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return fmt.Errorf("listing worktrees: %w", err)
	}
	target, err := resolveWorktree(repo, linkedWorktrees(wts), args[0])
	if err != nil {
		return err
	}
	oldName := repo.WorktreeName(target)
	newName := core.SanitizeBranchName(args[1])
	newPath := repo.ConventionalPath(newName)
	if _, statErr := os.Stat(newPath); statErr == nil {
		return fmt.Errorf("%w: %s", errDirExists, newPath)
	}
	if err := os.MkdirAll(repo.WorktreesDir(), dirPerm); err != nil {
		return fmt.Errorf("creating worktrees directory: %w", err)
	}
	if err := repo.MoveWorktree(target.Path, newPath); err != nil {
		return fmt.Errorf("renaming worktree: %w", err)
	}
	fmt.Printf("Renamed worktree %q -> %q\n  %s\n", oldName, newName, newPath)

	if renameBranchToo {
		if target.Branch == "" {
			warnf("--branch ignored: the worktree is on a detached HEAD")
			return nil
		}
		if err := repo.RenameBranch(target.Branch, args[1]); err != nil {
			return fmt.Errorf("worktree renamed, but renaming the branch failed: %w", err)
		}
		fmt.Printf("Renamed branch %q -> %q\n", target.Branch, args[1])
	} else if target.Branch != "" {
		fmt.Printf("The branch is still %q — rename it too with --branch, or `git branch -m`.\n", target.Branch)
	}
	return nil
}
