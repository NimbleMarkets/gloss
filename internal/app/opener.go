//go:build !js

package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/pgavlin/picky"
)

// opener browses the host's folders for a file to add to the session.
type opener struct {
	picker picky.Model
	reads  *readLog
	seen   int
	dir    string // The folder being listed, for the status bar.
}

type openResult struct {
	path string
	err  error
}

// readLog notes which folder the picker read last. The picker does not say
// where it is, and it reads from a command, outside the update loop.
//
// It is also where unsupported files are left out, when that is asked for.
type readLog struct {
	fs.ReadDirFS
	mu      sync.Mutex
	dir     string
	count   int
	allowed []string // Extensions the picker does not grey; nil allows all.
	hide    bool
}

func (l *readLog) ReadDir(name string) ([]fs.DirEntry, error) {
	l.mu.Lock()
	l.dir, l.count = name, l.count+1
	hide := l.hide && l.allowed != nil
	l.mu.Unlock()
	entries, err := l.ReadDirFS.ReadDir(name)
	if !hide {
		return entries, err
	}
	return slices.DeleteFunc(entries, func(e fs.DirEntry) bool {
		return !e.IsDir() && !slices.Contains(l.allowed, strings.ToLower(filepath.Ext(e.Name())))
	}), err
}

func (o *opener) view() string { return o.picker.View() }

func (o *opener) resize(w, h int) {
	if o == nil {
		return
	}
	// The filter's text input cannot draw itself narrower than its
	// placeholder; the view clips whatever does not fit.
	o.picker.SetWidth(max(w, 48))
	o.picker.SetHeight(max(1, h-1))
	// The picker shortens its window of rows but never lengthens it;
	// filtering again lays it out afresh around the cursor.
	o.picker.SetFilterValue(o.picker.FilterValue())
}

// The embedded gallery has no folders, and a preview never takes keys.
func (m *Model) canBrowse() bool { return m.opts.FilesFS == nil && !m.isPreview }

func (m *Model) openBrowser() tea.Cmd {
	if !m.canBrowse() {
		return nil
	}
	dir, err := os.Getwd()
	if len(m.opts.Files) > 0 && !strings.HasPrefix(filepath.Base(m.opts.Files[m.index]), "gloss-stdin-") {
		if file, fileErr := filepath.Abs(m.opts.Files[m.index]); fileErr == nil {
			dir, err = filepath.Dir(file), nil
		}
	}
	if err != nil {
		m.note = "cannot browse: " + safe(err.Error())
		return nil
	}
	return m.browseFrom(dir)
}

func (m *Model) browseFrom(dir string) tea.Cmd {
	dir, err := filepath.Abs(dir)
	if err != nil || !m.canBrowse() {
		return nil
	}
	reads := &readLog{ReadDirFS: os.DirFS(string(filepath.Separator)).(fs.ReadDirFS), hide: m.hideUnsupported}
	options := []picky.Option{picky.WithFS(reads)}
	if m.opts.Type == "" {
		// Judged by name alone. A file without an extension may still be
		// recognized by its content, so it is left for the taking.
		reads.allowed = append([]string{""}, document.Extensions...)
		options = append(options, picky.WithAllowedTypes(reads.allowed))
	}
	m.opener = &opener{picker: picky.New(filepath.ToSlash(dir), options...), reads: reads, dir: dir}
	m.opener.resize(m.width, m.bodyHeight())
	return m.opener.picker.Init()
}

func (m *Model) browse(msg tea.Msg) tea.Cmd {
	o := m.opener
	var cmd tea.Cmd
	o.picker, cmd = o.picker.Update(msg)
	// A path typed into the filter lists other folders in passing; the
	// folder being browsed is the one read while no path was being typed.
	o.reads.mu.Lock()
	read, count := o.reads.dir, o.reads.count
	o.reads.mu.Unlock()
	if count != o.seen && !strings.ContainsRune(o.picker.FilterValue(), filepath.Separator) {
		o.seen, o.dir = count, string(filepath.Separator)+filepath.FromSlash(read)
	}
	chosen := o.picker.Selected()
	if chosen == "" {
		return cmd
	}
	o.picker.ClearSelected()
	path, forced := string(filepath.Separator)+filepath.FromSlash(chosen), m.opts.Type
	return tea.Batch(cmd, func() tea.Msg {
		_, err := document.Probe(path, forced)
		return openResult{path: path, err: err}
	})
}

func (m *Model) toggleUnsupported() tea.Cmd {
	o := m.opener
	m.hideUnsupported = !m.hideUnsupported
	o.reads.mu.Lock()
	o.reads.hide = m.hideUnsupported
	o.reads.mu.Unlock()
	cmd := o.picker.Init() // Reads the folder again.
	// The folder of a typed path is read only when the text changes, so its
	// last character is typed again.
	if path := o.picker.FilterValue(); strings.ContainsRune(path, filepath.Separator) {
		last, size := utf8.DecodeLastRuneInString(path)
		o.picker.SetFilterValue("")
		o.picker.SetFilterValue(path[:len(path)-size])
		cmd = tea.Batch(cmd, m.browse(tea.KeyPressMsg{Code: last, Text: string(last)}))
	}
	return cmd
}

func (m *Model) opened(r openResult) tea.Cmd {
	if m.opener == nil {
		return nil // Cancelled while the file was being examined.
	}
	if r.err != nil {
		// The browser stays open: choosing something else is one key away.
		m.note = safe(filepath.Base(r.path)) + ": " + safe(document.SkipReason(r.err))
		return nil
	}
	return m.addDropped(dropResult{files: []string{r.path}})
}
