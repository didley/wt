package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// genManCmd is hidden: it exists for the release pipeline, which generates
// man pages from the live command tree so they can never drift from --help.
var genManCmd = &cobra.Command{
	Use:    "gen-man <dir>",
	Short:  "Generate man pages into a directory",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		err := os.MkdirAll(args[0], dirPerm)
		if err != nil {
			return fmt.Errorf("creating %s: %w", args[0], err)
		}
		rootCmd.DisableAutoGenTag = true // keep output reproducible
		// The man renderer (md2man) treats placeholders like <repo> as HTML
		// tags and silently drops them. Escaping is safe here because this
		// process exits right after generating; --help is untouched.
		escapeAngleBrackets(rootCmd)
		err = doc.GenManTree(rootCmd, &doc.GenManHeader{
			Title:   "WT",
			Section: "1",
			Source:  "wt " + version,
			Manual:  "wt manual",
		}, args[0])
		if err != nil {
			return fmt.Errorf("generating man pages: %w", err)
		}
		return nil
	},
}

func escapeAngleBrackets(cmd *cobra.Command) {
	esc := func(s string) string { return strings.ReplaceAll(s, "<", `\<`) }
	cmd.Use = esc(cmd.Use)
	cmd.Short = esc(cmd.Short)
	cmd.Long = esc(cmd.Long)
	cmd.Example = esc(cmd.Example)
	for _, sub := range cmd.Commands() {
		escapeAngleBrackets(sub)
	}
}
