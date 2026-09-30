package app

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
)

type fetchResult struct {
	address, path string
	err           error
}

// Fetched lists the files fetched from the web this session, for the
// caller to remove when gloss is done, unless they were picked.
func (m *Model) Fetched() []string { return m.fetched }

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
	m.fetched = append(m.fetched, r.path)
	m.note = "fetched " + safe(filepath.Base(r.path))
	return m.addDropped(dropResult{files: []string{r.path}})
}
