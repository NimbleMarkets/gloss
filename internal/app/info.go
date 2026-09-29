package app

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *Model) infoLines() []string {
	name := safe(filepath.Base(m.opts.Files[m.index]))
	if strings.HasPrefix(name, "gloss-stdin-") {
		name = "stdin"
	}
	bold := lipgloss.NewStyle().Bold(true)
	lines := []string{bold.Render(name)}
	if len(m.fields) == 0 && m.err == nil {
		return append(lines, "", "No details yet.")
	}
	width := 0
	for _, f := range m.fields {
		if f.Value != "" {
			width = max(width, lipgloss.Width(f.Label))
		}
	}
	for _, f := range m.fields {
		if f.Value == "" {
			lines = append(lines, "", bold.Render(safe(f.Label)))
			continue
		}
		lines = append(lines, "  "+safe(f.Label)+strings.Repeat(" ", width-lipgloss.Width(f.Label)+2)+safe(f.Value))
	}
	if m.err != nil {
		lines = append(lines, "", bold.Render("Cannot open file"), "  "+safe(m.err.Error()))
	}
	return lines
}

func (m *Model) infoView() string {
	lines := m.infoLines()
	m.infoOffset = max(0, min(m.infoOffset, len(lines)-m.bodyHeight()))
	return strings.Join(lines[m.infoOffset:], "\n")
}

func (m *Model) infoKey(k string) {
	page := m.bodyHeight()
	switch k {
	case "i":
		m.info = false
	case "j", "down":
		m.infoOffset++
	case "k", "up":
		m.infoOffset--
	case "space", "pgdown":
		m.infoOffset += page
	case "b", "pgup":
		m.infoOffset -= page
	case "home", "g":
		m.infoOffset = 0
	case "end", "G":
		m.infoOffset = len(m.infoLines())
	}
	m.infoOffset = max(0, min(m.infoOffset, len(m.infoLines())-page))
}
