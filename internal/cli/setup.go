package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var errUnsupportedShell = errors.New("unsupported shell")

var setupCmd = &cobra.Command{
	Use:   "setup <bash|zsh|fish>",
	Short: "Set up shell integration (cd on switch, tab completion)",
	Long: `Set up shell integration for wt: a shell function that makes
` + "`wt switch`" + ` (and ` + "`wt cd`" + `) change your shell's directory, and tab
completion for wt's commands and flags.

Install it by adding one line to your shell's rc file — this emits both
pieces so the one-liner keeps working unattended:

  bash:  eval "$(wt setup bash)"   # ~/.bashrc
  zsh:   eval "$(wt setup zsh)"    # ~/.zshrc
  fish:  wt setup fish | source    # ~/.config/fish/config.fish

Run it directly in a terminal (not piped into eval/source) to pick which
piece(s) you want instead.`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{shellBash, shellZsh, shellFish},
	RunE:      runSetup,
}

const (
	integWrapper     = "wrapper"
	integCompletions = "completions"
)

const (
	shellBash = "bash"
	shellZsh  = "zsh"
	shellFish = "fish"
)

func runSetup(_ *cobra.Command, args []string) error {
	shell, err := resolveShell(args)
	if err != nil {
		return err
	}
	if shell != shellBash && shell != shellZsh && shell != shellFish {
		return fmt.Errorf("%w %q (bash, zsh and fish are supported)", errUnsupportedShell, shell)
	}

	want := []string{integWrapper, integCompletions}
	// Only prompt when stdout is a real terminal — eval "$(...)" and
	// piped/sourced usage always have a captured, non-tty stdout, and must
	// keep working unattended (that's the common case, run on every shell
	// startup).
	if !yes && term.IsTerminal(int(os.Stdout.Fd())) {
		selected, err := promptIntegrations()
		if err != nil {
			return err
		}
		want = selected
	}

	var out bytes.Buffer
	for _, integ := range want {
		if err := writeIntegration(&out, integ, shell); err != nil {
			return err
		}
	}
	if _, err := io.Copy(os.Stdout, &out); err != nil {
		return fmt.Errorf("writing shell integration: %w", err)
	}
	return nil
}

// resolveShell resolves the target shell, either from args or, with none
// given, an interactive prompt.
func resolveShell(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if !interactive() {
		return "", fmt.Errorf("%w: wt setup <bash|zsh|fish>", errTargetRequired)
	}
	var shell string
	err := runPrompt(huh.NewSelect[string]().
		Title("Which shell?").
		Options(
			huh.NewOption("bash", shellBash),
			huh.NewOption("zsh", shellZsh),
			huh.NewOption("fish", shellFish),
		).
		Value(&shell))
	return shell, err
}

func writeIntegration(out *bytes.Buffer, integ, shell string) error {
	switch integ {
	case integWrapper:
		switch shell {
		case shellBash, shellZsh:
			out.WriteString(posixWrapper)
		case shellFish:
			out.WriteString(fishWrapper)
		}
	case integCompletions:
		if err := writeCompletion(out, shell); err != nil {
			return err
		}
	}
	return nil
}

func promptIntegrations() ([]string, error) {
	var selected []string
	err := runPrompt(huh.NewMultiSelect[string]().
		Title("Install which integration(s)?").
		Options(
			huh.NewOption("cd wrapper (`wt switch`/`cd` change directory)", integWrapper).Selected(true),
			huh.NewOption("tab completions", integCompletions).Selected(true),
		).
		Value(&selected))
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func writeCompletion(w io.Writer, shell string) error {
	var err error
	switch shell {
	case shellBash:
		err = rootCmd.GenBashCompletionV2(w, true)
	case shellZsh:
		err = rootCmd.GenZshCompletion(w)
	case shellFish:
		err = rootCmd.GenFishCompletion(w, true)
	default:
		return fmt.Errorf("%w %q", errUnsupportedShell, shell)
	}
	if err != nil {
		return fmt.Errorf("generating %s completion: %w", shell, err)
	}
	return nil
}

const posixWrapper = `# wt shell integration: makes 'wt switch' / 'wt cd' change directory.
wt() {
  case "$1" in
    switch|cd)
      local __wt_out
      __wt_out="$(command wt "$@")" || return $?
      if [ -n "$__wt_out" ] && [ -d "$__wt_out" ]; then
        cd "$__wt_out" || return $?
      elif [ -n "$__wt_out" ]; then
        printf '%s\n' "$__wt_out"
      fi
      ;;
    *)
      command wt "$@"
      ;;
  esac
}
`

const fishWrapper = `# wt shell integration: makes 'wt switch' / 'wt cd' change directory.
function wt --description "git worktrees, ergonomically"
    if test (count $argv) -gt 0; and contains -- $argv[1] switch cd
        set -l __wt_out (command wt $argv)
        or return $status
        if test -n "$__wt_out"; and test -d "$__wt_out"
            cd $__wt_out
        else if test -n "$__wt_out"
            echo $__wt_out
        end
    else
        command wt $argv
    end
end
`
