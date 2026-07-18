package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/didley/wt/internal/core"
)

const (
	mainBranch     = "main"
	featureXBranch = "feature/x"
	featureXWtPath = "/repo.worktrees/feature"
)

// newTestRepo creates a real git repo named my-app in a temp dir with one
// commit on main, isolated from the developer's global git config. Mirrors
// internal/core's own test helper since App methods exercise core end to end.
func newTestRepo(t *testing.T) *core.Repo {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	main := filepath.Join(t.TempDir(), "my-app")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, main, "init", "-b", mainBranch)
	mustGit(t, main, "config", "user.email", "wt@test.invalid")
	mustGit(t, main, "config", "user.name", "wt test")
	if err := os.WriteFile(filepath.Join(main, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, main, "add", ".")
	mustGit(t, main, "commit", "-m", "init")
	repo, err := core.Discover(main)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
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

func addWorktree(t *testing.T, repo *core.Repo, name, branch string) string {
	t.Helper()
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := repo.ConventionalPath(name)
	if err := repo.AddWorktree(path, branch, mainBranch, true); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	return path
}

func TestNewAppVersionOS(t *testing.T) {
	a := NewApp()
	if a == nil {
		t.Fatal("NewApp() = nil")
	}
	if got := a.Version(); got != version {
		t.Errorf("Version() = %q, want %q", got, version)
	}
	if got := a.OS(); got == "" {
		t.Error("OS() = empty string")
	}
}

func TestFindWorktree(t *testing.T) {
	wts := []core.Worktree{
		{Path: "/repo", IsMain: true},
		{Path: featureXWtPath},
	}
	if got := findWorktree(wts, featureXWtPath); got == nil || got.Path != featureXWtPath {
		t.Errorf("findWorktree() = %+v, want match", got)
	}
	if got := findWorktree(wts, "/missing"); got != nil {
		t.Errorf("findWorktree(missing) = %+v, want nil", got)
	}
}

func TestFindLinkedWorktree(t *testing.T) {
	wts := []core.Worktree{
		{Path: "/repo", IsMain: true},
		{Path: featureXWtPath},
	}
	if got := findLinkedWorktree(wts, "/repo"); got != nil {
		t.Errorf("findLinkedWorktree(main) = %+v, want nil (main excluded)", got)
	}
	if got := findLinkedWorktree(wts, featureXWtPath); got == nil {
		t.Error("findLinkedWorktree(linked) = nil, want match")
	}
}

func TestCheckRemovable(t *testing.T) {
	repo := newTestRepo(t)

	// Locked without override: refused.
	locked := &core.Worktree{Path: repo.MainPath, Locked: true, LockReason: "wip"}
	if err := checkRemovable(locked, "name", "", false); err == nil {
		t.Error("checkRemovable(locked, no override) = nil, want error")
	}
	if err := checkRemovable(locked, "name", "", true); err != nil {
		t.Errorf("checkRemovable(locked, forced) = %v, want nil", err)
	}

	// Prunable: always fine, no status check needed.
	prunable := &core.Worktree{Path: "/does/not/exist", Prunable: true}
	if err := checkRemovable(prunable, "name", "", false); err != nil {
		t.Errorf("checkRemovable(prunable) = %v, want nil", err)
	}

	// Clean worktree: fine with no action.
	clean := &core.Worktree{Path: repo.MainPath}
	if err := checkRemovable(clean, "name", "", false); err != nil {
		t.Errorf("checkRemovable(clean) = %v, want nil", err)
	}

	// Dirty worktree with no action chosen: needs a choice.
	if err := os.WriteFile(filepath.Join(repo.MainPath, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty := &core.Worktree{Path: repo.MainPath}
	if err := checkRemovable(dirty, "name", "", false); !strings.Contains(err.Error(), "stash or discard") {
		t.Errorf("checkRemovable(dirty, no action) = %v, want dirty-needs-choice error", err)
	}
	if err := checkRemovable(dirty, "name", actionDiscard, false); err != nil {
		t.Errorf("checkRemovable(dirty, discard) = %v, want nil", err)
	}
	if err := checkRemovable(dirty, "name", actionStash, false); err != nil {
		t.Errorf("checkRemovable(dirty, stash) = %v, want nil", err)
	}
}

func TestRenameBranchMessage(t *testing.T) {
	repo := newTestRepo(t)
	addWorktree(t, repo, "feature-x", featureXBranch)
	target := &core.Worktree{Path: repo.ConventionalPath("feature-x"), Branch: featureXBranch}

	// Rename worktree only, branch untouched.
	msg, err := renameBranchMessage(repo, target, "feature-y", "feature/y", false)
	if err != nil {
		t.Fatalf("renameBranchMessage(no branch rename) error: %v", err)
	}
	if !strings.Contains(msg, `still "feature/x"`) {
		t.Errorf("renameBranchMessage() = %q, want mention of unchanged branch", msg)
	}

	// Rename branch too.
	msg, err = renameBranchMessage(repo, target, "feature-y", "feature/y", true)
	if err != nil {
		t.Fatalf("renameBranchMessage(branch rename) error: %v", err)
	}
	if !strings.Contains(msg, `Branch renamed to "feature/y"`) {
		t.Errorf("renameBranchMessage() = %q, want mention of renamed branch", msg)
	}

	// No branch (detached): no branch clause at all.
	detached := &core.Worktree{Path: repo.MainPath}
	msg, err = renameBranchMessage(repo, detached, "whatever", "whatever", true)
	if err != nil {
		t.Fatalf("renameBranchMessage(no branch) error: %v", err)
	}
	if strings.Contains(msg, "Branch") {
		t.Errorf("renameBranchMessage(no branch) = %q, want no branch clause", msg)
	}
}

func TestBuildRepoViewAndWorktreeView(t *testing.T) {
	repo := newTestRepo(t)
	addWorktree(t, repo, "feature-x", featureXBranch)

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatalf("Worktrees(): %v", err)
	}
	view := buildRepoView(repo, wts)

	if view.Name != repo.Name() {
		t.Errorf("RepoView.Name = %q, want %q", view.Name, repo.Name())
	}
	if len(view.Worktrees) != 2 {
		t.Fatalf("RepoView.Worktrees len = %d, want 2", len(view.Worktrees))
	}
	for _, wv := range view.Worktrees {
		if wv.IsMain {
			if wv.Name != repo.Name() {
				t.Errorf("main WorktreeView.Name = %q, want %q", wv.Name, repo.Name())
			}
			continue
		}
		if wv.Branch != featureXBranch {
			t.Errorf("linked WorktreeView.Branch = %q, want feature/x", wv.Branch)
		}
		if wv.Dirty {
			t.Errorf("clean worktree reported Dirty = true")
		}
	}
	// feature/x is checked out, so it must not appear among available branches.
	for _, b := range view.AvailableBranches {
		if b == featureXBranch {
			t.Errorf("AvailableBranches contains checked-out branch %q", b)
		}
	}

	// Dirty worktree feeds through into the view's changes/state.
	wtPath := repo.ConventionalPath("feature-x")
	if err := os.WriteFile(filepath.Join(wtPath, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	wts, err = repo.Worktrees()
	if err != nil {
		t.Fatalf("Worktrees(): %v", err)
	}
	view = buildRepoView(repo, wts)
	for _, wv := range view.Worktrees {
		if wv.Path != wtPath {
			continue
		}
		if !wv.Dirty {
			t.Error("dirty worktree reported Dirty = false")
		}
		if len(wv.Changes) == 0 {
			t.Error("dirty worktree reported no Changes")
		}
	}
}

func TestLoadRepo(t *testing.T) {
	repo := newTestRepo(t)
	withConfigDir(t)
	a := &App{}

	view, err := a.LoadRepo(repo.MainPath)
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if view.Name != repo.Name() {
		t.Errorf("LoadRepo().Name = %q, want %q", view.Name, repo.Name())
	}

	// A non-repo path fails with the "not a git repo" wrapper.
	if _, err := a.LoadRepo(t.TempDir()); err == nil {
		t.Error("LoadRepo(non-repo) = nil error, want error")
	}
}

func TestCreateWorktree(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}

	msg, err := a.CreateWorktree(repo.MainPath, "feature/new", "")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if !strings.Contains(msg, "new branch") {
		t.Errorf("CreateWorktree() msg = %q, want mention of new branch", msg)
	}
	if _, err := os.Stat(repo.ConventionalPath("feature-new")); err != nil {
		t.Errorf("worktree directory missing: %v", err)
	}

	// Empty branch name is rejected.
	if _, err := a.CreateWorktree(repo.MainPath, "", ""); !strings.Contains(err.Error(), "branch name") {
		t.Errorf("CreateWorktree(empty branch) err = %v, want branch-name-required", err)
	}

	// Branch already checked out elsewhere is rejected.
	if _, err := a.CreateWorktree(repo.MainPath, "feature/new", ""); err == nil {
		t.Error("CreateWorktree(already checked out) = nil error, want error")
	}

	// Existing branch, no new branch created.
	mustGit(t, repo.MainPath, "branch", "existing")
	msg, err = a.CreateWorktree(repo.MainPath, "existing", "")
	if err != nil {
		t.Fatalf("CreateWorktree(existing branch): %v", err)
	}
	if !strings.Contains(msg, "existing branch") {
		t.Errorf("CreateWorktree(existing) msg = %q, want mention of existing branch", msg)
	}
}

func TestCreateWorktrees(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}

	msg, err := a.CreateWorktrees(repo.MainPath, []string{"feature/a", "feature/b"}, "")
	if err != nil {
		t.Fatalf("CreateWorktrees: %v", err)
	}
	if !strings.Contains(msg, "Created 2 worktree") {
		t.Errorf("CreateWorktrees() msg = %q, want mention of 2 created", msg)
	}
	if _, err := os.Stat(repo.ConventionalPath("feature-a")); err != nil {
		t.Errorf("worktree directory missing: %v", err)
	}
	if _, err := os.Stat(repo.ConventionalPath("feature-b")); err != nil {
		t.Errorf("worktree directory missing: %v", err)
	}

	// Blank entries are skipped rather than erroring.
	msg, err = a.CreateWorktrees(repo.MainPath, []string{"feature/c", "  "}, "")
	if err != nil {
		t.Fatalf("CreateWorktrees(blank entry): %v", err)
	}
	if !strings.Contains(msg, "Created 1 worktree") {
		t.Errorf("CreateWorktrees(blank entry) msg = %q, want 1 created", msg)
	}
}

func TestCreateWorktreesPartialFailure(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}
	addWorktree(t, repo, "feature-a", "feature/a")

	// One branch is already checked out; the other should still be created.
	msg, err := a.CreateWorktrees(repo.MainPath, []string{"feature/a", "feature/b"}, "")
	if err == nil {
		t.Error("CreateWorktrees(one already checked out) = nil error, want joined error")
	}
	if !strings.Contains(msg, "Created 1 worktree") {
		t.Errorf("CreateWorktrees(partial failure) msg = %q, want 1 created", msg)
	}
	if _, err := os.Stat(repo.ConventionalPath("feature-b")); err != nil {
		t.Errorf("worktree directory missing: %v", err)
	}
}

func TestRemoveWorktree(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}
	path := addWorktree(t, repo, "feature-x", featureXBranch)

	// Main checkout can never be removed.
	_, err := a.RemoveWorktree(repo.MainPath, repo.MainPath, "", false, false, false)
	if !strings.Contains(err.Error(), "main checkout") {
		t.Errorf("RemoveWorktree(main) err = %v, want main-not-removable", err)
	}

	// Unknown path.
	if _, err := a.RemoveWorktree(repo.MainPath, "/nowhere", "", false, false, false); err == nil {
		t.Error("RemoveWorktree(unknown) = nil error, want error")
	}

	// Clean removal, branch kept.
	msg, err := a.RemoveWorktree(repo.MainPath, path, "", false, false, false)
	if err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if !strings.Contains(msg, "kept") {
		t.Errorf("RemoveWorktree() msg = %q, want mention of kept branch", msg)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree directory still present after removal")
	}

	// Removal with branch deletion.
	path2 := addWorktree(t, repo, "feature-y", "feature/y")
	msg, err = a.RemoveWorktree(repo.MainPath, path2, "", true, false, false)
	if err != nil {
		t.Fatalf("RemoveWorktree(delete branch): %v", err)
	}
	if !strings.Contains(msg, "deleted") {
		t.Errorf("RemoveWorktree(delete branch) msg = %q, want mention of deletion", msg)
	}
}

