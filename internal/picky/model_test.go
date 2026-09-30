package picky

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// memFS is an in-memory FS for testing.
type memFS struct {
	wd    string
	files map[string][]byte
}

func newMemFS() *memFS {
	return &memFS{wd: ".", files: make(map[string][]byte)}
}

func (m *memFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	prefix := strings.TrimSuffix(name, "/") + "/"

	seen := map[string]bool{}
	var entries []fs.DirEntry
	for p, data := range m.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := p[len(prefix):]
		if rest == "" {
			continue
		}
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			dirName := rest[:i]
			if !seen[dirName] {
				seen[dirName] = true
				entries = append(entries, &memDirEntry{name: dirName, isDir: true})
			}
		} else {
			if !seen[rest] {
				seen[rest] = true
				entries = append(entries, &memDirEntry{name: rest, size: int64(len(data))})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	return entries, nil
}

type memDirEntry struct {
	name  string
	isDir bool
	size  int64
}

func (e *memDirEntry) Name() string { return e.name }
func (e *memDirEntry) IsDir() bool  { return e.isDir }
func (e *memDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}

func (e *memDirEntry) Info() (fs.FileInfo, error) {
	return &memFileInfo{name: e.name, isDir: e.isDir, size: e.size}, nil
}

type memFileInfo struct {
	name  string
	isDir bool
	size  int64
}

func (fi *memFileInfo) Name() string { return path.Base(fi.name) }
func (fi *memFileInfo) Size() int64  { return fi.size }
func (fi *memFileInfo) Mode() fs.FileMode {
	if fi.isDir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (fi *memFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *memFileInfo) IsDir() bool        { return fi.isDir }
func (fi *memFileInfo) Sys() any           { return nil }

// keyMsg constructs a tea.KeyPressMsg that produces the given String() value.
func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	default:
		if len(s) == 1 {
			return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
		}
		return tea.KeyPressMsg{Code: -1, Text: s}
	}
}

func testPicker() (Model, *memFS) {
	fs := newMemFS()
	fs.wd = "testdir"
	fs.files["testdir/readme.md"] = []byte("# Hello")
	fs.files["testdir/notes.md"] = []byte("# Notes")
	fs.files["testdir/image.png"] = []byte("PNG")
	fs.files["testdir/subdir/nested.md"] = []byte("# Nested")
	fs.files["testdir/another.txt"] = []byte("text")

	p := New("testdir", WithFS(fs), WithAllowedTypes([]string{".md"}))
	return p, fs
}

func applyReadDir(p Model) Model {
	cmd := p.readDir()
	msg := cmd()
	p, _ = p.Update(msg)
	return p
}

func TestPicker_ReadDir(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	if len(p.allEntries) == 0 {
		t.Fatal("expected entries after readDir")
	}

	// ".." should be first, then directories, then files.
	if p.allEntries[0].Name() != ".." {
		t.Errorf("expected first entry to be '..', got %q", p.allEntries[0].Name())
	}
	if p.allEntries[1].Name() != "subdir" {
		t.Errorf("expected second entry to be 'subdir', got %q", p.allEntries[1].Name())
	}
}

func TestPicker_FilterBasic(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	// Type "re" to filter.
	p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: "r"})
	p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: "e"})

	found := false
	for _, e := range p.filtered {
		if e.Name() == "readme.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'readme.md' in filtered results after typing 're'")
	}
}

func TestPicker_FilterSubsequence(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for _, ch := range "nmd" {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}

	found := false
	for _, e := range p.filtered {
		if e.Name() == "notes.md" {
			found = true
		}
	}
	if !found {
		names := make([]string, len(p.filtered))
		for i, e := range p.filtered {
			names[i] = e.Name()
		}
		t.Errorf("expected 'notes.md' in filtered results, got %v", names)
	}
}

func TestPicker_EmptyFilterShowsAll(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	total := len(p.filtered)
	if total != len(p.allEntries) {
		t.Errorf("empty filter: filtered=%d, allEntries=%d", len(p.filtered), len(p.allEntries))
	}
}

