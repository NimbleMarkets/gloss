package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/document"
)

// countFS counts what is opened, as the os would be asked to, and describes
// without opening, as os.DirFS does.
type countFS struct {
	fs.ReadDirFS
	mu    sync.Mutex
	opens []string
}

func (c *countFS) Open(name string) (fs.File, error) {
	c.mu.Lock()
	c.opens = append(c.opens, name)
	c.mu.Unlock()
	return c.ReadDirFS.Open(name)
}

func (c *countFS) Stat(name string) (fs.FileInfo, error) { return fs.Stat(c.ReadDirFS, name) }

func (c *countFS) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.opens)
}

func readLogOver(t *testing.T, dir string) (*readLog, *countFS) {
	t.Helper()
	root := os.DirFS(string(filepath.Separator)).(fs.ReadDirFS)
	c := &countFS{ReadDirFS: root}
	return &readLog{ReadDirFS: c, allowed: document.Extensions, text: true}, c
}

func relative(dir string) string { return filepath.ToSlash(dir)[1:] }

// A folder is listed without looking inside what has no extension: that is
// done as a file is drawn, for the rows that are, and once.
func TestListingAFolderOpensNothing(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 60; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%02d", i)), []byte("some text\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "blob"), []byte("\x00\x01\x02binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	reads, c := readLogOver(t, dir)
	entries, err := reads.ReadDir(relative(dir))
	if err != nil || len(entries) != 61 {
		t.Fatalf("ReadDir: %d entries, %v", len(entries), err)
	}
	if n := c.count(); n != 0 {
		t.Fatalf("listing the folder opened %d files; sniffing is for the rows drawn", n)
	}
	var blob, text fs.DirEntry
	for _, e := range entries {
		switch e.Name() {
		case "blob":
			blob = e
		case "file07":
			text = e
		}
	}
	if mark(text) != "📃" || mark(blob) != "  " {
		t.Errorf("marks %q, %q: text by content, and not", mark(text), mark(blob))
	}
	if n := c.count(); n != 2 {
		t.Errorf("drawing two rows opened %d files, want 2", n)
	}
	_ = mark(text)
	_ = reads.selectable(text)
	if n := c.count(); n != 2 {
		t.Errorf("looking again opened more: %d", n)
	}
	if !reads.selectable(text) || reads.selectable(blob) {
		t.Error("text may be chosen, binary may not")
	}
	// Hiding what cannot be shown needs to know of every file at once, and says so.
	reads.hide = true
	c.opens = nil
	hidden, _ := reads.ReadDir(relative(dir))
	if len(hidden) != 60 || c.count() != 61 {
		t.Errorf("hiding: %d entries, %d opens; want 60 and 61", len(hidden), c.count())
	}
}

// A link is described, not opened: opening a link to a FIFO would wait for ever.
func TestLinksAreStatedNotOpened(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "target"), []byte("text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, to := range map[string]string{"tofolder": "folder", "tofile": "target", "broken": "nowhere"} {
		if err := os.Symlink(filepath.Join(dir, to), filepath.Join(dir, name)); err != nil {
			t.Skip("cannot make a symlink here:", err)
		}
	}
	reads, c := readLogOver(t, dir)
	entries, _ := reads.ReadDir(relative(dir))
	byName := map[string]fs.DirEntry{}
	for _, e := range entries {
		byName[e.Name()] = e
	}
	if !byName["tofolder"].IsDir() || byName["tofile"].IsDir() || byName["broken"].IsDir() {
		t.Errorf("links to a folder are folders, the rest are not: %v %v %v", byName["tofolder"].IsDir(), byName["tofile"].IsDir(), byName["broken"].IsDir())
	}
	if mark(byName["tofile"]) != "📃" {
		t.Errorf("a link to a text file is text: %q", mark(byName["tofile"]))
	}
	if mark(byName["broken"]) != "  " || reads.selectable(byName["broken"]) {
		t.Error("a link to nothing is nothing to choose")
	}
	// Only the link to the text file was ever opened, to read its start.
	for _, name := range c.opens {
		if filepath.Base(name) != "tofile" {
			t.Errorf("%s was opened", name)
		}
	}
}
