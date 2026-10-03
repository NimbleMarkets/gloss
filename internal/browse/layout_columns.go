package browse

import (
	"path"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// minColumn is the least a column may be drawn in.
const minColumn = 22

type columnKind int

const (
	colTrail  columnKind = iota // A folder above the one shown, the path through it marked.
	colFocus                    // The folder shown, with the cursor and the filter.
	colPeek                     // The folder under the cursor.
	colDetail                   // The file under the cursor.
)

// column is one of the columns drawn: what it lists and where.
type column struct {
	kind columnKind
	dir  string // The folder it lists; for a detail, the folder of the file.
	sel  string // The name the path runs through, for a trail.
	x, w int
}

// columns lays out the columns in the width: as many as fit, up to the most
// asked for, the last ones of the chain from the root to what is under the
// cursor.
func (m Model) columns(width int) []column {
	focus := m.viewDir()
	var chain []column
	var dirs []string
	for d := focus; ; d = parentOf(d) {
		dirs = append(dirs, d)
		if d == "/" {
			break
		}
	}
	slices.Reverse(dirs)
	for i, d := range dirs {
		c := column{kind: colTrail, dir: d}
		if i+1 < len(dirs) {
			c.sel = path.Base(dirs[i+1])
		}
		if d == focus {
			c.kind = colFocus
		}
		chain = append(chain, c)
	}
	if peek := m.peekDir(); peek != "" {
		chain = append(chain, column{kind: colPeek, dir: peek})
	} else if r := m.rowAt(m.cursor); r != nil && !r.parent {
		chain = append(chain, column{kind: colDetail, dir: focus})
	}
	n := min(max((width+1)/(minColumn+1), 1), m.maxColumns)
	switch {
	case n == 1:
		chain = chain[len(chain)-1:]
		for _, c := range chain {
			if c.kind != colFocus {
				chain = []column{{kind: colFocus, dir: focus}}
			}
		}
	case len(chain) > n:
		chain = chain[len(chain)-n:]
	}
	room := width - (len(chain) - 1)
	x := 0
	for i := range chain {
		w := room / len(chain)
		if i < room%len(chain) {
			w++
		}
		chain[i].x, chain[i].w = x, w
		x += w + 1
	}
	return chain
}

// columnRows are the rows a column lists, and the row it starts at, in the
// room the body has.
func (m Model) columnRows(c column, h int) (rows []row, start int) {
	switch c.kind {
	case colFocus:
		return m.rows, m.top
	case colTrail:
		rows = m.rowsOf(c.dir, "", false)
		i := slices.IndexFunc(rows, func(r row) bool { return r.name == c.sel })
		return rows, max(0, min(i-h/2, len(rows)-h))
	case colPeek:
		return m.rowsOf(c.dir, "", false), 0
	}
	return nil, 0
}

// columnsView draws the columns side by side, in the width given.
func (m Model) columnsView(width, h int) []string {
	cols := m.columns(width)
	cells := make([][]string, len(cols))
	for i, c := range cols {
		cells[i] = m.columnCells(c, h)
	}
	sep := m.styles.Separator.Render("│")
	lines := make([]string, h)
	for y := range lines {
		var b strings.Builder
		for i, c := range cols {
			if i > 0 {
				b.WriteString(sep)
			}
			cell := ""
			if y < len(cells[i]) {
				cell = cells[i][y]
			}
			b.WriteString(cell)
			b.WriteString(strings.Repeat(" ", max(0, c.w-ansi.StringWidth(cell))))
		}
		lines[y] = b.String()
	}
	return lines
}

// columnCells draws one column's rows, h of them at most.
func (m Model) columnCells(c column, h int) []string {
	if c.kind == colDetail {
		var out []string
		for _, l := range m.detail(m.Current(), c.dir) {
			out = append(out, m.styles.Detail.Render(ansi.Truncate("  "+l, c.w, "…")))
		}
		return out
	}
	rows, start := m.columnRows(c, h)
	if len(rows) == 0 {
		l := m.lists[c.dir]
		switch {
		case c.kind == colFocus:
			return []string{m.empty()}
		case l == nil || !l.loaded:
			return []string{m.styles.Empty.Render("  …")}
		case l.err != nil:
			return []string{m.styles.Error.Render(ansi.Truncate("  ⚠ "+problem(l.err), c.w, "…"))}
		}
		return []string{m.styles.Empty.Render("  empty")}
	}
	var out []string
	for i := start; i < len(rows) && i < start+h; i++ {
		out = append(out, m.columnRow(c, rows[i], c.kind == colFocus && i == m.cursor, c.kind == colTrail && rows[i].name == c.sel))
	}
	return out
}

// columnRow draws a row of a column: ">" before the one under the cursor, "▸"
// before the one the path runs through.
func (m Model) columnRow(c column, r row, cursor, trail bool) string {
	disabled := !r.entry.IsDir() && !m.canChoose(r.entry)
	base, hl := m.styles.File, m.styles.Match
	prefix := "  "
	switch {
	case cursor && disabled:
		base, prefix = m.styles.DisabledSelected, m.styles.Cursor.Render(">")+" "
	case cursor:
		base, prefix = m.styles.Selected, m.styles.Cursor.Render(">")+" "
		hl = hl.Bold(true)
	case trail:
		base, prefix = m.styles.Trail, m.styles.Trail.Render("▸")+" "
	case disabled:
		base = m.styles.Disabled
	case r.entry.IsDir():
		base = m.styles.Directory
	}
	used := 2
	var mark string
	if m.marker != nil {
		s := m.marker(r.entry)
		mark = m.styles.Mark.Render(s) + " "
		used += ansi.StringWidth(s) + 1
	}
	return prefix + mark + m.name(r, c.w-used, base, hl)
}
