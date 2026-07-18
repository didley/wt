package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/didley/wt/internal/core"
)

func resetRemoveFlags(t *testing.T) {
	t.Helper()
	removeStash, removeDiscard, removeDelBranch, removeForceBranch = false, false, false, false
	t.Cleanup(func() {
		removeStash, removeDiscard, removeDelBranch, removeForceBranch = false, false, false, false
	})
}

func TestRunRemoveClean(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{testBranchX}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	if err := runRemove(removeCmd, []string{"feature-x"}); err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wts {
		if w.Branch == testBranchX {
			t.Errorf("worktree feature/x still present after remove: %+v", w)
		}
	}
	if !repo.BranchExists(testBranchX) {
		t.Error("branch feature/x deleted by remove without --delete-branch")
	}
}

func TestRunRemoveDeleteBranch(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/y"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	removeDelBranch = true

	if err := runRemove(removeCmd, []string{"feature-y"}); err != nil {
		t.Fatalf("runRemove: %v", err)
	}
	if repo.BranchExists("feature/y") {
		t.Error("branch feature/y still exists after --delete-branch")
	}
}

func TestRunRemoveNoCandidates(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	newTestRepo(t)
	if err := runRemove(removeCmd, nil); err == nil {
		t.Fatal("runRemove with nothing to remove: want error, got nil")
	}
}

func TestRunRemoveUnknownTarget(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/z"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runRemove(removeCmd, []string{"does-not-exist"}); err == nil {
		t.Fatal("runRemove with unknown target: want error, got nil")
	}
	_ = repo
}

func TestRunRemoveDirtyNonInteractiveNeedsFlag(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/dirty"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, w := range wts {
		if w.Branch == "feature/dirty" {
			path = w.Path
		}
	}
	writeFile(t, path+"/dirty.txt", "wip\n")

	// yes=true means anyDirty is reached but actions stay actKeepClean since
	// neither --stash nor --discard is set: with yes=true the "have
	// uncommitted changes" branch is skipped for the confirm, so removal
	// proceeds via force (t.Prunable false, actions[i] keepClean -> force
	// depends on Locked/Prunable). To exercise the real guard we turn yes off
	// and rely on non-interactive (no TTY) behavior instead.
	yes = false
	err = runRemove(removeCmd, []string{"feature-dirty"})
	if err == nil {
		t.Fatal("runRemove on dirty worktree without --stash/--discard, non-interactive: want error, got nil")
	}
}

func TestRunRemoveDirtyWithStash(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/stash"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, w := range wts {
		if w.Branch == "feature/stash" {
			path = w.Path
		}
	}
	writeFile(t, path+"/dirty.txt", "wip\n")
	removeStash = true

	if err := runRemove(removeCmd, []string{"feature-stash"}); err != nil {
		t.Fatalf("runRemove: %v", err)
	}
	stashes := mustGit(t, repo.MainPath, "stash", "list")
	if len(stashes) == 0 {
		t.Error("expected a stash entry after --stash removal, found none")
	}
}

const branchDiscard = "feature/discard"

func TestRunRemoveDirtyWithDiscard(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{branchDiscard}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, w := range wts {
		if w.Branch == branchDiscard {
			path = w.Path
		}
	}
	writeFile(t, path+"/dirty.txt", "wip\n")
	removeDiscard = true

	if err := runRemove(removeCmd, []string{"feature-discard"}); err != nil {
		t.Fatalf("runRemove: %v", err)
	}
	wts, err = repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wts {
		if w.Branch == branchDiscard {
			t.Errorf("worktree feature/discard still present after --discard remove: %+v", w)
		}
	}
}

