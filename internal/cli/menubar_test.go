package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const testMenuItemUnlock = "unlock"

func testMenuBarItems() []menuBarItem {
	return []menuBarItem{
		{name: "add", description: "Create a worktree"},
		{name: "remove", description: "Remove one or more worktrees"},
		{name: "switch", description: "Jump to a worktree"},
		{name: testMenuItemUnlock, description: "Unlock a previously locked worktree"},
	}
}

func TestClampCursor(t *testing.T) {
	cases := []struct {
		cursor, n, want int
	}{
		{0, 4, 0},
		{3, 4, 3},
		{4, 4, 0},  // wraps forward past the end
		{-1, 4, 3}, // wraps backward past the start
		{-5, 4, 3}, // wraps backward more than once
		{0, 0, 0},  // no items: never panics
		{10, 0, 0},
	}
	for _, tc := range cases {
		if got := clampCursor(tc.cursor, tc.n); got != tc.want {
			t.Errorf("clampCursor(%d, %d) = %d, want %d", tc.cursor, tc.n, got, tc.want)
		}
	}
}

func TestMenuBarFiltered(t *testing.T) {
	m := newMenuBarModel("Run a command", testMenuBarItems())

	if got := len(m.filtered()); got != 4 {
		t.Fatalf("filtered() with no filter = %d items, want 4", got)
	}

	m.filter = "un"
	got := m.filtered()
	if len(got) != 1 || got[0].name != testMenuItemUnlock {
		t.Errorf(`filtered() with filter "un" = %+v, want just %q`, got, testMenuItemUnlock)
	}

	m.filter = "UN"
	got = m.filtered()
	if len(got) != 1 || got[0].name != testMenuItemUnlock {
		t.Errorf("filtered() should be case-insensitive, got %+v", got)
	}

	m.filter = "zzz"
	if got := m.filtered(); len(got) != 0 {
		t.Errorf("filtered() with no matches = %+v, want empty", got)
	}
}

func TestMenuBarHandleKeyTyping(t *testing.T) {
	m := newMenuBarModel("Run a command", testMenuBarItems())
	m.cursor = 2 // move off zero so we can tell it gets reset

	m = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("un")})
	if m.filter != "un" {
		t.Errorf("filter after typing = %q, want %q", m.filter, "un")
	}
	if m.cursor != 0 {
		t.Errorf("cursor after typing = %d, want 0 (jump to first match)", m.cursor)
	}

	m = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.filter != "u" {
		t.Errorf("filter after backspace = %q, want %q", m.filter, "u")
	}
}

func TestMenuBarHandleKeyArrowsWrap(t *testing.T) {
	m := newMenuBarModel("Run a command", testMenuBarItems())

	m = m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if m.cursor != len(testMenuBarItems())-1 {
		t.Errorf("cursor after left from 0 = %d, want %d (wrap to last)", m.cursor, len(testMenuBarItems())-1)
	}

	m = m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	if m.cursor != 0 {
		t.Errorf("cursor after right from last = %d, want 0 (wrap to first)", m.cursor)
	}

	m = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("cursor after down = %d, want 1 (down behaves like right)", m.cursor)
	}
	m = m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor after up = %d, want 0 (up behaves like left)", m.cursor)
	}
}

func TestMenuBarHandleKeyEnter(t *testing.T) {
	m := newMenuBarModel("Run a command", testMenuBarItems())
	m = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("switch")})
	m = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.done {
		t.Fatal("handleKey(Enter): want done = true")
	}
	if m.aborted {
		t.Error("handleKey(Enter): want aborted = false")
	}
	if m.result != "switch" {
		t.Errorf("handleKey(Enter): result = %q, want %q", m.result, "switch")
	}
}

func TestMenuBarHandleKeyEnterNoMatches(t *testing.T) {
	m := newMenuBarModel("Run a command", testMenuBarItems())
	m = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	m = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.done {
		t.Error("handleKey(Enter) with no matches: want done = false")
	}
}

func TestMenuBarHandleKeyEscClearsFilterFirst(t *testing.T) {
	m := newMenuBarModel("Run a command", testMenuBarItems())
	m = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("un")})

	m = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.filter != "" {
		t.Errorf("handleKey(Esc) with a filter set: filter = %q, want cleared", m.filter)
	}
	if m.done || m.aborted {
		t.Error("handleKey(Esc) with a filter set: should clear the filter, not abort")
	}

	m = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.done || !m.aborted {
		t.Error("handleKey(Esc) with no filter set: want aborted = true")
	}
}

func TestMenuBarHandleKeyCtrlCAborts(t *testing.T) {
	m := newMenuBarModel("Run a command", testMenuBarItems())
	m = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("un")})
	m = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.done || !m.aborted {
		t.Error("handleKey(CtrlC): want aborted = true even with a filter set")
	}
}

func TestMenuBarUpdate(t *testing.T) {
	m := newMenuBarModel("Run a command", testMenuBarItems())

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 40})
	nm, ok := next.(menuBarModel)
	if !ok {
		t.Fatal("Update(WindowSizeMsg) did not return a menuBarModel")
	}
	if nm.width != 40 {
		t.Errorf("width after WindowSizeMsg = %d, want 40", nm.width)
	}
	if cmd != nil {
		t.Error("Update(WindowSizeMsg) returned a non-nil cmd, want nil")
	}

	next, cmd = nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm, ok = next.(menuBarModel)
	if !ok {
		t.Fatal("Update(KeyMsg) did not return a menuBarModel")
	}
	if !nm.done {
		t.Error("Update(Enter): want done = true")
	}
	if cmd == nil {
		t.Error("Update(Enter) with done = true: want a tea.Quit cmd, got nil")
	}
}

func TestMenuBarView(t *testing.T) {
	m := newMenuBarModel("Run a command", testMenuBarItems())
	out := m.View()
	if !contains(out, "add") || !contains(out, testMenuItemUnlock) {
		t.Errorf("View() = %q, want it to list every item name", out)
	}
	if !contains(out, "Create a worktree") {
		t.Errorf("View() = %q, want the focused item's description", out)
	}

	m.filter = "zzz"
	out = m.View()
	if !contains(out, "no matches") {
		t.Errorf("View() with no matches = %q, want a no-matches hint", out)
	}
}

func TestNewMenuBarModel(t *testing.T) {
	items := testMenuBarItems()
	m := newMenuBarModel("Run a command", items)
	if m.title != "Run a command" {
		t.Errorf("title = %q, want %q", m.title, "Run a command")
	}
	if len(m.items) != len(items) {
		t.Errorf("items = %d, want %d", len(m.items), len(items))
	}
	if m.width != defaultMenuBarWidth {
		t.Errorf("width = %d, want default %d", m.width, defaultMenuBarWidth)
	}
	if m.Init() != nil {
		t.Error("Init() should return a nil cmd")
	}
}
