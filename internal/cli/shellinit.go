package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var shellInitCmd = &cobra.Command{
	Use:       "shell-init <bash|zsh|fish>",
	Short:     "Print shell integration: the `wt switch`/`cd` wrapper and tab completions",
	Long: `Print shell integration for wt: a shell function that makes
` + "`wt switch`" + ` (and ` + "`wt cd`" + `) change your shell's directory, and tab
completion for wt's commands and flags.

Install it by adding one line to your shell's rc file — this emits both
pieces so the one-liner keeps working unattended:

  bash:  eval "$(wt shell-init bash)"   # ~/.bashrc
  zsh:   eval "$(wt shell-init zsh)"    # ~/.zshrc
  fish:  wt shell-init fish | source    # ~/.config/fish/config.fish

Run it directly in a terminal (not piped into eval/source) to pick which
piece(s) you want instead.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE:      runShellInit,
}

const (
	integWrapper     = "wrapper"
	integCompletions = "completions"
)

func runShellInit(cmd *cobra.Command, args []string) error {
	shell := args[0]
	if shell != "bash" && shell != "zsh" && shell != "fish" {
		return fmt.Errorf("unsupported shell %q (bash, zsh and fish are supported)", shell)
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
		switch integ {
		case integWrapper:
			switch shell {
			case "bash", "zsh":
				out.WriteString(posixWrapper)
			case "fish":
				out.WriteString(fishWrapper)
			}
		case integCompletions:
			if err := writeCompletion(&out, shell); err != nil {
				return err
			}
		}
	}
	_, err := io.Copy(os.Stdout, &out)
	return err
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
	switch shell {
	case "bash":
		return rootCmd.GenBashCompletionV2(w, true)
	case "zsh":
		return rootCmd.GenZshCompletion(w)
	case "fish":
		return rootCmd.GenFishCompletion(w, true)
	default:
		return fmt.Errorf("unsupported shell %q", shell)
	}
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
