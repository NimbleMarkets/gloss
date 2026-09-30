// Package picky provides a terminal file picker component for Bubble Tea.
//
// It supports fuzzy filtering, directory navigation, tab completion, path-based
// input, and optional directory selection. The picker accepts any [fs.ReadDirFS],
// making it easy to test with in-memory filesystems.
//
// This is github.com/pgavlin/picky, MIT licensed (see LICENSE beside this
// file), carried in gloss with two additions: WithMarker replaces the
// permission column with a mark of the caller's choosing, and WithSort and
// SetSort order the listing. Folders always come first, and the parent
// before them.
package picky

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	humanize "github.com/dustin/go-humanize"
)

// readDirMsg is sent when a directory read completes.
type readDirMsg struct {
	entries []fs.DirEntry
}

// pathReadDirMsg is sent when a path-mode directory read completes.
type pathReadDirMsg struct {
	dir     string
	entries []fs.DirEntry
}

// Option configures a Picker.
type Option func(*Model)

// WithAllowedTypes restricts file selection to the given extensions (e.g. ".md").
func WithAllowedTypes(exts []string) Option {
	return func(p *Model) {
		p.allowedTypes = exts
	}
}

// WithStyles sets the Picker's visual styles.
func WithStyles(s Styles) Option {
	return func(p *Model) {
		p.styles = s
	}
}

// WithAllowDir enables directory selection via the SelectDir key binding.
func WithAllowDir(allow bool) Option {
	return func(p *Model) {
		p.allowDir = allow
	}
}

// WithFS sets the filesystem used for directory reads.
// Defaults to os.DirFS("/").
func WithFS(fsys fs.ReadDirFS) Option {
	return func(p *Model) {
		p.fsys = fsys
	}
}

// WithShowHidden controls whether hidden files (dot-prefixed) are shown.
func WithShowHidden(show bool) Option {
	return func(p *Model) {
		p.showHidden = show
	}
}

// WithHome sets the directory used to expand a leading "~" in path input. It
// should be relative to the root of the Model's fs. Defaults to the value of
// [os.UserHomeDir] with any leading separator stripped; an empty value
// disables expansion.
func WithHome(dir string) Option {
	return func(p *Model) {
		for path.IsAbs(dir) {
			dir = dir[1:]
		}
		p.home = dir
	}
}

// KeyMap defines the key bindings for the Picker.
type KeyMap struct {
	Up        key.Binding
	Down      key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
	Select    key.Binding
	SelectDir key.Binding
	Complete  key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "ctrl+p"),
			key.WithHelp("↑/ctrl+p", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "ctrl+n"),
			key.WithHelp("↓/ctrl+n", "move down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		SelectDir: key.NewBinding(
			key.WithKeys("shift+enter"),
			key.WithHelp("shift+enter", "select directory"),
		),
		Complete: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "complete"),
		),
	}
}

// WithKeyMap sets the Picker's key bindings.
// WithMarker shows what mark returns for an entry, such as an icon, in
// place of its permissions. Marks should be of one width.
func WithMarker(mark func(fs.DirEntry) string) Option {
	return func(p *Model) {
		p.marker = mark
	}
}

// WithSort orders the entries of a folder: folders first as ever, and
// within folders and within files by cmp. Nil is by name.
func WithSort(cmp func(a, b fs.DirEntry) int) Option {
	return func(p *Model) {
		p.sort = cmp
	}
}

// SetPrompt changes what stands before the filter text.
func (p *Model) SetPrompt(prompt string) {
	p.input.Prompt = prompt
	p.input.SetWidth(p.width - lipgloss.Width(prompt) - 1)
}

// SetPath puts a path in the filter, the cursor after it, and reads the
// folder it names, as typing it would.
func (p *Model) SetPath(path string) tea.Cmd {
	p.input.SetValue(path)
	p.input.CursorEnd()
	return p.handleInputChange()
}

// Current is the entry under the cursor, or nil when nothing is listed.
func (p Model) Current() fs.DirEntry {
	if p.cursor < 0 || p.cursor >= len(p.filtered) {
		return nil
	}
	return p.filtered[p.cursor]
}