func TestPicker_CursorNavigation(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	if p.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", p.cursor)
	}

	p, _ = p.Update(keyMsg("down"))
	if p.cursor != 1 {
		t.Errorf("expected cursor=1 after down, got %d", p.cursor)
	}

	p, _ = p.Update(keyMsg("up"))
	if p.cursor != 0 {
		t.Errorf("expected cursor=0 after up, got %d", p.cursor)
	}

	p, _ = p.Update(keyMsg("up"))
	if p.cursor != 0 {
		t.Errorf("expected cursor=0 at top boundary, got %d", p.cursor)
	}
}

func TestPicker_CursorClampOnFilter(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for i := 0; i < len(p.filtered); i++ {
		p, _ = p.Update(keyMsg("down"))
	}
	lastCursor := p.cursor

	for _, ch := range "readme" {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}

	if p.cursor > len(p.filtered)-1 {
		t.Errorf("cursor=%d exceeds filtered length=%d (was %d)", p.cursor, len(p.filtered), lastCursor)
	}
}

func TestPicker_EnterDirectory(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	if p.filtered[0].Name() != ".." {
		t.Fatalf("expected first entry to be '..', got %q", p.filtered[0].Name())
	}
	p, _ = p.Update(keyMsg("down"))
	if p.filtered[p.cursor].Name() != "subdir" {
		t.Fatalf("expected cursor on 'subdir', got %q", p.filtered[p.cursor].Name())
	}

	p, cmd := p.Update(keyMsg("enter"))
	if p.dir != "testdir/subdir" {
		t.Errorf("expected dir='/testdir/subdir', got %q", p.dir)
	}
	if len(p.navStack) != 1 {
		t.Errorf("expected dirStack length=1, got %d", len(p.navStack))
	}
	if p.input.Value() != "" {
		t.Error("expected input cleared after entering directory")
	}

	if cmd != nil {
		msg := cmd()
		p, _ = p.Update(msg)
	}

	found := false
	for _, e := range p.filtered {
		if e.Name() == "nested.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'nested.md' in subdir listing")
	}
}

func TestPicker_NavigateBack(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	p, _ = p.Update(keyMsg("down"))
	p, cmd := p.Update(keyMsg("enter"))
	if cmd != nil {
		msg := cmd()
		p, _ = p.Update(msg)
	}
	if p.dir != "testdir/subdir" {
		t.Fatalf("expected dir='/testdir/subdir', got %q", p.dir)
	}

	p, cmd = p.Update(keyMsg("enter"))
	if p.dir != "testdir" {
		t.Errorf("expected dir='/testdir' after selecting '..', got %q", p.dir)
	}
	if len(p.navStack) != 0 {
		t.Errorf("expected empty dirStack after back, got length %d", len(p.navStack))
	}

	if cmd != nil {
		msg := cmd()
		p, _ = p.Update(msg)
	}

	if len(p.filtered) == 0 {
		t.Error("expected entries after navigating back")
	}
}

func TestPicker_SelectFile(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for i := 0; i < len(p.filtered); i++ {
		if p.filtered[p.cursor].Name() == "readme.md" {
			break
		}
		p, _ = p.Update(keyMsg("down"))
	}

	if p.filtered[p.cursor].Name() != "readme.md" {
		t.Fatalf("expected cursor on 'readme.md', got %q", p.filtered[p.cursor].Name())
	}

	p, _ = p.Update(keyMsg("enter"))
	path := p.Selected()
	if path == "" {
		t.Error("expected selection after enter on allowed file")
	}
	if path != "testdir/readme.md" {
		t.Errorf("expected path='/testdir/readme.md', got %q", path)
	}
}

