//go:build !js

package app

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/gloss/internal/browse"
	"github.com/NimbleMarkets/gloss/internal/document"
)

// The orders a folder can be listed in, cycled with Ctrl-S.
const (
	byName = iota
	byDate
	byKind
)

var orderNames = []string{"name", "date", "kind"}

// The marks of the kinds of file, by extension; a folder has its own, and
// what gloss cannot open has none.
var kindMarks = map[string]string{
	".png": "📷", ".jpg": "📷", ".jpeg": "📷", ".gif": "📷", ".webp": "📷", ".bmp": "📷", ".tif": "📷", ".tiff": "📷", ".heic": "📷", ".heif": "📷", ".hif": "📷",
	".svg": "🎨", ".pdf": "📕", ".stl": "🧊", ".3mf": "🧊",
	".md": "📝", ".markdown": "📝", ".mdown": "📝", ".html": "📝", ".htm": "📝", ".txt": "📝", ".text": "📝", ".log": "📝",
	".json": "🧾", ".jsonl": "🧾", ".ndjson": "🧾", ".ipynb": "📓",
	".docx": "📄", ".docm": "📄", ".xlsx": "📊", ".xlsm": "📊", ".csv": "📊", ".tsv": "📊", ".grist": "📊",
}

// typeFilters are the kinds of file the browser can be told to show alone, by
// the marks that say what a file is; the menu names them for people.
var typeFilters = []struct {
	name  string
	marks []string
}{
	{"Pictures", []string{"📷"}},
	{"Drawings (SVG)", []string{"🎨"}},
	{"PDF", []string{"📕"}},
	{"Meshes (STL, 3MF)", []string{"🧊"}},
	{"Markdown, HTML, text", []string{"📝", "📃"}},
	{"JSON", []string{"🧾"}},
	{"Notebooks", []string{"📓"}},
	{"Word", []string{"📄"}},
	{"Tables (CSV, Excel, Grist)", []string{"📊"}},
}

// browseFilters are those kinds as the chooser takes them.
func browseFilters() []browse.Filter {
	out := make([]browse.Filter, len(typeFilters))
	for i, f := range typeFilters {
		out[i] = browse.Filter{Name: f.name, Mark: f.marks[0], Match: func(e fs.DirEntry) bool { return slices.Contains(f.marks, mark(e)) }}
	}
	return out
}

// mark is what stands before a name in the listing, in place of its mode.
func mark(e fs.DirEntry) string {
	if e.IsDir() {
		return "📁"
	}
	if m, ok := kindMarks[strings.ToLower(filepath.Ext(e.Name()))]; ok {
		return m
	}
	if s, ok := e.(sniffed); ok && s.text {
		return "📃" // Text by its content, whatever its name.
	}
	return "  "
}

// ordering compares two files, or two folders, for the order named.
func ordering(by int) func(a, b fs.DirEntry) int {
	switch by {
	case byDate:
		return func(a, b fs.DirEntry) int {
			ai, aErr := a.Info()
			bi, bErr := b.Info()
			if aErr != nil || bErr != nil {
				return strings.Compare(a.Name(), b.Name())
			}
			if c := bi.ModTime().Compare(ai.ModTime()); c != 0 {
				return c // Newest first.
			}
			return strings.Compare(a.Name(), b.Name())
		}
	case byKind:
		return func(a, b fs.DirEntry) int {
			if c := strings.Compare(kindOrder(a), kindOrder(b)); c != 0 {
				return c
			}
			return strings.Compare(a.Name(), b.Name())
		}
	}
	return nil
}

// The kinds in the order they are listed: pictures, drawings, documents,
// meshes, text, data, notebooks, Word, and tables; the unmarked last.
var kindRanks = []string{"📷", "🎨", "📕", "🧊", "📝", "📃", "🧾", "📓", "📄", "📊"}

// kindOrder groups files by kind, then extension.
func kindOrder(e fs.DirEntry) string {
	rank := slices.Index(kindRanks, mark(e))
	if rank < 0 {
		rank = len(kindRanks)
	}
	return string(rune('a'+rank)) + strings.ToLower(filepath.Ext(e.Name()))
}

