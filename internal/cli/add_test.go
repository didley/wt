package cli

import (
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
	mustGit(t, repo.MainPath, "branch", "existing")

	if err := runAdd(addCmd, []string{"existing"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range wts {
		if w.Branch == "existing" {
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

	if err := runAdd(addCmd, []string{"feature/a"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runAdd(addCmd, []string{"feature/a"}); err == nil {
		t.Fatal("runAdd on already-checked-out branch: want error, got nil")
	}
	_ = repo
}

func TestRunAddNoArgsNonInteractive(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runAdd(addCmd, nil); err == nil {
		t.Fatal("runAdd with no args, non-interactive: want error, got nil")
	}
}