func TestPicker_DisabledFileNotSelected(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for i := 0; i < len(p.filtered); i++ {
		if p.filtered[p.cursor].Name() == "image.png" {
			break
		}
		p, _ = p.Update(keyMsg("down"))
	}

	if p.filtered[p.cursor].Name() != "image.png" {
		t.Skip("image.png not found in listing")
	}

	p, _ = p.Update(keyMsg("enter"))
	didSelect := p.Selected() != ""
	if didSelect {
		t.Error("expected no selection on disabled file type")
	}
}

func TestPicker_FilterClearsOnDirEnter(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for _, ch := range "sub" {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}
	if p.filtered[p.cursor].Name() != "subdir" {
		t.Fatalf("expected cursor on 'subdir', got %q", p.filtered[p.cursor].Name())
	}

	p, _ = p.Update(keyMsg("enter"))
	if p.input.Value() != "" {
		t.Error("expected filter cleared after entering directory")
	}
}

func TestPicker_FilterClearsOnNavigateBack(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	p, _ = p.Update(keyMsg("down"))
	p, cmd := p.Update(keyMsg("enter"))
	if cmd != nil {
		msg := cmd()
		p, _ = p.Update(msg)
	}

	p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: "."})
	p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: "."})
	if p.filtered[0].Name() != ".." {
		t.Fatalf("expected '..' as first filtered entry, got %q", p.filtered[0].Name())
	}

	p, _ = p.Update(keyMsg("enter"))

	if p.input.Value() != "" {
		t.Error("expected filter cleared after navigating back via '..'")
	}
}

func TestPicker_NoDuplicatesAfterRefilter(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for _, ch := range "readme" {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}
	if len(p.filtered) != 1 {
		t.Fatalf("expected 1 match for 'readme', got %d", len(p.filtered))
	}

	for i := 0; i < 5; i++ {
		p, _ = p.Update(keyMsg("backspace"))
	}

	seen := map[string]int{}
	for _, e := range p.filtered {
		seen[e.Name()]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("entry %q appears %d times in filtered results (expected 1)", name, count)
		}
	}
}

func TestPicker_SetHeight(t *testing.T) {
	p, _ := testPicker()
	p.SetHeight(5)
	if p.height != 5 {
		t.Errorf("expected height=5, got %d", p.height)
	}
}

func TestPicker_SetWidth(t *testing.T) {
	p, _ := testPicker()
	p.SetWidth(80)
	if p.width != 80 {
		t.Errorf("expected width=80, got %d", p.width)
	}
}

func TestPicker_View(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)
	p.SetHeight(20)
	p.SetWidth(80)

	view := p.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	if len(view) < 10 {
		t.Error("expected view to have meaningful content")
	}
}

func TestPicker_EmptyDirectory(t *testing.T) {
	fs := newMemFS()
	fs.wd = "/empty"
	p := New("/empty", WithFS(fs), WithAllowedTypes([]string{".md"}))
	p = applyReadDir(p)

	if len(p.filtered) != 1 || p.filtered[0].Name() != ".." {
		t.Errorf("expected only '..' entry, got %d entries", len(p.filtered))
	}

	view := p.View()
	if view == "" {
		t.Error("expected non-empty view even for empty directory")
	}
}

func TestPicker_RootHasNoParentEntry(t *testing.T) {
	fs := newMemFS()
	fs.wd = "."
	fs.files["file.md"] = []byte("# Root file")
	p := New(".", WithFS(fs), WithAllowedTypes([]string{".md"}))
	p = applyReadDir(p)

	for _, e := range p.filtered {
		if e.Name() == ".." {
			t.Error("root directory should not have '..' entry")
		}
	}
}

