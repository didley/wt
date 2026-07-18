package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/didley/wt/internal/core"
)

// Branch/worktree names shared across tests in this package.
const (
	testBranchA         = "feature/a"
	testBranchX         = "feature/x"
	testBranchGone      = "feature/gone"
	testBranchLock      = "feature-lock"
	testBranchLockSlash = "feature/lock"
	testNameNope        = "nope" // a name that never resolves to a worktree
)

// newTestRepo creates a real git repo named my-app in a temp dir with one
// commit on main, isolated from the developer's global git config, and
// chdirs the test into it (mirrors internal/core/core_test.go's helper,
// adapted for cli commands that call discover() -> core.Discover(".")).
func newTestRepo(t *testing.T) *core.Repo {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	main := filepath.Join(t.TempDir(), "my-app")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, main, "init", "-b", "main")
	mustGit(t, main, "config", "user.email", "wt@test.invalid")
	mustGit(t, main, "config", "user.name", "wt test")
	writeFile(t, filepath.Join(main, "README.md"), "hello\n")
	mustGit(t, main, "add", ".")
	mustGit(t, main, "commit", "-m", "init")
	repo, err := core.Discover(main)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	t.Chdir(main)
	return repo
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := core.Git(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// withYes sets the package-level --yes flag for the duration of the test,
// bypassing interactive prompts that need a real TTY.
func withYes(t *testing.T) {
	t.Helper()
	old := yes
	yes = true
	t.Cleanup(func() { yes = old })
}
