package app

import (
	"image"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
)

// The grid's tiles, in cells: wide enough for a name, high enough for a
// picture and its label.
const (
	tileCells = 24
	tileRows  = 8
	thumbEdge = 256
)

// grid shows the file list as thumbnails: one picture, composed from the
// files' own, drawn one after another in the background and kept.
type grid struct {
	tiles      map[string]*document.Tile
	loader     *document.Loader
	loading    string // The file whose thumbnail is being made.
	generation uint64
	cols, rows int // Of tiles across, and on screen.
	tileW      int // Of a tile, in pixels.
	tileH      int
	top        int // The first row on screen.
	image      *image.RGBA
}

type thumbResult struct {
	generation uint64
	path       string
	tile       document.Tile
}

// pending says whether a file still lacks a thumbnail.
func (g *grid) pending() bool { return g.loading != "" || g.next(nil) != "" }

// next is the first file without a thumbnail, from among those given.
func (g *grid) next(files []string) string {
	for _, path := range files {
		if _, ok := g.tiles[path]; !ok {
			return path
		}
	}
	return ""
}

// openGrid shows the list as thumbnails, the style kept for next time.
func (m *Model) openGrid() tea.Cmd {
	m.screen, m.thumbs = screenGrid, true
	if m.grid == nil {
		m.grid = &grid{tiles: map[string]*document.Tile{}, loader: &document.Loader{Files: m.opts.FilesFS}}
	}
	return tea.Sequence(m.disposePreview(), m.layoutGrid())
}

// closeGrid shows the list as a list again.
func (m *Model) closeGrid() tea.Cmd {
	m.screen, m.thumbs = screenList, false
	return tea.Batch(m.pic.SetImage(nil), m.updatePreview())
}

// layoutGrid composes the picture for the screen as it is, the selection
// in view, and asks for the next thumbnail wanted.
func (m *Model) layoutGrid() tea.Cmd {
	g := m.grid
	if g == nil || m.screen != screenGrid {
		return nil
	}
	w, h := max(1, m.width), max(1, m.bodyHeight())
	g.cols = max(1, w/tileCells)
	g.rows = max(1, h/tileRows)
	cw, ch := m.pic.CellPixelSize()
	if cw <= 1 || ch <= 1 {
		cw, ch = 8, 16
	}
	g.tileW, g.tileH = (w/g.cols)*cw, tileRows*ch
	files := m.opts.Files
	m.selection = max(0, min(len(files)-1, m.selection))
	row := m.selection / g.cols
	g.top = max(0, min(g.top, (len(files)-1)/g.cols-g.rows+1))
	if row < g.top {
		g.top = row
	}
	if row >= g.top+g.rows {
		g.top = row - g.rows + 1
	}
	first := g.top * g.cols
	tiles := make([]document.Tile, g.cols*g.rows)
	for i := range tiles {
		if first+i >= len(files) {
			break
		}
		path := files[first+i]
		if tile, ok := g.tiles[path]; ok {
			tiles[i] = *tile
		} else {
			tiles[i] = document.Tile{Name: filepath.Base(path), Loading: true}
		}
	}
	g.image = document.Mosaic(tiles, g.cols, g.tileW, g.tileH, m.selection-first)
	var load tea.Cmd
	if g.loading == "" {
		// Those on screen first, then the rest.
		shown := files[min(first, len(files)):min(first+len(tiles), len(files))]
		if path := g.next(shown); path != "" {
			load = m.loadThumb(path)
		} else if path := g.next(files); path != "" {
			load = m.loadThumb(path)
		}
	}
	return tea.Batch(m.pic.SetImage(g.image), load)
}

// loadThumb makes one file's thumbnail: its picture, a mesh drawn small
// on the CPU, or a stand-in that names its kind.
func (m *Model) loadThumb(path string) tea.Cmd {
	g := m.grid
	g.loading = path
	g.generation++
	loader, generation, forced := g.loader, g.generation, m.opts.Type
	return func() tea.Msg {
		r := loader.Load(document.Request{Path: path, Type: forced, Page: 1, DPI: 48, MaxEdge: thumbEdge, Preview: true, Generation: generation})
		tile := document.Tile{Name: filepath.Base(path), Kind: kindWord(r.Kind)}
		switch {
		case r.Err != nil:
			tile.Failed = true
		case r.Image != nil:
			tile.Image = r.Image
		case r.Mesh != nil:
			if img, err := document.ExportImage(document.Result{Kind: r.Kind, Mesh: r.Mesh, CPU: true}, thumbEdge); err == nil {
				tile.Image = img
			}
		}
		return thumbResult{generation: generation, path: path, tile: tile}
	}
}

// kindWord is what a tile says for a file that has no picture.
func kindWord(kind string) string {
	if word, ok := map[string]string{"ipynb": "notebook", "docx": "Word", "xlsx": "Excel", "grist": "Grist", "csv": "CSV", "json": "JSON", "html": "HTML"}[kind]; ok {
		return word
	}
	return kind
}

func (m *Model) thumbnailMade(r thumbResult) tea.Cmd {
	g := m.grid
	if g == nil || r.generation != g.generation {
		return nil
	}
	g.loading = ""
	tile := r.tile
	g.tiles[r.path] = &tile
	return m.layoutGrid()
}

// gridKey moves about the grid, and reports whether the key was its own.
func (m *Model) gridKey(k string) (tea.Cmd, bool) {
	g := m.grid
	n := len(m.opts.Files)
	switch k {
	case "l", "right", "tab":
		m.selection++
	case "h", "left", "shift+tab":
		m.selection--
	case "j", "down":
		m.selection += g.cols
	case "k", "up":
		m.selection -= g.cols
	case "pgdown":
		m.selection += g.cols * g.rows
	case "pgup":
		m.selection -= g.cols * g.rows
	case "home", "g":
		m.selection = 0
	case "end", "G":
		m.selection = n - 1
	default:
		return nil, false
	}
	m.selection = max(0, min(n-1, m.selection))
	return m.layoutGrid(), true
}

// gridMouse selects the tile under a click, and scrolls with the wheel.
func (m *Model) gridMouse(msg tea.MouseMsg) tea.Cmd {
	g := m.grid
	mouse := msg.Mouse()
	switch v := msg.(type) {
	case tea.MouseWheelMsg:
		if v.Button == tea.MouseWheelUp {
			g.top--
		} else if v.Button == tea.MouseWheelDown {
			g.top++
		}
		rows := (len(m.opts.Files) + g.cols - 1) / g.cols
		g.top = max(0, min(g.top, rows-g.rows))
		// The selection follows the screen.
		m.selection = max(g.top*g.cols, min(m.selection, (g.top+g.rows)*g.cols-1))
		return m.layoutGrid()
	case tea.MouseClickMsg:
		if v.Button != tea.MouseLeft || mouse.Y >= g.rows*tileRows {
			return nil
		}
		cells := max(1, m.width/g.cols)
		i := (g.top+mouse.Y/tileRows)*g.cols + min(g.cols-1, mouse.X/cells)
		if i < 0 || i >= len(m.opts.Files) {
			return nil
		}
		if i == m.selection {
			return m.closeMenu(true) // A second click opens.
		}
		m.selection = i
		return m.layoutGrid()
	}
	return nil
}