func TestSubsequenceMatch(t *testing.T) {
	tests := []struct {
		haystack string
		needle   string
		want     bool
	}{
		{"readme.md", "rmd", true},
		{"readme.md", "readme", true},
		{"readme.md", "xyz", false},
		{"notes.md", "nmd", true},
		{"notes.md", "nm", true},
		{"image.png", "img", true},
		{"image.png", "ipg", true},
		{"image.png", "ipa", false},
		{"", "a", false},
		{"abc", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		got := subsequenceMatch(tt.haystack, tt.needle)
		if got != tt.want {
			t.Errorf("subsequenceMatch(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
		}
	}
}

func TestPicker_PageDown(t *testing.T) {
	fs := newMemFS()
	fs.wd = "bigdir"
	for i := 0; i < 30; i++ {
		name := "bigdir/" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + ".md"
		fs.files[name] = []byte("content")
	}

	p := New("bigdir", WithFS(fs), WithAllowedTypes([]string{".md"}))
	p.SetHeight(10)
	p = applyReadDir(p)

	if len(p.filtered) < 20 {
		t.Fatalf("expected at least 20 entries, got %d", len(p.filtered))
	}

	p, _ = p.Update(keyMsg("pgdown"))
	if p.cursor < 5 {
		t.Errorf("expected cursor to advance on pgdown, got %d", p.cursor)
	}
}

// --- Path mode tests ---

func applyPathInput(p Model, text string) Model {
	for _, ch := range text {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
		if p.isPathMode() {
			resolvedDir, _ := p.splitPathInput()
			if resolvedDir != p.pathDir {
				cmd := p.pathReadDir(resolvedDir)
				msg := cmd()
				p, _ = p.Update(msg)
			}
		}
	}
	return p
}

func TestPicker_PathMode_ShowsDirContents(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	p = applyPathInput(p, "subdir/")

	names := make([]string, len(p.filtered))
	for i, e := range p.filtered {
		names[i] = e.Name()
	}
	if len(p.filtered) != 2 {
		t.Fatalf("expected 2 entries (.. and nested.md), got %d: %v", len(p.filtered), names)
	}
	if p.filtered[0].Name() != ".." {
		t.Errorf("expected first entry '..', got %q", p.filtered[0].Name())
	}
	if p.filtered[1].Name() != "nested.md" {
		t.Errorf("expected second entry 'nested.md', got %q", p.filtered[1].Name())
	}
}

func TestPicker_PathMode_FilterInDir(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	p = applyPathInput(p, "subdir/ne")

	if len(p.filtered) != 1 {
		names := make([]string, len(p.filtered))
		for i, e := range p.filtered {
			names[i] = e.Name()
		}
		t.Fatalf("expected 1 match for 'ne' in subdir, got %d: %v", len(p.filtered), names)
	}
	if p.filtered[0].Name() != "nested.md" {
		t.Errorf("expected 'nested.md', got %q", p.filtered[0].Name())
	}
}

func TestPicker_PathMode_SelectFile(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	p = applyPathInput(p, "subdir/")

	for p.cursor < len(p.filtered)-1 && p.filtered[p.cursor].Name() != "nested.md" {
		p, _ = p.Update(keyMsg("down"))
	}
	if p.filtered[p.cursor].Name() != "nested.md" {
		t.Fatalf("expected cursor on 'nested.md', got %q", p.filtered[p.cursor].Name())
	}

	p, _ = p.Update(keyMsg("enter"))
	path := p.Selected()
	if path == "" {
		t.Fatal("expected selection in path mode")
	}
	if path != "testdir/subdir/nested.md" {
		t.Errorf("selected path = %q, want %q", path, "testdir/subdir/nested.md")
	}
}

func TestPicker_PathMode_EnterOnDirUpdatesInput(t *testing.T) {
	p, mfs := testPicker()
	mfs.files["testdir/subdir/deep/file.md"] = []byte("deep")
	p = applyReadDir(p)

	p = applyPathInput(p, "subdir/")

	for p.cursor < len(p.filtered)-1 && p.filtered[p.cursor].Name() != "deep" {
		p, _ = p.Update(keyMsg("down"))
	}
	if p.filtered[p.cursor].Name() != "deep" {
		t.Fatalf("expected cursor on 'deep', got %q", p.filtered[p.cursor].Name())
	}

	p, cmd := p.Update(keyMsg("enter"))
	if p.input.Value() != "subdir/deep/" {
		t.Errorf("expected input='subdir/deep/', got %q", p.input.Value())
	}

	if cmd != nil {
		msg := cmd()
		p, _ = p.Update(msg)
	}

	found := false
	for _, e := range p.filtered {
		if e.Name() == "file.md" {
			found = true
		}
	}
	if !found {
		names := make([]string, len(p.filtered))
		for i, e := range p.filtered {
			names[i] = e.Name()
		}
		t.Errorf("expected 'file.md' in deep dir listing, got %v", names)
	}
}

func TestPicker_PathMode_ParentUpdatesInput(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	p = applyPathInput(p, "subdir/")

	if p.filtered[0].Name() != ".." {
		t.Fatalf("expected first entry '..', got %q", p.filtered[0].Name())
	}
	p.cursor = 0

	p, _ = p.Update(keyMsg("enter"))
	if p.input.Value() != "" {
		t.Errorf("expected input='', got %q", p.input.Value())
	}
}

func TestPicker_PathMode_AbsolutePath(t *testing.T) {
	p, mfs := testPicker()
	mfs.files["other/doc.md"] = []byte("# Other")
	p = applyReadDir(p)

	p = applyPathInput(p, "/other/")

	found := false
	for _, e := range p.filtered {
		if e.Name() == "doc.md" {
			found = true
		}
	}
	if !found {
		names := make([]string, len(p.filtered))
		for i, e := range p.filtered {
			names[i] = e.Name()
		}
		t.Fatalf("expected 'doc.md' in /other/ listing, got %v", names)
	}
}

func TestPicker_PathMode_TildeExpansion(t *testing.T) {
	fs := newMemFS()
	fs.wd = "testdir"
	fs.files["testdir/readme.md"] = []byte("# Hello")
	fs.files["home/user/doc.md"] = []byte("# Doc")
	fs.files["home/user/notes.md"] = []byte("# Notes")
	fs.files["home/user/sub/nested.md"] = []byte("# Nested")

	p := New("testdir", WithFS(fs), WithHome("home/user"), WithAllowedTypes([]string{".md"}))
	p = applyReadDir(p)

	p = applyPathInput(p, "~/")

	names := make([]string, len(p.filtered))
	for i, e := range p.filtered {
		names[i] = e.Name()
	}
	hasDoc, hasNotes, hasSub := false, false, false
	for _, n := range names {
		switch n {
		case "doc.md":
			hasDoc = true
		case "notes.md":
			hasNotes = true
		case "sub":
			hasSub = true
		}
	}
	if !hasDoc || !hasNotes || !hasSub {
		t.Errorf("expected home contents (doc.md, notes.md, sub) in ~/ listing, got %v", names)
	}

	// Selecting a file under ~/ resolves to the fs-relative home path.
	for p.cursor < len(p.filtered)-1 && p.filtered[p.cursor].Name() != "doc.md" {
		p, _ = p.Update(keyMsg("down"))
	}
	if p.filtered[p.cursor].Name() != "doc.md" {
		t.Fatalf("expected cursor on 'doc.md', got %q", p.filtered[p.cursor].Name())
	}
	p, _ = p.Update(keyMsg("enter"))
	if p.Selected() != "home/user/doc.md" {
		t.Errorf("expected selection 'home/user/doc.md', got %q", p.Selected())
	}
}

func TestPicker_PathMode_TildeWithSubdir(t *testing.T) {
	fs := newMemFS()
	fs.wd = "testdir"
	fs.files["testdir/readme.md"] = []byte("# Hello")
	fs.files["home/user/sub/nested.md"] = []byte("# Nested")

	p := New("testdir", WithFS(fs), WithHome("home/user"), WithAllowedTypes([]string{".md"}))
	p = applyReadDir(p)

	p = applyPathInput(p, "~/sub/")

	found := false
	for _, e := range p.filtered {
		if e.Name() == "nested.md" {
			found = true
		}
	}
	if !found {
		names := make([]string, len(p.filtered))
		for i, e := range p.filtered {
			names[i] = e.Name()
		}
		t.Errorf("expected 'nested.md' in ~/sub/ listing, got %v", names)
	}
}

func TestPicker_PathMode_TildeAbsoluteHome(t *testing.T) {
	fs := newMemFS()
	fs.wd = "testdir"
	fs.files["home/user/doc.md"] = []byte("# Doc")

	// Absolute home should be normalized to fs-relative form.
	p := New("testdir", WithFS(fs), WithHome("/home/user"), WithAllowedTypes([]string{".md"}))
	p = applyReadDir(p)

	p = applyPathInput(p, "~/")

	found := false
	for _, e := range p.filtered {
		if e.Name() == "doc.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'doc.md' after expanding absolute home")
	}
}

func TestPicker_PathMode_BadDirShowsEmpty(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	p = applyPathInput(p, "nonexistent/")

	if len(p.filtered) != 1 || p.filtered[0].Name() != ".." {
		names := make([]string, len(p.filtered))
		for i, e := range p.filtered {
			names[i] = e.Name()
		}
		t.Errorf("expected only '..' for bad dir, got %v", names)
	}
}

func TestPicker_TabCompletion_MultipleMatches(t *testing.T) {
	fs := newMemFS()
	fs.wd = "testdir"
	fs.files["testdir/readme.md"] = []byte("# Readme")
	fs.files["testdir/release.md"] = []byte("# Release")
	fs.files["testdir/other.md"] = []byte("# Other")

	p := New("testdir", WithFS(fs), WithAllowedTypes([]string{".md"}))
	p = applyReadDir(p)

	for _, ch := range "re" {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}

	p, _ = p.Update(keyMsg("tab"))
	got := p.input.Value()
	if got != "re" {
		t.Errorf("expected input='re' after tab, got %q", got)
	}
}

func TestPicker_TabCompletion_IgnoresSubsequenceOnlyMatches(t *testing.T) {
	fs := newMemFS()
	fs.wd = "testdir"
	fs.files["testdir/helppage.go"] = []byte("hp")
	fs.files["testdir/cache.go"] = []byte("c")
	fs.files["testdir/help.md"] = []byte("h")
	fs.files["testdir/scheme.go"] = []byte("s")

	p := New("testdir", WithFS(fs))
	p = applyReadDir(p)

	for _, ch := range "he" {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}

	hasSubseqMatch := false
	for _, e := range p.filtered {
		if e.Name() == "scheme.go" {
			hasSubseqMatch = true
		}
	}
	if !hasSubseqMatch {
		t.Fatal("expected 'scheme.go' as subsequence match in filtered results")
	}

	p, _ = p.Update(keyMsg("tab"))
	got := p.input.Value()
	if got != "help" {
		t.Errorf("expected input='help' after tab, got %q", got)
	}
}

func TestPicker_TabCompletion_SingleDirAppendsSep(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for _, ch := range "sub" {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}
	if len(p.filtered) != 1 || p.filtered[0].Name() != "subdir" {
		t.Fatalf("expected single match 'subdir', got %d entries", len(p.filtered))
	}

	p, _ = p.Update(keyMsg("tab"))
	got := p.input.Value()
	if got != "subdir/" {
		t.Errorf("expected input='subdir/', got %q", got)
	}
}

func TestPicker_TabCompletion_SingleFile(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for _, ch := range "read" {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}

	var nonParent []string
	for _, e := range p.filtered {
		if e.Name() != ".." {
			nonParent = append(nonParent, e.Name())
		}
	}
	if len(nonParent) != 1 || nonParent[0] != "readme.md" {
		t.Fatalf("expected single non-parent match 'readme.md', got %v", nonParent)
	}

	p, _ = p.Update(keyMsg("tab"))
	got := p.input.Value()
	if got != "readme.md" {
		t.Errorf("expected input='readme.md', got %q", got)
	}
}

func TestPicker_TabCompletion_NoMatches(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for _, ch := range "zzzzz" {
		p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}
	if len(p.filtered) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(p.filtered))
	}

	p, _ = p.Update(keyMsg("tab"))
	got := p.input.Value()
	if got != "zzzzz" {
		t.Errorf("expected input unchanged at 'zzzzz', got %q", got)
	}
}

