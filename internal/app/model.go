package app

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/ntcharts-svg/svg"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/charmbracelet/x/ansi"
)

type Options struct {
	STLCamera              *charts.Camera  // Optional initial/reset view for an embedded gallery.
	Views                  []document.View // Views of a mesh asked for: the first is where the viewer starts.
	FilesFS                fs.FS           // Optional embedded files for the browser demo.
	Files                  []string
	Type, Render, Render3D string
	Page, DPI              int
	Menu, Preview          bool
	Browse                 string // Folder to open the file browser in at the start.
	KeepScreen             bool   // Draw on the main screen; the last view stays after quitting.
	Pick                   bool   // The caller waits for files: Enter sends them, and ends the viewer.
	Prompt                 string // What the caller asks of the user, shown in a box throughout.
	PromptTop              bool   // The box above the body, rather than below it.
	// Drops delivers files handed over from outside the terminal, as the
	// page serving the viewer does with what is dropped on it.
	Drops <-chan []string
	// Fetch downloads a web address to a file and returns its path. Nil
	// leaves the network alone, which is the default.
	Fetch func(address string) (string, error)
	// Columns picks the columns of every sheet to show; the rest are hidden.
	Columns document.ColumnFilter
	// Parts picks the parts of every 3MF to show; the rest are left out.
	Parts document.PartFilter
	// Color paints the faces of a mesh that its file left plain.
	Color      *color.RGBA
	keptScreen bool // Set once picture numbers have been moved; previews inherit it.
	// Save stores an export and returns the name it was given. Nil writes to
	// the working directory; the browser demo offers a download instead.
	Save              func(name string, png []byte) (string, error)
	Output, OutputDir string
	MaxEdge           int
	VisionProfile     string
	MarkdownBase      string // Base directory for piped Markdown assets.
}

type Model struct {
	opts                     Options
	loader                   *document.Loader
	pic                      picture.Model
	chart                    *charts.Model
	width, height, index     int
	page, pages              int
	generation               uint64
	kind                     string
	loading, help, autoKitty bool
	err                      error
	source                   image.Image
	zoom                     int
	panX, panY               float64
	triangles                int
	parts                    []document.Part // What a 3MF build places, when a mesh is made of parts.
	assemble                 func([]bool) (*document.Mesh, error)
	partsShown               []bool // Which parts are on the chart; nil for all.
	partPicker               *partPicker
	mesh                     *document.Mesh // The mesh on the chart.
	tint                     *color.RGBA    // Paint given to the mesh, if any.
	tintAll                  bool           // On every face, not only the plain ones.
	colorPicker              *colorPicker
	menu                     bool
	selection                int
	preview                  *Model
	isPreview                bool
	kittyID                  int
	chartID                  int
	previewDrag              bool
	suspended                bool
	savedCamera              *charts.Camera
	savedMarkdown            *markdownView
	markdown                 *markdownView
	sheet                    *sheetView
	note                     string // Outcome of the last drop, shown until the next key.
	info                     bool   // The details box floats over the document.
	fields                   []document.Field
	opener                   *opener
	hideUnsupported          bool         // The browser's choice outlasts any one visit.
	sortBy                   int          // The order the browser lists in; kept likewise.
	noTextFiles              bool         // The browser sets text files aside; kept likewise.
	quitting                 bool         // The view being drawn is the one left behind.
	added                    []string     // What the user has handed over, by full path.
	fetched                  []fetchedDoc // Files fetched from the web this session.
	savedSheet               *sheetView   // The cell to return to when a fetched file closes.
	picked                   []string
	home                     charts.Camera // Where the camera starts, and returns on reset.
	skipped                  []string      // Reported on stderr once the terminal is restored.
}

var nextModelID atomic.Int64