func TestRemoveWorktreeDirtyRequiresChoice(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}
	path := addWorktree(t, repo, "feature-x", featureXBranch)
	if err := os.WriteFile(filepath.Join(path, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := a.RemoveWorktree(repo.MainPath, path, "", false, false, false); err == nil {
		t.Error("RemoveWorktree(dirty, no action) = nil error, want error")
	}

	msg, err := a.RemoveWorktree(repo.MainPath, path, actionDiscard, false, false, false)
	if err != nil {
		t.Fatalf("RemoveWorktree(discard): %v", err)
	}
	if !strings.Contains(msg, "Removed worktree") {
		t.Errorf("RemoveWorktree(discard) msg = %q", msg)
	}
}

func TestRemoveWorktrees(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}
	p1 := addWorktree(t, repo, "feature-a", "feature/a")
	p2 := addWorktree(t, repo, "feature-b", "feature/b")

	msg, err := a.RemoveWorktrees(repo.MainPath, []string{p1, p2, "/unknown"}, "", false, false, false)
	if err != nil {
		t.Fatalf("RemoveWorktrees: %v", err)
	}
	if !strings.Contains(msg, "Removed 2 worktree") {
		t.Errorf("RemoveWorktrees() msg = %q, want mention of 2 removed", msg)
	}

	// Removing the main checkout among a batch is silently skipped, not an error.
	p3 := addWorktree(t, repo, "feature-c", "feature/c")
	msg, err = a.RemoveWorktrees(repo.MainPath, []string{repo.MainPath, p3}, "", false, false, false)
	if err != nil {
		t.Fatalf("RemoveWorktrees(with main): %v", err)
	}
	if !strings.Contains(msg, "Removed 1 worktree") {
		t.Errorf("RemoveWorktrees(with main) msg = %q, want 1 removed", msg)
	}
}

