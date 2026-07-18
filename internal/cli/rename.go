package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var renameBranchToo bool

// renameArgCount is <worktree> and <new-name>.
const renameArgCount = 2

var renameCmd = &cobra.Command{
	Use:   "rename <worktree> <new-name>",
	Short: "Rename a worktree directory (branch unchanged unless --branch)",
	Args:  cobra.MaximumNArgs(renameArgCount),
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
	target, newArg, err := resolveRenameArgs(repo, wts, args)
	if err != nil {
		return err
	}

	oldName := repo.WorktreeName(target)
	newName := core.SanitizeBranchName(newArg)
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
		if err := repo.RenameBranch(target.Branch, newArg); err != nil {
			return fmt.Errorf("worktree renamed, but renaming the branch failed: %w", err)
		}
		fmt.Printf("Renamed branch %q -> %q\n", target.Branch, newArg)
	} else if target.Branch != "" {
		fmt.Printf("The branch is still %q — rename it too with --branch, or `git branch -m`.\n", target.Branch)
	}
	return nil
}

// resolveRenameArgs resolves the worktree to rename and its new name, either
// from args or, with none given, interactive prompts.
func resolveRenameArgs(repo *core.Repo, wts []core.Worktree, args []string) (core.Worktree, string, error) {
	candidates := linkedWorktrees(wts)
	if len(args) == renameArgCount {
		target, err := resolveWorktree(repo, candidates, args[0])
		return target, args[1], err
	}
	if len(candidates) == 0 {
		return core.Worktree{}, "", errNoCandidates
	}
	if !interactive() {
		return core.Worktree{}, "", fmt.Errorf("%w: wt rename <worktree> <new-name>", errTargetRequired)
	}
	target, err := pickWorktree(repo, candidates, "Rename which worktree?")
	if err != nil {
		return core.Worktree{}, "", err
	}
	var newName string
	err = runPrompt(huh.NewInput().
		Title("New name").
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return errBranchNameEmpty
			}
			return nil
		}).
		Value(&newName))
	return target, strings.TrimSpace(newName), err
}
