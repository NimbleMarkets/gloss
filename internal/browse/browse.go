// Package browse is a file chooser for Bubble Tea: a folder shown in one
// of several layouts, filtered by what is typed, and walked with the arrow
// keys, Tab completion, breadcrumbs, or the mouse.
//
// It is a component of its own. It knows nothing of gloss: what gloss adds
// (marks for the kinds of file, a sort, which files may be chosen) comes
// in by options. Any [fs.ReadDirFS] will do, so tests use an in-memory one.
//
// Paths are slash-separated and rooted at the filesystem's "/", as
// os.DirFS("/") presents them.
//
// The layouts only draw one state (see [Layout]); the state, the keys, and
// the filter are the same in each.
//
// A host drives it like any bubble:
//
//	m := browse.New(os.DirFS("/").(fs.ReadDirFS), "/home/me")
//	cmd := m.Init()
//	m, cmd = m.Update(msg)
//	if path, ok := m.Chosen(); ok { ... }
//
// Esc is the host's: it should clear a filter ([Model.ClearFilter]) while
// there is one and leave otherwise.
package browse

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Layout is how a folder is laid out: the same state, drawn another way.
type Layout int

const (
	// LayoutList is one column, each row with its size and date.
	LayoutList Layout = iota
	// LayoutColumns is Miller columns, as in Finder: the folders above the
	// one shown, the one shown, and what is under the cursor.
	LayoutColumns
	// LayoutPlaces is the columns with a sidebar of places beside them:
	// the home folders, and the folders visited lately.
	LayoutPlaces
	layoutCount
)

func (l Layout) String() string {
	switch l {
	case LayoutColumns:
		return "columns"
	case LayoutPlaces:
		return "places"
	}
	return "list"
}

// ParseLayout is the layout a name stands for.
func ParseLayout(s string) (Layout, bool) {
	for l := Layout(0); l < layoutCount; l++ {
		if l.String() == strings.ToLower(s) {
			return l, true
		}
	}
	return LayoutList, false
}

// Option configures a chooser.
type Option func(*Model)

// WithMarker draws what mark returns for an entry (an icon, say) before its
// name. Marks should be of one width. With none, a folder's name ends in a
// slash.
func WithMarker(mark func(fs.DirEntry) string) Option { return func(m *Model) { m.marker = mark } }

// WithSort orders the entries of a folder: folders first as ever, and among
// folders and among files by cmp. Nil is by name.
func WithSort(cmp func(a, b fs.DirEntry) int) Option { return func(m *Model) { m.sort = cmp } }

// WithSelectable says which files may be chosen; the others are greyed. By
// default all may.
func WithSelectable(can func(fs.DirEntry) bool) Option { return func(m *Model) { m.selectable = can } }

// WithShowHidden shows dot-files without being asked by a filter that
// starts with a dot.
func WithShowHidden(show bool) Option { return func(m *Model) { m.showHidden = show } }

// WithHome says where "~" leads, and what the breadcrumbs call "~"; none, to
// leave "~" an ordinary name. The default is the user's home folder.
func WithHome(dir string) Option {
	return func(m *Model) {
		m.home = ""
		if dir != "" {
			m.home = cleanAbs(dir)
		}
	}
}

// WithStyles sets how the chooser is drawn.
func WithStyles(s Styles) Option { return func(m *Model) { m.styles = s } }

// WithKeyMap sets the keys.
func WithKeyMap(k KeyMap) Option { return func(m *Model) { m.keys = k } }

// WithLayout starts in a layout.
func WithLayout(l Layout) Option { return func(m *Model) { m.layout = l } }

// WithPrompt sets what stands before the filter.
func WithPrompt(p string) Option { return func(m *Model) { m.prompt = p } }

// WithFooter shows, or hides, the line under the folder that counts its
// rows and says what is wrong with it.
func WithFooter(show bool) Option { return func(m *Model) { m.footer = show } }

// WithMaxColumns limits how many columns the columns layout draws.
func WithMaxColumns(n int) Option { return func(m *Model) { m.maxColumns = max(1, n) } }

// WithPlaces sets the places the sidebar lists, in place of the home
// folders ([DefaultPlaces]); none, to have no places beyond the recent.
func WithPlaces(p []Place) Option {
	return func(m *Model) { m.places, m.placesSet = p, true }
}

// WithDirsOnlyCompletion has Tab complete the names of folders alone, as when
// the aim is to go to a folder; files are still listed, for the look of what is
// in it.
func WithDirsOnlyCompletion(on bool) Option { return func(m *Model) { m.dirsOnly = on } }

// WithLinksFollowed says the filesystem already presents a link to a folder as
// a folder (its host did [FollowLinks] before filtering the listing), so the
// chooser need not look at the links again.
func WithLinksFollowed() Option { return func(m *Model) { m.linksFollowed = true } }

