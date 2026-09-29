package app

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
)

// DropMsg adds files to the session. Hosts without bracketed paste, such as
// the browser demo, send it directly.
type DropMsg struct{ Paths []string }

type dropResult struct {
	files   []string
	skipped []string
}

// Skipped lists the dropped files that could not be added.
func (m *Model) Skipped() []string { return m.skipped }

// Probing reads from disk, so it runs outside the update loop.
func (m *Model) probeDrop(paths []string) tea.Cmd {
	if len(paths) == 0 {
		return nil
	}
	files, forced := m.opts.FilesFS, m.opts.Type
	return func() tea.Msg {
		var r dropResult
		for _, path := range paths {
			if _, err := document.ProbeFS(files, path, forced); err != nil {
				r.skipped = append(r.skipped, document.Skipped(path, err))
				continue
			}
			r.files = append(r.files, path)
		}
		return r
	}
}

func (m *Model) fileIndex(path string) int {
	same := func(a, b string) bool { return a == b }
	if m.opts.FilesFS == nil {
		same = func(a, b string) bool {
			a, errA := filepath.Abs(a)
			b, errB := filepath.Abs(b)
			return errA == nil && errB == nil && a == b
		}
	}
	for i, file := range m.opts.Files {
		if same(file, path) {
			return i
		}
	}
	return -1
}

func (m *Model) addDropped(r dropResult) tea.Cmd {
	m.skipped = append(m.skipped, r.skipped...)
	first, added := -1, 0
	for _, path := range r.files {
		i := m.fileIndex(path)
		if i < 0 {
			m.opts.Files = append(m.opts.Files, path)
			i = len(m.opts.Files) - 1
			if added == 0 {
				first = i
			}
			added++
		}
		if first < 0 {
			first = i
		}
	}
	var notes []string
	if added > 0 {
		notes = append(notes, fmt.Sprintf("added %d", added))
	}
	if len(r.skipped) > 0 {
		notes = append(notes, fmt.Sprintf("skipped %d", len(r.skipped)))
	}
	m.note = strings.Join(notes, " · ")
	if first < 0 {
		return nil
	}
	m.help = false
	switch {
	case added > 1 && m.menu:
		m.selection = first
		return m.updatePreview()
	case added > 1:
		return m.openMenu(first)
	case m.menu:
		m.selection = first
		return m.closeMenu(true)
	case first == m.index:
		return m.load(true)
	}
	return m.switchFile(first - m.index)
}
