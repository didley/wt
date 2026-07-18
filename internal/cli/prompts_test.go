package cli

import (
	"testing"

	"github.com/didley/wt/internal/core"
)

func TestWarnf(t *testing.T) {
	out := captureStderr(t, func() { warnf("something %s happened", "bad") })
	if !contains(out, "something bad happened") {
		t.Errorf("warnf output = %q, want it to contain the formatted message", out)
	}
}

func TestWorktreeOptions(t *testing.T) {
	repo := newTestRepo(t)
	wts := []core.Worktree{
		{Path: repo.MainPath, IsMain: true, Branch: "main"},
		{Path: "/wt/feature", Branch: "feature/a"},
		{Path: "/wt/detached"},
		{Path: "/wt/gone", Branch: "feature/gone", Prunable: true},
	}
	opts := worktreeOptions(repo, wts)
	if len(opts) != len(wts) {
		t.Fatalf("worktreeOptions returned %d options, want %d", len(opts), len(wts))
	}

	labels := make([]string, len(opts))
	for i, o := range opts {
		labels[i] = o.Key
	}

	if !contains(labels[0], "(main checkout)") {
		t.Errorf("main worktree label = %q, want it to mention main checkout", labels[0])
	}
	if !contains(labels[1], "[feature/a]") {
		t.Errorf("branch worktree label = %q, want it to mention the branch", labels[1])
	}
	if !contains(labels[2], "detached HEAD") {
		t.Errorf("detached worktree label = %q, want it to mention detached HEAD", labels[2])
	}
	if !contains(labels[3], "directory missing") {
		t.Errorf("prunable worktree label = %q, want it to mention directory missing", labels[3])
	}
}
