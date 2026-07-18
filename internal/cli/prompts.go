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

// menuHeight caps the visible rows in the "Run a command" list so it stays
// usable on short terminal windows: beyond this it scrolls (huh shows a
// scroll indicator) rather than growing the prompt to fit every command.
const menuHeight = 7

// menuExitIdx and menuVerboseListIdx are menuOptions' two synthetic entries,
// alongside the real command indices into menuCommands().
const (
	menuExitIdx        = -1
	menuVerboseListIdx = -2 // list already ran; offer its verbose form instead of listing it as its own command
)

// menuOptions builds the "Run a command" select's options and per-index
// descriptions from cmds — split out from runMenu (which also drives the
// interactive huh.Form) so this pure part can be unit tested directly.
func menuOptions(cmds []*cobra.Command) ([]huh.Option[int], map[int]string) {
	const extraOptions = 2 // "list --verbose" and "Exit"
	descriptions := map[int]string{
		menuExitIdx:        "Leave this menu",
		menuVerboseListIdx: verboseHelp,
	}
	opts := make([]huh.Option[int], 0, len(cmds)+extraOptions)
	for i, c := range cmds {
		opts = append(opts, huh.NewOption(c.Name(), i))
		descriptions[i] = c.Short
	}
	opts = append(opts, huh.NewOption("list --verbose", menuVerboseListIdx))
	opts = append(opts, huh.NewOption("Exit", menuExitIdx))
	return opts, descriptions
}

// runMenu offers an interactive choice of what to do next after `wt` (no
// args) has printed the worktree list, then runs the chosen command.
//
// Rows show just the command name — the description is shown once, in a
// single line below the title, and swaps in dynamically for whichever
// command is currently focused (huh.DescriptionFunc bound to the value
// pointer). That keeps every command's name visible at once (a flat
// "name — description" list doesn't fit a narrow terminal and buries the
// names in prose) while still surfacing what each one does.
//
// It also loops: once a command finishes, the menu comes back around
// rather than exiting, so you're never dropped out of the session after
// one action — picking a command and then returning here plays the role a
// "back" option would.
func runMenu() error {
	const exitIdx = menuExitIdx
	const verboseListIdx = menuVerboseListIdx
	cmds := menuCommands()
	opts, descriptions := menuOptions(cmds)

	// Up/down are the default select keys; left/right are enabled too, as
	// equivalent ways to move through the (still vertical) list.
	menuKeyMap := huh.NewDefaultKeyMap()
	menuKeyMap.Select.Left.SetEnabled(true)
	menuKeyMap.Select.Right.SetEnabled(true)

	for {
		idx := exitIdx
		err := runPrompt(huh.NewSelect[int]().
			Title("Run a command").
			Height(menuHeight).
			Options(opts...).
			DescriptionFunc(func() string { return descriptions[idx] }, &idx).
			Value(&idx).
			WithKeyMap(menuKeyMap))
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
			if err := runList(); err != nil {
				return err
			}
			fmt.Println()
			continue
		}

		cmd := cmds[idx]
		if err := cmd.RunE(cmd, nil); err != nil {
			// cmd.RunE is one of this package's own run* functions (e.g.
			// runAdd); wrapping here would break errors.Is(err, errAborted)
			// checks upstream.
			return err //nolint:wrapcheck
		}
		fmt.Println()
	}
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
