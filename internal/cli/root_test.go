package cli

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

// captureStderr redirects os.Stderr for the duration of fn and returns
// whatever was written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = orig
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestConventionCheckSkippedCommands(t *testing.T) {
	t.Chdir(t.TempDir()) // not even a repo — proves these names return before discover()
	for _, name := range []string{
		rootCmd.Name(), listCmd.Name(), organizeCmd.Name(), pruneCmd.Name(), setupCmd.Name(), genManCmd.Name(),
		"version", "__complete", "__completeNoDesc",
	} {
		cmd := &cobra.Command{Use: name}
		out := captureStderr(t, func() { conventionCheck(cmd) })
		if out != "" {
			t.Errorf("conventionCheck(%q) wrote to stderr: %q", name, out)
		}
	}
}

func TestConventionCheckNotARepo(t *testing.T) {
	t.Chdir(t.TempDir())
	cmd := &cobra.Command{Use: addCmd.Name()}
	out := captureStderr(t, func() { conventionCheck(cmd) })
	if out != "" {
		t.Errorf("conventionCheck outside a repo: want silent, got %q", out)
	}
}

func TestConventionCheckClean(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	cmd := &cobra.Command{Use: addCmd.Name()}
	out := captureStderr(t, func() { conventionCheck(cmd) })
	if out != "" {
		t.Errorf("conventionCheck on a clean repo: want silent, got %q", out)
	}
}

func TestConventionCheckWarnsOnStray(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	stray := repo.MainPath + "-stray"
	mustGit(t, repo.MainPath, "worktree", "add", "-b", "stray/branch", stray)

	cmd := &cobra.Command{Use: addCmd.Name()}
	out := captureStderr(t, func() { conventionCheck(cmd) })
	if out == "" {
		t.Fatal("conventionCheck with a stray worktree: want a warning, got none")
	}
	if !contains(out, stray) {
		t.Errorf("conventionCheck output = %q, want it to mention %q", out, stray)
	}
	if !contains(out, "wt organize") {
		t.Errorf("conventionCheck output = %q, want it to mention `wt organize`", out)
	}
}

// TestConventionCheckSkipsListAndRoot ensures `wt` and `wt list` never emit
// the stray-worktree warning themselves: their own row-based output already
// covers it, so conventionCheck would otherwise duplicate it.
func TestConventionCheckSkipsListAndRoot(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	stray := repo.MainPath + "-stray"
	mustGit(t, repo.MainPath, "worktree", "add", "-b", "stray/branch", stray)

	for _, name := range []string{listCmd.Name(), rootCmd.Name()} {
		cmd := &cobra.Command{Use: name}
		out := captureStderr(t, func() { conventionCheck(cmd) })
		if out != "" {
			t.Errorf("conventionCheck(%q) with a stray worktree: want silent, got %q", name, out)
		}
	}
}

// TestRootRunNonInteractive covers `wt` (no args) with --yes: it must list
// worktrees and skip the interactive menu since --yes forces non-interactive.
func TestRootRunNonInteractive(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := rootCmd.RunE(rootCmd, nil); err != nil {
		t.Fatalf("rootCmd.RunE: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
