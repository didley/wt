package cli

import (
	"os"
	"testing"
)

func TestRunPruneNothingToPrune(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runPrune(pruneCmd, nil); err != nil {
		t.Fatalf("runPrune: %v", err)
	}
}

func TestRunPruneStaleEntry(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/gone"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, w := range wts {
		if w.Branch == "feature/gone" {
			path = w.Path
		}
	}
	if path == "" {
		t.Fatal("worktree feature/gone not found")
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}

	if err := runPrune(pruneCmd, nil); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	wts, err = repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wts {
		if w.Path == path {
			t.Errorf("stale worktree %q still registered after prune", path)
		}
	}
}

func TestRunPruneNonInteractiveNeedsYes(t *testing.T) {
	repo := newTestRepo(t)
	yes = true
	if err := runAdd(addCmd, []string{"feature/gone2"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, w := range wts {
		if w.Branch == "feature/gone2" {
			path = w.Path
		}
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	yes = false
	t.Cleanup(func() { yes = false })

	if err := runPrune(pruneCmd, nil); err == nil {
		t.Fatal("runPrune stale entry, non-interactive without --yes: want error, got nil")
	}
}
