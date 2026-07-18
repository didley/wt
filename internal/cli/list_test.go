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