func TestPicker_TabCompletion_PathMode(t *testing.T) {
	fs := newMemFS()
	fs.wd = "testdir"
	fs.files["testdir/subdir/alpha.md"] = []byte("a")
	fs.files["testdir/subdir/also.md"] = []byte("b")

	p := New("testdir", WithFS(fs), WithAllowedTypes([]string{".md"}))
	p = applyReadDir(p)

	p = applyPathInput(p, "subdir/")

	p, _ = p.Update(keyMsg("tab"))
	got := p.input.Value()
	if got != "subdir/al" {
		t.Errorf("expected input='subdir/al', got %q", got)
	}
}

func TestPicker_EnterDir_CursorAtEnd(t *testing.T) {
	p, mfs := testPicker()
	mfs.files["testdir/subdir/deep/file.md"] = []byte("deep")
	p = applyReadDir(p)

	p = applyPathInput(p, "subdir/")

	for p.cursor < len(p.filtered)-1 && p.filtered[p.cursor].Name() != "deep" {
		p, _ = p.Update(keyMsg("down"))
	}
	if p.filtered[p.cursor].Name() != "deep" {
		t.Fatalf("expected cursor on 'deep', got %q", p.filtered[p.cursor].Name())
	}

	p, _ = p.Update(keyMsg("enter"))
	val := p.input.Value()
	if val != "subdir/deep/" {
		t.Fatalf("expected input='subdir/deep/', got %q", val)
	}
	if p.input.Position() != len(val) {
		t.Errorf("expected cursor at position %d, got %d", len(val), p.input.Position())
	}
}