func TestRemoveWorktreesPartialFailure(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}
	path := addWorktree(t, repo, "feature-a", "feature/a")
	if err := os.WriteFile(filepath.Join(path, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := a.RemoveWorktrees(repo.MainPath, []string{path}, "", false, false, false)
	if err == nil {
		t.Error("RemoveWorktrees(dirty, no action) = nil error, want joined error")
	}
}

func TestRenameWorktree(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}
	path := addWorktree(t, repo, "feature-x", featureXBranch)

	msg, err := a.RenameWorktree(repo.MainPath, path, "feature/y", false)
	if err != nil {
		t.Fatalf("RenameWorktree: %v", err)
	}
	if !strings.Contains(msg, "Renamed worktree") {
		t.Errorf("RenameWorktree() msg = %q", msg)
	}
	if _, err := os.Stat(repo.ConventionalPath("feature-y")); err != nil {
		t.Errorf("renamed worktree directory missing: %v", err)
	}

	// Empty new name rejected.
	if _, err := a.RenameWorktree(repo.MainPath, repo.ConventionalPath("feature-y"), "///", false); err == nil {
		t.Error("RenameWorktree(empty sanitized name) = nil error, want error")
	}

	// Main checkout cannot be renamed (findLinkedWorktree excludes it).
	if _, err := a.RenameWorktree(repo.MainPath, repo.MainPath, "whatever", false); err == nil {
		t.Error("RenameWorktree(main) = nil error, want error")
	}
}

