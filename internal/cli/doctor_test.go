package cli

import (
	"os"
	"testing"
)

func TestRunDoctorClean(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runDoctor(doctorCmd, nil); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
}

func TestRunDoctorFixesStrayAndPrunable(t *testing.T) {
	withYes(t)
	doctorFix = true
	t.Cleanup(func() { doctorFix = false })
	repo := newTestRepo(t)

	// A stray worktree, created outside <repo>.worktrees/.
	stray := repo.MainPath + "-stray"
	mustGit(t, repo.MainPath, "worktree", "add", "-b", "stray/branch", stray)

	// A prunable worktree: added conventionally, then its directory removed.
	if err := runAdd(addCmd, []string{testBranchGone}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var gonePath string
	for _, w := range wts {
		if w.Branch == testBranchGone {
			gonePath = w.Path
		}
	}
	if err := os.RemoveAll(gonePath); err != nil {
		t.Fatal(err)
	}

	if err := runDoctor(doctorCmd, nil); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}

	wts, err = repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	if vs := repo.Violations(wts); len(vs) != 0 {
		t.Errorf("violations remain after doctor --fix: %+v", vs)
	}
	for _, w := range wts {
		if w.Path == gonePath {
			t.Errorf("stale worktree %q still registered after doctor --fix", gonePath)
		}
	}
}

func TestRunDoctorNoFixNonInteractiveWarns(t *testing.T) {
	repo := newTestRepo(t)
	yes = true
	stray := repo.MainPath + "-stray2"
	mustGit(t, repo.MainPath, "worktree", "add", "-b", "stray/branch2", stray)
	if err := runAdd(addCmd, []string{testBranchGone}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var gonePath string
	for _, w := range wts {
		if w.Branch == testBranchGone {
			gonePath = w.Path
		}
	}
	if err := os.RemoveAll(gonePath); err != nil {
		t.Fatal(err)
	}
	yes = false
	t.Cleanup(func() { yes = false })

	out := captureStderr(t, func() {
		if err := runDoctor(doctorCmd, nil); err != nil {
			t.Fatalf("runDoctor: %v", err)
		}
	})
	if !contains(out, "--fix") {
		t.Errorf("runDoctor without --fix, non-interactive: want a hint to re-run with --fix, got %q", out)
	}

	wts, err = repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	if vs := repo.Violations(wts); len(vs) == 0 {
		t.Error("stray worktree should remain unfixed without --fix")
	}
	found := false
	for _, w := range wts {
		if w.Path == gonePath {
			found = true
		}
	}
	if !found {
		t.Error("stale worktree should remain unpruned without --fix")
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1); got != "entry" {
		t.Errorf("plural(1) = %q, want entry", got)
	}
	if got := plural(2); got != "entries" {
		t.Errorf("plural(2) = %q, want entries", got)
	}
}
