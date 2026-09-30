//go:build !js

package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

// finder searches the folder the browser shows for files by glob, kind,
// or extension, within the limits a search is kept to, and lists what it
// finds for opening one, or all.
type finder struct {
	input    textinput.Model
	dir      string
	query    string // What the last search was for.
	results  []string
	note     string
	cursor   int
	running  bool
	searched bool
}

type findResult struct {
	dir   string
	query string
	found document.Found
	err   error
}

func newFinder(dir string, w int) *finder {
	input := textinput.New()
	input.Prompt = "  Find: "
	input.Placeholder = "a glob, an extension, or a kind such as images"
	input.SetWidth(max(1, w-lipgloss.Width(input.Prompt)-1))
	input.Focus()
	return &finder{input: input, dir: dir}
}

// search runs the query typed, unless it is the one already answered.
func (f *finder) search() tea.Cmd {
	query := strings.TrimSpace(f.input.Value())
	if query == "" {
		return nil
	}
	f.query, f.running, f.searched = query, true, true
	dir := f.dir
	return func() tea.Msg {
		found, err := document.Find(dir, strings.Fields(query), document.Limits{})
		return findResult{dir: dir, query: query, found: found, err: err}
	}
}

func (f *finder) answered(r findResult) {
	if r.dir != f.dir || r.query != f.query {
		return
	}
	f.running, f.cursor = false, 0
	if r.err != nil {
		f.results, f.note = nil, safe(r.err.Error())
		return
	}
	f.results, f.note = r.found.Paths, r.found.Note
}

// current is the result under the cursor.
func (f *finder) current() string {
	if f.cursor < 0 || f.cursor >= len(f.results) {
		return ""
	}
	return f.results[f.cursor]
}

func (f *finder) view(w, h int) string {
	lines := []string{f.input.View()}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	switch {
	case f.running:
		lines = append(lines, dim.Render("  searching…"))
	case !f.searched:
		lines = append(lines, dim.Render("  Type what to look for, and press Enter. Hidden folders are not searched."))
	case len(f.results) == 0:
		lines = append(lines, dim.Render("  nothing found"))
	default:
		rows := max(1, h-1)
		top := max(0, min(f.cursor-rows/2, len(f.results)-rows))
		for i := top; i < len(f.results) && len(lines) < h; i++ {
			rel, err := filepath.Rel(f.dir, f.results[i])
			if err != nil {
				rel = f.results[i]
			}
			line := "  " + safe(filepath.ToSlash(rel))
			if i == f.cursor {
				line = lipgloss.NewStyle().Reverse(true).Render(ansi.Truncate("> "+safe(filepath.ToSlash(rel)), w, "…"))
			}
			lines = append(lines, ansi.Truncate(line, w, "…"))
		}
	}
	return strings.Join(lines, "\n")
}

// status describes the search for the status bar.
func (f *finder) status() string {
	switch {
	case f.running:
		return "searching"
	case !f.searched:
		return "search " + safe(f.dir)
	}
	out := fmt.Sprintf("%d found", len(f.results))
	if f.note != "" {
		out += " · " + f.note
	}
	return out
}

// findKey handles a key while the finder is open, and reports whether it
// was one of its own. Esc is the viewer's, and closes the finder there.
func (m *Model) findKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	o := m.opener
	f := o.find
	switch msg.String() {
	case "enter":
		if strings.TrimSpace(f.input.Value()) != f.query || !f.searched {
			return f.search(), true
		}
		if path := f.current(); path != "" {
			forced := m.opts.Type
			return func() tea.Msg {
				_, err := document.Probe(path, forced)
				return openResult{path: path, err: err}
			}, true
		}
		return nil, true
	case "up", "ctrl+p":
		f.cursor = max(0, f.cursor-1)
		return nil, true
	case "down", "ctrl+n":
		f.cursor = min(max(0, len(f.results)-1), f.cursor+1)
		return nil, true
	case "ctrl+a":
		if len(f.results) == 0 {
			return nil, true
		}
		return m.probeDrop(f.results), true
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return cmd, true
}