func TestLockUnlockWorktree(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}
	path := addWorktree(t, repo, "feature-x", featureXBranch)

	msg, err := a.LockWorktree(repo.MainPath, path, "wip")
	if err != nil {
		t.Fatalf("LockWorktree: %v", err)
	}
	if !strings.Contains(msg, "wip") {
		t.Errorf("LockWorktree() msg = %q, want reason included", msg)
	}

	// Locking an already-locked worktree fails.
	if _, err := a.LockWorktree(repo.MainPath, path, "again"); err == nil {
		t.Error("LockWorktree(already locked) = nil error, want error")
	}

	msg, err = a.UnlockWorktree(repo.MainPath, path)
	if err != nil {
		t.Fatalf("UnlockWorktree: %v", err)
	}
	if !strings.Contains(msg, "Unlocked") {
		t.Errorf("UnlockWorktree() msg = %q", msg)
	}

	// Unlocking an already-unlocked worktree fails.
	if _, err := a.UnlockWorktree(repo.MainPath, path); err == nil {
		t.Error("UnlockWorktree(not locked) = nil error, want error")
	}

	// Locking/unlocking an unknown path fails.
	if _, err := a.LockWorktree(repo.MainPath, "/nowhere", ""); err == nil {
		t.Error("LockWorktree(unknown) = nil error, want error")
	}
	if _, err := a.UnlockWorktree(repo.MainPath, "/nowhere"); err == nil {
		t.Error("UnlockWorktree(unknown) = nil error, want error")
	}
}

