package cli

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// defaultMenuBarWidth is used when the terminal hasn't reported its width
// yet (e.g. the very first frame, before a tea.WindowSizeMsg arrives).
const defaultMenuBarWidth = 80

// menuBarItem is one entry in a menuBar: a name shown in the horizontal row,
// and a description shown below for whichever item is currently focused.
type menuBarItem struct {
	name        string
	description string
}

// menuBarModel is a horizontal, all-options-visible-at-once alternative to
// huh.Select: every item's name is laid out on one row (wrapping to more
// rows on a narrow terminal), the focused one bracketed ("[name]") in the
// same two colors huh's own default theme uses for a selected option —
// huhFuchsia for the brackets (huh's select-cursor color) and huhGreen for
// the name text (huh's selected-option color; both defined in prompts.go)
// — arrow keys move focus, typing filters (no leading "/" needed, unlike
// huh's default), and the focused item's description is shown on its own
// line below — mirroring what huh.Select's DescriptionFunc gave the
// vertical version.
//
// huh.Select has no such layout (only one-per-line vertical, or a
// single-item Inline carousel), hence a small bubbletea model instead of
// huh for this one screen.
type menuBarModel struct {
	title   string
	items   []menuBarItem
	filter  string
	cursor  int
	width   int
	result  string
	aborted bool
	done    bool
}

func newMenuBarModel(title string, items []menuBarItem) menuBarModel {
	return menuBarModel{title: title, items: items, width: defaultMenuBarWidth}
}

func (m menuBarModel) Init() tea.Cmd { return nil }

func (m menuBarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyMsg:
		m = m.handleKey(msg)
		if m.done {
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

func (m menuBarModel) View() string {
	var b strings.Builder

	items := m.filtered()
	cursor := clampCursor(m.cursor, len(items))

	title := m.title
	if m.filter != "" {
		title = "/" + m.filter
	}
	b.WriteString(lipgloss.NewStyle().Foreground(huhIndigo).Bold(true).Render(title))
	b.WriteString("\n")

	if m.showMoveHint(items, cursor) {
		b.WriteString(stDim.Render("←→/↑↓ move  •  type to filter"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(renderMenuBarRow(items, cursor, m.rowWidth()))
	b.WriteString("\n\n")

	switch {
	case len(items) == 0:
		b.WriteString(stDim.Render("no matches"))
	default:
		b.WriteString(stDim.Render(items[cursor].description))
	}
	return b.String()
}

// showMoveHint reports whether the "←→/↑↓ move / type to filter" hint
// should show: only while the very first command (m.items[0],
// "add") is what's focused — i.e. someone hasn't moved or typed yet, or has
// arrowed back to the start — so it says its piece once rather than on
// every frame.
func (m menuBarModel) showMoveHint(items []menuBarItem, cursor int) bool {
	return len(items) > 0 && len(m.items) > 0 && items[cursor].name == m.items[0].name
}

// rowWidth is the terminal width to wrap the item row at.
func (m menuBarModel) rowWidth() int {
	if m.width <= 0 {
		return defaultMenuBarWidth
	}
	return m.width
}

// renderMenuBarRow lays out items on one or more lines, wrapping at width,
// with the item at cursor bracketed in huh's select-cursor/selected-option
// colors.
func renderMenuBarRow(items []menuBarItem, cursor, width int) string {
	bracketStyle := lipgloss.NewStyle().Foreground(huhFuchsia).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(huhGreen).Bold(true)
	const bracketWidth = len("[]")

	var b strings.Builder
	lineWidth := 0
	for i, it := range items {
		name := it.name
		displayLen := len(it.name)
		if i == cursor {
			name = bracketStyle.Render("[") + nameStyle.Render(it.name) + bracketStyle.Render("]")
			displayLen += bracketWidth
		}

		switch {
		case lineWidth == 0:
			// first item on the line, nothing to separate
		case lineWidth+displayLen+2 > width:
			b.WriteString("\n")
			lineWidth = 0
		default:
			b.WriteString("  ")
			lineWidth += 2
		}
		b.WriteString(name)
		lineWidth += displayLen
	}
	return b.String()
}

// filtered returns the items matching the current filter, case-insensitive
// substring match — the same semantics huh.Select's own filter uses.
func (m menuBarModel) filtered() []menuBarItem {
	if m.filter == "" {
		return m.items
	}
	needle := strings.ToLower(m.filter)
	out := make([]menuBarItem, 0, len(m.items))
	for _, it := range m.items {
		if strings.Contains(strings.ToLower(it.name), needle) {
			out = append(out, it)
		}
	}
	return out
}

// handleKey applies one keypress and returns the resulting model. Split out
// from Update so it's unit-testable without a real tea.Program.
func (m menuBarModel) handleKey(msg tea.KeyMsg) menuBarModel {
	// This is a picker, not a text editor: every other key is a no-op below.
	switch msg.Type { //nolint:exhaustive
	case tea.KeyCtrlC:
		m.aborted, m.done = true, true
		return m
	case tea.KeyEsc:
		if m.filter != "" {
			m.filter = ""
			m.cursor = 0
			return m
		}
		m.aborted, m.done = true, true
		return m
	case tea.KeyEnter:
		items := m.filtered()
		if len(items) == 0 {
			return m
		}
		m.result = items[clampCursor(m.cursor, len(items))].name
		m.done = true
		return m
	case tea.KeyLeft, tea.KeyUp:
		m.cursor--
	case tea.KeyRight, tea.KeyDown:
		m.cursor++
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.cursor = 0
		}
	case tea.KeyRunes, tea.KeySpace:
		m.filter += string(msg.Runes)
		m.cursor = 0
	default:
		// every other key (function keys, ctrl+letter, page up/down, ...)
		// is a no-op here — this is a picker, not a text editor.
	}
	m.cursor = clampCursor(m.cursor, len(m.filtered()))
	return m
}

// clampCursor keeps the cursor within the bounds of the current filtered
// set, wrapping to the last item if it fell off the front and vice versa.
func clampCursor(cursor, n int) int {
	if n == 0 {
		return 0
	}
	return ((cursor % n) + n) % n
}

// runMenuBar runs the horizontal command bar and returns the chosen item's
// name, or errAborted if the user cancelled (Ctrl+C, or Esc with no filter
// to clear first).
func runMenuBar(title string, items []menuBarItem) (string, error) {
	p := tea.NewProgram(newMenuBarModel(title, items), tea.WithOutput(os.Stderr), tea.WithInput(os.Stdin))
	final, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("menu: %w", err)
	}
	// tea.Program.Run() always returns the model type passed to NewProgram.
	m := final.(menuBarModel) //nolint:forcetypeassert
	if m.aborted {
		return "", errAborted
	}
	return m.result, nil
}
