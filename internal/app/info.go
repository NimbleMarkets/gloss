package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// infoBox draws the details of the current file, no larger than w by h cells.
// It returns "" when the terminal has no room for a box.
func (m *Model) infoBox(w, h int) string {
	inner, rows := min(56, w-8), h-2
	if inner < 16 || rows < 2 {
		return ""
	}
	labels := 0
	for _, f := range m.fields {
		if f.Value != "" {
			labels = max(labels, lipgloss.Width(f.Label))
		}
	}
	text := boxText
	heading, label := text.Bold(true).Foreground(lipgloss.Color("231")), boxDim
	var lines []string
	for _, f := range m.fields {
		if f.Value == "" {
			lines = append(lines, heading.Render(ansi.Truncate(safe(f.Label), inner, "…")))
			continue
		}
		name, value := safe(f.Label), safe(f.Value)
		room := max(1, inner-labels-3)
		if over := ansi.StringWidth(value) - room; over > 0 && f.Label == "Path" {
			// The name at the end of a path says more than the root at its start.
			value = "…" + ansi.TruncateLeft(value, over+1, "")
		}
		value = ansi.Truncate(value, room, "…")
		lines = append(lines, label.Render(" "+name+strings.Repeat(" ", labels-ansi.StringWidth(name)+2))+text.Render(value))
	}
	if len(lines) == 0 {
		lines = []string{text.Render("No details yet.")}
	}
	if len(lines) > rows {
		lines = append(lines[:rows-1], text.Render("…"))
	}
	return box(lines)
}

// The corner boxes' colours: a shade, plain text on it, and dim text.
var (
	boxShade = lipgloss.Color("235")
	boxText  = lipgloss.NewStyle().Background(boxShade).Foreground(lipgloss.Color("252"))
	boxDim   = boxText.Foreground(lipgloss.Color("245"))
)

// box frames styled lines as a shaded corner box, each padded to the widest.
func box(lines []string) string {
	shade, text := boxShade, boxText
	width := 0
	for _, line := range lines {
		width = max(width, lipgloss.Width(line))
	}
	for i, line := range lines {
		lines[i] = line + text.Render(strings.Repeat(" ", width-lipgloss.Width(line)))
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("244")).BorderBackground(shade).
		Background(shade).Padding(0, 1).Render(strings.Join(lines, "\n"))
}

// overlay lays box over the right of body, which is w cells wide, top rows
// below its first.
// Rows are cut between cells, so a Kitty image keeps the placeholders that
// remain visible and gives up only those the box covers.
func overlay(body, box string, w, top int) string {
	if box == "" {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, row := range strings.Split(box, "\n") {
		i += top
		if i >= len(lines) {
			break
		}
		x := max(0, w-ansi.StringWidth(row))
		left := ansi.Truncate(lines[i], x, "")
		lines[i] = left + "\x1b[m" + strings.Repeat(" ", max(0, x-ansi.StringWidth(left))) + row
	}
	return strings.Join(lines, "\n")
}
