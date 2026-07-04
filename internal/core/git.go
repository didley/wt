package core

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GitError carries the stderr of a failed git invocation so callers can show
// git's own explanation instead of a bare exit status.
type GitError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *GitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

// Git runs git with the given arguments in dir and returns stdout with the
// trailing newline removed.
//
// Inside a Flatpak sandbox (the GUI) there is no git binary and the
// sandboxed filesystem view differs from the host's, so the command is
// delegated to the host via flatpak-spawn; repos then see the user's real
// git config and credential helpers.
func Git(dir string, args ...string) (string, error) {
	var cmd *exec.Cmd
	if os.Getenv("FLATPAK_ID") != "" {
		cmd = exec.Command("flatpak-spawn", append([]string{"--host", "git", "-C", dir}, args...)...)
	} else {
		cmd = exec.Command("git", args...)
		cmd.Dir = dir
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", &GitError{Args: args, Stderr: stderr.String(), Err: err}
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}
