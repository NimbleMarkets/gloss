package app

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io/fs"
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
	STLCamera              *charts.Camera // Optional initial/reset view for an embedded gallery.
	FilesFS                fs.FS          // Optional embedded files for the browser demo.
	Files                  []string
	Type, Render, Render3D string
	Page, DPI              int
	Menu, Preview          bool
	TUI                    bool // Start without files, waiting for a drop.
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
	note                     string // Outcome of the last drop, shown until the next key.
	info                     bool   // The details box floats over the document.
	fields                   []document.Field
	opener                   *opener
	hideUnsupported          bool     // The browser's choice outlasts any one visit.
	skipped                  []string // Reported on stderr once the terminal is restored.
}

var nextModelID atomic.Int64

func New(opts Options) *Model {
	switch opts.Render {
	case "kitty":
		picture.ForceKittyCapability(picture.KittyCapabilitySupported)
	case "glyph":
		picture.ForceKittyCapability(picture.KittyCapabilityUnsupported)
	}
	id := 100 + int(nextModelID.Add(1))*1000
	m := &Model{opts: opts, loader: &document.Loader{Files: opts.FilesFS}, kittyID: id, pic: picture.NewWithConfig(picture.Config{KittyID: id, KittyZ: -1, Background: color.RGBA{R: 24, G: 26, B: 30, A: 255}}), page: opts.Page, pages: 1, autoKitty: opts.Render != "glyph", menu: (opts.Menu || opts.Preview) && len(opts.Files) > 0}
	if opts.Render == "kitty" {
		m.pic.Toggle()
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	if m.menu {
		m.suspended = true
		return tea.Batch(m.pic.Init(), m.updatePreview())
	}
	return tea.Batch(m.pic.Init(), m.load(false))
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

func (m *Model) load(reload bool) tea.Cmd {
	if len(m.opts.Files) == 0 {
		return nil
	}
	m.generation++
	m.loading, m.err = true, nil
	q := document.Request{Path: m.opts.Files[m.index], Type: m.opts.Type, Page: m.page, DPI: m.opts.DPI, Generation: m.generation, Reload: reload}
	if strings.HasPrefix(filepath.Base(q.Path), "gloss-stdin-") {
		q.BaseDir = m.opts.MarkdownBase
	}
	preview := m.isPreview
	return func() tea.Msg {
		r := m.loader.Load(q)
		if preview {
			return previewResult{owner: m.kittyID, result: r}
		}
		return r
	}
}

func (m *Model) bodyHeight() int { return max(1, m.height-2) }

func (m *Model) switchFile(delta int) tea.Cmd {
	i := m.index + delta
	if i < 0 || i >= len(m.opts.Files) {
		return nil
	}
	m.index, m.page, m.pages, m.kind = i, 1, 1, ""
	m.source, m.zoom, m.panX, m.panY = nil, 0, 0, 0
	m.savedCamera, m.savedMarkdown, m.fields = nil, nil, nil
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

func (m *Model) movePage(page int) tea.Cmd {
	if m.kind != "pdf" {
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
				return m, tea.Sequence(tea.Batch(m.clearGraphics(), m.disposePreview()), tea.Quit)
			case "esc":
				m.opener = nil
				return m, nil
			case "ctrl+t":
				return m, m.toggleUnsupported()
			}
			return m, m.browse(msg)
		case tea.MouseMsg:
			return m, nil
		}
	}
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
	case dropResult:
		return m, m.addDropped(v)
	case exportResult:
		m.note = v.note()
		return m, nil
	case openResult:
		return m, m.opened(v)
	case document.Result:
		if v.Generation != m.generation || m.suspended {
			return m, nil
		}
		m.loading, m.err, m.fields = false, v.Err, v.Info
		if v.Err != nil {
			return m, m.clearGraphics()
		}
		cleanup := tea.Batch(m.clearMarkdown(), m.clearChart())
		m.kind, m.page, m.pages = v.Kind, v.Page, v.Pages
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
			m.chart.SetAxes(charts.Axes{X: charts.Axis{Hidden: true}, Y: charts.Axis{Hidden: true}, Z: charts.Axis{Hidden: true}})
			m.chart.SetColorLegendVisible(false)
			m.chart.SetSeries(v.Mesh)
			if m.savedCamera != nil {
				m.chart.SetCamera(*m.savedCamera)
				m.savedCamera = nil
			} else if m.opts.STLCamera != nil {
				m.chart.SetCamera(*m.opts.STLCamera)
			}
			// Apply a capability already established before this chart existed.
			_, _ = m.chart.Update(struct{}{})
			m.triangles, m.err = v.Mesh.Triangles(), m.chart.Err()
			return m, tea.Sequence(tea.Batch(cleanup, m.pic.SetImage(nil)), m.chart.Init())
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
	case tea.KeyPressMsg:
		m.note = ""
		if k := v.String(); (k == "o" || k == "O") && !m.help {
			return m, m.openBrowser()
		}
		switch v.String() {
		case "q", "ctrl+c":
			return m, tea.Sequence(tea.Batch(m.clearGraphics(), m.disposePreview()), tea.Quit)
		case "?":
			m.help = !m.help
			return m, nil
		case "esc":
			if !m.help && m.menu {
				return m, m.closeMenu(false)
			}
			if !m.help {
				m.info = false
			}
			m.help = false
			return m, nil
		}
		if m.help || len(m.opts.Files) == 0 {
			return m, nil
		}
		if m.menu {
			return m, m.menuKey(v.String())
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
			if m.kind == "pdf" {
				return m, m.movePage(m.page + 1)
			}
			return m, m.switchFile(1)
		case "p", "b", "pgup":
			if m.kind == "pdf" {
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
				if m.opts.STLCamera != nil {
					return m, m.chart.SetCamera(*m.opts.STLCamera)
				}
				return m, m.chart.SetCamera(charts.DefaultCamera())
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
			"+ / -          zoom\nh j k l / arrows  pan image / orbit STL\n" +
			"f / 0          fit / reset view\ng              toggle Kitty / glyph\n" +
			"R              reload file\n" +
			"e / i          export as PNG / file details\n" +
			"r              auto-rotate STL (reload other files)\n\n" +
			"Drop files on the terminal to add them\n" +
			"STL: drag to orbit, Shift-drag to pan, wheel to zoom, 5 orthographic\n" +
			"Markdown: arrows/wheel scroll, Space/b page, s source\n"
	case m.opener != nil:
		body = m.opener.view()
	case len(m.opts.Files) == 0:
		body = "Drop files here to open\n\nDrag them from a file manager, or paste their paths."
		if m.canBrowse() {
			body = "Drop files here to open\n\nDrag them from a file manager, paste their paths, or press o to browse."
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
		body, mouse = v.Content, v.MouseMode
	case m.markdown != nil:
		body, mouse = m.markdown.view(), tea.MouseModeCellMotion
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
	if m.info && !m.help && !m.menu && !empty && !browsing {
		// The mesh view keeps its own title on the first row.
		top := 0
		if m.chart != nil {
			top = 1
		}
		body = overlay(body, m.infoBox(w, h-top), w, top)
	}
	name := "no files"
	if !empty {
		name = safe(filepath.Base(m.opts.Files[m.index]))
	}
	if strings.HasPrefix(name, "gloss-stdin-") {
		name = "stdin"
	}
	mode := "glyph"
	if m.pic.Mode() == picture.PictureKitty {
		mode = "kitty"
	}
	if m.chart != nil {
		mode = m.chart.RenderMode().String()
		if m.chart.PictureMode() == picture.PictureKitty {
			mode += "/kitty"
		} else {
			mode += "/glyph"
		}
	}
	detail := fmt.Sprintf("%s · %s · %dx", m.kind, mode, 1<<m.zoom)
	if m.kind == "pdf" {
		detail += fmt.Sprintf(" · page %d/%d", m.page, m.pages)
	}
	if m.kind == "stl" {
		detail = fmt.Sprintf("STL · %d triangles · %s", m.triangles, mode)
	}
	if m.markdown != nil {
		mode := "rendered"
		if m.markdown.raw {
			mode = "source"
		}
		detail = fmt.Sprintf("Markdown · %s · line %d/%d", mode, min(m.markdown.offset+1, len(m.markdown.lines)), len(m.markdown.lines))
	}
	status := fmt.Sprintf(" %s  [%d/%d]  %s", name, m.index+1, len(m.opts.Files), detail)
	if empty {
		status = " No files yet"
	}
	if m.note != "" {
		status += " · " + m.note
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236")).Width(w).Render(ansi.Truncate(status, w, "…"))
	keys := " q quit · ? help · m files · [/] files · n/p pages · +/- zoom · e export · i info"
	if m.markdown != nil {
		keys = " q quit · m files · ↑/↓ scroll · Space/b page · s source · g graphics"
	}
	if empty {
		keys = " q quit · ? help"
	}
	if m.canBrowse() {
		keys += " · o browse"
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
		dir, note := safe(m.opener.dir), ""
		if m.note != "" {
			note = " · " + m.note
		}
		if over := ansi.StringWidth(" Open · "+dir+note) - w; over > 0 && over+1 < ansi.StringWidth(dir) {
			dir = "…" + ansi.TruncateLeft(dir, over+1, "")
		}
		bar = lipgloss.NewStyle().Width(w).Render(ansi.Truncate(" Open · "+dir+note, w, "…"))
		unsupported := "hide unsupported"
		if m.hideUnsupported {
			unsupported = "show all"
		}
		hint = ansi.Truncate(" ↑/↓ select · Enter open · Tab complete · Ctrl-T "+unsupported+" · Esc cancel", w, "")
	}
	content := body + "\n" + bar + "\n" + hint
	if m.height < 3 || (!m.menu && m.chart != nil && m.height < 5) {
		content = ansi.Truncate("gloss: enlarge terminal", w, "")
	}
	v := tea.NewView(content)
	v.AltScreen, v.MouseMode = true, mouse
	return v
}

func safe(s string) string { return svg.SanitizeForTerminal(s) }
