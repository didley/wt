package cli

import (
	"os"
	"testing"
)

func TestRunAddNewBranch(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	addFrom = ""
	t.Cleanup(func() { addFrom = "" })

	if err := runAdd(addCmd, []string{"feature/new"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range wts {
		if w.Branch == "feature/new" {
			found = true
		}
	}
	if !found {
		t.Errorf("worktree for feature/new not found: %+v", wts)
	}
}

func TestRunAddExistingBranch(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	mustGit(t, repo.MainPath, "branch", testBranchExisting)

	if err := runAdd(addCmd, []string{testBranchExisting}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range wts {
		if w.Branch == testBranchExisting {
			found = true
		}
	}
	if !found {
		t.Errorf("worktree for existing branch not found: %+v", wts)
	}
}

func TestRunAddAlreadyCheckedOut(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)

	if err := runAdd(addCmd, []string{testBranchA}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runAdd(addCmd, []string{testBranchA}); err == nil {
		t.Fatal("runAdd on already-checked-out branch: want error, got nil")
	}
	_ = repo
}

func TestRunAddBatch(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)

	if err := runAdd(addCmd, []string{"feature/one", "feature/two"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, w := range wts {
		found[w.Branch] = true
	}
	for _, b := range []string{"feature/one", "feature/two"} {
		if !found[b] {
			t.Errorf("worktree for %s not found: %+v", b, wts)
		}
	}
}

func TestRunAddBatchSkipsCheckedOutAndKeepsGoing(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	mustGit(t, repo.MainPath, "branch", testBranchExisting)

	if err := runAdd(addCmd, []string{testBranchA, testBranchExisting}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runAdd(addCmd, []string{testBranchA, "feature/fresh"}); err != nil {
		t.Fatalf("runAdd with one already-checked-out branch: %v", err)
	}

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range wts {
		if w.Branch == "feature/fresh" {
			found = true
		}
	}
	if !found {
		t.Error("batch add should still create the branch that wasn't already checked out")
	}
}

func TestRunAddFromIgnoredForExistingBranch(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	mustGit(t, repo.MainPath, "branch", testBranchExisting)
	addFrom = repo.DefaultBranch()
	t.Cleanup(func() { addFrom = "" })

	out := captureStderr(t, func() {
		if err := runAdd(addCmd, []string{testBranchExisting}); err != nil {
			t.Fatalf("runAdd: %v", err)
		}
	})
	if !contains(out, "--from is ignored") {
		t.Errorf("runAdd with --from on an existing branch: want a warning, got %q", out)
	}
}

func TestRunAddBatchContinuesPastCreateFailure(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)

	// Occupy the conventional path for "feature/blocked" so createWorktree fails for it.
	blockedPath := repo.ConventionalPath("feature-blocked")
	if err := os.MkdirAll(blockedPath, dirPerm); err != nil {
		t.Fatal(err)
	}

	out := captureStderr(t, func() {
		if err := runAdd(addCmd, []string{"feature/blocked", "feature/ok"}); err != nil {
			t.Fatalf("runAdd: %v", err)
		}
	})
	if !contains(out, "feature/blocked") {
		t.Errorf("runAdd: want a warning naming the failed branch, got %q", out)
	}

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range wts {
		if w.Branch == "feature/ok" {
			found = true
		}
	}
	if !found {
		t.Error("batch add should still create the branch after an earlier one failed")
	}
}

func TestRunAddNotARepo(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runAdd(addCmd, []string{testBranchX}); err == nil {
		t.Fatal("runAdd outside a repo: want error, got nil")
	}
}

func TestRunAddNoArgsNonInteractive(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runAdd(addCmd, nil); err == nil {
		t.Fatal("runAdd with no args, non-interactive: want error, got nil")
	}
}
