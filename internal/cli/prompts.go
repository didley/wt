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

// menuAccent is huh's own default ("Charm" theme) accent color for a
// select's cursor/selected option — reused by menuBarModel (menubar.go) so
// the custom horizontal bar reads as the same widget family as every
// huh.Select/MultiSelect elsewhere in the app, not a one-off.
const menuAccent = lipgloss.Color("#F780E2")

// wtTheme is huh's default theme with the select cursor changed to a
// bracket style ("[•] "), so a single-choice huh.Select reads visually
// consistent with a huh.MultiSelect's own "[ ]"/"[x]" checkboxes.
func wtTheme() *huh.Theme {
	t := huh.ThemeCharm()
	t.Focused.SelectSelector = lipgloss.NewStyle().Foreground(menuAccent).SetString("[•] ")
	return t
}

func warnf(format string, a ...any) {
	fmt.Fprintln(os.Stderr, stWarn.Render(fmt.Sprintf(format, a...)))
}

func interactive() bool {
	return !yes && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// runPrompt renders huh fields on stderr so stdout stays clean for command
// output (the shell wrapper for `wt switch` captures stdout).
func runPrompt(fields ...huh.Field) error {
	err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(wtTheme()).WithOutput(os.Stderr).Run()
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

// menuExitIdx and menuVerboseListIdx are menuBarEntries' two synthetic
// entries, alongside the real command indices into menuCommands().
const (
	menuExitIdx        = -1
	menuVerboseListIdx = -2 // list already ran; offer its verbose form instead of listing it as its own command
)

// menuBarEntries builds the "Run a command" bar's items and a name ->
// dispatch-index map (command indices into cmds, plus the synthetic
// menuExitIdx/menuVerboseListIdx) — split out from runMenu (which also
// drives the interactive tea.Program) so this pure part is unit testable
// without a real terminal.
func menuBarEntries(cmds []*cobra.Command) ([]menuBarItem, map[string]int) {
	const extraEntries = 2 // "list --verbose" and "Exit"
	items := make([]menuBarItem, 0, len(cmds)+extraEntries)
	dispatch := make(map[string]int, len(cmds)+extraEntries)
	for i, c := range cmds {
		items = append(items, menuBarItem{name: c.Name(), description: c.Short})
		dispatch[c.Name()] = i
	}
	items = append(items, menuBarItem{name: "list --verbose", description: verboseHelp})
	dispatch["list --verbose"] = menuVerboseListIdx
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

		switch idx := dispatch[name]; idx {
		case menuExitIdx:
			return nil
		case menuVerboseListIdx:
			listVerbose = true
			if err := runList(); err != nil {
				return err
			}
			fmt.Println()
		default:
			cmd := cmds[idx]
			if err := cmd.RunE(cmd, nil); err != nil {
				if errors.Is(err, errAborted) {
					fmt.Println(stDim.Render("Cancelled — back to the menu."))
					fmt.Println()
					continue
				}
				// cmd.RunE is one of this package's own run* functions (e.g.
				// runAdd); wrapping here would break errors.Is(err, errAborted)
				// checks upstream.
				return err //nolint:wrapcheck
			}
			fmt.Println()
		}
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
