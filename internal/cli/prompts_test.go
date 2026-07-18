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

func TestMenuCommands(t *testing.T) {
	withYes(t)
	newTestRepo(t)

	cmds := menuCommands()
	const wantCommands = 9
	if len(cmds) != wantCommands {
		t.Fatalf("menuCommands() = %d commands, want %d", len(cmds), wantCommands)
	}
	for _, c := range cmds {
		if c.Name() == "" {
			t.Error("menu command has an empty name")
		}
		if c.Short == "" {
			t.Errorf("menu command %q has an empty -h description (Short)", c.Name())
		}
		if c.RunE == nil {
			t.Errorf("menu command %q has a nil RunE", c.Name())
			continue
		}
		// Every command is exercised elsewhere with real targets; here we
		// only need each RunE invoked at least once for coverage, so a
		// no-op/error outcome (e.g. nothing to unlock) is fine.
		_ = c.RunE(c, nil)
	}
}

// TestMenuCommandsCoverAllVisibleCommands guards against a forgotten menu
// entry: every non-hidden subcommand except `list` (already shown before the
// menu appears) must be in menuCommands(), whatever order it's curated in.
func TestMenuCommandsCoverAllVisibleCommands(t *testing.T) {
	want := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		if c == listCmd || c.Hidden {
			continue
		}
		want[c.Name()] = true
	}
	for _, c := range menuCommands() {
		if !want[c.Name()] {
			t.Errorf("menuCommands() includes %q, not in rootCmd.Commands()", c.Name())
		}
		delete(want, c.Name())
	}
	for name := range want {
		t.Errorf("menuCommands() is missing %q", name)
	}
}

func TestMenuOptions(t *testing.T) {
	cmds := menuCommands()
	opts, descriptions := menuOptions(cmds)

	const wantExtra = 2 // "list --verbose" and "Exit"
	if len(opts) != len(cmds)+wantExtra {
		t.Fatalf("menuOptions() returned %d options, want %d", len(opts), len(cmds)+wantExtra)
	}

	for i, c := range cmds {
		if opts[i].Key != c.Name() {
			t.Errorf("option %d label = %q, want command name %q", i, opts[i].Key, c.Name())
		}
		if descriptions[i] != c.Short {
			t.Errorf("description[%d] = %q, want %q", i, descriptions[i], c.Short)
		}
	}

	if descriptions[menuVerboseListIdx] != verboseHelp {
		t.Errorf("description[verboseListIdx] = %q, want %q", descriptions[menuVerboseListIdx], verboseHelp)
	}
	if descriptions[menuExitIdx] == "" {
		t.Error("description[exitIdx] is empty")
	}

	last, secondLast := opts[len(opts)-1], opts[len(opts)-2]
	if last.Key != "Exit" || last.Value != menuExitIdx {
		t.Errorf("last option = %+v, want Exit/%d", last, menuExitIdx)
	}
	if secondLast.Key != "list --verbose" || secondLast.Value != menuVerboseListIdx {
		t.Errorf("second-to-last option = %+v, want list --verbose/%d", secondLast, menuVerboseListIdx)
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
