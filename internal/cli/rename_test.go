package cli

import "testing"

func TestRunRename(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/old"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	if err := runRename(renameCmd, []string{"feature-old", "renamed"}); err != nil {
		t.Fatalf("runRename: %v", err)
	}

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, w := range wts {
		if w.Branch == "feature/old" {
			found = true
			if repo.WorktreeName(w) != "renamed" {
				t.Errorf("WorktreeName after rename = %q, want renamed", repo.WorktreeName(w))
			}
		}
	}
	if !found {
		t.Fatal("worktree with branch feature/old not found after rename")
	}
}

func TestRunRenameWithBranch(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	renameBranchToo = true
	t.Cleanup(func() { renameBranchToo = false })
	if err := runAdd(addCmd, []string{"feature/oldb"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	if err := runRename(renameCmd, []string{"feature-oldb", "newb"}); err != nil {
		t.Fatalf("runRename: %v", err)
	}
	if repo.BranchExists("feature/oldb") {
		t.Error("old branch name still exists after --branch rename")
	}
	if !repo.BranchExists("newb") {
		t.Error("new branch name does not exist after --branch rename")
	}
}

func TestRunRenameUnknownTarget(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runRename(renameCmd, []string{"nope", "new"}); err == nil {
		t.Fatal("runRename on unknown target: want error, got nil")
	}
}

func TestRunRenameExistingDestination(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/a"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runAdd(addCmd, []string{"feature/b"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runRename(renameCmd, []string{"feature-a", "feature-b"}); err == nil {
		t.Fatal("runRename onto an existing directory: want error, got nil")
	}
}
