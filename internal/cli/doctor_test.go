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
	if err := runAdd(addCmd, []string{"feature/gone"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var gonePath string
	for _, w := range wts {
		if w.Branch == "feature/gone" {
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

func TestPlural(t *testing.T) {
	if got := plural(1, "y", "ies"); got != "y" {
		t.Errorf("plural(1) = %q, want y", got)
	}
	if got := plural(2, "y", "ies"); got != "ies" {
		t.Errorf("plural(2) = %q, want ies", got)
	}
}
