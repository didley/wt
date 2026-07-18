package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoveWorktreeStash covers removeOneWorktree's actionStash branch,
// which TestRemoveWorktree/TestRemoveWorktreeDirtyRequiresChoice don't
// exercise (they only cover the no-action and actionDiscard paths).
func TestRemoveWorktreeStash(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}
	path := addWorktree(t, repo, "feature-x", featureXBranch)
	if err := os.WriteFile(filepath.Join(path, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg, err := a.RemoveWorktree(repo.MainPath, path, actionStash, false, false, false)
	if err != nil {
		t.Fatalf("RemoveWorktree(stash): %v", err)
	}
	if !strings.Contains(msg, "stash") {
		t.Errorf("RemoveWorktree(stash) msg = %q, want mention of stash", msg)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree directory still present after stash removal")
	}
}

// TestRemoveWorktreeBranchDeleteFails covers removeOneWorktree's
// DeleteBranch-error path: a non-force branch delete on an unmerged branch
// should remove the worktree but report the branch as kept.
func TestRemoveWorktreeBranchDeleteFails(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}
	path := addWorktree(t, repo, "feature-x", featureXBranch)
	if err := os.WriteFile(filepath.Join(path, "unmerged.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, path, "add", ".")
	mustGit(t, path, "commit", "-m", "unmerged change")

	_, err := a.RemoveWorktree(repo.MainPath, path, "", true, false, false)
	if err == nil {
		t.Fatal("RemoveWorktree(delete unmerged branch, no force) = nil error, want error")
	}
	if !strings.Contains(err.Error(), "branch was kept") {
		t.Errorf("RemoveWorktree() err = %v, want mention of branch kept", err)
	}
}

// TestConfigPath_UserConfigDirUnavailable covers configPath/saveConfig/
// loadConfig's early-return branches when os.UserConfigDir() can't resolve
// a directory (no HOME/XDG_CONFIG_HOME).
func TestConfigPath_UserConfigDirUnavailable(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if got := configPath(); got != "" {
		t.Errorf("configPath() = %q, want empty when UserConfigDir fails", got)
	}

	// Must not panic even though there's nowhere to write.
	saveConfig(guiConfig{Recent: []string{"/a"}})

	if cfg := loadConfig(); len(cfg.Recent) != 0 {
		t.Errorf("loadConfig() = %+v, want empty when UserConfigDir fails", cfg)
	}
}
