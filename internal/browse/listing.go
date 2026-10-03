package browse

import (
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// listing is what was read of a folder.
type listing struct {
	entries []fs.DirEntry
	err     error
	loaded  bool
	loading bool

	sorted  []fs.DirEntry // entries in order, with every one.
	visible []fs.DirEntry // And without the hidden.
	gen     int           // Of the sort and hiding they are for.
}

// listedMsg is a folder read.
type listedMsg struct {
	id      uint64
	epoch   int // Of the reading, so that one begun before a reload is let go.
	dir     string
	entries []fs.DirEntry
	err     error
}

// load reads a folder, unless it has been or is being.
func (m *Model) load(dir string) tea.Cmd {
	if l := m.lists[dir]; l != nil {
		return nil
	}
	m.lists[dir] = &listing{loading: true}
	fsys, id, epoch := m.fsys, m.id, m.epoch
	return func() tea.Msg {
		entries, err := fsys.ReadDir(fsName(dir))
		return listedMsg{id: id, epoch: epoch, dir: dir, entries: FollowLinks(fsys, dir, entries), err: err}
	}
}

// linked is an entry that is a link to a folder, which is to be treated as
// one.
type linked struct {
	fs.DirEntry
}

func (linked) IsDir() bool         { return true }
func (l linked) Type() fs.FileMode { return l.DirEntry.Type()&^fs.ModeSymlink | fs.ModeDir }

// FollowLinks finds which of the entries of a folder are links to folders,
// which a listing does not say, and returns them as folders. A host that
// filters a listing before the chooser sees it should do this first.
func FollowLinks(fsys fs.FS, dir string, entries []fs.DirEntry) []fs.DirEntry {
	out, cloned := entries, false
	for i, e := range entries {
		if e.Type()&fs.ModeSymlink == 0 {
			continue
		}
		if info, err := fs.Stat(fsys, path.Join(fsName(dir), e.Name())); err == nil && info.IsDir() {
			if !cloned {
				out, cloned = slices.Clone(entries), true
			}
			out[i] = linked{e}
		}
	}
	return out
}

// listed takes a read folder in.
func (m *Model) listed(msg listedMsg) {
	m.lists[msg.dir] = &listing{entries: msg.entries, err: msg.err, loaded: true}
}

// order is a folder's entries in the order shown, hidden ones included or
// not, kept until the order changes.
func (m *Model) order(dir string, hidden bool) []fs.DirEntry {
	l := m.lists[dir]
	if l == nil || !l.loaded {
		return nil
	}
	if l.sorted == nil || l.gen != m.gen {
		l.sorted = slices.Clone(l.entries)
		sortEntries(l.sorted, m.sort)
		l.visible = slices.DeleteFunc(slices.Clone(l.sorted), func(e fs.DirEntry) bool { return strings.HasPrefix(e.Name(), ".") })
		l.gen = m.gen
	}
	if hidden || m.showHidden {
		return m.byType(l.sorted)
	}
	return m.byType(l.visible)
}

// sortEntries puts folders first, and orders each kind by cmp, or by name.
func sortEntries(entries []fs.DirEntry, cmp func(a, b fs.DirEntry) int) {
	if cmp == nil {
		cmp = func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) }
	}
	slices.SortStableFunc(entries, func(a, b fs.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		return cmp(a, b)
	})
}

// row is an entry as listed, with how the filter took it.
type row struct {
	entry  fs.DirEntry
	name   string
	at     []int // The letters the filter matched, by index in the name.
	score  int
	parent bool
}

// parentEntry stands for the folder above, listed as "..".
type parentEntry struct{}

func (parentEntry) Name() string               { return ".." }
func (parentEntry) IsDir() bool                { return true }
func (parentEntry) Type() fs.FileMode          { return fs.ModeDir }
func (parentEntry) Info() (fs.FileInfo, error) { return parentInfo{}, nil }

type parentInfo struct{}

func (parentInfo) Name() string       { return ".." }
func (parentInfo) Size() int64        { return 0 }
func (parentInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (parentInfo) ModTime() time.Time { return time.Time{} }
func (parentInfo) IsDir() bool        { return true }
func (parentInfo) Sys() any           { return nil }

// rowsOf lists a folder narrowed by a filter: all in order if it is empty,
// else those it matches, best first. A filter starting with a dot reaches
// the hidden. The parent is listed first when asked for.
func (m *Model) rowsOf(dir string, filter string, parent bool) []row {
	q := parseQuery(filter)
	entries := m.order(dir, strings.HasPrefix(filter, "."))
	rows := make([]row, 0, len(entries)+1)
	if parent && dir != "/" {
		rows = append(rows, row{entry: parentEntry{}, name: "..", parent: true})
	}
	if q.empty() {
		for _, e := range entries {
			rows = append(rows, row{entry: e, name: e.Name()})
		}
		return rows
	}
	rows = rows[:0]
	if dir != "/" {
		if score, at, ok := q.match(".."); ok {
			rows = append(rows, row{entry: parentEntry{}, name: "..", at: at, score: score, parent: true})
		}
	}
	for _, e := range entries {
		if score, at, ok := q.match(e.Name()); ok {
			rows = append(rows, row{entry: e, name: e.Name(), at: at, score: score})
		}
	}
	if !q.glob {
		slices.SortStableFunc(rows, func(a, b row) int { return b.score - a.score })
	}
	return rows
}

// problem says in a few words what went wrong reading a folder.
func problem(err error) string {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	case errors.Is(err, fs.ErrNotExist):
		return "no such folder"
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// needs reads what the view draws that has not been: the folder shown, the
// one under the cursor, and in the columns layout the ones above.
func (m *Model) needs() tea.Cmd {
	var cmds []tea.Cmd
	dir := m.viewDir()
	cmds = append(cmds, m.load(dir))
	if m.layout != LayoutList {
		for d, n := dir, 0; d != "/" && n < 8; n++ {
			d = parentOf(d)
			cmds = append(cmds, m.load(d))
		}
		if peek := m.peekDir(); peek != "" {
			cmds = append(cmds, m.load(peek))
		}
	}
	return tea.Batch(cmds...)
}

// peekDir is the folder under the cursor, "" when it is a file or nothing.
func (m Model) peekDir() string {
	if m.cursor < 0 || m.cursor >= len(m.rows) || m.rows[m.cursor].parent || !m.rows[m.cursor].entry.IsDir() {
		return ""
	}
	return path.Join(m.viewDir(), m.rows[m.cursor].name)
}
