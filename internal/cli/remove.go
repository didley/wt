package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var (
	removeStash       bool
	removeDiscard     bool
	removeDelBranch   bool
	removeForceBranch bool
)

var removeCmd = &cobra.Command{
	Use:     "remove [worktree...]",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove one or more worktrees (branches are always kept)",
	Long: `Remove one or more worktree directories.

Removing a worktree never deletes its branch: the branch stays in the
repository and can be checked out again from anywhere. Deleting the
branch is a separate, explicit step (a prompt, or --delete-branch).

If a worktree has uncommitted changes, wt lists them and asks whether
to stash them (kept in the repo's stash, recoverable later) or discard
them permanently.`,
	Args: cobra.ArbitraryArgs,
	RunE: runRemove,
}

func init() {
	removeCmd.Flags().BoolVar(&removeStash, "stash", false, "stash uncommitted changes before removing")
	removeCmd.Flags().BoolVar(&removeDiscard, "discard", false, "permanently discard uncommitted changes")
	removeCmd.Flags().BoolVar(&removeDelBranch, "delete-branch", false, "also delete the branch(es) (refused if unmerged)")
	removeCmd.Flags().BoolVar(&removeForceBranch, "force-delete-branch", false, "also delete the branch(es), even if unmerged")
	removeCmd.MarkFlagsMutuallyExclusive("stash", "discard")
}

