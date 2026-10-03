package browse

import (
	"io/fs"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Filter is a kind of file the chooser can be told to show alone: "Pictures",
// "PDF". With some chosen, only files of those kinds are listed, and all
// folders, which are how one gets to the files; with none, everything is.
type Filter struct {
	Name  string
	Mark  string // Drawn before the name in the menu, if the host marks kinds.
	Match func(fs.DirEntry) bool
}

// WithFilters offers the kinds of file in a menu of checkboxes (Ctrl-F, or a
// click on the footer's chip).
func WithFilters(f []Filter) Option {
	return func(m *Model) { m.filters, m.picked = f, make([]bool, len(f)) }
}

// WithActiveFilters starts with the filters of these names chosen.
func WithActiveFilters(names ...string) Option {
	return func(m *Model) {
		for _, n := range names {
			if i := slices.IndexFunc(m.filters, func(f Filter) bool { return f.Name == n }); i >= 0 {
				m.picked[i] = true
			}
		}
	}
}

// ActiveFilters are the names of the filters chosen.
func (m Model) ActiveFilters() []string {
	var names []string
	for i, f := range m.filters {
		if m.picked[i] {
			names = append(names, f.Name)
		}
	}
	return names
}

// SetActiveFilters chooses the filters of these names, and no others.
func (m *Model) SetActiveFilters(names []string) tea.Cmd {
	for i, f := range m.filters {
		m.picked[i] = slices.Contains(names, f.Name)
	}
	return m.filtered()
}

// InMenu says whether the menu of filters is open, where Esc closes it.
func (m Model) InMenu() bool { return m.menu }

// CloseMenu closes the menu of filters.
func (m *Model) CloseMenu() { m.menu = false }

// filtered follows from the kinds chosen having changed: the rows are made anew.
func (m *Model) filtered() tea.Cmd {
	m.moved = false
	cmd := m.needs()
	m.rebuild()
	return cmd
}

// typed says whether an entry passes the kinds chosen.
func (m Model) typed(e fs.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	for i, f := range m.filters {
		if m.picked[i] && f.Match(e) {
			return true
		}
	}
	return false
}

func (m Model) anyPicked() bool { return slices.Contains(m.picked, true) }

// byType leaves out what the kinds chosen do not take in.
func (m Model) byType(entries []fs.DirEntry) []fs.DirEntry {
	if !m.anyPicked() {
		return entries
	}
	out := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		if m.typed(e) {
			out = append(out, e)
		}
	}
	return out
}

// openMenu opens the menu, the cursor on the first kind chosen.
func (m *Model) openMenu() {
	if len(m.filters) == 0 {
		return
	}
	m.menu, m.side = true, false
	m.menuAt = 0
	if i := slices.Index(m.picked, true); i >= 0 {
		m.menuAt = i + 1
	}
}

// toggle flips the kind at a row of the menu; the first row is "All types",
// which clears the choice.
func (m *Model) toggle(row int) tea.Cmd {
	if row == 0 {
		clear(m.picked)
	} else if row-1 < len(m.picked) {
		m.picked[row-1] = !m.picked[row-1]
	}
	return m.filtered()
}

// menuKey takes a key while the menu is open: the arrows move, Space or Enter
// checks, and Esc closes it. Any other key closes it and goes on to be what it
// would have been, so typing is never lost.
func (m Model) menuKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	last := len(m.filters)
	switch s := msg.String(); {
	case s == "up" || s == "ctrl+p":
		m.menuAt = max(m.menuAt-1, 0)
	case s == "down" || s == "ctrl+n":
		m.menuAt = min(m.menuAt+1, last)
	case s == "home":
		m.menuAt = 0
	case s == "end":
		m.menuAt = last
	case s == "space" || s == "enter" || s == "x":
		return m, m.toggle(m.menuAt), true
	case s == "esc" || s == "tab" || s == "ctrl+f" || s == "left":
		m.menu = false
	default:
		m.menu = false
		return m, nil, false
	}
	return m, nil, true
}

// menuRect is where the menu is drawn: below the filter, at the right.
func (m Model) menuRect() (x, y, w, h int) {
	w = ansi.StringWidth(" space toggles · esc closes ") + 2
	for _, f := range m.filters {
		w = max(w, 4+4+ansi.StringWidth(f.Mark)+1+ansi.StringWidth(f.Name)+2)
	}
	w = min(w, max(m.width-2, 10))
	h = min(len(m.filters)+1+2, max(m.bodyHeight(), 3))
	return max(m.width-w-1, 0), 2, w, h
}

// menuView draws the menu as a box of rows: the box checked for each kind
// chosen, "All types" when none is.
func (m Model) menuView() []string {
	_, _, w, h := m.menuRect()
	inner := w - 2
	rows := h - 2
	start := 0
	if m.menuAt >= rows {
		start = m.menuAt - rows + 1
	}
	edge := m.styles.Separator
	title := " File types "
	top := edge.Render("╭─") + m.styles.CrumbCurrent.Render(title) + edge.Render(strings.Repeat("─", max(inner-1-ansi.StringWidth(title), 0))+"╮")
	lines := []string{top}
	for i := start; i < start+rows && i <= len(m.filters); i++ {
		checked, label := false, "All types"
		if i == 0 {
			checked = !m.anyPicked()
		} else {
			f := m.filters[i-1]
			checked, label = m.picked[i-1], f.Name
			if f.Mark != "" {
				label = f.Mark + " " + f.Name
			}
		}
		box := "[ ]"
		if checked {
			box = "[x]"
		}
		cursor, style := " ", m.styles.File
		if i == m.menuAt {
			cursor, style = m.styles.Cursor.Render(">"), m.styles.Selected
		} else if checked {
			style = m.styles.Trail
		}
		cell := cursor + style.Render(" "+box+" "+ansi.Truncate(label, max(inner-6, 1), "…"))
		cell += strings.Repeat(" ", max(inner-ansi.StringWidth(cell), 0))
		lines = append(lines, edge.Render("│")+cell+edge.Render("│"))
	}
	hint := " space toggles · esc closes "
	foot := edge.Render("╰") + m.styles.Footer.Render(ansi.Truncate(hint, max(inner, 0), "")) + edge.Render(strings.Repeat("─", max(inner-ansi.StringWidth(hint), 0))+"╯")
	return append(lines, foot)
}

// overlay draws a box over lines, at a column and row, keeping what is
// around it.
func overlay(lines, box []string, x, y int) {
	for i, b := range box {
		if y+i >= len(lines) {
			return
		}
		row := lines[y+i]
		left := ansi.Truncate(row, x, "")
		left += strings.Repeat(" ", max(x-ansi.StringWidth(left), 0))
		lines[y+i] = left + b + ansi.TruncateLeft(row, x+ansi.StringWidth(b), "")
	}
}

// chip is what the footer shows at its right end to open the menu: which
// kinds are chosen, and a mark that it opens.
func (m Model) chip() string {
	if len(m.filters) == 0 {
		return ""
	}
	label := "types"
	if names := m.ActiveFilters(); len(names) > 0 {
		label = "types: " + strings.Join(names, ", ")
	}
	room := max(m.width/2, 12)
	return " " + ansi.Truncate(label, room, "…") + " ▾ "
}

// chipStyle draws the chip, bright when kinds are chosen.
func (m Model) chipStyle() lipgloss.Style {
	if m.anyPicked() {
		return m.styles.Trail
	}
	return m.styles.Footer
}

// onChip says whether a click at a column of the footer is on the chip.
func (m Model) onChip(x int) bool {
	c := m.chip()
	return c != "" && x >= m.width-ansi.StringWidth(c)
}
