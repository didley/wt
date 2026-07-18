package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var errAborted = errors.New("aborted")

var errNoSelection = errors.New("select at least one worktree")

var (
	stDim  = lipgloss.NewStyle().Faint(true)
	stGood = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	stWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	stBold = lipgloss.NewStyle().Bold(true)
)

func warnf(format string, a ...any) {
	fmt.Fprintln(os.Stderr, stWarn.Render(fmt.Sprintf(format, a...)))
}

func interactive() bool {
	return !yes && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// runPrompt renders huh fields on stderr so stdout stays clean for command
// output (the shell wrapper for `wt switch` captures stdout).
func runPrompt(fields ...huh.Field) error {
	err := huh.NewForm(huh.NewGroup(fields...)).WithOutput(os.Stderr).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return errAborted
	}
	if err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	return nil
}

// menuCommands lists every subcommand worth offering interactively — the
// whole tree except `list` itself (already shown before the menu appears)
// and hidden commands (e.g. `gen-man`, which exists only for the release
// pipeline) — deliberately ordered by usage frequency and grouped by what
// they're used for (add next to remove, lock next to unlock), rather than
// alphabetically. TestMenuCommandsCoverAllVisibleCommands checks this list
// against rootCmd.Commands() so a forgotten new command fails loudly instead
// of just being missing from the menu.
func menuCommands() []*cobra.Command {
	return []*cobra.Command{
		addCmd, removeCmd, switchCmd, renameCmd,
		lockCmd, unlockCmd, organizeCmd, pruneCmd, setupCmd,
	}
}

// runMenu offers an interactive choice of what to do next after `wt` (no
// args) has printed the worktree list, then runs the chosen command. Labels
// are derived from each command's own name and -h description (Short) so
// there's a single source of truth for both.
func runMenu() error {
	const exitIdx = -1
	const verboseListIdx = -2 // list already ran; offer its verbose form instead of listing it as its own command
	const extraOptions = 2    // "list --verbose" and "Exit"
	cmds := menuCommands()
	opts := make([]huh.Option[int], 0, len(cmds)+extraOptions)
	for i, c := range cmds {
		opts = append(opts, huh.NewOption(c.Name()+" — "+c.Short, i))
	}
	opts = append(opts, huh.NewOption("list --verbose — "+verboseHelp, verboseListIdx))
	opts = append(opts, huh.NewOption("Exit", exitIdx))

	idx := exitIdx
	err := runPrompt(huh.NewSelect[int]().Title("What next?").Options(opts...).Value(&idx))
	if err != nil {
		if errors.Is(err, errAborted) {
			return nil
		}
		return err
	}
	switch idx {
	case exitIdx:
		return nil
	case verboseListIdx:
		listVerbose = true
		return runList()
	}
	cmd := cmds[idx]
	// cmd.RunE is one of this package's own run* functions (e.g. runAdd);
	// wrapping here would break errors.Is(err, errAborted) checks upstream.
	return cmd.RunE(cmd, nil) //nolint:wrapcheck
}

func confirm(title, description string, def bool) (bool, error) {
	v := def
	err := runPrompt(huh.NewConfirm().
		Title(title).
		Description(description).
		Value(&v))
	return v, err
}

func worktreeOptions(repo *core.Repo, wts []core.Worktree) []huh.Option[int] {
	opts := make([]huh.Option[int], len(wts))
	for i, w := range wts {
		label := repo.WorktreeName(w)
		switch {
		case w.IsMain:
			label += "  (main checkout)"
		case w.Branch != "":
			label += "  [" + w.Branch + "]"
		default:
			label += "  [detached HEAD]"
		}
		if w.Prunable {
			label += "  (directory missing)"
		}
		opts[i] = huh.NewOption(label, i)
	}
	return opts
}

func pickWorktree(repo *core.Repo, wts []core.Worktree, title string) (core.Worktree, error) {
	var idx int
	err := runPrompt(huh.NewSelect[int]().Title(title).Options(worktreeOptions(repo, wts)...).Value(&idx))
	if err != nil {
		return core.Worktree{}, err
	}
	return wts[idx], nil
}

// pickWorktrees lets the user select one or more worktrees at once.
func pickWorktrees(repo *core.Repo, wts []core.Worktree, title string) ([]core.Worktree, error) {
	var idxs []int
	err := runPrompt(huh.NewMultiSelect[int]().
		Title(title).
		Options(worktreeOptions(repo, wts)...).
		Validate(func(vals []int) error {
			if len(vals) == 0 {
				return errNoSelection
			}
			return nil
		}).
		Value(&idxs))
	if err != nil {
		return nil, err
	}
	out := make([]core.Worktree, len(idxs))
	for i, idx := range idxs {
		out[i] = wts[idx]
	}
	return out, nil
}
