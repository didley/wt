package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var (
	removeStash       bool
	removeDiscard     bool
	removeYes         bool
	removeDelBranch   bool
	removeForceBranch bool
)

var removeCmd = &cobra.Command{
	Use:     "remove [worktree]",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a worktree (the branch is always kept)",
	Long: `Remove a worktree directory.

Removing a worktree never deletes its branch: the branch stays in the
repository and can be checked out again from anywhere. Deleting the
branch is a separate, explicit step (a prompt, or --delete-branch).

If the worktree has uncommitted changes, wt lists them and asks whether
to stash them (kept in the repo's stash, recoverable later) or discard
them permanently.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRemove,
}

func init() {
	removeCmd.Flags().BoolVar(&removeStash, "stash", false, "stash uncommitted changes before removing")
	removeCmd.Flags().BoolVar(&removeDiscard, "discard", false, "permanently discard uncommitted changes")
	removeCmd.Flags().BoolVarP(&removeYes, "yes", "y", false, "skip confirmation prompts")
	removeCmd.Flags().BoolVar(&removeDelBranch, "delete-branch", false, "also delete the branch (refused if unmerged)")
	removeCmd.Flags().BoolVar(&removeForceBranch, "force-delete-branch", false, "also delete the branch, even if unmerged")
	removeCmd.MarkFlagsMutuallyExclusive("stash", "discard")
}

func runRemove(cmd *cobra.Command, args []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return err
	}
	candidates := linkedWorktrees(wts)
	if len(candidates) == 0 {
		return errors.New("no worktrees to remove (the main checkout is not removable)")
	}

	var target core.Worktree
	if len(args) == 1 {
		target, err = resolveWorktree(repo, candidates, args[0])
	} else {
		if !interactive() {
			return errors.New("worktree name required when not running interactively: wt remove <worktree>")
		}
		target, err = pickWorktree(repo, candidates, "Remove which worktree?")
	}
	if err != nil {
		return err
	}
	name := repo.WorktreeName(target)

	// Friction point #1: surface the open diff and make the choice explicit.
	const (
		actKeepClean = "clean"
		actStash     = "stash"
		actDiscard   = "discard"
	)
	action := actKeepClean
	var changes []core.FileChange
	if !target.Prunable {
		changes, err = core.WorktreeStatus(target.Path)
		if err != nil {
			return fmt.Errorf("cannot inspect worktree state: %w", err)
		}
	}
	if len(changes) > 0 {
		fmt.Fprintf(os.Stderr, "%s has uncommitted changes:\n", stBold.Render(name))
		printChanges(changes)
		fmt.Fprintln(os.Stderr)
		switch {
		case removeStash:
			action = actStash
		case removeDiscard:
			action = actDiscard
		case !interactive():
			return errors.New("worktree has uncommitted changes: re-run with --stash (keep them in the repo's stash) or --discard (delete them permanently)")
		default:
			err = runPrompt(huh.NewSelect[string]().
				Title("What should happen to these changes?").
				Options(
					huh.NewOption("Stash them — saved in the repo's stash, recover later with `git stash pop`", actStash),
					huh.NewOption("Discard them — permanently deletes the changes listed above", actDiscard),
					huh.NewOption("Cancel — keep the worktree as it is", "cancel"),
				).
				Value(&action))
			if err != nil {
				return err
			}
			if action == "cancel" {
				return errAborted
			}
		}
	}

	// Friction point #2: removal never touches the branch — say so up front.
	if !removeYes {
		if interactive() {
			desc := "This worktree is on a detached HEAD; no branch is affected."
			if target.Branch != "" {
				desc = fmt.Sprintf("The branch %q is NOT deleted — it stays in the repository and can be checked out again from any worktree.", target.Branch)
			}
			ok, err := confirm(fmt.Sprintf("Remove worktree %q?", name), desc, true)
			if err != nil {
				return err
			}
			if !ok {
				return errAborted
			}
		} else if action == actKeepClean {
			return errors.New("non-interactive removal needs --yes (or --stash/--discard when dirty)")
		}
	}

	if action == actStash {
		msg := fmt.Sprintf("wt: removed worktree %q (branch %s)", name, target.Branch)
		if err := core.Stash(target.Path, msg); err != nil {
			return fmt.Errorf("stash failed, worktree untouched: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Changes stashed — recover them anytime with %s\n", stBold.Render("git stash pop"))
	}
	force := action != actKeepClean || target.Prunable
	if err := repo.RemoveWorktree(target.Path, force); err != nil {
		return err
	}

	if target.Branch == "" {
		fmt.Printf("Removed worktree %q (was on a detached HEAD).\n", name)
		return nil
	}
	fmt.Printf("Removed worktree %q. The branch %q is still in the repository", name, target.Branch)
	fmt.Printf(" — recreate a worktree for it anytime with %s\n", stBold.Render("wt create "+target.Branch))

	// Branch deletion is deliberately a separate, opt-in step.
	switch {
	case removeForceBranch:
		if err := repo.DeleteBranch(target.Branch, true); err != nil {
			return fmt.Errorf("worktree removed, but deleting the branch failed: %w", err)
		}
		fmt.Printf("Deleted branch %q.\n", target.Branch)
	case removeDelBranch:
		if err := repo.DeleteBranch(target.Branch, false); err != nil {
			return fmt.Errorf("worktree removed, but the branch was kept: %w (use --force-delete-branch if you are sure)", err)
		}
		fmt.Printf("Deleted branch %q.\n", target.Branch)
	case interactive() && !removeYes:
		del, err := confirm(
			fmt.Sprintf("Also delete the branch %q?", target.Branch),
			"Usually you keep it: removing the worktree already freed the checkout. Delete only if the branch itself is finished with.",
			false)
		if err != nil {
			return err
		}
		if del {
			if err := repo.DeleteBranch(target.Branch, false); err != nil {
				warnf("branch %q was kept: %v", target.Branch, err)
				warnf("(delete an unmerged branch with `git branch -D %s` if you are certain)", target.Branch)
				return nil
			}
			fmt.Printf("Deleted branch %q.\n", target.Branch)
		}
	}
	return nil
}