func New(opts Options) *Model {
	switch opts.Render {
	case "kitty":
		picture.ForceKittyCapability(picture.KittyCapabilitySupported)
	case "glyph":
		picture.ForceKittyCapability(picture.KittyCapabilityUnsupported)
	}
	if opts.KeepScreen && !opts.keptScreen {
		// Pictures left in the scrollback are owned by the terminal, under
		// their numbers. Start somewhere new, so that the next gloss does
		// not draw over what the last one left.
		opts.keptScreen = true
		nextModelID.Store(rand.Int64N(8000))
	}
	id := 100 + int(nextModelID.Add(1))*1000
	m := &Model{opts: opts, tint: opts.Color, loader: &document.Loader{Files: opts.FilesFS}, kittyID: id, pic: picture.NewWithConfig(picture.Config{KittyID: id, KittyZ: -1, Background: color.RGBA{R: 24, G: 26, B: 30, A: 255}}), page: opts.Page, pages: 1, autoKitty: opts.Render != "glyph", menu: (opts.Menu || opts.Preview) && len(opts.Files) > 0}
	if opts.Render == "kitty" {
		m.pic.Toggle()
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	var browse tea.Cmd
	if m.opts.Browse != "" {
		browse = m.browseFrom(m.opts.Browse)
	}
	if m.menu {
		m.suspended = true
		return tea.Batch(m.pic.Init(), m.updatePreview(), browse, m.awaitDrops())
	}
	return tea.Batch(m.pic.Init(), m.load(false), browse, m.awaitDrops())
}

func (m *Model) Close() error {
	if m.markdown != nil {
		m.markdown.close()
	}
	if m.preview != nil {
		_ = m.preview.Close()
	}
	if m.chart != nil {
		_ = m.chart.Close()
	}
	return m.loader.Close()
}

func (m *Model) Err() error { return m.err }

// quit gives the terminal back. Graphics are taken down with the alternate
// screen. On the main screen the document is left as it was last seen.
func (m *Model) quit() tea.Cmd {
	if !m.opts.KeepScreen {
		return tea.Sequence(tea.Batch(m.clearGraphics(), m.disposePreview()), tea.Quit)
	}
	m.quitting, m.help, m.opener = true, false, nil
	return tea.Quit
}

// request is what the loader is asked for the current file and page.
func (m *Model) request(reload bool) document.Request {
	q := document.Request{Path: m.opts.Files[m.index], Type: m.opts.Type, Page: m.page, DPI: m.opts.DPI, Generation: m.generation, Reload: reload, Preview: m.isPreview,
		Parts: m.opts.Parts, Shown: m.partsShown, Color: m.tint, PaintAll: m.tintAll}
	if strings.HasPrefix(filepath.Base(q.Path), "gloss-stdin-") {
		q.BaseDir = m.opts.MarkdownBase
	}
	return q
}

func (m *Model) load(reload bool) tea.Cmd {
	if len(m.opts.Files) == 0 {
		return nil
	}
	m.generation++
	m.loading, m.err = true, nil
	q := m.request(reload)
	preview := m.isPreview
	return func() tea.Msg {
		r := m.loader.Load(q)
		if preview {
			return previewResult{owner: m.kittyID, result: r}
		}
		return r
	}
}

func (m *Model) bodyHeight() int { return max(1, m.height-2-m.promptHeight()) }

func (m *Model) switchFile(delta int) tea.Cmd {
	i := m.index + delta
	if i < 0 || i >= len(m.opts.Files) {
		return nil
	}
	return m.show(i)
}

// show loads the file at i afresh: its page, zoom, and camera start over.
func (m *Model) show(i int) tea.Cmd {
	m.index, m.page, m.pages, m.kind = i, 1, 1, ""
	m.source, m.zoom, m.panX, m.panY = nil, 0, 0, 0
	m.savedCamera, m.savedMarkdown, m.fields = nil, nil, nil
	m.partsShown = nil // A choice of parts is for one file.
	return tea.Sequence(m.clearGraphics(), m.load(false))
}

// Close releases renderer resources; Kitty images must also be explicitly
// deleted while the terminal is still running.
func (m *Model) clearChart() tea.Cmd {
	if m.chart == nil {
		return nil
	}
	kitty := m.chart.PictureMode() == picture.PictureKitty
	_ = m.chart.Close()
	m.chart = nil
	if !kitty {
		return nil
	}
	seq := fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", m.chartID)
	if os.Getenv("TMUX") != "" {
		seq = ansi.TmuxPassthrough(seq)
	}
	return tea.Raw(seq)
}

func (m *Model) clearGraphics() tea.Cmd {
	m.sheet = nil
	return tea.Batch(m.clearChart(), m.clearMarkdown(), m.pic.SetImage(nil))
}

func (m *Model) clearMarkdown() tea.Cmd {
	if m.markdown == nil {
		return nil
	}
	cmd := m.markdown.close()
	m.markdown = nil
	return cmd
}

func (m *Model) layoutMarkdown() tea.Cmd {
	if m.markdown == nil {
		return nil
	}
	cw, ch := m.pic.CellPixelSize()
	return m.markdown.layout(m.width, m.bodyHeight(), cw, ch)
}

// paged reports whether n and p turn pages, or sheets, rather than files.
func (m *Model) paged() bool { return m.kind == "pdf" || m.kind == "xlsx" }

func (m *Model) movePage(page int) tea.Cmd {
	if !m.paged() {
		return nil
	}
	page = max(1, min(page, m.pages))
	if page == m.page {
		return nil
	}
	m.page, m.zoom, m.panX, m.panY = page, 0, 0, 0
	return m.load(false)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.opener != nil {
		// The browser's filter takes every key: letters are text here.
		switch v := msg.(type) {
		case tea.KeyPressMsg:
			m.note = ""
			switch v.String() {
			case "ctrl+c":
				return m, m.quit()
			case "esc":
				if m.opener.going {
					m.opener.stopGoing()
					return m, nil
				}
				m.opener = nil
				return m, nil
			case "ctrl+t":
				return m, m.toggleUnsupported()
			case "ctrl+s":
				return m, m.reorder()
			case "ctrl+x":
				return m, m.toggleText()
			}
			return m, m.browse(msg)
		case tea.MouseMsg:
			return m, nil
		}
	}
	msg = m.onBody(msg)
	if mouse, ok := msg.(tea.MouseMsg); ok && m.menu {
		if m.help {
			return m, nil
		}
		return m, m.previewMouse(mouse)
	}
	switch v := msg.(type) {
	case previewResult:
		if m.preview != nil && v.owner == m.preview.kittyID {
			_, cmd := m.preview.Update(v.result)
			return m, cmd
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, v.Width), max(1, v.Height)
		m.opener.resize(m.width, m.bodyHeight())
		cmd := tea.Batch(m.pic.SetSize(m.width, m.bodyHeight()), m.resizePreview(), m.layoutMarkdown())
		if m.chart != nil {
			return m, tea.Batch(cmd, m.chart.SetSize(m.width, m.bodyHeight()))
		}
		return m, cmd
	case tea.PasteMsg:
		// Terminals deliver dropped files as a bracketed paste of their paths.
		if m.isPreview || m.opts.FilesFS != nil {
			return m, nil
		}
		return m, m.probeDrop(document.ParseDrop(v.Content))
	case DropMsg:
		if m.isPreview {
			return m, nil
		}
		return m, m.probeDrop(v.Paths)
	case outsideDrop:
		return m, tea.Batch(m.probeDrop(v.paths), m.awaitDrops())
	case dropResult:
		return m, m.addDropped(v)
	case exportResult:
		m.note = v.note()
		return m, nil
	case openResult:
		return m, m.opened(v)
	case fetchResult:
		return m, m.fetchedFile(v)
	case partsResult:
		return m, m.assembled(v)
	case document.Result:
		if v.Generation != m.generation || m.suspended {
			return m, nil
		}
		m.loading, m.err, m.fields = false, v.Err, v.Info
		if v.Err != nil {
			return m, m.clearGraphics()
		}
		cleanup := tea.Batch(m.clearMarkdown(), m.clearChart())
		m.sheet = nil
		m.kind, m.page, m.pages = v.Kind, v.Page, v.Pages
		if v.Sheet != nil {
			m.source, m.sheet = nil, newSheetView(v.Sheet)
			m.sheet.setHidden(m.opts.Columns.Hidden(v.Sheet))
			if m.savedSheet != nil {
				m.sheet.setHidden(m.savedSheet.hidden)
				m.sheet.row, m.sheet.col, m.sheet.at = m.savedSheet.row, m.savedSheet.col, m.savedSheet.at
				m.sheet.settle(m.width, m.bodyHeight()-1)
				m.savedSheet = nil
			}
			return m, tea.Sequence(cleanup, m.pic.SetImage(nil))
		}
		if v.Markdown != nil {
			m.source = nil
			m.markdown = newMarkdownView(v.Markdown, 100+int(nextModelID.Add(1))*1000)
			if m.savedMarkdown != nil {
				m.markdown.raw, m.markdown.offset = m.savedMarkdown.raw, m.savedMarkdown.offset
				m.savedMarkdown = nil
			}
			return m, tea.Sequence(tea.Batch(cleanup, m.pic.SetImage(nil)), tea.Batch(m.markdown.setKitty(m.pic.Mode() == picture.PictureKitty), m.layoutMarkdown()))
		}
		if v.Mesh != nil {
			mode := charts.WebGPU
			switch m.opts.Render3D {
			case "software":
				mode = charts.Software
			case "wireframe":
				mode = charts.Wireframe
			}
			m.chartID = 100 + int(nextModelID.Add(1))*1000
			m.chart = charts.New(m.width, m.bodyHeight(), charts.WithKittyID(m.chartID), charts.WithAutoRotate(false), charts.WithRenderMode(mode), charts.WithBackground(color.RGBA{R: 24, G: 26, B: 30, A: 255}))
			// The chart draws only through the commands it returns, so each
			// is kept, whether or not it has anything to say before Init.
			var setup []tea.Cmd
			setup = append(setup, m.chart.SetAxes(charts.Axes{X: charts.Axis{Hidden: true}, Y: charts.Axis{Hidden: true}, Z: charts.Axis{Hidden: true}}))
			setup = append(setup, m.chart.SetColorLegendVisible(false), m.chart.SetSeries(v.Mesh))
			m.mesh = v.Mesh
			m.parts, m.assemble, m.partsShown, m.partPicker, m.colorPicker = v.Parts, v.Assemble, v.Shown, nil, nil
			m.home = charts.DefaultCamera()
			switch {
			case len(m.opts.Views) > 0:
				// Fitted to this mesh, so worked out once it is known.
				m.home = m.opts.Views[0].Camera(v.Mesh)
			case m.opts.STLCamera != nil:
				m.home = *m.opts.STLCamera
			}
			if m.savedCamera != nil {
				setup = append(setup, m.chart.SetCamera(*m.savedCamera))
				m.savedCamera = nil
			} else if m.home != charts.DefaultCamera() {
				setup = append(setup, m.chart.SetCamera(m.home))
			}
			// Apply a capability already established before this chart existed.
			_, applied := m.chart.Update(struct{}{})
			setup = append(setup, applied)
			m.triangles, m.err = v.Mesh.Triangles(), m.chart.Err()
			return m, tea.Sequence(tea.Batch(cleanup, m.pic.SetImage(nil)), tea.Batch(setup...), m.chart.Init())
		}
		m.source = v.Image
		return m, tea.Sequence(cleanup, m.refreshImage())
	case tea.MouseWheelMsg:
		if m.markdown != nil && !m.help && !m.menu {
			if v.Button == tea.MouseWheelUp {
				m.markdown.scroll(-3)
			} else if v.Button == tea.MouseWheelDown {
				m.markdown.scroll(3)
			}
			return m, nil
		}
		if m.sheet != nil && !m.help && !m.menu {
			if v.Button == tea.MouseWheelUp {
				m.sheet.scroll(-3, m.width, m.bodyHeight()-1)
			} else if v.Button == tea.MouseWheelDown {
				m.sheet.scroll(3, m.width, m.bodyHeight()-1)
			}
			return m, nil
		}
	case tea.KeyPressMsg:
		m.note = ""
		if k := v.String(); (k == "o" || k == "O") && !m.help && !m.pickingColumns() && !m.pickingParts() {
			return m, m.openBrowser()
		}
		switch v.String() {
		case "q", "ctrl+c":
			return m, m.quit()
		case "?":
			m.help = !m.help
			return m, nil
		case "esc":
			if !m.help && m.menu {
				return m, m.closeMenu(false)
			}
			if m.pickingColumns() {
				m.sheet.picker = nil
				return m, nil
			}
			if m.pickingParts() {
				m.partPicker = nil
				return m, nil
			}
			if m.pickingColor() {
				p := m.colorPicker
				m.colorPicker = nil
				if p.wasTint == nil {
					return m, m.unpaint()
				}
				return m, m.paint(*p.wasTint, p.wasAll)
			}
			if !m.help && !m.info {
				if cmd, ok := m.closeFetched(); ok {
					return m, cmd
				}
			}
			if !m.help {
				m.info = false
			}
			m.help = false
			return m, nil
		}
		if len(m.opts.Files) == 0 && v.String() == "i" && !m.help {
			m.info = !m.info
			return m, nil
		}
		if m.help || len(m.opts.Files) == 0 {
			return m, nil
		}
		if m.menu {
			return m, m.menuKey(v.String())
		}
		if m.pickingParts() {
			if cmd, ok := m.partsKey(v.String()); ok {
				return m, cmd
			}
		}
		if m.pickingColor() {
			if cmd, ok := m.colorKey(v.String()); ok {
				return m, cmd
			}
		}
		if m.chart != nil && m.mesh != nil && v.String() == "C" && !m.help && !m.menu {
			m.openColorPicker()
			return m, nil
		}
		if m.hasParts() && !m.help && !m.menu {
			switch v.String() {
			case "c":
				m.partPicker = &partPicker{}
				return m, nil
			case "X":
				return m, m.showParts(nil, false)
			}
		}
		if m.sheet != nil && m.sheet.key(v.String(), m.width, m.bodyHeight()-1) {
			return m, nil
		}
		if m.sheet != nil && v.String() == "enter" && m.sheet.url() != "" {
			return m, m.open(m.sheet.url())
		}
		if m.markdown != nil {
			if m.markdown.key(v.String()) {
				return m, nil
			}
			if v.String() == "s" {
				m.markdown.raw = !m.markdown.raw
				m.markdown.offset = 0
				return m, m.layoutMarkdown()
			}
		}
		switch v.String() {
		case "m":
			return m, m.openMenu(m.index)
		case "]", "tab":
			return m, m.switchFile(1)
		case "[", "shift+tab":
			return m, m.switchFile(-1)
		case "R":
			return m, m.load(true)
		case "e":
			return m, m.export()
		case "enter":
			if m.opts.Pick {
				return m, m.pick()
			}
		case "i":
			m.info = !m.info
			return m, nil
		case "5":
			// NTCharts3d binds the projection to o, which opens the browser here.
			if m.chart != nil {
				_, cmd := m.chart.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
				return m, cmd
			}
			return m, nil
		case "n", "space", "pgdown":
			if m.paged() {
				return m, m.movePage(m.page + 1)
			}
			return m, m.switchFile(1)
		case "p", "b", "pgup":
			if m.paged() {
				return m, m.movePage(m.page - 1)
			}
			return m, m.switchFile(-1)
		case "home":
			return m, m.movePage(1)
		case "end", "G":
			return m, m.movePage(m.pages)
		case "g":
			m.autoKitty = false
			if m.chart == nil {
				cmd := m.pic.Toggle()
				if m.markdown != nil {
					cmd = tea.Batch(cmd, m.markdown.setKitty(m.pic.Mode() == picture.PictureKitty))
				}
				return m, cmd
			}
		case "0", "f":
			if m.chart != nil {
				return m, m.chart.SetCamera(m.home)
			}
			m.zoom, m.panX, m.panY = 0, 0, 0
			return m, m.refreshImage()
		}
		if m.chart == nil {
			switch v.String() {
			case "+", "=":
				m.zoom = min(6, m.zoom+1)
			case "-", "_":
				m.zoom = max(0, m.zoom-1)
			case "h", "left":
				m.panX -= .1 / float64(int(1)<<m.zoom)
			case "l", "right":
				m.panX += .1 / float64(int(1)<<m.zoom)
			case "k", "up":
				m.panY -= .1 / float64(int(1)<<m.zoom)
			case "j", "down":
				m.panY += .1 / float64(int(1)<<m.zoom)
			case "r":
				return m, m.load(true)
			default:
				return m, nil
			}
			return m, m.refreshImage()
		}
	}
	var cmds []tea.Cmd
	if m.opener != nil {
		cmds = append(cmds, m.browse(msg))
	}
	// Picture messages carry image IDs and sequence numbers, so late frames
	// from an earlier page cannot overwrite the current source.
	cmds = append(cmds, m.pic.Update(msg))
	if m.autoKitty && m.pic.KittySupported() == picture.KittyCapabilitySupported && m.pic.Mode() != picture.PictureKitty {
		cmds = append(cmds, m.pic.Toggle())
	}
	if m.markdown != nil {
		cmds = append(cmds, m.markdown.update(msg), m.markdown.setKitty(m.pic.Mode() == picture.PictureKitty))
	}
	_, mouseMessage := msg.(tea.MouseMsg)
	if m.chart != nil && !((m.help || m.menu) && mouseMessage) {
		before := m.chart.Camera()
		_, cmd := m.chart.Update(msg)
		cmds = append(cmds, cmd)
		// NTCharts orbits the camera toward horizontal drags. In a model
		// viewer, drag the object instead. Preserve picking, pan, and keys.
		if motion, ok := msg.(tea.MouseMotionMsg); ok && motion.Mod&tea.ModShift == 0 {
			after := m.chart.Camera()
			if after.Beta != before.Beta {
				after.Beta = 2*before.Beta - after.Beta
				cmds = append(cmds, m.chart.SetCamera(after))
			}
		}
	}
	if m.preview != nil && !mouseMessage {
		_, cmd := m.preview.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) refreshImage() tea.Cmd {
	if m.source == nil {
		return nil
	}
	span := 1 / float64(int(1)<<m.zoom)
	limit := (1 - span) / 2
	m.panX, m.panY = max(-limit, min(limit, m.panX)), max(-limit, min(limit, m.panY))
	return m.pic.SetImage(crop(m.source, m.zoom, m.panX, m.panY))
}

func crop(src image.Image, zoom int, panX, panY float64) image.Image {
	if zoom == 0 {
		return src
	}
	b := src.Bounds()
	w, h := max(1, b.Dx()>>zoom), max(1, b.Dy()>>zoom)
	x := b.Min.X + int(float64(b.Dx()-w)/2+panX*float64(b.Dx()))
	y := b.Min.Y + int(float64(b.Dy()-h)/2+panY*float64(b.Dy()))
	x, y = max(b.Min.X, min(b.Max.X-w, x)), max(b.Min.Y, min(b.Max.Y-h, y))
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), src, image.Pt(x, y), draw.Src)
	return out
}

