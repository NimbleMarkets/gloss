package browse

import (
	tea "charm.land/bubbletea/v2"
	"io/fs"
	"strings"
	"unicode"
	"unicode/utf8"
)

// cycle is Tab stepping through the names a typed beginning could be
// completed to, when there is no more to be said of it than that: the folder
// keeps listing what was typed, and the filter shows the candidate.
type cycle struct {
	base  string   // What was typed when the stepping began.
	names []string // The candidates, a folder's with its slash.
	i     int
}

// completion is what a typed name could be completed to.
type completion struct {
	dirPart string   // What was typed up to the last slash, kept as typed.
	rest    string   // What was typed after it.
	names   []string // Candidates, a folder's with its slash, best first.
	prefix  bool     // They begin with rest, rather than being fuzzy matches.
	lcp     string   // The longest start they share, when prefix.
}

// completions finds what the end of what is typed could become.
func (m *Model) completions(typed string) completion {
	c := completion{}
	c.dirPart, c.rest = split(typed)
	dir := m.resolve(c.dirPart)
	hidden := strings.HasPrefix(c.rest, ".")
	exact := strings.ContainsFunc(c.rest, unicode.IsUpper)
	restFold := c.rest
	if !exact {
		restFold = strings.ToLower(c.rest)
	}
	var entries []fs.DirEntry
	for _, e := range m.order(dir, hidden) {
		name := e.Name()
		if !exact {
			name = strings.ToLower(name)
		}
		if strings.HasPrefix(name, restFold) && (!m.dirsOnly || e.IsDir()) {
			entries = append(entries, e)
		}
	}
	if len(entries) > 0 {
		c.prefix = true
		for _, e := range entries {
			c.names = append(c.names, spell(e))
		}
		c.lcp = commonStart(entries, !exact)
		return c
	}
	if c.rest == "" {
		return c
	}
	for _, r := range m.rowsOf(dir, c.rest, false) {
		if !r.parent && (!m.dirsOnly || r.entry.IsDir()) { // The parent is no candidate.
			c.names = append(c.names, spell(r.entry))
		}
	}
	return c
}

// spell is an entry's name as completed: a folder's ends in a slash.
func spell(e fs.DirEntry) string {
	if e.IsDir() {
		return e.Name() + "/"
	}
	return e.Name()
}

// commonStart is the longest start the names share; the first one's spelling.
func commonStart(entries []fs.DirEntry, fold bool) string {
	first := []rune(entries[0].Name())
	n := len(first)
	for _, e := range entries[1:] {
		other := []rune(e.Name())
		i := 0
		for i < n && i < len(other) && same(first[i], other[i], fold) {
			i++
		}
		n = i
	}
	return string(first[:n])
}

func same(a, b rune, fold bool) bool {
	return a == b || fold && unicode.ToLower(a) == unicode.ToLower(b)
}

// complete does Tab (dir 1) and Shift-Tab (dir -1): a single candidate is
// completed, the start the candidates share is added if there is any, and
// else the candidates are stepped through, the filter showing each.
func (m *Model) complete(dir int) tea.Cmd {
	if c := m.cycle; c != nil {
		c.i = (c.i + dir + len(c.names)) % len(c.names)
		m.showCandidate()
		return m.needs()
	}
	typed := m.input.Value()
	c := m.completions(typed)
	switch {
	case len(c.names) == 0 && c.dirPart != "":
		return m.expand(typed) // Its folders may be only begun.
	case len(c.names) == 0:
		return nil
	case len(c.names) == 1:
		m.input.SetValue(c.dirPart + c.names[0])
		m.input.CursorEnd()
		return m.edited()
	case c.prefix && len([]rune(c.lcp)) > len([]rune(c.rest)):
		m.input.SetValue(c.dirPart + c.lcp)
		m.input.CursorEnd()
		return m.edited()
	}
	m.cycle = &cycle{base: typed, names: c.names}
	if dir < 0 {
		m.cycle.i = len(c.names) - 1
	}
	m.showCandidate()
	return m.needs()
}

// showCandidate puts the candidate Tab has come to in the filter, and the
// cursor on its row.
func (m *Model) showCandidate() {
	dirPart, _ := split(m.cycle.base)
	name := m.cycle.names[m.cycle.i]
	m.input.SetValue(dirPart + name)
	m.input.CursorEnd()
	m.sel = strings.TrimSuffix(name, "/")
	m.rebuild()
}

// ghost is what Tab would add to the end of what is typed, if it would
// complete it at once: drawn dim after the cursor, and taken by →. It also
// says the whole of the name the end of the typed text would become.
func (m *Model) ghost() (suffix string) {
	_, suffix = m.ghostName()
	return suffix
}

func (m *Model) ghostName() (name, suffix string) {
	if m.cycle != nil || m.input.Value() == "" || !m.cursorAtEnd() {
		return "", ""
	}
	c := m.completions(m.input.Value())
	if c.rest == "" || !c.prefix {
		return "", ""
	}
	full := c.lcp
	if len(c.names) == 1 {
		full = c.names[0]
	}
	have, want := []rune(c.rest), []rune(full)
	if len(want) <= len(have) {
		return "", ""
	}
	return c.dirPart + full, string(want[len(have):])
}

// cursorAtEnd says whether the cursor is after the last letter typed. The
// input counts its position in letters (runes), so the end is that many, not
// the number of bytes.
func (m Model) cursorAtEnd() bool {
	return m.input.Position() == utf8.RuneCountInString(m.input.Value())
}