// Dir is the folder being browsed, relative to the filesystem's root.
func (p Model) Dir() string { return p.dir }

// PathDir says, when a path is being typed, which folder it names so far
// and what has been typed since its last separator.
func (p *Model) PathDir() (dir, query string, ok bool) {
	if !p.isPathMode() {
		return "", "", false
	}
	dir, query = p.splitPathInput()
	return dir, query, true
}

// GoTo makes dir the folder browsed, with the filter cleared, as if it had
// been entered from where the browsing was: the parent entry leads back.
func (p *Model) GoTo(dir string) tea.Cmd {
	p.navStack = append(p.navStack, navState{dir: p.dir, cursor: p.cursor, minIdx: p.minIdx, maxIdx: p.maxIdx})
	p.dir = filepath.Clean(dir)
	p.cursor, p.minIdx, p.maxIdx = 0, 0, p.height-1
	p.input.SetValue("")
	p.pathDir, p.pathEntries = "", nil
	return p.readDir()
}

// SetSort changes the order, and lays out what is listed anew.
func (p *Model) SetSort(cmp func(a, b fs.DirEntry) int) {
	p.sort = cmp
	if len(p.allEntries) == 0 {
		return
	}
	entries := p.allEntries
	if _, ok := entries[0].(parentDirEntry); ok {
		entries = entries[1:]
	}
	sortEntries(entries, cmp)
	p.filter()
}

func WithKeyMap(km KeyMap) Option {
	return func(p *Model) {
		p.keyMap = km
	}
}

// navState stores the browsing state for a directory, used as a stack entry
// when navigating into subdirectories.
type navState struct {
	dir    string
	cursor int
	minIdx int
	maxIdx int
}

// Model is a file picker with fuzzy text filtering.
type Model struct {
	input        textinput.Model
	fsys         fs.ReadDirFS
	wd           string        // program working directory (for resolving relative paths in path mode)
	dir          string        // current browsing directory
	allEntries   []fs.DirEntry // full dir listing (dirs first, sorted)
	filtered     []fs.DirEntry // fuzzy-matched subset
	cursor       int           // index into filtered
	minIdx       int           // first visible index
	maxIdx       int           // last visible index
	height       int
	width        int
	allowedTypes []string
	allowDir     bool
	showHidden   bool
	home         string // fs-relative home directory for "~" expansion
	selected     string // set when a file or directory is selected
	styles       Styles
	keyMap       KeyMap
	marker       func(fs.DirEntry) string   // In place of the permission column.
	sort         func(a, b fs.DirEntry) int // Among folders, and among files; nil is by name.

	// Path mode state (ephemeral — does not affect dir or nav stacks).
	pathDir     string        // resolved directory for current path input
	pathEntries []fs.DirEntry // entries read from pathDir

	// Navigation stack for directory enter/back.
	navStack []navState
}

// parentDirEntry is a synthetic fs.DirEntry for the ".." parent directory.
type parentDirEntry struct{}

func (parentDirEntry) Name() string               { return ".." }
func (parentDirEntry) IsDir() bool                { return true }
func (parentDirEntry) Type() fs.FileMode          { return fs.ModeDir }
func (parentDirEntry) Info() (fs.FileInfo, error) { return parentFileInfo{}, nil }

type parentFileInfo struct{}