// Model is the chooser. Like bubbles' own, it is a value: Update returns
// the new one.
type Model struct {
	id    uint64
	fsys  fs.ReadDirFS
	home  string
	input textinput.Model

	dir    string              // The folder the browsing is in.
	lists  map[string]*listing // What was read of each folder.
	memo   map[string]string   // The name the cursor was on in each folder left.
	sel    string              // The name under the cursor.
	cursor int                 // Its row.
	top    int                 // The first row drawn.
	rows   []row               // The rows of the folder shown, after the filter.
	moved  bool                // The cursor was moved since the filter last changed.
	cycle  *cycle              // Tab is stepping through candidates.
	chosen string
	gen    int // Counts changes to how folders are sorted and filtered.
	epoch  int // Counts reloads, to tell readings begun before one.

	hist   []string // The folders browsed, in order, and where in them this is.
	histAt int
	travel bool // Moving through the history, which does not add to it.

	dirsOnly      bool // Tab completes folders alone.
	linksFollowed bool // The host has made links to folders folders.

	filters []Filter // The kinds of file that can be chosen to show alone.
	picked  []bool   // Which are.
	menu    bool     // The menu of them is open.
	menuAt  int      // Its row under the cursor; 0 is "All types".

	places    []Place
	placesSet bool
	side      bool // The cursor is in the sidebar.
	sideAt    int  // Which of its places.

	width, height int

	layout     Layout
	marker     func(fs.DirEntry) string
	sort       func(a, b fs.DirEntry) int
	selectable func(fs.DirEntry) bool
	showHidden bool
	prompt     string
	footer     bool
	maxColumns int
	styles     Styles
	keys       KeyMap
}

var nextID atomic.Uint64

// New makes a chooser in a folder of the filesystem.
func New(fsys fs.ReadDirFS, dir string, opts ...Option) Model {
	in := textinput.New()
	in.Prompt = ""
	in.Focus() // Its blink command is not wanted: the cursor is drawn here.
	home, _ := os.UserHomeDir()
	m := Model{
		id:         nextID.Add(1),
		fsys:       fsys,
		home:       cleanAbs(home),
		input:      in,
		dir:        cleanAbs(dir),
		lists:      map[string]*listing{},
		memo:       map[string]string{},
		width:      80,
		height:     24,
		prompt:     "  Filter: ",
		footer:     true,
		maxColumns: 4,
		styles:     DefaultStyles(),
		keys:       DefaultKeyMap(),
	}
	if home == "" {
		m.home = ""
	}
	for _, opt := range opts {
		opt(&m)
	}
	if !m.placesSet {
		m.places = DefaultPlaces(fsys, m.home)
	}
	m.hist = []string{m.dir}
	return m
}

// Init reads the folder, and the ones the layout shows beside it.
func (m Model) Init() tea.Cmd {
	return m.needs()
}

// Dir is the folder browsed: where Enter on a folder or ← goes from.
func (m Model) Dir() string { return m.dir }

// ViewDir is the folder shown, which is Dir unless a path has been typed
// into the filter and names another.
func (m Model) ViewDir() string { return m.viewDir() }

// Layout is the layout in use.
func (m Model) Layout() Layout { return m.layout }

// SetLayout draws another way.
func (m *Model) SetLayout(l Layout) tea.Cmd {
	m.layout = l % layoutCount
	m.side = false
	cmd := m.needs()
	m.rebuild() // The layouts list different rows: the parent is a row of the list alone.
	return cmd
}

// Current is the entry under the cursor: nil when nothing is listed, or the
// parent entry, whose name is "..".
func (m Model) Current() fs.DirEntry {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].entry
}

// CurrentPath is the path of the entry under the cursor, or "".
func (m Model) CurrentPath() string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	return path.Join(m.viewDir(), m.rows[m.cursor].name)
}

// Rows is how many rows are listed after the filter.
func (m Model) Rows() int { return len(m.rows) }

// Chosen is the file chosen with Enter, if one was.
func (m Model) Chosen() (string, bool) { return m.chosen, m.chosen != "" }

// ClearChosen forgets the choice, once the host has taken it.
func (m *Model) ClearChosen() { m.chosen = "" }

// FilterValue is what is typed.
func (m Model) FilterValue() string { return m.input.Value() }

// SetFilter types a filter, or a path, as if it had been typed.
func (m *Model) SetFilter(s string) tea.Cmd {
	m.input.SetValue(s)
	m.input.CursorEnd()
	return m.edited()
}

// ClearFilter empties the filter.
func (m *Model) ClearFilter() tea.Cmd { return m.SetFilter("") }

// GoTo browses a folder, with the filter cleared.
func (m *Model) GoTo(dir string) tea.Cmd {
	m.input.SetValue("")
	return m.enter(cleanAbs(dir), "")
}

// Reload reads the folders again.
func (m *Model) Reload() tea.Cmd {
	m.lists = map[string]*listing{}
	m.gen++
	m.epoch++
	cmd := m.needs()
	m.rebuild()
	return cmd
}

// SetSort orders the entries; see [WithSort].
func (m *Model) SetSort(cmp func(a, b fs.DirEntry) int) {
	m.sort = cmp
	m.gen++
	m.rebuild()
}

// SetShowHidden shows or hides dot-files.
func (m *Model) SetShowHidden(show bool) {
	m.showHidden = show
	m.gen++
	m.rebuild()
}

// SetSelectable changes which files may be chosen; see [WithSelectable].
func (m *Model) SetSelectable(can func(fs.DirEntry) bool) { m.selectable = can }

// SetDirsOnlyCompletion turns completion of folders alone on or off; see
// [WithDirsOnlyCompletion].
func (m *Model) SetDirsOnlyCompletion(on bool) { m.dirsOnly = on }

// SetPrompt changes what stands before the filter.
func (m *Model) SetPrompt(p string) { m.prompt = p }

// SetSize sets the room the view may take.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = max(1, w), max(1, h)
	// A popup that can no longer be drawn does not keep the keys.
	if m.menu && !m.canShowMenu() {
		m.menu = false
	}
	if m.side && m.sideWidth() == 0 {
		m.side = false
	}
	m.reveal()
}

// Size is the room the view takes.
func (m Model) Size() (w, h int) { return m.width, m.height }

// canChoose says whether a file may be chosen.
func (m Model) canChoose(e fs.DirEntry) bool {
	return m.selectable == nil || m.selectable(e)
}
