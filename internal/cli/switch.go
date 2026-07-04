package cli

import (
	"errors"
	"fmt"

	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:     "switch [worktree]",
	Aliases: []string{"cd"},
	Short:   "Jump to a worktree (prints its path; cds with shell-init installed)",
	Long: `Pick a worktree and print its path to stdout.

With the shell integration installed (see ` + "`wt shell-init`" + `), the wt
shell function captures the path and cd's to it. Without it, compose it
yourself: cd "$(wt switch my-branch)".`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSwitch,
}

func runSwitch(cmd *cobra.Command, args []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return err
	}

	var target core.Worktree
	if len(args) == 1 {
		target, err = resolveWorktree(repo, wts, args[0])
	} else {
		if !interactive() {
			return errors.New("worktree name required when not running interactively: wt switch <worktree>")
		}
		target, err = pickWorktree(repo, wts, "Switch to which worktree?")
	}
	if err != nil {
		return err
	}
	if target.Prunable {
		return fmt.Errorf("the directory of %q no longer exists (run `wt doctor`)", repo.WorktreeName(target))
	}
	// The path is the only stdout output; the shell wrapper depends on this.
	fmt.Println(target.Path)
	return nil
}