func TestPicker_PathMode_ExitReturnsToNormal(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)
	originalCount := len(p.filtered)

	p = applyPathInput(p, "subdir/")
	if len(p.filtered) == originalCount {
		t.Fatal("expected different entry count in path mode")
	}

	for i := 0; i < len("subdir/"); i++ {
		p, _ = p.Update(keyMsg("backspace"))
	}

	if len(p.filtered) != originalCount {
		t.Errorf("expected %d entries after exiting path mode, got %d", originalCount, len(p.filtered))
	}
}

func shiftEnterMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
}

func testDirPicker() (Model, *memFS) {
	fs := newMemFS()
	fs.wd = "testdir"
	fs.files["testdir/readme.md"] = []byte("# Hello")
	fs.files["testdir/notes.md"] = []byte("# Notes")
	fs.files["testdir/image.png"] = []byte("PNG")
	fs.files["testdir/subdir/nested.md"] = []byte("# Nested")
	fs.files["testdir/another.txt"] = []byte("text")

	p := New("testdir", WithFS(fs), WithAllowedTypes([]string{".md"}), WithAllowDir(true))
	return p, fs
}

func TestPicker_SelectDir_Normal(t *testing.T) {
	p, _ := testDirPicker()
	p = applyReadDir(p)

	// Move to "subdir".
	for p.cursor < len(p.filtered)-1 && p.filtered[p.cursor].Name() != "subdir" {
		p, _ = p.Update(keyMsg("down"))
	}
	if p.filtered[p.cursor].Name() != "subdir" {
		t.Fatalf("expected cursor on 'subdir', got %q", p.filtered[p.cursor].Name())
	}

	p, _ = p.Update(shiftEnterMsg())
	if p.Selected() != "testdir/subdir" {
		t.Errorf("expected selectedDir='/testdir/subdir', got %q", p.Selected())
	}
}

