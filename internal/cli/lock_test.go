package cli

import "testing"

func TestRunLockAndUnlock(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/lock"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	lockReason = "keep me around"
	t.Cleanup(func() { lockReason = "" })

	if err := runLock(lockCmd, []string{"feature-lock"}); err != nil {
		t.Fatalf("runLock: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, w := range wts {
		if w.Branch == "feature/lock" {
			found = true
			if !w.Locked || w.LockReason != "keep me around" {
				t.Errorf("worktree not locked as expected: %+v", w)
			}
		}
	}
	if !found {
		t.Fatal("worktree feature/lock not found")
	}

	// Locking again should fail.
	if err := runLock(lockCmd, []string{"feature-lock"}); err == nil {
		t.Error("runLock on already-locked worktree: want error, got nil")
	}

	if err := runUnlock(unlockCmd, []string{"feature-lock"}); err != nil {
		t.Fatalf("runUnlock: %v", err)
	}
	wts, err = repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wts {
		if w.Branch == "feature/lock" && w.Locked {
			t.Errorf("worktree still locked after unlock: %+v", w)
		}
	}

	if err := runUnlock(unlockCmd, []string{"feature-lock"}); err == nil {
		t.Error("runUnlock when nothing is locked: want error, got nil")
	}
}

func TestRunLockUnknownTarget(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/lk"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runLock(lockCmd, []string{"nope"}); err == nil {
		t.Fatal("runLock on unknown target: want error, got nil")
	}
}

func TestRunLockNoCandidates(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runLock(lockCmd, nil); err == nil {
		t.Fatal("runLock with no worktrees and no args: want error, got nil")
	}
}