// opener browses the host's folders for a file to add to the session. The
// browsing itself is the browse package's; this is what gloss adds to it.
type opener struct {
	picker browse.Model
	reads  *readLog
	dir    string // The folder being browsed, for the status bar.
	going  bool   // A folder's path is being typed, to go to.
	find   *finder
	width  int
	height int
}

const filterPrompt, goPrompt = "  Filter: ", "  Go to: "

// goTo asks for a folder's path, starting from the one shown, with the
// picker's completion at hand.
func (o *opener) goTo() tea.Cmd {
	o.going = true
	o.picker.SetPrompt(goPrompt)
	o.picker.SetDirsOnlyCompletion(true) // The aim is a folder; its files are still listed.
	return o.picker.SetFilter(o.dir + string(filepath.Separator))
}

// leavePopup closes what floats over the folder, the menu of kinds or the
// cursor in the sidebar of places, and says whether there was any.
func (o *opener) leavePopup() bool {
	switch {
	case o.picker.InMenu():
		o.picker.CloseMenu()
	case o.picker.InSidebar():
		o.picker.LeaveSidebar()
	default:
		return false
	}
	return true
}

// stopGoing ends the go-to, the filter cleared.
func (o *opener) stopGoing() tea.Cmd {
	o.going = false
	o.picker.SetPrompt(filterPrompt)
	o.picker.SetDirsOnlyCompletion(false)
	return o.picker.ClearFilter()
}

type openResult struct {
	path string
	err  error
}

// readLog is the filesystem the picker reads: where files that gloss cannot
// show are looked into, and left out when that is asked for. It reads from a
// command, outside the update loop, so its choices are kept under a lock.
type readLog struct {
	fs.ReadDirFS
	mu      sync.Mutex
	allowed []string // Extensions the picker does not grey; nil allows all.
	hide    bool
	text    bool // Files that read as text may be chosen, whatever their name.
}

// A file as listed, with what a look at its start said of it.
type sniffed struct {
	fs.DirEntry
	text bool
}

// ReadDir lists a folder, each file that no extension accounts for looked
// at to see whether it is text, and the files that cannot be chosen left
// out when that is asked for.
func (l *readLog) ReadDir(name string) ([]fs.DirEntry, error) {
	l.mu.Lock()
	hide, allowed := l.hide && l.allowed != nil, l.allowed
	l.mu.Unlock()
	entries, err := l.ReadDirFS.ReadDir(name)
	entries = browse.FollowLinks(l.ReadDirFS, name, entries) // Before any are left out.
	if allowed == nil {
		return entries, err
	}
	for i, e := range entries {
		if e.IsDir() || slices.Contains(allowed, strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		entries[i] = sniffed{e, l.readsAsText(path.Join(name, e.Name()))}
	}
	if !hide {
		return entries, err
	}
	return slices.DeleteFunc(entries, func(e fs.DirEntry) bool { return !e.IsDir() && !l.selectable(e) }), err
}

// readsAsText looks at the start of a file.
func (l *readLog) readsAsText(name string) bool {
	f, err := l.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, document.TextSniff+1)
	n, _ := io.ReadFull(f, head)
	return document.IsText(head[:n])
}

// selectable says whether a listed file may be chosen: one of gloss's
// kinds by extension, or text while text files are offered.
func (l *readLog) selectable(e fs.DirEntry) bool {
	if s, ok := e.(sniffed); ok {
		l.mu.Lock()
		defer l.mu.Unlock()
		return s.text && l.text
	}
	return true
}

func (o *opener) view() string {
	if o.find != nil {
		return o.find.view(o.width, o.height)
	}
	return o.picker.View()
}

func (o *opener) resize(w, h int) {
	if o == nil {
		return
	}
	o.width, o.height = w, h
	if o.find != nil {
		o.find.input.SetWidth(max(1, w-lipgloss.Width(o.find.input.Prompt)-1))
	}
	o.picker.SetSize(w, h)
}