// TestRunRemoveDirtyNonInteractiveWithStashFlag exercises confirmRemoval's
// !interactive() path when the dirty action is already resolved (not
// actKeepClean), which should proceed without needing --yes.
func TestRunRemoveDirtyNonInteractiveWithStashFlag(t *testing.T) {
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	yes = true
	if err := runAdd(addCmd, []string{"feature/stash2"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, w := range wts {
		if w.Branch == "feature/stash2" {
			path = w.Path
		}
	}
	writeFile(t, path+"/dirty.txt", "wip\n")
	yes = false
	t.Cleanup(func() { yes = false })
	removeStash = true

	if err := runRemove(removeCmd, []string{"feature-stash2"}); err != nil {
		t.Fatalf("runRemove (dirty, non-interactive, --stash): %v", err)
	}
}

func TestRunRemoveCleanNonInteractiveNeedsYes(t *testing.T) {
	resetRemoveFlags(t)
	newTestRepo(t)
	yes = true
	if err := runAdd(addCmd, []string{"feature/clean-ni"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	yes = false
	t.Cleanup(func() { yes = false })

	if err := runRemove(removeCmd, []string{"feature-clean-ni"}); err == nil {
		t.Fatal("runRemove on a clean worktree, non-interactive without --yes: want error, got nil")
	}
}

func TestRunRemoveLockedYesBypass(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/locked-rm"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runLock(lockCmd, []string{"feature-locked-rm"}); err != nil {
		t.Fatalf("runLock: %v", err)
	}

	if err := runRemove(removeCmd, []string{"feature-locked-rm"}); err != nil {
		t.Fatalf("runRemove on a locked worktree with --yes: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wts {
		if w.Branch == "feature/locked-rm" {
			t.Errorf("locked worktree still present after remove --yes: %+v", w)
		}
	}
}

func TestRunRemoveLockedNonInteractiveNeedsOverride(t *testing.T) {
	resetRemoveFlags(t)
	newTestRepo(t)
	yes = true
	if err := runAdd(addCmd, []string{"feature/locked-rm2"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runLock(lockCmd, []string{"feature-locked-rm2"}); err != nil {
		t.Fatalf("runLock: %v", err)
	}
	yes = false
	t.Cleanup(func() { yes = false })

	if err := runRemove(removeCmd, []string{"feature-locked-rm2"}); err == nil {
		t.Fatal("runRemove on a locked worktree, non-interactive without --yes: want error, got nil")
	}
}

func TestDirtyActionsMixedPrunable(t *testing.T) {
	repo := newTestRepo(t)
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	cleanPath := repo.ConventionalPath("feature-clean-da")
	if err := repo.AddWorktree(cleanPath, "feature/clean-da", "main", true); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	targets := []core.Worktree{
		{Path: testMissingPath, Prunable: true},
		{Path: cleanPath},
	}
	actions, changes, err := dirtyActions(repo, targets)
	if err != nil {
		t.Fatalf("dirtyActions: %v", err)
	}
	if actions[0] != actKeepClean || changes[0] != nil {
		t.Errorf("prunable target: actions[0] = %q, changes[0] = %+v, want keepClean/nil", actions[0], changes[0])
	}
	if actions[1] != actKeepClean {
		t.Errorf("clean target: actions[1] = %q, want keepClean", actions[1])
	}
}

func TestDirtyActionsStatusError(t *testing.T) {
	repo := newTestRepo(t)
	targets := []core.Worktree{{Path: testMissingPath}}
	if _, _, err := dirtyActions(repo, targets); err == nil {
		t.Fatal("dirtyActions on a non-worktree path: want error, got nil")
	}
}

func TestRemoveTargetsStashFailure(t *testing.T) {
	repo := newTestRepo(t)
	targets := []core.Worktree{{Path: testMissingPath, Branch: "feature/x"}}
	actions := []string{actStash}
	out := captureStderr(t, func() {
		removed := removeTargets(repo, targets, actions)
		if len(removed) != 0 {
			t.Errorf("removeTargets with a stash failure: want nothing removed, got %+v", removed)
		}
	})
	if !contains(out, "stash failed") {
		t.Errorf("expected a \"stash failed\" warning, got: %q", out)
	}
}

func TestRemoveTargetsRemoveFailure(t *testing.T) {
	repo := newTestRepo(t)
	targets := []core.Worktree{{Path: testMissingPath, Branch: "feature/x"}}
	actions := []string{actKeepClean}
	out := captureStderr(t, func() {
		removed := removeTargets(repo, targets, actions)
		if len(removed) != 0 {
			t.Errorf("removeTargets on a non-worktree path: want nothing removed, got %+v", removed)
		}
	})
	if !contains(out, "could not remove") {
		t.Errorf("expected a \"could not remove\" warning, got: %q", out)
	}
}

func TestRunRemovePrunableTarget(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/prune-rm"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, w := range wts {
		if w.Branch == "feature/prune-rm" {
			path = w.Path
		}
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}

	if err := runRemove(removeCmd, []string{"feature-prune-rm"}); err != nil {
		t.Fatalf("runRemove on a prunable worktree: %v", err)
	}
}

func TestRunRemoveDeleteBranchUnmergedKept(t *testing.T) {
	withYes(t)
	resetRemoveFlags(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/unmerged"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, w := range wts {
		if w.Branch == "feature/unmerged" {
			path = w.Path
		}
	}
	writeFile(t, filepath.Join(path, "extra.txt"), "unmerged work\n")
	mustGit(t, path, "add", ".")
	mustGit(t, path, "commit", "-m", "unmerged commit")

	removeDelBranch = true
	out := captureStderr(t, func() {
		if err := runRemove(removeCmd, []string{"feature-unmerged"}); err != nil {
			t.Fatalf("runRemove: %v", err)
		}
	})
	if !repo.BranchExists("feature/unmerged") {
		t.Error("unmerged branch was deleted despite -d refusing it")
	}
	if !contains(out, "was kept") {
		t.Errorf("expected a \"was kept\" warning for the unmerged branch, got: %q", out)
	}
}