func TestPicker_SelectDir_Parent(t *testing.T) {
	p, _ := testDirPicker()
	p = applyReadDir(p)

	// ".." should be first entry.
	if p.filtered[0].Name() != ".." {
		t.Fatalf("expected first entry '..', got %q", p.filtered[0].Name())
	}
	p.cursor = 0

	p, _ = p.Update(shiftEnterMsg())
	if p.Selected() != "." {
		t.Errorf("expected selectedDir='.', got %q", p.Selected())
	}
}

func TestPicker_SelectDir_IgnoresFiles(t *testing.T) {
	p, _ := testDirPicker()
	p = applyReadDir(p)

	// Move to a file.
	for p.cursor < len(p.filtered)-1 && p.filtered[p.cursor].Name() != "readme.md" {
		p, _ = p.Update(keyMsg("down"))
	}
	if p.filtered[p.cursor].Name() != "readme.md" {
		t.Fatalf("expected cursor on 'readme.md', got %q", p.filtered[p.cursor].Name())
	}

	p, _ = p.Update(shiftEnterMsg())
	if p.Selected() != "" {
		t.Errorf("expected no dir selection on a file, got %q", p.Selected())
	}
}

func TestPicker_SelectDir_PathMode(t *testing.T) {
	p, mfs := testDirPicker()
	mfs.files["testdir/subdir/deep/file.md"] = []byte("deep")
	p = applyReadDir(p)

	p = applyPathInput(p, "subdir/")

	for p.cursor < len(p.filtered)-1 && p.filtered[p.cursor].Name() != "deep" {
		p, _ = p.Update(keyMsg("down"))
	}
	if p.filtered[p.cursor].Name() != "deep" {
		t.Fatalf("expected cursor on 'deep', got %q", p.filtered[p.cursor].Name())
	}

	p, _ = p.Update(shiftEnterMsg())
	if p.Selected() != "testdir/subdir/deep" {
		t.Errorf("expected selectedDir='/testdir/subdir/deep', got %q", p.Selected())
	}
}

