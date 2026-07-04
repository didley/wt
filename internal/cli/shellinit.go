package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var shellInitCmd = &cobra.Command{
	Use:       "shell-init <bash|zsh|fish>",
	Short:     "Print shell integration that makes `wt switch` change directory",
	Long: `Print a shell function that wraps wt so that ` + "`wt switch`" + ` (and
` + "`wt cd`" + `) change your shell's directory.

Install it by adding one line to your shell's rc file:

  bash:  eval "$(wt shell-init bash)"   # ~/.bashrc
  zsh:   eval "$(wt shell-init zsh)"    # ~/.zshrc
  fish:  wt shell-init fish | source    # ~/.config/fish/config.fish`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash", "zsh":
			io.WriteString(os.Stdout, posixWrapper)
		case "fish":
			io.WriteString(os.Stdout, fishWrapper)
		default:
			return fmt.Errorf("unsupported shell %q (bash, zsh and fish are supported)", args[0])
		}
		return nil
	},
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
