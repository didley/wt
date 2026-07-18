package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestEscapeAngleBrackets(t *testing.T) {
	sub := &cobra.Command{
		Use:     "child <arg>",
		Short:   "short <x>",
		Long:    "long <y>",
		Example: "example <z>",
	}
	root := &cobra.Command{
		Use:     "root <repo>",
		Short:   "short <a>",
		Long:    "long <b>",
		Example: "example <c>",
	}
	root.AddCommand(sub)

	escapeAngleBrackets(root)

	for _, got := range []string{root.Use, root.Short, root.Long, root.Example} {
		if !strings.Contains(got, `\<`) {
			t.Errorf("root field %q does not contain escaped angle bracket", got)
		}
	}
	for _, got := range []string{sub.Use, sub.Short, sub.Long, sub.Example} {
		if !strings.Contains(got, `\<`) {
			t.Errorf("child field %q does not contain escaped angle bracket", got)
		}
	}
}