func (m *Model) View() tea.View {
	w, h := max(1, m.width), m.bodyHeight()
	body := ""
	mouse := tea.MouseModeNone
	switch {
	case m.help:
		// Kept to 22 lines, the room a 24-row terminal leaves.
		body = "gloss — a visual pager\n\n" +
			"q / Ctrl-C     quit\n? / Esc        help / dismiss\n" +
			"] / [ / Tab    next / previous file\n" +
			"m              file menu (v toggles preview)\n" +
			"o              browse for a file to open\n" +
			"n / p / Space  next / previous PDF page (or file)\n" +
			"Home / End     first / last PDF page\n" +
			"+ / -          zoom\nh j k l / arrows  pan image / orbit mesh\n" +
			"f / 0          fit / reset view\ng              toggle Kitty / glyph\n" +
			"R              reload file\n" +
			"e / i          export as PNG / file details\n" +
			"r              auto-rotate mesh (reload other files)\n\n" +
			"Drop files on the terminal to add them\n" +
			"Meshes: drag to orbit, Shift-drag to pan, wheel to zoom, 5 orthographic\n" +
			"Markdown: arrows/wheel scroll, Space/b page, s source\n\n" +
			"gloss --help lists the command-line options\n"
	case m.opener != nil:
		body = m.opener.view()
	case len(m.opts.Files) == 0:
		body = "Drop files here to open\n\nDrag them from a file manager, or paste their paths."
		if m.canBrowse() {
			body = "Drop files here to open\n\nDrag them from a file manager, paste their paths, or press o to browse."
		}
		if m.opts.Pick {
			body = "Drop a file here to send it\n\nDrag it from a file manager, paste its path, or press o to browse."
		}
		switch h := m.bodyHeight(); {
		case h >= 12:
			// Padded to one width, the lines stay aligned once centred.
			body += "\n\n" + lipgloss.NewStyle().Width(lipgloss.Width(strings.Join(document.Formats, "\n"))).Render(strings.Join(document.Formats, "\n\n"))
		case h >= 5:
			body += "\n\n" + lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render(document.FormatsShort)
		}
	case m.menu:
		body = m.menuView()
		if m.preview != nil && m.previewWidth() > 0 {
			mouse = m.preview.View().MouseMode
		}
	case m.loading:
		body = "Loading " + safe(filepath.Base(m.opts.Files[m.index])) + "…"
	case m.err != nil:
		body = "Cannot open file\n\n" + safe(m.err.Error()) + "\n\nR retry · ] next file · q quit"
	case m.chart != nil:
		v := m.chart.View()
		body, mouse = m.meshFrame(v.Content, w), v.MouseMode
	case m.markdown != nil:
		body, mouse = m.markdown.view(), tea.MouseModeCellMotion
	case m.sheet != nil:
		body, mouse = m.sheet.view(w, h), tea.MouseModeCellMotion
	default:
		body = m.pic.View().Content
	}
	// Avoid wrapping filenames, errors, or help beyond the viewport. Do not
	// truncate the graphics body: Kitty's zero-width escapes must survive.
	empty := len(m.opts.Files) == 0
	browsing := m.opener != nil && !m.help
	if m.help || empty || browsing || (!m.menu && (m.loading || m.err != nil)) {
		lines := strings.Split(body, "\n")
		for i := range lines {
			lines[i] = ansi.Truncate(lines[i], w, "")
		}
		body = strings.Join(lines[:min(len(lines), h)], "\n")
	}
	frame := lipgloss.NewStyle().Width(w).Height(h)
	if empty && !m.help && !browsing {
		frame = frame.Align(lipgloss.Center, lipgloss.Center)
	}
	body = frame.Render(body)
	if m.info && !m.help && !m.menu && !browsing && !m.pickingParts() && !m.pickingColor() && !m.pickingColumns() {
		// The mesh view keeps its own title on the first row.
		top := 0
		if m.chart != nil {
			top = 1
		}
		body = overlay(body, m.infoBox(w, h-top), w, top)
	}
	if m.pickingColumns() && !m.help && !m.menu {
		body = overlay(body, m.sheet.picker.view(m.sheet, w, h), w, 0)
	}
	if m.pickingParts() && !m.help && !m.menu {
		// Below the mesh view's title row.
		body = overlay(body, m.partsView(w, h-1), w, 1)
	}
	if m.pickingColor() && !m.help && !m.menu {
		body = overlay(body, m.colorView(w, h-1), w, 1)
	}
	name := "no files"
	if !empty {
		name = safe(filepath.Base(m.opts.Files[m.index]))
	}
	if strings.HasPrefix(name, "gloss-stdin-") {
		name = "stdin"
	}
	// How things are drawn, the renderer and the transport, is the info
	// box's to say; the status bar keeps to the file.
	detail := fmt.Sprintf("%s · %dx", m.kind, 1<<m.zoom)
	if m.kind == "pdf" {
		detail += fmt.Sprintf(" · page %d/%d", m.page, m.pages)
	}
	switch {
	case m.chart != nil:
		detail = fmt.Sprintf("%s · %s", strings.ToUpper(m.kind), plural(m.triangles, "triangle"))
		if m.hasParts() && m.partsShown != nil {
			detail += fmt.Sprintf(" · parts %d/%d", m.partsOnScreen(), len(m.parts))
		}
		if m.tint != nil {
			detail += fmt.Sprintf(" · painted #%02x%02x%02x", m.tint.R, m.tint.G, m.tint.B)
		}
	case m.kind == "3mf":
		// Too large to draw, or a preview: the picture the file carries.
		detail = fmt.Sprintf("3MF · thumbnail · %dx", 1<<m.zoom)
	}
	if m.markdown != nil {
		mode := "rendered"
		if m.markdown.raw {
			mode = "source"
		}
		format := map[string]string{"docx": "Word", "json": "JSON", "ipynb": "notebook", "html": "HTML", "text": "text"}[m.kind]
		if format == "" {
			format = "Markdown"
		}
		detail = fmt.Sprintf("%s · %s · line %d/%d", format, mode, min(m.markdown.offset+1, len(m.markdown.lines)), len(m.markdown.lines))
	}
	if m.sheet != nil {
		detail = fmt.Sprintf("%s · %s", m.kind, m.sheet.status(w, h))
		if m.kind == "xlsx" {
			detail = fmt.Sprintf("xlsx · sheet %d/%d · %s · %s", m.page, m.pages, safe(m.sheet.sheet.Name), m.sheet.status(w, h))
		}
	}
	// A note comes before the detail: it is brief, and must not be cut.
	if m.note != "" {
		detail = m.note + " · " + detail
	}
	status := fmt.Sprintf(" %s  [%d/%d]  %s", name, m.index+1, len(m.opts.Files), detail)
	if empty {
		status = " No files yet"
		if m.note != "" {
			status += " · " + m.note
		}
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236")).Width(w).Render(ansi.Truncate(status, w, "…"))
	keys := " q quit · ? help · m files · [/] files · n/p pages · +/- zoom · e export · i info"
	if m.markdown != nil {
		keys = " q quit · m files · ↑/↓ scroll · Space/b page · s source · g graphics"
	}
	if m.sheet != nil {
		keys = " q quit · m files · ↑/↓ ←/→ move · Space/b page · n/p sheets · c columns · i info"
		if m.kind != "xlsx" {
			keys = " q quit · m files · ↑/↓ ←/→ move · Space/b page · n/p files · c columns · i info"
		}
		if m.sheet.url() != "" && m.opts.Fetch != nil {
			keys = " Enter open address ·" + strings.TrimPrefix(keys, " q quit ·")
		}
		if m.pickingColumns() {
			keys = " ↑/↓ select · Space show/hide · a all · n none · Esc close"
		}
	}
	if empty {
		keys = " q quit · ? help · i info"
	}
	if m.chart != nil && m.mesh != nil {
		keys = " q quit · ? help · m files · [/] files · e export · i info · C color"
	}
	if m.hasParts() {
		keys += " · c parts"
	}
	if m.pickingColor() {
		keys = " Tab mode · ↑/↓ ←/→ choose · Space apply · r file's colors · Enter keep · Esc undo"
	}
	if m.pickingParts() {
		keys = " ↑/↓ select · Space show/hide · Enter focus · a all · n only · X all and close · Esc close"
	}
	if m.isFetched() {
		keys = " Esc close ·" + strings.TrimPrefix(keys, " q quit ·")
	}
	if m.canBrowse() && !m.pickingColumns() && !m.pickingParts() && !m.pickingColor() {
		keys += " · o browse"
	}
	if what := m.picking(); what != "" {
		// Say what will be sent before it is.
		keys = " Enter send " + what + " · q cancel ·" + strings.TrimPrefix(keys, " q quit ·")
	}
	hint := ansi.Truncate(keys, w, "")
	if m.menu {
		status = fmt.Sprintf(" Files · %d/%d selected · current %d", m.selection+1, len(m.opts.Files), m.index+1)
		if m.note != "" {
			status += " · " + m.note
		}
		bar = lipgloss.NewStyle().Width(w).Render(ansi.Truncate(status, w, ""))
		hint = ansi.Truncate(" ↑/↓ select · Enter open · Esc cancel · v preview · q quit", w, "")
	}
	if browsing {
		// The end of a long path says where you are; the start rarely does.
		dir, note := safe(m.opener.dir), " · by "+orderNames[m.sortBy]
		if m.note != "" {
			note += " · " + m.note
		}
		if over := ansi.StringWidth(" Open · "+dir+note) - w; over > 0 && over+1 < ansi.StringWidth(dir) {
			dir = "…" + ansi.TruncateLeft(dir, over+1, "")
		}
		bar = lipgloss.NewStyle().Width(w).Render(ansi.Truncate(" Open · "+dir+note, w, "…"))
		greyed, text := "hide greyed", "no text"
		if m.hideUnsupported {
			greyed = "show greyed"
		}
		if m.noTextFiles {
			text = "text"
		}
		hint = ansi.Truncate(" Enter open · G go to · Tab complete · Ctrl-S sort · Ctrl-T "+greyed+" · Ctrl-X "+text+" · Esc cancel", w, "")
		if m.opener.going {
			hint = ansi.Truncate(" Type a folder's path · Tab complete · Enter go · Esc back", w, "")
		}
	}
	body = m.framed(body)
	content := body + "\n" + bar + "\n" + hint
	if m.quitting {
		// Keys no longer answer. The frame keeps its height, or Bubble Tea
		// draws it below the last one; and it ends on an empty row, because
		// the last row is erased on the way out. The prompt lands there.
		content, mouse = body+"\n"+bar+"\n", tea.MouseModeNone
	}
	if m.height < 3 || (!m.menu && m.chart != nil && m.height < 5) {
		content = ansi.Truncate("gloss: enlarge terminal", w, "")
	}
	v := tea.NewView(content)
	v.AltScreen, v.MouseMode = !m.opts.KeepScreen, mouse
	return v
}

func safe(s string) string { return svg.SanitizeForTerminal(s) }
