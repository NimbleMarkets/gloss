package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The prompt says at most this many lines; the rest is cut.
const maxPromptLines = 4

// promptLines wraps the caller's prompt to the width of its box.
func (m *Model) promptLines() []string {
	inner := m.width - 4
	if m.opts.Prompt == "" || inner < 8 {
		return nil
	}
	text := lipgloss.NewStyle().Width(inner).Render(strings.TrimSpace(safe(m.opts.Prompt)))
	lines := strings.Split(text, "\n")
	if len(lines) > maxPromptLines {
		lines = lines[:maxPromptLines]
		lines[maxPromptLines-1] = lipgloss.NewStyle().Width(inner).Render(strings.TrimRight(lines[maxPromptLines-1], " ") + "…")
		lines = lines[:maxPromptLines]
	}
	return lines
}

// promptHeight is the rows the prompt's box takes, none when the screen
// has no room for a body beside it.
func (m *Model) promptHeight() int {
	lines := m.promptLines()
	if len(lines) == 0 || m.height-2-(len(lines)+2) < 3 {
		return 0
	}
	return len(lines) + 2
}

// promptBox is the prompt in a frame as wide as the screen.
func (m *Model) promptBox() string {
	lines := m.promptLines()
	if m.promptHeight() == 0 {
		return ""
	}
	return lipgloss.NewStyle().Bold(true).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(colorNimbleRed)).
		Padding(0, 1).Width(m.width - 2).Render(strings.Join(lines, "\n"))
}

// framed puts the prompt's box above or below the body.
func (m *Model) framed(body string) string {
	box := m.promptBox()
	switch {
	case box == "":
		return body
	case m.opts.PromptTop:
		return box + "\n" + body
	default:
		return body + "\n" + box
	}
}

// onBody moves a mouse event from the screen to the body, which begins
// below the prompt when that is on top.
func (m *Model) onBody(msg tea.Msg) tea.Msg {
	top := 0
	if m.opts.PromptTop {
		top = m.promptHeight()
	}
	if top == 0 {
		return msg
	}
	switch v := msg.(type) {
	case tea.MouseClickMsg:
		v.Y -= top
		return v
	case tea.MouseMotionMsg:
		v.Y -= top
		return v
	case tea.MouseReleaseMsg:
		v.Y -= top
		return v
	case tea.MouseWheelMsg:
		v.Y -= top
		return v
	}
	return msg
}
