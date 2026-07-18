package cli

import (
	"testing"

	"github.com/didley/wt/internal/core"
)

func TestRunListPlain(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/list"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	listPorcelain = false
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
}

// withVerbose sets listVerbose for the duration of the test, mirroring
// withYes.
func withVerbose(t *testing.T) {
	t.Helper()
	old := listVerbose
	listVerbose = true
	t.Cleanup(func() { listVerbose = old })
}

func TestRunListVerbose(t *testing.T) {
	withYes(t)
	withVerbose(t)
	newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/verbose"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	listPorcelain = false
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
}

func TestRunListPorcelain(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/porc"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	listPorcelain = true
	t.Cleanup(func() { listPorcelain = false })
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
}

func TestRunListNotARepo(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runList(); err == nil {
		t.Fatal("runList outside a repo: want error, got nil")
	}
}

func TestLockSuffix(t *testing.T) {
	cases := []struct {
		name string
		row  listRow
		want string
	}{
		{"unlocked", listRow{wt: core.Worktree{Locked: false}}, ""},
		{"locked no reason", listRow{wt: core.Worktree{Locked: true}}, " 🔒"},
		{"locked with reason", listRow{wt: core.Worktree{Locked: true, LockReason: "wip"}}, " 🔒 wip"},
	}
	for _, tc := range cases {
		if got := tc.row.lockSuffix(); got != tc.want {
			t.Errorf("%s: lockSuffix() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSetRowStatePrunable(t *testing.T) {
	var row listRow
	setRowState(&row, core.Worktree{Prunable: true})
	if !row.dirty {
		t.Error("setRowState on a prunable worktree: want dirty = true")
	}
	if row.state != "prunable — directory missing" {
		t.Errorf("setRowState on a prunable worktree: state = %q", row.state)
	}
}

func TestSetRowStateStatusUnavailable(t *testing.T) {
	var row listRow
	setRowState(&row, core.Worktree{Path: testMissingPath})
	if row.state != "status unavailable" {
		t.Errorf("setRowState on a missing path: state = %q, want %q", row.state, "status unavailable")
	}
	if row.dirty {
		t.Error("setRowState on a missing path: want dirty = false")
	}
}

func TestBranchLabel(t *testing.T) {
	if got := branchLabel(core.Worktree{Branch: testBranchA}); got != "["+testBranchA+"]" {
		t.Errorf("branchLabel(%s) = %q, want %q", testBranchA, got, "["+testBranchA+"]")
	}
	if got := branchLabel(core.Worktree{Detached: true}); got != "(detached HEAD)" {
		t.Errorf("branchLabel(detached) = %q, want %q", got, "(detached HEAD)")
	}
}

func TestRunListDetachedWorktree(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	head := mustGit(t, repo.MainPath, "rev-parse", "HEAD")
	head = head[:len(head)-1] // trim trailing newline
	detachedPath := repo.MainPath + ".worktrees/detached"
	mustGit(t, repo.MainPath, "worktree", "add", "--detach", detachedPath, head)

	listPorcelain = false
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
	listPorcelain = true
	t.Cleanup(func() { listPorcelain = false })
	if err := runList(); err != nil {
		t.Fatalf("runList porcelain: %v", err)
	}
}

func TestRunListStrayWorktree(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	stray := repo.MainPath + "-stray"
	mustGit(t, repo.MainPath, "worktree", "add", "-b", "stray/branch", stray)

	listPorcelain = false
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
	withVerbose(t)
	if err := runList(); err != nil {
		t.Fatalf("runList verbose: %v", err)
	}
	listVerbose = false
	listPorcelain = true
	t.Cleanup(func() { listPorcelain = false })
	if err := runList(); err != nil {
		t.Fatalf("runList porcelain: %v", err)
	}
}

func TestRunListOnlyMain(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
}

func TestNameAndDirLabel(t *testing.T) {
	stray := listRow{name: "agent-x", dir: "agent-x", stray: true}
	if got := nameLabel(stray); got != "agent-x"+strayMarker {
		t.Errorf("nameLabel(stray) = %q, want %q", got, "agent-x"+strayMarker)
	}
	if got := dirLabel(stray); got != "agent-x"+strayMarker {
		t.Errorf("dirLabel(stray) = %q, want %q", got, "agent-x"+strayMarker)
	}

	conforming := listRow{name: testBranchA, dir: testBranchA}
	if got := nameLabel(conforming); got != testBranchA {
		t.Errorf("nameLabel(conforming) = %q, want %q", got, testBranchA)
	}
	if got := dirLabel(conforming); got != testBranchA {
		t.Errorf("dirLabel(conforming) = %q, want %q", got, testBranchA)
	}
}

func TestMaxWidth(t *testing.T) {
	if got := maxWidth("X", "a", "bb", "ccccc"); got != len("ccccc") {
		t.Errorf("maxWidth with a longer value = %d, want %d", got, len("ccccc"))
	}
	if got := maxWidth("BRANCH", "a", "b"); got != len("BRANCH") {
		t.Errorf("maxWidth with a longer header = %d, want %d", got, len("BRANCH"))
	}
}

func TestShortHead(t *testing.T) {
	cases := map[string]string{
		"":    "",
		"abc": "abc",
		"1234567890abcdef1234567890abcdef12345678": "1234567",
	}
	for in, want := range cases {
		if got := shortHead(in); got != want {
			t.Errorf("shortHead(%q) = %q, want %q", in, got, want)
		}
	}
}