func TestPruneStale(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}

	msg, err := a.PruneStale(repo.MainPath)
	if err != nil {
		t.Fatalf("PruneStale(nothing to prune): %v", err)
	}
	if msg != "Nothing to prune." {
		t.Errorf("PruneStale() msg = %q, want %q", msg, "Nothing to prune.")
	}

	path := addWorktree(t, repo, "feature-x", featureXBranch)
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}

	msg, err = a.PruneStale(repo.MainPath)
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if !strings.Contains(msg, "Pruned 1 stale worktree entry") {
		t.Errorf("PruneStale() msg = %q, want mention of 1 pruned entry", msg)
	}
}

func TestMoveStrays(t *testing.T) {
	repo := newTestRepo(t)
	a := &App{}

	msg, err := a.MoveStrays(repo.MainPath)
	if err != nil {
		t.Fatalf("MoveStrays(nothing to move): %v", err)
	}
	if msg != "Nothing to move." {
		t.Errorf("MoveStrays() msg = %q, want %q", msg, "Nothing to move.")
	}

	// A worktree living outside <repo>.worktrees/ is a stray.
	strayPath := filepath.Join(filepath.Dir(repo.MainPath), "stray-checkout")
	if err := repo.AddWorktree(strayPath, "feature/stray", mainBranch, true); err != nil {
		t.Fatalf("AddWorktree(stray): %v", err)
	}

	msg, err = a.MoveStrays(repo.MainPath)
	if err != nil {
		t.Fatalf("MoveStrays: %v", err)
	}
	if !strings.Contains(msg, "Moved 1 worktree") {
		t.Errorf("MoveStrays() msg = %q, want mention of 1 moved", msg)
	}
	if _, err := os.Stat(repo.ConventionalPath("feature-stray")); err != nil {
		t.Errorf("stray worktree not moved into convention: %v", err)
	}
}
