package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

type previewResult struct {
	owner  int
	result document.Result
}

func (m *Model) previewWidth() int {
	if !m.menu || !m.opts.Preview || m.width < 64 || m.bodyHeight() < 7 {
		return 0
	}
	return m.width - min(40, m.width/2) - 1
}

func (m *Model) resizePreview() tea.Cmd {
	if m.preview == nil {
		return m.updatePreview()
	}
	if w := m.previewWidth(); w > 0 {
		_, cmd := m.preview.Update(tea.WindowSizeMsg{Width: w, Height: m.bodyHeight()})
		return cmd
	}
	return nil
}

func (m *Model) updatePreview() tea.Cmd {
	if !m.menu || !m.opts.Preview {
		return nil
	}
	if m.preview == nil {
		opts := m.opts
		opts.Menu, opts.Preview, opts.Page = false, false, 1
		// Keep previews inexpensive and independent of the active camera.
		opts.Render3D, opts.DPI = "software", min(opts.DPI, 96)
		m.preview = New(opts)
		m.preview.isPreview, m.preview.index = true, m.selection
		_, resize := m.preview.Update(tea.WindowSizeMsg{Width: max(1, m.previewWidth()), Height: m.bodyHeight()})
		return tea.Batch(resize, m.preview.Init())
	}
	if m.preview.index != m.selection {
		return m.preview.switchFile(m.selection - m.preview.index)
	}
	return nil
}

func (m *Model) closeMenu(open bool) tea.Cmd {
	m.menu = false
	var cleanup tea.Cmd
	if m.preview != nil {
		cleanup = m.preview.pic.SetImage(nil)
		_ = m.preview.Close()
		m.preview = nil
	}
	if open && m.selection != m.index {
		return tea.Batch(cleanup, m.switchFile(m.selection-m.index))
	}
	return cleanup
}

func (m *Model) menuKey(k string) tea.Cmd {
	switch k {
	case "enter":
		return m.closeMenu(true)
	case "m":
		return m.closeMenu(false)
	case "v":
		m.opts.Preview = !m.opts.Preview
		if !m.opts.Preview && m.preview != nil {
			cmd := m.preview.pic.SetImage(nil)
			_ = m.preview.Close()
			m.preview = nil
			return cmd
		}
		return m.updatePreview()
	case "j", "down", "tab":
		m.selection++
	case "k", "up", "shift+tab":
		m.selection--
	case "pgdown":
		m.selection += m.bodyHeight()
	case "pgup":
		m.selection -= m.bodyHeight()
	case "home", "g":
		m.selection = 0
	case "end", "G":
		m.selection = len(m.opts.Files) - 1
	default:
		return nil
	}
	m.selection = max(0, min(len(m.opts.Files)-1, m.selection))
	return m.updatePreview()
}

func (m *Model) menuView() string {
	w, h := max(1, m.width), m.bodyHeight()
	pw := m.previewWidth()
	if pw > 0 {
		w -= pw + 1
	}
	start := max(0, min(m.selection-h/2, len(m.opts.Files)-h))
	lines := make([]string, 0, h)
	for i := start; i < len(m.opts.Files) && len(lines) < h; i++ {
		cursor, current := " ", " "
		if i == m.selection {
			cursor = ">"
		}
		if i == m.index {
			current = "*"
		}
		line := ansi.Truncate(fmt.Sprintf("%s%s %d  %s", cursor, current, i+1, safe(m.opts.Files[i])), w, "…")
		style := lipgloss.NewStyle().Width(w)
		if i == m.selection {
			style = style.Reverse(true)
		}
		lines = append(lines, style.Render(line))
	}
	list := lipgloss.NewStyle().Width(w).Height(h).Render(strings.Join(lines, "\n"))
	if pw > 0 && m.preview != nil {
		return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", m.preview.View().Content)
	}
	return list
}
