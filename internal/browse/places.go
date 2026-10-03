package browse

import (
	"io/fs"
	"path"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Place is a folder worth a shortcut: the sidebar lists it by name.
type Place struct {
	Name string
	Path string // Absolute, slash-separated.
}

// DefaultPlaces are the folders of a home that most people keep things in,
// those that exist, and the root of the filesystem.
func DefaultPlaces(fsys fs.FS, home string) []Place {
	var places []Place
	if home != "" && home != "/" {
		places = append(places, Place{"Home", home})
		for _, name := range []string{"Desktop", "Documents", "Downloads", "Pictures", "Movies", "Videos", "Music"} {
			p := path.Join(home, name)
			if info, err := fs.Stat(fsys, fsName(p)); err == nil && info.IsDir() {
				places = append(places, Place{name, p})
			}
		}
	}
	return append(places, Place{"Root", "/"})
}

// A sideItem is a row of the sidebar: a heading, or a place to go to.
type sideItem struct {
	label  string
	path   string
	header bool
}

// sideItems lists the sidebar: the places, then the folders visited lately,
// the one shown left out of them.
func (m Model) sideItems() []sideItem {
	var items []sideItem
	known := map[string]bool{}
	if len(m.places) > 0 {
		items = append(items, sideItem{label: "Places", header: true})
		for _, p := range m.places {
			items = append(items, sideItem{label: cleanName(p.Name), path: p.Path})
			known[p.Path] = true
		}
	}
	var recent []sideItem
	current := m.viewDir()
	for i := len(m.hist) - 1; i >= 0 && len(recent) < 6; i-- {
		d := m.hist[i]
		if known[d] || d == current {
			continue
		}
		known[d] = true
		recent = append(recent, sideItem{label: m.recentLabel(d), path: d})
	}
	if len(recent) > 0 {
		items = append(items, sideItem{label: "Recent", header: true})
		items = append(items, recent...)
	}
	return items
}

// recentLabel names a folder by its last name, and what is above it where
// that helps: "gloss", or "~/projects/gloss" in full if it is short.
func (m Model) recentLabel(dir string) string {
	if m.home != "" && m.home != "/" {
		if dir == m.home {
			return "~"
		}
		if rest, ok := strings.CutPrefix(dir, m.home+"/"); ok {
			dir = "~/" + rest
		}
	}
	return cleanName(dir)
}

// sideWidth is the room the sidebar takes in this layout and width, with
// its separator; none if the layout has no sidebar or there is no room.
func (m Model) sideWidth() int {
	if m.layout != LayoutPlaces || m.width < 60 {
		return 0
	}
	return min(26, max(18, m.width/5)) + 1
}

// InSidebar says whether the cursor is in the sidebar, where Esc leaves it.
func (m Model) InSidebar() bool { return m.side }

// LeaveSidebar puts the cursor back in the folder.
func (m *Model) LeaveSidebar() { m.side = false }

// focusSidebar moves the cursor to the sidebar, on the place shown if it is
// one.
func (m *Model) focusSidebar() {
	if m.sideWidth() == 0 {
		return
	}
	m.side = true
	items := m.sideItems()
	m.sideAt = 0
	for i, it := range items {
		if !it.header && it.path == m.viewDir() {
			m.sideAt = i
		}
	}
	m.sideStep(0)
}

// sideStep moves the cursor to the next place in a direction, over the
// headings; by 0 only settles it on a place, the nearest at or after it.
func (m *Model) sideStep(by int) {
	items := m.sideItems()
	if len(items) == 0 {
		m.side = false
		return
	}
	i := min(max(m.sideAt, 0), len(items)-1)
	if by == 0 {
		for j := i; j < len(items); j++ {
			if !items[j].header {
				m.sideAt = j
				return
			}
		}
		by = -1
	}
	for j := i + by; j >= 0 && j < len(items); j += by {
		if !items[j].header {
			m.sideAt = j
			return
		}
	}
}

// sideKey takes a key while the cursor is in the sidebar: the arrows move,
// Enter goes, and anything else gives the cursor back to the folder, a letter
// being typed into the filter as it would have been.
func (m Model) sideKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch {
	case msg.String() == "up" || msg.String() == "ctrl+p":
		m.sideStep(-1)
	case msg.String() == "down" || msg.String() == "ctrl+n":
		m.sideStep(1)
	case msg.String() == "enter" || msg.String() == "right":
		items := m.sideItems()
		if m.sideAt < 0 || m.sideAt >= len(items) || items[m.sideAt].header {
			return m, nil, true
		}
		m.side = false
		return m, m.enter(items[m.sideAt].path, ""), true
	case msg.String() == "esc" || msg.String() == "tab" || msg.String() == "left" || msg.String() == "ctrl+g":
		m.side = false
	default:
		m.side = false
		return m, nil, false
	}
	return m, nil, true
}

// sideCells draws the sidebar's rows, in h rows and w cells each.
func (m Model) sideCells(w, h int) []string {
	items := m.sideItems()
	start := 0
	if m.side && m.sideAt >= h {
		start = m.sideAt - h + 1
	}
	var out []string
	for i := start; i < len(items) && len(out) < h; i++ {
		it := items[i]
		var cell string
		switch {
		case it.header:
			cell = m.styles.Footer.Render(" " + it.label)
		case m.side && i == m.sideAt:
			cell = m.styles.Cursor.Render(">") + " " + m.styles.Selected.Render(ansi.Truncate(it.label, w-2, "…"))
		case it.path == m.viewDir():
			cell = m.styles.Trail.Render("▸") + " " + m.styles.Trail.Render(ansi.Truncate(it.label, w-2, "…"))
		default:
			cell = "  " + m.styles.Directory.Render(ansi.Truncate(it.label, w-2, "…"))
		}
		out = append(out, cell)
	}
	return out
}

// placesView draws the sidebar beside the columns.
func (m Model) placesView(h int) []string {
	side := m.sideWidth()
	if side == 0 {
		return m.columnsView(m.width, h)
	}
	w := side - 1
	cells := m.sideCells(w, h)
	cols := m.columnsView(m.width-side, h)
	sep := m.styles.Separator.Render("│")
	lines := make([]string, h)
	for y := range lines {
		cell := ""
		if y < len(cells) {
			cell = cells[y]
		}
		lines[y] = cell + strings.Repeat(" ", max(0, w-ansi.StringWidth(cell))) + sep + cols[y]
	}
	return lines
}

// visit notes a folder browsed, for Back and for the recent places.
func (m *Model) visit(dir string) {
	if m.travel {
		return
	}
	if m.histAt < len(m.hist)-1 {
		m.hist = m.hist[:m.histAt+1] // Branching off from the past drops the future.
	}
	if m.hist[m.histAt] == dir {
		return
	}
	m.hist = append(m.hist, dir)
	m.histAt = len(m.hist) - 1
}

// goHistory goes back (-1) or forward (1) through the folders visited.
func (m *Model) goHistory(by int) tea.Cmd {
	to := m.histAt + by
	if to < 0 || to >= len(m.hist) {
		return nil
	}
	m.histAt = to
	m.travel = true
	defer func() { m.travel = false }()
	return m.enter(m.hist[to], "")
}