// The embedded gallery has no folders, and a preview never takes keys.
func (m *Model) canBrowse() bool { return m.opts.FilesFS == nil && !m.isPreview }

func (m *Model) openBrowser() tea.Cmd {
	if !m.canBrowse() {
		return nil
	}
	dir, err := os.Getwd()
	if len(m.opts.Files) > 0 && !m.opts.IsStdin(m.opts.Files[m.index]) {
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
	reads := &readLog{ReadDirFS: os.DirFS(string(filepath.Separator)).(fs.ReadDirFS), hide: m.hideUnsupported, text: !m.noTextFiles}
	options := []browse.Option{browse.WithMarker(mark), browse.WithSort(ordering(m.sortBy)), browse.WithLayout(browse.Layout(m.browseLayout)), browse.WithPrompt(filterPrompt),
		browse.WithFilters(browseFilters()), browse.WithActiveFilters(m.browseTypes...)}
	if m.opts.Type == "" {
		// Judged by name where the name says; the rest by a look inside.
		reads.allowed = document.Extensions
		options = append(options, browse.WithSelectable(reads.selectable))
	}
	m.opener = &opener{picker: browse.New(reads, filepath.ToSlash(dir), options...), reads: reads, dir: dir}
	m.opener.resize(m.width, m.bodyHeight())
	m.screen, m.help = screenBrowser, false
	m.keepLayer()
	return m.opener.picker.Init()
}

func (m *Model) browse(msg tea.Msg) tea.Cmd {
	o := m.opener
	entered := false
	if k, ok := msg.(tea.KeyPressMsg); ok {
		if o.find != nil {
			if cmd, ok := m.findKey(k); ok {
				return cmd
			}
		}
		switch {
		case k.String() == "/" && !o.going && o.picker.FilterValue() == "":
			o.find = newFinder(o.dir, o.width)
			return nil
		case k.String() == "G" && !o.going && o.picker.FilterValue() == "":
			return o.goTo()
		case k.String() == "enter" && o.going:
			entered = true
		}
	}
	var cmd tea.Cmd
	o.picker, cmd = o.picker.Update(msg)
	o.dir, m.browseLayout, m.browseTypes = filepath.FromSlash(o.picker.Dir()), int(o.picker.Layout()), o.picker.ActiveFilters()
	if entered && o.picker.FilterValue() == "" {
		// The folder typed was gone to.
		o.going = false
		o.picker.SetPrompt(filterPrompt)
		o.picker.SetDirsOnlyCompletion(false)
	}
	chosen, ok := o.picker.Chosen()
	if !ok {
		return cmd
	}
	o.picker.ClearChosen()
	path, forced := filepath.FromSlash(chosen), m.opts.Type
	return tea.Batch(cmd, func() tea.Msg {
		_, err := document.Probe(path, forced)
		return openResult{path: path, err: err}
	})
}

// reorder lists the folder in the next order.
func (m *Model) reorder() tea.Cmd {
	m.sortBy = (m.sortBy + 1) % len(orderNames)
	m.opener.picker.SetSort(ordering(m.sortBy))
	return nil
}

func (m *Model) toggleUnsupported() tea.Cmd {
	m.hideUnsupported = !m.hideUnsupported
	return m.reread()
}

// toggleText offers text files, or sets them aside.
func (m *Model) toggleText() tea.Cmd {
	m.noTextFiles = !m.noTextFiles
	return m.reread()
}

// reread lists the folders again under the choices made.
func (m *Model) reread() tea.Cmd {
	o := m.opener
	o.reads.mu.Lock()
	o.reads.hide, o.reads.text = m.hideUnsupported, !m.noTextFiles
	o.reads.mu.Unlock()
	return o.picker.Reload()
}

func (m *Model) opened(r openResult) tea.Cmd {
	if !m.browsing() {
		return nil // Cancelled while the file was being examined.
	}
	if r.err != nil {
		// The browser stays open: choosing something else is one key away.
		m.note = safe(filepath.Base(r.path)) + ": " + safe(document.SkipReason(r.err))
		return nil
	}
	return m.addDropped(dropResult{files: []string{r.path}})
}
