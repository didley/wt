// Package cli implements wt's command-line interface: the cobra command
// tree, prompts, and terminal rendering built on top of internal/core.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

// version is overridden at release time via -ldflags "-X ...cli.version=".
var version = "dev"

var yes bool

var rootCmd = &cobra.Command{
	Use:   "wt",
	Short: "Ergonomic git worktrees",
	Long: `wt keeps every worktree of a repository in one predictable place —
a sibling directory named <repo>.worktrees/ — and makes creating,
listing, switching, renaming and removing worktrees painless.

Run wt with no arguments to list the worktrees of the current repo.`,
	Version:       version,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		conventionCheck(cmd)
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		return runList()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&yes, "yes", "y", false, "assume yes: skip confirmations, never prompt")
	rootCmd.CompletionOptions.DisableDefaultCmd = true   // completions are generated via `wt shell-init`
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true}) // -h/--help covers this; no standalone `help` command
	rootCmd.AddCommand(
		addCmd, listCmd, removeCmd, renameCmd, switchCmd,
		lockCmd, unlockCmd, doctorCmd, pruneCmd, shellInitCmd, genManCmd,
	)
}

// Execute runs the wt root command, printing any error and exiting non-zero
// on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, errAborted) {
			fmt.Fprintln(os.Stderr, "Aborted — nothing was changed.")
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
}

// conventionCheck runs before every repo-touching command: it detects
// worktrees living outside <repo>.worktrees/ (e.g. created with raw
// `git worktree add`) and prints a heads-up. It never prompts — moving
// strays into place is an opt-in action via `wt organize`.
func conventionCheck(cmd *cobra.Command) {
	switch cmd.Name() {
	case "doctor", "prune", "shell-init", "gen-man", "version", "__complete", "__completeNoDesc":
		return
	}
	repo, err := core.Discover(".")
	if err != nil {
		return // each command reports its own "not a repo" error
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return
	}
	vs := repo.Violations(wts)
	if len(vs) == 0 {
		return
	}
	warnf("%d worktree(s) live outside %s:", len(vs), repo.WorktreesDir())
	for _, v := range vs {
		fmt.Fprintf(os.Stderr, "    %s\n", v.Worktree.Path)
	}
	warnf("run `wt organize` to move them into place.")
	fmt.Fprintln(os.Stderr)
}
