package cli

import "testing"

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
	if err := runAdd(addCmd, []string{"feature/x"}); err != nil {
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
		if w.Branch == "feature/x" {
			t.Errorf("worktree feature/x still present after remove: %+v", w)
		}
	}
	if !repo.BranchExists("feature/x") {
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
