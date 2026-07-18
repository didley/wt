package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/didley/wt/internal/core"
)

func TestLinkedWorktrees(t *testing.T) {
	wts := []core.Worktree{
		{Path: "/main", IsMain: true},
		{Path: "/bare", Bare: true},
		{Path: "/feature", Branch: "feature/x"},
	}
	got := linkedWorktrees(wts)
	if len(got) != 1 || got[0].Path != "/feature" {
		t.Errorf("linkedWorktrees = %+v, want only /feature", got)
	}
}

func TestLinkedWorktreesEmpty(t *testing.T) {
	if got := linkedWorktrees(nil); got != nil {
		t.Errorf("linkedWorktrees(nil) = %+v, want nil", got)
	}
}

func TestResolveWorktree(t *testing.T) {
	repo := newTestRepo(t)
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := repo.ConventionalPath("feature-x")
	if err := repo.AddWorktree(path, "feature/x", "main", true); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}

	cases := []string{"feature-x", "feature/x", path}
	for _, arg := range cases {
		got, err := resolveWorktree(repo, wts, arg)
		if err != nil {
			t.Errorf("resolveWorktree(%q): %v", arg, err)
			continue
		}
		if got.Path != path {
			t.Errorf("resolveWorktree(%q).Path = %q, want %q", arg, got.Path, path)
		}
	}

	if _, err := resolveWorktree(repo, wts, "nope"); err == nil {
		t.Error("resolveWorktree(nope): want error, got nil")
	}
}

func TestDiscover(t *testing.T) {
	newTestRepo(t)
	repo, err := discover()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if repo.Name() != "my-app" {
		t.Errorf("Name() = %q, want my-app", repo.Name())
	}
}

func TestDiscoverNotARepo(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := discover(); err == nil {
		t.Fatal("discover() outside a repo: want error, got nil")
	}
}

func TestDiscoverBare(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	dir := filepath.Join(t.TempDir(), "bare.git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Git(dir, "init", "--bare"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	_, err := discover()
	if err == nil {
		t.Fatal("discover() on bare repo: want error, got nil")
	}
	if errors.Is(err, core.ErrBareRepo) {
		t.Error("discover() should translate ErrBareRepo into its own message, not pass it through raw")
	}
	if !strings.Contains(err.Error(), "bare") {
		t.Errorf("discover() error = %q, want it to mention bare repositories", err.Error())
	}
}
