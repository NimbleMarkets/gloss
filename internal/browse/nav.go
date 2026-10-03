package browse

import (
	"path"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update takes a message, as every bubble's does.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, m.needs()
	case listedMsg:
		if msg.id != m.id || msg.epoch != m.epoch {
			return m, nil
		}
		m.listed(msg)
		m.rebuild()
		return m, m.needs()
	case expandedMsg:
		if msg.id == m.id && msg.from == m.input.Value() && msg.to != msg.from {
			m.input.SetValue(msg.to)
			m.input.CursorEnd()
			return m, m.edited()
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.PasteMsg:
		m.input, _ = m.input.Update(msg)
		return m, m.edited()
	case tea.MouseClickMsg:
		return m.click(msg)
	case tea.MouseWheelMsg:
		return m.wheel(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// A popup takes the keys it knows. Any other key closes it and goes on to be
	// what it would have been, so the popup's closing is kept, not just for
	// this key.
	if m.side {
		next, cmd, taken := m.sideKey(msg)
		if taken {
			return next, cmd
		}
		m = next
	}
	if m.menu {
		next, cmd, taken := m.menuKey(msg)
		if taken {
			return next, cmd
		}
		m = next
	}
	empty := m.input.Value() == ""
	if m.cycle != nil && !key.Matches(msg, m.keys.Complete, m.keys.CompleteBack) {
		m.cycle = nil
		m.rebuild()
	}
	switch {
	case key.Matches(msg, m.keys.Up):
		m.move(-1)
	case key.Matches(msg, m.keys.Down):
		m.move(1)
	case key.Matches(msg, m.keys.PageUp):
		m.move(-m.bodyHeight())
	case key.Matches(msg, m.keys.PageDown):
		m.move(m.bodyHeight())
	case empty && key.Matches(msg, m.keys.Top):
		m.moveTo(0)
	case empty && key.Matches(msg, m.keys.Bottom):
		m.moveTo(len(m.rows) - 1)
	case key.Matches(msg, m.keys.Layout):
		return m, m.SetLayout(m.layout + 1)
	case key.Matches(msg, m.keys.Types):
		if len(m.filters) > 0 {
			m.openMenu()
		}
		return m, nil
	case key.Matches(msg, m.keys.Places):
		if m.sideWidth() == 0 {
			return m, m.SetLayout(LayoutPlaces)
		}
		m.focusSidebar()
		return m, nil
	case key.Matches(msg, m.keys.Back):
		return m, m.goHistory(-1)
	case key.Matches(msg, m.keys.Forward):
		return m, m.goHistory(1)
	case key.Matches(msg, m.keys.Complete):
		return m, m.complete(1)
	case key.Matches(msg, m.keys.CompleteBack):
		return m, m.complete(-1)
	case key.Matches(msg, m.keys.Parent):
		return m, m.up()
	case key.Matches(msg, m.keys.Open):
		return m, m.activate()
	case msg.String() == "left" && empty, msg.String() == "backspace" && empty:
		return m, m.up()
	case key.Matches(msg, m.keys.Into):
		if empty {
			if m.Current() != nil && m.Current().IsDir() {
				return m, m.activate()
			}
			return m, nil
		}
		if m.cursorAtEnd() {
			if name, _ := m.ghostName(); name != "" {
				m.input.SetValue(name)
				m.input.CursorEnd()
				return m, m.edited()
			}
			return m, nil
		}
		fallthrough
	default:
		before := m.input.Value()
		m.input, _ = m.input.Update(msg)
		if m.input.Value() != before {
			return m, m.edited()
		}
		return m, nil
	}
	return m, m.needs()
}

// edited follows from the filter having been changed by typing: what it
// names is listed, the cursor goes to the best match, and a folder named
// that is not read yet is.
func (m *Model) edited() tea.Cmd {
	m.cycle = nil
	m.moved, m.sel = false, ""
	cmd := m.needs()
	m.rebuild()
	return tea.Batch(cmd, m.needs())
}

// rebuild lists the folder shown anew, the cursor staying on its name if the
// folder still has it.
func (m *Model) rebuild() {
	filter := m.filterText()
	dir := m.viewDir()
	m.rows = m.rowsOf(dir, filter, m.layout == LayoutList)
	i := -1
	if m.sel != "" {
		i = slices.IndexFunc(m.rows, func(r row) bool { return r.name == m.sel })
	}
	if i >= 0 {
		m.cursor = i
		m.reveal()
		return
	}
	if l := m.lists[dir]; m.sel != "" && (l == nil || !l.loaded) {
		// The folder is still being read, which is why the name is not there:
		// keep it, to be found when the rows arrive. Until then, the first.
		m.cursor = max(slices.IndexFunc(m.rows, func(r row) bool { return !r.parent }), 0)
		m.reveal()
		return
	}
	// Not where it was: the first real row. With only the parent listed,
	// as while a folder is being read, nothing is remembered, so that the
	// rows arriving are chosen from afresh.
	m.sel = ""
	if i = slices.IndexFunc(m.rows, func(r row) bool { return !r.parent }); i >= 0 {
		m.sel = m.rows[i].name
	}
	m.cursor = max(i, 0)
	m.reveal()
}

// filterText is the filter that narrows the rows: the typed one, or while
// Tab steps through candidates, the one it started from.
func (m Model) filterText() string {
	_, rest := split(m.activeInput())
	return rest
}

func (m Model) activeInput() string {
	if m.cycle != nil {
		return m.cycle.base
	}
	return m.input.Value()
}

// move goes down by n rows, or up by -n.
func (m *Model) move(n int) { m.moveTo(m.cursor + n) }

func (m *Model) moveTo(i int) {
	m.moved = true
	if len(m.rows) == 0 {
		return
	}
	m.cursor = min(max(i, 0), len(m.rows)-1)
	m.sel = m.rows[m.cursor].name
	m.reveal()
}

// bodyHeight is how many rows the folder is drawn in.
func (m Model) bodyHeight() int {
	h := m.height - 2 // The breadcrumbs and the filter.
	if m.footer && m.height >= 5 {
		h--
	}
	return max(1, h)
}

// reveal scrolls so the cursor is drawn.
func (m *Model) reveal() {
	h := m.bodyHeight()
	switch {
	case m.cursor < m.top:
		m.top = m.cursor
	case m.cursor >= m.top+h:
		m.top = m.cursor - h + 1
	}
	m.top = max(0, min(m.top, len(m.rows)-h))
}

// enter makes a folder the one browsed, the cursor on a name in it.
func (m *Model) enter(dir, sel string) tea.Cmd {
	if m.sel != "" && m.input.Value() == "" {
		m.memo[m.dir] = m.sel
	}
	m.dir = dir
	m.visit(dir)
	m.input.SetValue("")
	m.cycle, m.moved = nil, false
	m.sel = sel
	if sel == "" {
		m.sel = m.memo[dir]
	}
	cmd := m.needs()
	m.rebuild()
	return tea.Batch(cmd, m.needs())
}

// up goes to the folder above, the cursor on the one it left.
func (m *Model) up() tea.Cmd {
	from := m.viewDir()
	if from == "/" {
		return nil
	}
	return m.enter(parentOf(from), path.Base(from))
}

// activate does what Enter does: goes into the folder under the cursor,
// chooses the file, or goes to the folder that has been typed.
func (m *Model) activate() tea.Cmd {
	dirPart, rest := split(m.input.Value())
	if m.cycle == nil && dirPart != "" && rest == "" && !m.moved {
		return m.enter(m.viewDir(), "") // A folder typed in full, and no row picked.
	}
	r := m.rowAt(m.cursor)
	if r == nil {
		return nil
	}
	switch {
	case r.parent:
		return m.up()
	case r.entry.IsDir():
		return m.enter(path.Join(m.viewDir(), r.name), "")
	case m.canChoose(r.entry):
		m.chosen = path.Join(m.viewDir(), r.name)
	}
	return nil
}

func (m Model) rowAt(i int) *row {
	if i < 0 || i >= len(m.rows) {
		return nil
	}
	return &m.rows[i]
}
