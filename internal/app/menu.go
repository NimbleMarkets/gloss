package app

import (
	"fmt"
	"path/filepath"
	"strconv"
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
	if m.screen != screenList || !m.opts.Preview || m.width < 64 || m.bodyHeight() < 7 {
		return 0
	}
	return m.width - m.listWidth() - 1
}

// listWidth is the column the names take beside a preview: as wide as the
// longest, with its number and marks, and no wider than the old bound.
func (m *Model) listWidth() int {
	longest := 0
	for _, f := range m.opts.Files {
		longest = max(longest, ansi.StringWidth(safe(filepath.Base(f))))
	}
	// A cursor, a mark, a number and two spaces, then the name.
	return max(16, min(min(40, m.width/2), longest+len(strconv.Itoa(len(m.opts.Files)))+5))
}

func (m *Model) resizePreview() tea.Cmd {
	if m.preview == nil {
		return m.updatePreview()
	}
	if w := m.previewWidth(); w > 0 {
		_, cmd := m.preview.Update(tea.WindowSizeMsg{Width: w, Height: m.bodyHeight()})
		return cmd
	}
	return m.disposePreview()
}

func (m *Model) updatePreview() tea.Cmd {
	if m.previewWidth() == 0 {
		return nil
	}
	var cleanup tea.Cmd
	if m.preview != nil && m.preview.index != m.selection {
		cleanup = m.disposePreview()
	}
	if m.preview == nil {
		opts := m.opts
		opts.Menu, opts.Preview, opts.Page, opts.Browse = false, false, 1, ""
		opts.Pick, opts.Drops, opts.Fetch, opts.Prompt = false, nil, nil, ""
		opts.Color = m.tint // Previews are painted as the viewer is.
		// Keep previews inexpensive and independent of the active camera.
		opts.DPI = min(opts.DPI, 96)
		m.preview = New(opts)
		m.preview.isPreview, m.preview.index = true, m.selection
		_, resize := m.preview.Update(tea.WindowSizeMsg{Width: max(1, m.previewWidth()), Height: m.bodyHeight()})
		return tea.Sequence(cleanup, tea.Batch(resize, m.preview.Init()))
	}
	return nil
}

func (m *Model) disposePreview() tea.Cmd {
	m.previewDrag = false
	if m.preview == nil {
		return nil
	}
	cleanup := m.preview.clearGraphics()
	_ = m.preview.Close()
	m.preview = nil
	return cleanup
}

// openMenu shows the file list, in the style last used. The active
// document is set aside, not unloaded: cancelling restores its page,
// camera, and scroll position.
func (m *Model) openMenu(selection int) tea.Cmd {
	m.selection, m.suspended = selection, true
	m.keepLayer()
	m.savedMarkdown = m.markdown
	if m.chart != nil {
		camera := m.chart.Camera()
		m.savedCamera = &camera
	}
	if m.thumbs {
		return tea.Sequence(m.clearGraphics(), m.openGrid())
	}
	m.screen = screenList
	return tea.Sequence(m.clearGraphics(), m.updatePreview())
}

// closeMenu leaves the list for the file chosen, or the one set aside.
func (m *Model) closeMenu(open bool) tea.Cmd {
	m.screen = screenDocument
	cleanup := tea.Batch(m.disposePreview(), m.pic.SetImage(nil))
	suspended := m.suspended
	m.suspended = false
	if open && m.selection != m.index {
		return tea.Sequence(cleanup, m.switchFile(m.selection-m.index))
	}
	if suspended {
		return tea.Sequence(cleanup, m.load(false))
	}
	return cleanup
}

// The chart's zones are local to the right pane. Capture a drag so release
// outside that pane still reaches the chart and ends the gesture.
func (m *Model) previewMouse(msg tea.MouseMsg) tea.Cmd {
	pw := m.previewWidth()
	if m.preview == nil || pw == 0 {
		return nil
	}
	mouse := msg.Mouse()
	left := m.width - pw
	inside := mouse.X >= left && mouse.X < m.width && mouse.Y >= 0 && mouse.Y < m.bodyHeight()
	if !inside && !m.previewDrag {
		return nil
	}
	switch msg.(type) {
	case tea.MouseClickMsg:
		m.previewDrag = inside && mouse.Button == tea.MouseLeft
	case tea.MouseReleaseMsg:
		m.previewDrag = false
	}
	mouse.X -= left
	var translated tea.Msg
	switch msg.(type) {
	case tea.MouseClickMsg:
		translated = tea.MouseClickMsg(mouse)
	case tea.MouseMotionMsg:
		translated = tea.MouseMotionMsg(mouse)
	case tea.MouseReleaseMsg:
		translated = tea.MouseReleaseMsg(mouse)
	case tea.MouseWheelMsg:
		translated = tea.MouseWheelMsg(mouse)
	default:
		return nil
	}
	_, cmd := m.preview.Update(translated)
	return cmd
}

// menuKey handles a key on the list; the grid has keys of its own.
func (m *Model) menuKey(k string) tea.Cmd {
	if m.screen == screenGrid {
		if cmd, ok := m.gridKey(k); ok {
			return cmd
		}
	}
	switch k {
	case "enter":
		return m.closeMenu(true)
	case "m", "esc":
		return m.closeMenu(false)
	case "t":
		if m.screen == screenGrid {
			return m.closeGrid()
		}
		return m.openGrid()
	case "v":
		m.opts.Preview = !m.opts.Preview
		if !m.opts.Preview && m.preview != nil {
			return m.disposePreview()
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
	if m.screen == screenGrid {
		return m.pic.View().Content
	}
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
		name := safe(m.opts.Files[i])
		if pw > 0 {
			name = safe(filepath.Base(m.opts.Files[i])) // The preview says the rest.
		}
		line := ansi.Truncate(fmt.Sprintf("%s%s %d  %s", cursor, current, i+1, name), w, "…")
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
