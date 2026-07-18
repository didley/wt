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

func TestMenuBarEntries(t *testing.T) {
	cmds := menuCommands()
	items, dispatch := menuBarEntries(cmds)

	const wantExtra = 3 // "list -v", "help" and "Exit"
	if len(items) != len(cmds)+wantExtra {
		t.Fatalf("menuBarEntries() returned %d items, want %d", len(items), len(cmds)+wantExtra)
	}
	if len(dispatch) != len(items) {
		t.Fatalf("dispatch map has %d entries, want %d (one per item)", len(dispatch), len(items))
	}

	for i, c := range cmds {
		if items[i].name != c.Name() {
			t.Errorf("item %d name = %q, want command name %q", i, items[i].name, c.Name())
		}
		if items[i].description != c.Short {
			t.Errorf("item %d description = %q, want %q", i, items[i].description, c.Short)
		}
		if dispatch[c.Name()] != i {
			t.Errorf("dispatch[%q] = %d, want %d", c.Name(), dispatch[c.Name()], i)
		}
	}

	last := items[len(items)-1]
	help := items[len(items)-2]
	listVerboseItem := items[len(items)-3]

	if last.name != "Exit" || dispatch["Exit"] != menuExitIdx {
		t.Errorf("last item = %+v (dispatch %d), want Exit/%d", last, dispatch["Exit"], menuExitIdx)
	}
	if last.description == "" {
		t.Error("Exit item has an empty description")
	}

	if help.name != "help" || dispatch["help"] != menuHelpIdx {
		t.Errorf("second-to-last item = %+v (dispatch %d), want help/%d", help, dispatch["help"], menuHelpIdx)
	}
	if help.description == "" {
		t.Error("help item has an empty description")
	}

	if listVerboseItem.name != "list -v" || dispatch["list -v"] != menuVerboseListIdx {
		t.Errorf("third-to-last item = %+v (dispatch %d), want list -v/%d",
			listVerboseItem, dispatch["list -v"], menuVerboseListIdx)
	}
	if listVerboseItem.description != "verbose: "+verboseHelp {
		t.Errorf("list -v description = %q, want %q", listVerboseItem.description, "verbose: "+verboseHelp)
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