func (parentFileInfo) Name() string       { return ".." }
func (parentFileInfo) Size() int64        { return 0 }
func (parentFileInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (parentFileInfo) ModTime() time.Time { return time.Time{} }
func (parentFileInfo) IsDir() bool        { return true }
func (parentFileInfo) Sys() any           { return nil }

// New creates a new Picker rooted at dir. By default it uses os.DirFS("/")
// for directory reads; use [WithFS] to override.
//
// NOTE: due to the use of fs.FS, dir should not be an absolute path. dir will
// be interpreted relative to the root of the Model's fs.
func New(dir string, opts ...Option) Model {
	ti := textinput.New()
	ti.Prompt = "  Filter: "
	ti.Placeholder = "type to filter..."
	ti.Focus() // set focused state; Init returns the blink cmd

	for path.IsAbs(dir) {
		dir = dir[1:]
	}

	home, _ := os.UserHomeDir()
	for path.IsAbs(home) {
		home = home[1:]
	}

	p := Model{
		input:  ti,
		fsys:   os.DirFS("/").(fs.ReadDirFS),
		wd:     dir,
		dir:    dir,
		home:   home,
		height: 20,
		styles: DefaultStyles(),
		keyMap: DefaultKeyMap(),
	}
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

// Selected returns the path of the selected file or directory, or "" if nothing
// is selected. Directory selection requires [WithAllowDir](true) and the
// SelectDir key binding. The path will be relative to the root of the Model's
// file system.
func (p *Model) Selected() string {
	return p.selected
}

// ClearSelected clears the current selection.
func (p *Model) ClearSelected() {
	p.selected = ""
}

// FilterValue returns the current filter input text.
func (p *Model) FilterValue() string {
	return p.input.Value()
}

// SetFilterValue sets the filter input text and re-filters the entries.
func (p *Model) SetFilterValue(s string) {
	p.input.SetValue(s)
	p.filter()
}

// Focus focuses the filter text input and returns a blink command.
func (p *Model) Focus() tea.Cmd {
	return p.input.Focus()
}

// Init initializes the picker by focusing the text input and reading the
// initial directory listing.
func (p Model) Init() tea.Cmd {
	return tea.Batch(p.input.Focus(), p.readDir())
}

func (p Model) readDir() tea.Cmd {
	dir := p.dir
	fsys := p.fsys
	showHidden, cmp := p.showHidden, p.sort
	return func() tea.Msg {
		entries := readAndSortDir(fsys, dir, showHidden, cmp)
		return readDirMsg{entries: entries}
	}
}

func (p Model) pathReadDir(dir string) tea.Cmd {
	fsys := p.fsys
	showHidden, cmp := p.showHidden, p.sort
	return func() tea.Msg {
		entries := readAndSortDir(fsys, dir, showHidden, cmp)
		return pathReadDirMsg{dir: dir, entries: entries}
	}
}

// readAndSortDir reads a directory, sorts dirs first then alphabetically,
// and optionally filters hidden files.
func readAndSortDir(fsys fs.ReadDirFS, dir string, showHidden bool, cmp func(a, b fs.DirEntry) int) []fs.DirEntry {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return nil
	}
	sortEntries(entries, cmp)

	if showHidden {
		return entries
	}

	var visible []fs.DirEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		visible = append(visible, e)
	}
	return visible
}

// isPathMode returns true when the input contains a path separator.
func (p *Model) isPathMode() bool {
	return strings.ContainsRune(p.input.Value(), filepath.Separator)
}

// splitPathInput splits the input into a resolved directory and a filter query.
// The directory part is everything up to and including the last separator.
// Relative paths are resolved against the program's working directory; a
// leading "~" is expanded to the configured home directory.
func (p *Model) splitPathInput() (resolvedDir, query string) {
	value := p.input.Value()
	lastSep := strings.LastIndexByte(value, filepath.Separator)
	if lastSep < 0 {
		return p.dir, value
	}
	dirPart := value[:lastSep]
	query = value[lastSep+1:]
	if dirPart == "" {
		dirPart = string(filepath.Separator)
	}
	if p.home != "" && (dirPart == "~" || strings.HasPrefix(dirPart, "~"+string(filepath.Separator))) {
		rest := strings.TrimPrefix(dirPart, "~")
		rest = strings.TrimPrefix(rest, string(filepath.Separator))
		resolvedDir = filepath.Clean(filepath.Join(p.home, rest))
		return
	}
	if filepath.IsAbs(dirPart) {
		resolvedDir = filepath.Clean(dirPart)[1:]
		if resolvedDir == "" {
			resolvedDir = "."
		}
	} else {
		resolvedDir = filepath.Clean(filepath.Join(p.wd, dirPart))
	}
	return
}

// rawDirPart returns the directory portion of the input (up to and including
// the last separator), preserving the user's original text.
func (p *Model) rawDirPart() string {
	value := p.input.Value()
	lastSep := strings.LastIndexByte(value, filepath.Separator)
	if lastSep < 0 {
		return ""
	}
	return value[:lastSep+1]
}

