package app

import (
	"strings"

	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

// pickingColumns says whether the column list is open over a sheet.
func (m *Model) pickingColumns() bool { return m.sheet != nil && m.sheet.picker != nil }

// columnPicker is the list of a sheet's columns, each ticked when shown.
type columnPicker struct {
	at, top int // The column under the cursor, and the first listed.
}

// key handles a key while the picker is open, and reports whether it was
// one of its own. Escape is the viewer's, and closes the picker there.
func (p *columnPicker) key(v *sheetView, k string) bool {
	switch k {
	case "j", "down":
		p.at++
	case "k", "up":
		p.at--
	case "home", "g":
		p.at = 0
	case "end", "G":
		p.at = len(v.hidden) - 1
	case "space":
		if v.hidden[p.at] {
			v.hidden[p.at] = false
			v.rebuild()
		} else {
			v.hide(p.at)
		}
	case "a":
		v.showAll()
	case "n":
		for c := range v.hidden {
			v.hide(c)
		}
	case "enter", "c":
		v.picker = nil
	default:
		return false
	}
	p.at = max(0, min(p.at, len(v.hidden)-1))
	return true
}

// view lists the columns in a box no taller than h rows, the cursor kept
// in sight.
func (p *columnPicker) view(v *sheetView, w, h int) string {
	rows := h - 2
	if rows < 1 || w < 20 {
		return ""
	}
	p.top = max(0, min(p.top, p.at, len(v.hidden)-rows))
	if p.at >= p.top+rows {
		p.top = p.at - rows + 1
	}
	var header []string
	if len(v.sheet.Rows) > 0 {
		header = v.sheet.Rows[0]
	}
	lines := make([]string, 0, rows)
	for c := p.top; c < len(v.hidden) && len(lines) < rows; c++ {
		tick := "[x]"
		if v.hidden[c] {
			tick = "[ ]"
		}
		name := ""
		if c < len(header) {
			name = ansi.Truncate(safe(strings.TrimSpace(header[c])), min(maxColumnWidth, w-16), "…")
		}
		line := strings.TrimRight(tick+" "+document.ColumnName(c)+"  "+name, " ")
		style := boxText
		if v.hidden[c] {
			style = boxDim
		}
		if c == p.at {
			style = style.Reverse(true)
		}
		lines = append(lines, style.Render(line))
	}
	return box(lines)
}
