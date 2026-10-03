package browse

import (
	"fmt"
	"io/fs"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	humanize "github.com/dustin/go-humanize"
)

// View draws the chooser in the room it was given: breadcrumbs, the filter,
// the folder in the layout chosen, and a footer.
func (m Model) View() string {
	body := m.bodyHeight()
	lines := []string{m.crumbsView(), m.filterView()}
	switch m.layout {
	case LayoutColumns:
		lines = append(lines, m.columnsView(m.width, body)...)
	case LayoutPlaces:
		lines = append(lines, m.placesView(body)...)
	default:
		lines = append(lines, m.listView(body)...)
	}
	for len(lines) < 2+body {
		lines = append(lines, "") // The footer is at the bottom, not after the last row.
	}
	if m.footer && m.height >= 5 {
		lines = append(lines, m.footerView())
	}
	if m.menu && m.canShowMenu() {
		x, y, _, _ := m.menuRect()
		overlay(lines, m.menuView(), x, y)
	}
	if m.height < 2 {
		lines = lines[1:2] // Only the filter fits.
	}
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	lines = lines[:m.height]
	for i, l := range lines {
		if ansi.StringWidth(l) > m.width {
			lines[i] = ansi.Truncate(l, m.width, "")
		}
	}
	return strings.Join(lines, "\n")
}

// filterView draws the line that is typed on: the prompt, the text with its
// cursor, and dim after it what Tab would add.
func (m Model) filterView() string {
	value := []rune(m.input.Value())
	pos := min(m.input.Position(), len(value))
	room := m.width - ansi.StringWidth(m.prompt) - 1
	var before, at, after string
	before = string(value[:pos])
	at = " "
	if pos < len(value) {
		at, after = string(value[pos]), string(value[pos+1:])
	}
	before, at, after = cleanName(before), cleanName(at), cleanName(after)
	if len(at) == 0 {
		at = " "
	}
	ghost := cleanName(m.ghost())
	if len(value) == 0 {
		hint := "type to filter, or a path: / ~ ../"
		return m.styles.Prompt.Render(m.prompt) + m.styles.InputCursor.Render(hint[:1]) + m.styles.Placeholder.Render(ansi.Truncate(hint[1:], max(0, room-1), "…"))
	}
	if over := ansi.StringWidth(before+at+after+ghost) - room; over > 0 && room > 1 {
		// Too long: the end of what is typed is what matters.
		cut := min(over+1, ansi.StringWidth(before))
		before = "…" + ansi.TruncateLeft(before, cut, "")
	}
	line := m.styles.Prompt.Render(m.prompt) + m.styles.Input.Render(before)
	if ghost != "" {
		// The cursor sits on the first letter of what Tab would add.
		first, rest := []rune(ghost)[0], string([]rune(ghost)[1:])
		return line + m.styles.InputCursor.Render(string(first)) + m.styles.Ghost.Render(rest)
	}
	return line + m.styles.InputCursor.Render(at) + m.styles.Input.Render(after)
}

// footerView says which row of how many the cursor is on, and what is
// amiss with the folder; and at its right end opens the menu of kinds.
func (m Model) footerView() string {
	l := m.lists[m.viewDir()]
	text := ""
	switch {
	case l != nil && l.err != nil:
		return m.styles.Error.Render(ansi.Truncate(" ⚠ "+problem(l.err), m.width, "…"))
	case len(m.rows) == 0:
		text = " 0 rows"
	case m.filterText() != "":
		total := len(m.order(m.viewDir(), strings.HasPrefix(m.filterText(), ".")))
		text = fmt.Sprintf(" %d/%d matches · %d in folder", m.cursor+1, len(m.rows), total)
	default:
		text = fmt.Sprintf(" %d/%d", m.cursor+1, len(m.rows))
	}
	text += " · " + m.layout.String()
	chip := m.chip()
	room := m.width - ansi.StringWidth(chip)
	if chip == "" || room < 8 {
		return m.styles.Footer.Render(ansi.Truncate(text, m.width, "…"))
	}
	text = ansi.Truncate(text, room, "…")
	pad := strings.Repeat(" ", max(room-ansi.StringWidth(text), 0))
	return m.styles.Footer.Render(text+pad) + m.chipStyle().Render(chip)
}

// empty says why there is nothing to list.
func (m Model) empty() string {
	l := m.lists[m.viewDir()]
	switch {
	case l == nil || !l.loaded:
		return m.styles.Empty.Render("  Reading…")
	case l.err != nil && len(l.entries) == 0:
		return m.styles.Error.Render("  ⚠ " + problem(l.err))
	case m.filterText() != "":
		return m.styles.Empty.Render("  No matching files.")
	case m.anyPicked() && len(l.entries) > 0:
		return m.styles.Empty.Render("  Nothing of that type.")
	}
	return m.styles.Empty.Render("  Empty folder.")
}

