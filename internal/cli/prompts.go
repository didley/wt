package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/didley/wt/internal/core"
	"golang.org/x/term"
)

var errAborted = errors.New("aborted")

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
	return !noInput && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// runPrompt renders huh fields on stderr so stdout stays clean for command
// output (the shell wrapper for `wt switch` captures stdout).
func runPrompt(fields ...huh.Field) error {
	err := huh.NewForm(huh.NewGroup(fields...)).WithOutput(os.Stderr).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return errAborted
	}
	return err
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
				return errors.New("select at least one worktree")
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
