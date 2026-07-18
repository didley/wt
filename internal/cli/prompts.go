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

// multiSelectHelp documents huh.MultiSelect's toggle key as a field
// Description (always visible) rather than relying on huh's help footer:
// that footer is replaced by the validation error message the instant a
// multiselect requiring at least one item renders with none picked (see
// huh@v1.0.0's group.go Footer(): "if g.showHelp && len(errors) <= 0"),
// so "space/x toggle" would otherwise never be shown to someone who hasn't
// picked anything yet — precisely who needs to see it.
const multiSelectHelp = "space or x to select, enter to confirm"

// huhIndigo/huhFuchsia/huhGreen/huhRed are copied verbatim from huh's
// default theme (ThemeCharm, github.com/charmbracelet/huh@v1.0.0's
// theme.go: Title, SelectSelector/MultiSelectSelector, SelectedOption and
// ErrorIndicator/ErrorMessage respectively) so every bit of color this
// package prints on its own — outside a huh.Form, which already uses these
// via ThemeCharm — reads as the same palette rather than an unrelated
// ANSI-256 guess. huh's Charm theme has no yellow, so stWarn (warnings,
// dirty state, "needs attention") maps to its red (the only "pay attention"
// color it defines) instead.
var (
	huhIndigo  = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7571F9"}
	huhFuchsia = lipgloss.Color("#F780E2")
	huhGreen   = lipgloss.AdaptiveColor{Light: "#02BA84", Dark: "#02BF87"}
	huhRed     = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#ED567A"}
)

var (
	stDim  = lipgloss.NewStyle().Faint(true)
	stGood = lipgloss.NewStyle().Foreground(huhGreen).Bold(true)
	stWarn = lipgloss.NewStyle().Foreground(huhRed)
	stBold = lipgloss.NewStyle().Foreground(huhIndigo).Bold(true)
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

// menuExitIdx, menuVerboseListIdx and menuHelpIdx are menuBarEntries' three
// synthetic entries, alongside the real command indices into
// menuCommands().
const (
	menuExitIdx        = -1
	menuVerboseListIdx = -2 // list already ran; offer its verbose form instead of listing it as its own command
	menuHelpIdx        = -3
)

// menuBarEntries builds the "Run a command" bar's items and a name ->
// dispatch-index map (command indices into cmds, plus the synthetic
// menuExitIdx/menuVerboseListIdx/menuHelpIdx) — split out from runMenu
// (which also drives the interactive tea.Program) so this pure part is
// unit testable without a real terminal.
func menuBarEntries(cmds []*cobra.Command) ([]menuBarItem, map[string]int) {
	const extraEntries = 3 // "list -v", "help" and "Exit"
	items := make([]menuBarItem, 0, len(cmds)+extraEntries)
	dispatch := make(map[string]int, len(cmds)+extraEntries)
	for i, c := range cmds {
		items = append(items, menuBarItem{name: c.Name(), description: c.Short})
		dispatch[c.Name()] = i
	}
	items = append(items, menuBarItem{name: "list -v", description: "verbose: " + verboseHelp})
	dispatch["list -v"] = menuVerboseListIdx
	items = append(items, menuBarItem{name: "help", description: "Show the full `wt --help` reference"})
	dispatch["help"] = menuHelpIdx
	items = append(items, menuBarItem{name: "Exit", description: "Leave this menu"})
	dispatch["Exit"] = menuExitIdx
	return items, dispatch
}

// runMenu offers an interactive choice of what to do next after `wt` (no
// args) has printed the worktree list, then runs the chosen command. It's a
// horizontal bar (runMenuBar, in menubar.go): every command name laid out on
// one row, filterable by typing, arrow keys to move — see menubar.go's
// menuBarModel doc comment for why this isn't a huh.Select.
//
// It loops: once a command finishes, the menu comes back around rather than
// exiting, so you're never dropped out of the session after one action.
// Cancelling out of a command's own sub-prompts (Ctrl+C/Esc) also returns
// here rather than exiting `wt` entirely — same idea, one level down.
func runMenu() error {
	cmds := menuCommands()
	items, dispatch := menuBarEntries(cmds)

	for {
		name, err := runMenuBar("Run a command", items)
		if err != nil {
			if errors.Is(err, errAborted) {
				return nil
			}
			return err
		}

		exit, err := dispatchMenuChoice(cmds, dispatch[name])
		if err != nil {
			return err
		}
		if exit {
			return nil
		}
	}
}

// dispatchMenuChoice runs whatever runMenu's bar returned idx for (one of
// the synthetic menu*Idx constants, or a real index into cmds), reporting
// whether the menu should exit. Split out from runMenu to keep its loop
// body under the cyclomatic/cognitive complexity gates.
func dispatchMenuChoice(cmds []*cobra.Command, idx int) (bool, error) {
	switch idx {
	case menuExitIdx:
		return true, nil
	case menuVerboseListIdx:
		listVerbose = true
		if err := runList(); err != nil {
			return false, err
		}
		fmt.Println()
		return false, nil
	case menuHelpIdx:
		if err := rootCmd.Help(); err != nil {
			return false, fmt.Errorf("showing help: %w", err)
		}
		fmt.Println()
		return false, nil
	default:
		cmd := cmds[idx]
		if err := cmd.RunE(cmd, nil); err != nil {
			if errors.Is(err, errAborted) {
				fmt.Println(stDim.Render("Cancelled — back to the menu."))
				fmt.Println()
				return false, nil
			}
			// cmd.RunE is one of this package's own run* functions (e.g.
			// runAdd); wrapping here would break errors.Is(err, errAborted)
			// checks upstream.
			return false, err //nolint:wrapcheck
		}
		fmt.Println()
		return false, nil
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
		Description(multiSelectHelp).
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
