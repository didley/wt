package cli

import (
	"os"
	"testing"
)

func TestRunSwitch(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/sw"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if err := runSwitch(switchCmd, []string{"feature-sw"}); err != nil {
		t.Fatalf("runSwitch: %v", err)
	}
	_ = repo
}

func TestRunSwitchUnknown(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runSwitch(switchCmd, []string{testNameNope}); err == nil {
		t.Fatal("runSwitch on unknown target: want error, got nil")
	}
}

func TestRunSwitchNoArgsNonInteractive(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runSwitch(switchCmd, nil); err == nil {
		t.Fatal("runSwitch with no args, non-interactive: want error, got nil")
	}
}

func TestRunSwitchPrunable(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/pr"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, w := range wts {
		if w.Branch == "feature/pr" {
			path = w.Path
		}
	}
	if path == "" {
		t.Fatal("worktree feature/pr not found")
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}

	if err := runSwitch(switchCmd, []string{"feature-pr"}); err == nil {
		t.Fatal("runSwitch on a prunable worktree: want error, got nil")
	}
}
