package cli

import (
	"fmt"
	"os"

	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var renameBranchToo bool

var renameCmd = &cobra.Command{
	Use:   "rename <worktree> <new-name>",
	Short: "Rename a worktree directory (branch unchanged unless --branch)",
	Args:  cobra.ExactArgs(2),
	RunE:  runRename,
}

func init() {
	renameCmd.Flags().BoolVar(&renameBranchToo, "branch", false, "also rename the branch to <new-name>")
}

func runRename(cmd *cobra.Command, args []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return err
	}
	target, err := resolveWorktree(repo, linkedWorktrees(wts), args[0])
	if err != nil {
		return err
	}
	oldName := repo.WorktreeName(target)
	newName := core.SanitizeBranchName(args[1])
	newPath := repo.ConventionalPath(newName)
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("directory already exists: %s", newPath)
	}
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		return err
	}
	if err := repo.MoveWorktree(target.Path, newPath); err != nil {
		return err
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