func TestPicker_SelectDir_DisabledByDefault(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	for p.cursor < len(p.filtered)-1 && p.filtered[p.cursor].Name() != "subdir" {
		p, _ = p.Update(keyMsg("down"))
	}
	if p.filtered[p.cursor].Name() != "subdir" {
		t.Fatalf("expected cursor on 'subdir', got %q", p.filtered[p.cursor].Name())
	}

	p, _ = p.Update(shiftEnterMsg())
	if p.Selected() != "" {
		t.Errorf("expected no selection when allowDir is false, got %q", p.Selected())
	}
}

func TestPicker_SelectDir_PathMode_Parent(t *testing.T) {
	p, _ := testDirPicker()
	p = applyReadDir(p)

	p = applyPathInput(p, "subdir/")

	if p.filtered[0].Name() != ".." {
		t.Fatalf("expected first entry '..', got %q", p.filtered[0].Name())
	}
	p.cursor = 0

	p, _ = p.Update(shiftEnterMsg())
	if p.Selected() != "testdir" {
		t.Errorf("expected selectedDir='/testdir', got %q", p.Selected())
	}
}

func TestPicker_Selected(t *testing.T) {
	p, _ := testPicker()
	if p.Selected() != "" {
		t.Error("expected empty selection initially")
	}
	p.selected = "testdir/readme.md"
	if p.Selected() != "testdir/readme.md" {
		t.Errorf("expected '/testdir/readme.md', got %q", p.Selected())
	}
}

func TestPicker_ClearSelected(t *testing.T) {
	p, _ := testPicker()
	p.selected = "testdir/readme.md"
	p.ClearSelected()
	if p.Selected() != "" {
		t.Error("expected empty selection after ClearSelected")
	}
}

func TestPicker_FilterValueAndSetFilterValue(t *testing.T) {
	p, _ := testPicker()
	p = applyReadDir(p)

	if p.FilterValue() != "" {
		t.Error("expected empty filter value initially")
	}
	p.SetFilterValue("readme")
	if p.FilterValue() != "readme" {
		t.Errorf("expected 'readme', got %q", p.FilterValue())
	}
	// Setting filter value should trigger re-filter.
	if len(p.filtered) != 1 {
		t.Errorf("expected 1 filtered entry, got %d", len(p.filtered))
	}
}
