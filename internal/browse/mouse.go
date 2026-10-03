package browse

import (
	tea "charm.land/bubbletea/v2"
)

// click takes a click of the left button: on a crumb it goes to that folder;
// on a row it moves the cursor there, and a click on the row the cursor is on
// does what Enter does. In the columns layout a row of a column beside the
// one shown goes to that column's folder, the row chosen.
func (m Model) click(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	if m.menu {
		x, y, w, h := m.menuRect()
		if msg.X >= x && msg.X < x+w && msg.Y > y && msg.Y < y+h-1 {
			row := msg.Y - y - 1
			if m.menuAt >= h-2 {
				row += m.menuAt - (h - 2) + 1
			}
			m.menuAt = min(row, len(m.filters))
			return m, m.toggle(m.menuAt)
		}
		m.menu = false // A click elsewhere closes it, and does no more.
		return m, nil
	}
	if m.footer && m.height >= 5 && msg.Y == m.height-1 && m.onChip(msg.X) {
		m.openMenu()
		return m, nil
	}
	switch {
	case msg.Y == 0:
		c, ok := m.crumbAt(msg.X)
		if !ok {
			return m, nil
		}
		return m, m.enter(c.path, c.child)
	case msg.Y < 2:
		return m, nil
	}
	h := m.bodyHeight()
	line := msg.Y - 2
	if line >= h {
		return m, nil
	}
	side := m.sideWidth()
	if side > 0 && msg.X < side {
		items := m.sideItems()
		i := line
		if m.side && m.sideAt >= h {
			i += m.sideAt - h + 1
		}
		if i >= len(items) || items[i].header {
			return m, nil
		}
		m.side = false
		return m, m.enter(items[i].path, "")
	}
	if m.layout == LayoutColumns || m.layout == LayoutPlaces {
		x := msg.X - side
		for _, c := range m.columns(m.width - side) {
			if x < c.x || x >= c.x+c.w || c.kind == colDetail {
				continue
			}
			rows, start := m.columnRows(c, h)
			if start+line >= len(rows) {
				return m, nil
			}
			switch r := rows[start+line]; c.kind {
			case colPeek:
				return m, m.enter(c.dir, r.name)
			case colTrail:
				return m, m.enter(c.dir, r.name)
			}
			return m.clickRow(m.top + line)
		}
		return m, nil
	}
	return m.clickRow(m.top + line)
}

func (m Model) clickRow(i int) (Model, tea.Cmd) {
	if i < 0 || i >= len(m.rows) {
		return m, nil
	}
	if i == m.cursor {
		return m, m.activate()
	}
	m.moveTo(i)
	return m, m.needs()
}

// wheel takes a turn of the wheel by what is under it: the menu of kinds moves
// its own cursor, one row to a notch, if the wheel is over it, and the folder
// is left alone while the menu is open; the sidebar does the same while it has
// the cursor; and otherwise the wheel scrolls the folder, three rows to a
// notch, but not from over the sidebar, which has nothing to scroll.
func (m Model) wheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	by := 0
	switch msg.Button {
	case tea.MouseWheelUp:
		by = -1
	case tea.MouseWheelDown:
		by = 1
	default:
		return m, nil
	}
	switch {
	case m.menu:
		if x, y, w, h := m.menuRect(); msg.X >= x && msg.X < x+w && msg.Y >= y && msg.Y < y+h {
			m.menuAt = min(max(m.menuAt+by, 0), len(m.filters))
		}
		return m, nil
	case m.side:
		m.sideStep(by)
		return m, nil
	case msg.X < m.sideWidth():
		return m, nil
	}
	m.move(3 * by)
	return m, m.needs()
}
