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
	listPorcelainVersion = ""
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
	listPorcelainVersion = ""
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
	listPorcelainVersion = porcelainV1
	t.Cleanup(func() { listPorcelainVersion = "" })
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
}

func TestPorcelainFlagBareDefaultsToV1(t *testing.T) {
	t.Cleanup(func() {
		listPorcelainVersion = ""
		_ = listCmd.Flags().Set("porcelain", "")
	})
	if err := listCmd.Flags().Parse([]string{"--porcelain"}); err != nil {
		t.Fatalf("parsing bare --porcelain: %v", err)
	}
	if listPorcelainVersion != porcelainV1 {
		t.Errorf("bare --porcelain = %q, want %q", listPorcelainVersion, porcelainV1)
	}
}

func TestRunListPorcelainUnsupportedVersion(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	listPorcelainVersion = "v2"
	t.Cleanup(func() { listPorcelainVersion = "" })
	if err := runList(); err == nil {
		t.Fatal("runList with --porcelain=v2: want error, got nil")
	}
}

func TestRunListNotARepo(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runList(); err == nil {
		t.Fatal("runList outside a repo: want error, got nil")
	}
}

const testWorktreeName = "app"

func TestNameLabelLockedAndStray(t *testing.T) {
	cases := []struct {
		name string
		row  listRow
		want string
	}{
		{"plain", listRow{name: testWorktreeName}, testWorktreeName},
		{"locked", listRow{name: testWorktreeName, wt: core.Worktree{Locked: true}}, testWorktreeName + lockedMarker},
		{"stray", listRow{name: testWorktreeName, stray: true}, testWorktreeName + strayMarker},
		{
			"stray and locked",
			listRow{name: testWorktreeName, stray: true, wt: core.Worktree{Locked: true}},
			testWorktreeName + strayMarker + lockedMarker,
		},
	}
	for _, tc := range cases {
		if got := nameLabel(tc.row); got != tc.want {
			t.Errorf("%s: nameLabel() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDirLabelIncludesLockReason(t *testing.T) {
	cases := []struct {
		name string
		row  listRow
		want string
	}{
		{"unlocked", listRow{dir: testWorktreeName}, testWorktreeName},
		{
			"locked, no reason",
			listRow{dir: testWorktreeName, wt: core.Worktree{Locked: true}}, testWorktreeName + lockedMarker,
		},
		{
			"locked, with reason",
			listRow{dir: testWorktreeName, wt: core.Worktree{Locked: true, LockReason: "wip"}},
			testWorktreeName + lockedMarker + " (wip)",
		},
	}
	for _, tc := range cases {
		if got := dirLabel(tc.row); got != tc.want {
			t.Errorf("%s: dirLabel() = %q, want %q", tc.name, got, tc.want)
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

func TestSetRowStateLockedDirectoryMissing(t *testing.T) {
	var row listRow
	setRowState(&row, core.Worktree{Path: testMissingPath, Locked: true})
	if !row.dirty {
		t.Error("setRowState on a locked worktree with a missing directory: want dirty = true")
	}
	if row.state != "locked — directory missing" {
		t.Errorf("setRowState on a locked worktree with a missing directory: state = %q, want %q",
			row.state, "locked — directory missing")
	}
}

func TestSetRowStateStatusUnavailable(t *testing.T) {
	// An existing directory that isn't a git worktree: os.Stat succeeds so
	// the locked/missing-directory branch doesn't apply, but `git status`
	// still fails.
	dir := t.TempDir()
	var row listRow
	setRowState(&row, core.Worktree{Path: dir})
	if row.state != "status unavailable" {
		t.Errorf("setRowState on a non-worktree directory: state = %q, want %q", row.state, "status unavailable")
	}
	if row.dirty {
		t.Error("setRowState on a non-worktree directory: want dirty = false")
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

	listPorcelainVersion = ""
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
	listPorcelainVersion = porcelainV1
	t.Cleanup(func() { listPorcelainVersion = "" })
	if err := runList(); err != nil {
		t.Fatalf("runList porcelain: %v", err)
	}
}

func TestRunListStrayWorktree(t *testing.T) {
	withYes(t)
	repo := newTestRepo(t)
	stray := repo.MainPath + "-stray"
	mustGit(t, repo.MainPath, "worktree", "add", "-b", "stray/branch", stray)

	listPorcelainVersion = ""
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
	withVerbose(t)
	if err := runList(); err != nil {
		t.Fatalf("runList verbose: %v", err)
	}
	listVerbose = false
	listPorcelainVersion = porcelainV1
	t.Cleanup(func() { listPorcelainVersion = "" })
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
