package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestThumbnailsShowTheListAsAGrid(t *testing.T) {
	m := viewing(t, Options{Files: []string{samples + "shapes.svg", samples + "readme.md", samples + "landscape.png", samples + "tetrahedron.stl"}, Render3D: "software"}, "svg")
	send(m, tea.WindowSizeMsg{Width: 100, Height: 30}, press("m"))
	if !m.listing() || m.thumbs {
		t.Fatalf("menu=%v thumbs=%v", m.listing(), m.thumbs)
	}
	send(m, press("t"))
	if !m.thumbs || m.grid == nil {
		t.Fatal("t did not switch to thumbnails")
	}
	if hints := plain(m)[len(plain(m))-1]; !strings.Contains(hints, "t list") || !strings.Contains(hints, "Enter open") {
		t.Fatalf("hints: %s", hints)
	}
	// Thumbnails come one after another, until every file has one; here
	// each is made in turn, however long a render takes.
	for i := 0; i < 8 && m.grid.pending(); i++ {
		path := m.grid.loading
		if path == "" {
			path = m.grid.next(m.opts.Files)
		}
		m.grid.loading = ""
		m.Update(m.loadThumb(path)())
	}
	for _, path := range m.opts.Files {
		tile, ok := m.grid.tiles[path]
		if !ok || tile.Loading {
			t.Fatalf("%s has no thumbnail yet: %+v", path, tile)
		}
		if strings.HasSuffix(path, ".md") && (tile.Image != nil || tile.Kind != "markdown") {
			t.Errorf("markdown tile: %+v", tile)
		}
		if !strings.HasSuffix(path, ".md") && tile.Image == nil {
			t.Errorf("%s has no picture: %+v", path, tile)
		}
	}
	// The grid is one picture, laid out to the screen's cells.
	if m.grid.cols < 2 || m.grid.image == nil || m.grid.image.Bounds().Dx() != m.grid.cols*m.grid.tileW {
		t.Fatalf("cols=%d image=%v", m.grid.cols, m.grid.image != nil)
	}
	// Keys move across and down; Enter opens the one selected.
	send(m, press("l"))
	if m.selection != 1 {
		t.Fatalf("after l: %d", m.selection)
	}
	send(m, press("j"))
	if m.selection != min(3, 1+m.grid.cols) {
		t.Fatalf("after j: %d (cols %d)", m.selection, m.grid.cols)
	}
	send(m, press("h"), press("k"), press("l"), press("l"), enter)
	if m.listing() || m.index != 2 {
		t.Fatalf("Enter: menu=%v index=%d", m.listing(), m.index)
	}
	// Esc comes back to the grid, the style last used.
	send(m, escape)
	if !m.listing() || !m.thumbs {
		t.Fatalf("Esc: menu=%v thumbs=%v", m.listing(), m.thumbs)
	}
	send(m, press("t"))
	if m.thumbs {
		t.Fatal("t did not return to the list")
	}
}