// listView draws the folder as one column of rows: the cursor, a mark, the
// size, the date when there is room, and the name.
func (m Model) listView(h int) []string {
	dates := m.width >= 60
	var lines []string
	for i := m.top; i < len(m.rows) && i < m.top+h; i++ {
		lines = append(lines, m.listRow(m.rows[i], i == m.cursor, dates))
	}
	// Nothing but the parent, or not even that: say why.
	if len(m.rows) == 0 || len(m.rows) == 1 && m.rows[0].parent {
		lines = append(lines, m.empty())
	}
	return lines
}

func (m Model) listRow(r row, cursor, dates bool) string {
	disabled := !r.entry.IsDir() && !m.canChoose(r.entry)
	base, hl := m.styles.File, m.styles.Match
	switch {
	case cursor && disabled:
		base = m.styles.DisabledSelected
	case cursor:
		base = m.styles.Selected
		hl = hl.Bold(true)
	case disabled:
		base = m.styles.Disabled
	case r.entry.IsDir():
		base = m.styles.Directory
	}
	var b strings.Builder
	if cursor {
		b.WriteString(m.styles.Cursor.Render(">") + " ")
	} else {
		b.WriteString("  ")
	}
	used := 2
	if m.marker != nil {
		mark := m.marker(r.entry)
		b.WriteString(m.styles.Mark.Render(mark) + " ")
		used += ansi.StringWidth(mark) + 1
	}
	size, date := "", ""
	if info, err := r.entry.Info(); err == nil && !r.entry.IsDir() {
		size = strings.Replace(humanize.Bytes(uint64(max(info.Size(), 0))), " ", "", 1) //nolint:gosec
		if dates {
			date = info.ModTime().Format("2006-01-02")
		}
	}
	if !r.parent && r.entry.IsDir() && dates {
		if info, err := r.entry.Info(); err == nil && !info.ModTime().IsZero() {
			date = info.ModTime().Format("2006-01-02")
		}
	}
	b.WriteString(m.styles.Size.Render(fmt.Sprintf("%7s", size)) + " ")
	used += 8
	if dates {
		b.WriteString(m.styles.Date.Render(fmt.Sprintf("%-10s", date)) + " ")
		used += 11
	}
	b.WriteString(m.name(r, m.width-used, base, hl))
	return b.String()
}

// name draws an entry's name in the room there is, the letters the filter
// matched stood out. A folder's ends in a slash if no mark says what it is.
func (m Model) name(r row, room int, base, hl lipgloss.Style) string {
	clean, at := cleanRunes(r.name)
	shown := string(clean)
	if m.marker == nil && !r.parent && r.entry.IsDir() {
		shown += "/"
	}
	cut := false
	if ansi.StringWidth(shown) > room {
		shown, cut = ansi.Truncate(shown, max(room, 1), "…"), true
	}
	runes := []rune(shown)
	if cut {
		runes = runes[:max(len(runes)-1, 0)] // The ellipsis is not a match.
	}
	matched := map[int]bool{}
	for _, i := range r.at {
		if i >= 0 && i < len(at) && at[i] >= 0 {
			matched[at[i]] = true
		}
	}
	var b strings.Builder
	for i := 0; i < len(runes); {
		j := i
		for j < len(runes) && matched[j] == matched[i] {
			j++
		}
		style := base
		if matched[i] {
			style = hl
		}
		b.WriteString(style.Render(string(runes[i:j])))
		i = j
	}
	if cut {
		b.WriteString(base.Render("…"))
	}
	return b.String()
}

// detail is the lines that say what a file is, for the last column.
func (m Model) detail(e fs.DirEntry, dir string) []string {
	lines := []string{cleanName(e.Name())}
	if m.marker != nil {
		lines[0] = m.marker(e) + " " + cleanName(e.Name())
	}
	info, err := e.Info()
	if err != nil {
		return append(lines, "", problem(err))
	}
	lines = append(lines, "",
		"Size      "+humanize.Bytes(uint64(max(info.Size(), 0))), //nolint:gosec
		"Modified  "+info.ModTime().Format("2006-01-02 15:04"),
		"Folder    "+cleanName(dir))
	if !m.canChoose(e) {
		lines = append(lines, "", "gloss cannot show this file")
	}
	return lines
}