// filter applies case-insensitive subsequence matching.
// In path mode it filters pathEntries; otherwise it filters allEntries.
func (p *Model) filter() {
	var source []fs.DirEntry
	var query string

	if p.isPathMode() {
		var resolvedDir string
		resolvedDir, query = p.splitPathInput()
		// Use pathEntries if they match the current resolved dir.
		if p.pathDir == resolvedDir {
			source = p.pathEntries
		}
		// Prepend ".." if not at root.
		if filepath.Dir(resolvedDir) != resolvedDir {
			source = append([]fs.DirEntry{parentDirEntry{}}, source...)
		}
	} else {
		source = p.allEntries
		query = p.input.Value()
		// Clear stale path state.
		p.pathDir = ""
		p.pathEntries = nil
	}

	queryLower := strings.ToLower(query)
	p.filtered = nil
	for _, e := range source {
		if queryLower == "" || subsequenceMatch(strings.ToLower(e.Name()), queryLower) {
			p.filtered = append(p.filtered, e)
		}
	}

	// Clamp cursor.
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = max(0, len(p.filtered)-1)
	}
	// Reset viewport to include cursor.
	p.minIdx = 0
	p.maxIdx = p.height - 1
	if p.cursor > p.maxIdx {
		p.minIdx = p.cursor - p.height + 1
		p.maxIdx = p.cursor
	}
}

// handleInputChange re-filters and, if in path mode with a new directory,
// triggers an async directory read.
func (p *Model) handleInputChange() tea.Cmd {
	p.filter()
	if p.isPathMode() {
		resolvedDir, _ := p.splitPathInput()
		if resolvedDir != p.pathDir {
			return p.pathReadDir(resolvedDir)
		}
	}
	return nil
}

