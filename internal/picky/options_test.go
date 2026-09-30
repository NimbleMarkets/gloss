package picky

import (
	"io/fs"
	"strings"
	"testing"
	"time"
)

// A file with a modification time, for sorting by date.
type datedEntry struct {
	memDirEntry
	at time.Time
}

func (e *datedEntry) Info() (fs.FileInfo, error) {
	return &datedInfo{memFileInfo{name: e.name, isDir: e.isDir, size: e.size}, e.at}, nil
}

type datedInfo struct {
	memFileInfo
	at time.Time
}

func (i *datedInfo) ModTime() time.Time { return i.at }

type datedFS struct {
	*memFS
	dates map[string]time.Time
}

func (d *datedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := d.memFS.ReadDir(name)
	for i, e := range entries {
		if me, ok := e.(*memDirEntry); ok {
			entries[i] = &datedEntry{*me, d.dates[me.name]}
		}
	}
	return entries, err
}

func TestMarkerReplacesTheModeColumn(t *testing.T) {
	mem := newMemFS()
	mem.files["d/readme.md"] = []byte("#")
	mem.files["d/photo.png"] = []byte("png")
	p := New("d", WithFS(mem), WithMarker(func(e fs.DirEntry) string {
		if strings.HasSuffix(e.Name(), ".png") {
			return "📷"
		}
		return "  "
	}))
	p = applyReadDir(p)
	p.SetHeight(10)
	p.SetWidth(60)
	view := p.View()
	if !strings.Contains(view, "📷") || strings.Contains(view, "-rw") || strings.Contains(view, "drwx") {
		t.Fatalf("view:\n%s", view)
	}
}

func TestSortOrders(t *testing.T) {
	base := newMemFS()
	base.files["d/b.md"] = []byte("1")
	base.files["d/a.png"] = []byte("22")
	base.files["d/c.txt"] = []byte("333")
	base.files["d/sub/x"] = []byte("x")
	now := time.Now()
	dated := &datedFS{base, map[string]time.Time{"b.md": now.Add(-2 * time.Hour), "a.png": now.Add(-time.Hour), "c.txt": now}}
	names := func(p Model) string {
		var out []string
		for _, e := range p.allEntries {
			out = append(out, e.Name())
		}
		return strings.Join(out, " ")
	}
	p := applyReadDir(New("d", WithFS(dated)))
	if got := names(p); got != ".. sub a.png b.md c.txt" {
		t.Fatalf("by name: %s", got)
	}
	// Newest first, folders still first.
	byDate := func(a, b fs.DirEntry) int {
		ai, _ := a.Info()
		bi, _ := b.Info()
		return bi.ModTime().Compare(ai.ModTime())
	}
	p = applyReadDir(New("d", WithFS(dated), WithSort(byDate)))
	if got := names(p); got != ".. sub c.txt a.png b.md" {
		t.Fatalf("by date: %s", got)
	}
	// A sort set later reorders what is listed, without another read.
	p.SetSort(nil)
	if got := names(p); got != ".. sub a.png b.md c.txt" {
		t.Fatalf("back to name: %s", got)
	}
	p.SetSort(func(a, b fs.DirEntry) int { return strings.Compare(b.Name(), a.Name()) })
	if got := names(p); got != ".. sub c.txt b.md a.png" {
		t.Fatalf("reversed: %s", got)
	}
}