// Friction point #1: surface the open diff and make the choice explicit.
const (
	actKeepClean = "clean"
	actStash     = "stash"
	actDiscard   = "discard"
)

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

	var targets []core.Worktree
	if len(args) > 0 {
		for _, arg := range args {
			t, err := resolveWorktree(repo, candidates, arg)
			if err != nil {
				return err
			}
			targets = append(targets, t)
		}
	} else {
		if !interactive() {
			return errors.New("worktree name required when not running interactively: wt remove <worktree>")
		}
		targets, err = pickWorktrees(repo, candidates, "Remove which worktree(s)?")
		if err != nil {
			return err
		}
	}

	// Per-target dirty state, resolved before any removal happens.
	actions := make([]string, len(targets))
	changesByTarget := make([][]core.FileChange, len(targets))
	anyDirty := false
	for i, t := range targets {
		actions[i] = actKeepClean
		if !t.Prunable {
			changes, err := core.WorktreeStatus(t.Path)
			if err != nil {
				return fmt.Errorf("cannot inspect worktree state for %q: %w", repo.WorktreeName(t), err)
			}
			changesByTarget[i] = changes
			if len(changes) > 0 {
				anyDirty = true
			}
		}
	}

	if anyDirty {
		for i, t := range targets {
			if len(changesByTarget[i]) == 0 {
				continue
			}
			fmt.Fprintf(os.Stderr, "%s has uncommitted changes:\n", stBold.Render(repo.WorktreeName(t)))
			printChanges(changesByTarget[i])
			fmt.Fprintln(os.Stderr)
		}
		switch {
		case removeStash:
			setDirtyAction(actions, changesByTarget, actStash)
		case removeDiscard:
			setDirtyAction(actions, changesByTarget, actDiscard)
		case !interactive():
			return errors.New("worktree(s) have uncommitted changes: re-run with --stash (keep them in the repo's stash) or --discard (delete them permanently)")
		default:
			var choice string
			err = runPrompt(huh.NewSelect[string]().
				Title("What should happen to these changes?").
				Description("Applies to every dirty worktree listed above.").
				Options(
					huh.NewOption("Stash them — saved in each repo's stash, recover later with `git stash pop`", actStash),
					huh.NewOption("Discard them — permanently deletes the changes listed above", actDiscard),
					huh.NewOption("Cancel — keep the worktrees as they are", "cancel"),
				).
				Value(&choice))
			if err != nil {
				return err
			}
			if choice == "cancel" {
				return errAborted
			}
			setDirtyAction(actions, changesByTarget, choice)
		}
	}

	// Friction point #1.5: locked worktrees need an explicit override.
	var lockedTargets []core.Worktree
	for _, t := range targets {
		if t.Locked {
			lockedTargets = append(lockedTargets, t)
		}
	}
	if len(lockedTargets) > 0 {
		for _, t := range lockedTargets {
			fmt.Fprintf(os.Stderr, "%s is locked%s\n", stBold.Render(repo.WorktreeName(t)), reasonSuffix(t.LockReason))
		}
		switch {
		case yes:
			// --yes is explicit consent to override everything, locks included.
		case !interactive():
			return errors.New("worktree(s) are locked: unlock first with `wt unlock`, or re-run with --yes to remove anyway")
		default:
			ok, err := confirm("Remove locked worktree(s) anyway?", "Locking usually means \"don't touch this\" — make sure that's still true.", false)
			if err != nil {
				return err
			}
			if !ok {
				return errAborted
			}
		}
	}

	// Friction point #2: removal never touches the branch — say so up front.
	if !yes {
		if interactive() {
			var desc string
			if len(targets) == 1 {
				desc = "This worktree is on a detached HEAD; no branch is affected."
				if targets[0].Branch != "" {
					desc = fmt.Sprintf("The branch %q is NOT deleted — it stays in the repository and can be checked out again from any worktree.", targets[0].Branch)
				}
			} else {
				desc = "Branches are NOT deleted — they stay in the repository and can be checked out again from any worktree."
			}
			title := fmt.Sprintf("Remove worktree %q?", repo.WorktreeName(targets[0]))
			if len(targets) > 1 {
				names := make([]string, len(targets))
				for i, t := range targets {
					names[i] = repo.WorktreeName(t)
				}
				title = fmt.Sprintf("Remove %d worktrees (%s)?", len(targets), strings.Join(names, ", "))
			}
			ok, err := confirm(title, desc, true)
			if err != nil {
				return err
			}
			if !ok {
				return errAborted
			}
		} else {
			for _, a := range actions {
				if a == actKeepClean {
					return errors.New("non-interactive removal needs --yes (or --stash/--discard when dirty)")
				}
			}
		}
	}

	var removed []core.Worktree
	for i, t := range targets {
		name := repo.WorktreeName(t)
		if actions[i] == actStash {
			msg := fmt.Sprintf("wt: removed worktree %q (branch %s)", name, t.Branch)
			if err := core.Stash(t.Path, msg); err != nil {
				warnf("stash failed for %q, worktree untouched: %v", name, err)
				continue
			}
			fmt.Fprintf(os.Stderr, "Changes stashed for %q — recover them anytime with %s\n", name, stBold.Render("git stash pop"))
		}
		force := actions[i] != actKeepClean || t.Prunable || t.Locked
		if err := repo.RemoveWorktree(t.Path, force); err != nil {
			warnf("could not remove %q: %v", name, err)
			continue
		}
		removed = append(removed, t)
		if t.Branch == "" {
			fmt.Printf("Removed worktree %q (was on a detached HEAD).\n", name)
		} else {
			fmt.Printf("Removed worktree %q. The branch %q is still in the repository", name, t.Branch)
			fmt.Printf(" — recreate a worktree for it anytime with %s\n", stBold.Render("wt add "+t.Branch))
		}
	}

	// Branch deletion is deliberately a separate, opt-in step, batched
	// across everything that was actually removed.
	var branches []string
	seen := map[string]bool{}
	for _, t := range removed {
		if t.Branch != "" && !seen[t.Branch] {
			seen[t.Branch] = true
			branches = append(branches, t.Branch)
		}
	}
	if len(branches) == 0 {
		return nil
	}

	switch {
	case removeForceBranch:
		deleteBranches(repo, branches, true)
	case removeDelBranch:
		deleteBranches(repo, branches, false)
	case interactive() && !yes:
		title := fmt.Sprintf("Also delete the branch %q?", branches[0])
		if len(branches) > 1 {
			title = fmt.Sprintf("Also delete %d branches (%s)?", len(branches), strings.Join(branches, ", "))
		}
		del, err := confirm(
			title,
			"Usually you keep them: removing the worktree already freed the checkout. Delete only if the branch itself is finished with.",
			false)
		if err != nil {
			return err
		}
		if del {
			deleteBranches(repo, branches, false)
		}
	}
	return nil
}

// setDirtyAction applies action to every target that has uncommitted changes.
func setDirtyAction(actions []string, changesByTarget [][]core.FileChange, action string) {
	for i := range actions {
		if len(changesByTarget[i]) > 0 {
			actions[i] = action
		}
	}
}

func deleteBranches(repo *core.Repo, branches []string, force bool) {
	for _, b := range branches {
		if err := repo.DeleteBranch(b, force); err != nil {
			warnf("branch %q was kept: %v", b, err)
			if !force {
				warnf("(delete an unmerged branch with `git branch -D %s` if you are certain)", b)
			}
			continue
		}
		fmt.Printf("Deleted branch %q.\n", b)
	}
}