// subsequenceMatch returns true if all characters in needle appear in haystack
// in order (case-insensitive matching should be done by caller).
func subsequenceMatch(haystack, needle string) bool {
	hi := 0
	for _, c := range needle {
		found := false
		for hi < len(haystack) {
			r, size := utf8.DecodeRuneInString(haystack[hi:])
			hi += size
			if r == c {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// longestCommonPrefix returns the longest common prefix of the entry names,
// compared case-insensitively but preserving the case of the first entry.
func longestCommonPrefix(entries []fs.DirEntry) string {
	if len(entries) == 0 {
		return ""
	}
	prefix := []rune(entries[0].Name())
	for _, e := range entries[1:] {
		name := []rune(strings.ToLower(e.Name()))
		prefixLower := []rune(strings.ToLower(string(prefix)))
		n := min(len(prefixLower), len(name))
		i := 0
		for i < n && prefixLower[i] == name[i] {
			i++
		}
		prefix = prefix[:i]
		if len(prefix) == 0 {
			break
		}
	}
	return string(prefix)
}

// canSelect checks if a filename has an allowed extension.
func (p *Model) canSelect(name string) bool {
	if len(p.allowedTypes) == 0 {
		return true
	}
	return slices.Contains(p.allowedTypes, strings.ToLower(filepath.Ext(name)))
}

// Update handles messages and returns the updated model and any commands.
func (p Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case readDirMsg:
		// Prepend ".." entry when not at filesystem root.
		if filepath.Dir(p.dir) != p.dir {
			p.allEntries = append([]fs.DirEntry{parentDirEntry{}}, msg.entries...)
		} else {
			p.allEntries = msg.entries
		}
		p.filter()
		return p, nil

	case pathReadDirMsg:
		p.pathDir = msg.dir
		p.pathEntries = msg.entries
		p.filter()
		return p, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, p.keyMap.Up):
			p.cursor--
			if p.cursor < 0 {
				p.cursor = 0
			}
			if p.cursor < p.minIdx {
				p.minIdx = p.cursor
				p.maxIdx = p.minIdx + p.height - 1
			}
			return p, nil

		case key.Matches(msg, p.keyMap.Down):
			p.cursor++
			if p.cursor >= len(p.filtered) {
				p.cursor = len(p.filtered) - 1
			}
			if p.cursor < 0 {
				p.cursor = 0
			}
			if p.cursor > p.maxIdx {
				p.maxIdx = p.cursor
				p.minIdx = p.maxIdx - p.height + 1
			}
			return p, nil

		case key.Matches(msg, p.keyMap.PageUp):
			p.cursor -= p.height
			if p.cursor < 0 {
				p.cursor = 0
			}
			p.minIdx -= p.height
			if p.minIdx < 0 {
				p.minIdx = 0
			}
			p.maxIdx = p.minIdx + p.height - 1
			return p, nil

		case key.Matches(msg, p.keyMap.PageDown):
			p.cursor += p.height
			if p.cursor >= len(p.filtered) {
				p.cursor = max(0, len(p.filtered)-1)
			}
			p.maxIdx += p.height
			if p.maxIdx >= len(p.filtered) {
				p.maxIdx = max(0, len(p.filtered)-1)
			}
			p.minIdx = p.maxIdx - p.height + 1
			if p.minIdx < 0 {
				p.minIdx = 0
			}
			return p, nil

		case key.Matches(msg, p.keyMap.SelectDir):
			if !p.allowDir || len(p.filtered) == 0 || p.cursor < 0 {
				return p, nil
			}
			entry := p.filtered[p.cursor]
			if !entry.IsDir() {
				return p, nil
			}
			if p.isPathMode() {
				resolvedDir, _ := p.splitPathInput()
				if entry.Name() == ".." {
					p.selected = filepath.Dir(resolvedDir)
				} else {
					p.selected = filepath.Join(resolvedDir, entry.Name())
				}
			} else {
				if entry.Name() == ".." {
					p.selected = filepath.Dir(p.dir)
				} else {
					p.selected = filepath.Join(p.dir, entry.Name())
				}
			}
			return p, nil

		case key.Matches(msg, p.keyMap.Select):
			if len(p.filtered) == 0 || p.cursor < 0 {
				return p, nil
			}
			entry := p.filtered[p.cursor]

			if p.isPathMode() {
				resolvedDir, _ := p.splitPathInput()
				if entry.Name() == ".." || entry.IsDir() {
					// Update the input text to navigate into the directory.
					raw := p.rawDirPart()
					if entry.Name() == ".." {
						// Remove the last path element: "subdir/" → "", "a/b/" → "a/".
						trimmed := strings.TrimSuffix(raw, string(filepath.Separator))
						if i := strings.LastIndexByte(trimmed, filepath.Separator); i >= 0 {
							p.input.SetValue(trimmed[:i+1])
						} else {
							p.input.SetValue("")
						}
					} else {
						p.input.SetValue(raw + entry.Name() + string(filepath.Separator))
					}
					p.input.CursorEnd()
					return p, p.handleInputChange()
				}
				// File selection in path mode.
				p.selected = filepath.Join(resolvedDir, entry.Name())
				return p, nil
			}

			// Normal mode.
			if entry.Name() == ".." {
				return p, p.navigateBack()
			}
			if entry.IsDir() {
				// Push current state.
				p.navStack = append(p.navStack, navState{
					dir:    p.dir,
					cursor: p.cursor,
					minIdx: p.minIdx,
					maxIdx: p.maxIdx,
				})
				// Navigate into directory.
				p.dir = filepath.Join(p.dir, entry.Name())
				p.cursor = 0
				p.minIdx = 0
				p.maxIdx = p.height - 1
				p.input.SetValue("")
				return p, p.readDir()
			}
			// File: check if allowed.
			if p.canSelect(entry.Name()) {
				p.selected = filepath.Join(p.dir, entry.Name())
			}
			return p, nil

		case key.Matches(msg, p.keyMap.Complete):
			// Tab completion: complete to the longest common prefix of matches.
			// Only consider entries whose names prefix-match the current query
			// (subsequence-only matches are excluded so the LCP is meaningful).
			var query string
			if p.isPathMode() {
				_, query = p.splitPathInput()
			} else {
				query = p.input.Value()
			}
			queryLower := strings.ToLower(query)
			var candidates []fs.DirEntry
			for _, e := range p.filtered {
				if e.Name() != ".." && strings.HasPrefix(strings.ToLower(e.Name()), queryLower) {
					candidates = append(candidates, e)
				}
			}
			if len(candidates) == 0 {
				return p, nil
			}
			lcp := longestCommonPrefix(candidates)
			if len(candidates) == 1 {
				if candidates[0].IsDir() {
					lcp += string(filepath.Separator)
				}
			}
			newVal := lcp
			if p.isPathMode() {
				newVal = p.rawDirPart() + lcp
			}
			if newVal == p.input.Value() {
				return p, nil
			}
			p.input.SetValue(newVal)
			p.input.CursorEnd()
			return p, p.handleInputChange()

		default:
			// Forward to textinput.
			prevValue := p.input.Value()
			var cmd tea.Cmd
			p.input, cmd = p.input.Update(msg)
			if p.input.Value() != prevValue {
				if pathCmd := p.handleInputChange(); pathCmd != nil {
					cmd = tea.Batch(cmd, pathCmd)
				}
			}
			return p, cmd
		}
	}

	// Forward other messages (e.g. cursor blink) to textinput.
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd
}

// navigateBack goes to the parent directory, restoring saved state.
func (p *Model) navigateBack() tea.Cmd {
	parent := filepath.Dir(p.dir)
	if parent == p.dir {
		return nil // already at root
	}
	p.dir = parent
	if n := len(p.navStack); n > 0 {
		prev := p.navStack[n-1]
		p.navStack = p.navStack[:n-1]
		p.dir = prev.dir
		p.cursor = prev.cursor
		p.minIdx = prev.minIdx
		p.maxIdx = prev.maxIdx
	} else {
		p.cursor = 0
		p.minIdx = 0
		p.maxIdx = p.height - 1
	}
	p.input.SetValue("")
	return p.readDir()
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

// View renders the picker as a string.
func (p Model) View() string {
	var s strings.Builder

	// Text input.
	s.WriteString(p.input.View())
	s.WriteRune('\n')

	if len(p.filtered) == 0 {
		s.WriteString(p.styles.Empty.Render("  No matching files."))
		s.WriteRune('\n')
	} else {
		for i, entry := range p.filtered {
			if i < p.minIdx || i > p.maxIdx {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			name := entry.Name()
			isDir := entry.IsDir()
			size := ""
			if !isDir {
				size = strings.Replace(humanize.Bytes(uint64(info.Size())), " ", "", 1) //nolint:gosec
			}
			disabled := !p.canSelect(name) && !isDir

			sizeCol := p.styles.FileSize.GetWidth()
			mark := info.Mode().String()
			if p.marker != nil {
				mark = p.marker(entry)
			}
			if i == p.cursor {
				selected := " " + mark
				if isDir {
					selected += fmt.Sprintf("%"+strconv.Itoa(sizeCol)+"s", "")
				} else {
					selected += fmt.Sprintf("%"+strconv.Itoa(sizeCol)+"s", size)
				}
				selected += " " + name

				if disabled {
					s.WriteString(p.styles.DisabledCursor.Render(">") + p.styles.DisabledSelected.Render(selected))
				} else {
					s.WriteString(p.styles.Cursor.Render(">") + p.styles.Selected.Render(selected))
				}
			} else {
				style := p.styles.File
				if isDir {
					style = p.styles.Directory
				} else if disabled {
					style = p.styles.Disabled
				}

				s.WriteString(p.styles.Cursor.Render(" "))
				s.WriteString(" " + p.styles.Permission.Render(mark))
				if isDir {
					s.WriteString(p.styles.FileSize.Render(""))
				} else {
					s.WriteString(p.styles.FileSize.Render(size))
				}
				s.WriteString(" " + style.Render(name))
			}
			s.WriteRune('\n')
		}
	}

	// Pad remaining height.
	rendered := lipgloss.Height(s.String())
	for i := rendered; i <= p.height+1; i++ { // +1 for the input line
		s.WriteRune('\n')
	}

	return s.String()
}

// SetHeight sets the visible file list height (not counting the input line).
func (p *Model) SetHeight(h int) {
	p.height = h
	if p.maxIdx > p.minIdx+p.height-1 {
		p.maxIdx = p.minIdx + p.height - 1
	}
}

// SetWidth sets the width of the picker.
func (p *Model) SetWidth(w int) {
	p.width = w
	p.input.SetWidth(w - lipgloss.Width(p.input.Prompt) - 1)
}
