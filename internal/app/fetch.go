package app

import (
	"os"
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"
)

type fetchResult struct {
	address, path string
	err           error
}

// A file fetched from a cell, which Escape closes, returning to that cell.
type fetchedDoc struct {
	path  string
	from  int        // The sheet's place in the file list.
	sheet *sheetView // The sheet as it was, cursor included.
}

// Fetched lists the files fetched from the web this session, for the
// caller to remove when gloss is done, unless they were picked.
func (m *Model) Fetched() []string {
	paths := make([]string, len(m.fetched))
	for i, f := range m.fetched {
		paths[i] = f.path
	}
	return paths
}

// open fetches the address under the cursor and shows what it names.
// Nothing is fetched unless --fetch allowed it.
func (m *Model) open(address string) tea.Cmd {
	if m.opts.Fetch == nil {
		m.note = "run with --fetch to open addresses"
		return nil
	}
	fetch := m.opts.Fetch
	m.note = "fetching " + safe(address)
	return func() tea.Msg {
		path, err := fetch(address)
		return fetchResult{address: address, path: path, err: err}
	}
}

func (m *Model) fetchedFile(r fetchResult) tea.Cmd {
	if r.err != nil {
		m.note = "fetch failed: " + safe(r.err.Error())
		return nil
	}
	m.fetched = append(m.fetched, fetchedDoc{path: r.path, from: m.index, sheet: m.sheet})
	m.note = "fetched " + safe(filepath.Base(r.path))
	return m.addDropped(dropResult{files: []string{r.path}})
}

// isFetched says whether the file on screen came from a cell.
func (m *Model) isFetched() bool {
	return m.screen == screenDocument && m.index < len(m.opts.Files) && slices.ContainsFunc(m.fetched, func(f fetchedDoc) bool { return f.path == m.opts.Files[m.index] })
}

// closeFetched drops the fetched file on screen, from the list and from
// disk, and returns to the cell it was opened from.
func (m *Model) closeFetched() (tea.Cmd, bool) {
	if !m.isFetched() {
		return nil, false
	}
	i := slices.IndexFunc(m.fetched, func(f fetchedDoc) bool { return f.path == m.opts.Files[m.index] })
	f := m.fetched[i]
	m.fetched = slices.Delete(m.fetched, i, i+1)
	m.added = slices.DeleteFunc(m.added, func(p string) bool { return p == f.path })
	m.opts.Files = slices.Delete(m.opts.Files, m.index, m.index+1)
	for j := range m.fetched {
		if m.fetched[j].from > m.index {
			m.fetched[j].from--
		}
	}
	os.Remove(f.path)
	m.savedSheet = f.sheet
	m.note = "closed " + safe(filepath.Base(f.path))
	return m.show(min(f.from, len(m.opts.Files)-1)), true
}
