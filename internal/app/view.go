package app

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

// View draws the screen on show: its body, a layer over it where there is
// one, the status bar, and the keys that work. The help page and the
// quitting frame lie over any screen.
func (m *Model) View() tea.View {
	w, h := max(1, m.width), m.bodyHeight()
	body, mouse := m.body(w, h)
	body = m.layered(body, w, h)
	bar, hint := m.statusLine(w, h)
	body = m.framed(body)
	content := body + "\n" + bar + "\n" + hint
	if m.quitting {
		// Keys no longer answer. The frame keeps its height, or Bubble Tea
		// draws it below the last one; and it ends on an empty row, because
		// the last row is erased on the way out. The prompt lands there.
		content, mouse = body+"\n"+bar+"\n", tea.MouseModeNone
	}
	if m.height < 3 || (m.screen == screenDocument && m.chart != nil && m.height < 5) {
		content = ansi.Truncate("gloss: enlarge terminal", w, "")
	}
	v := tea.NewView(content)
	v.AltScreen, v.MouseMode = !m.opts.KeepScreen, mouse
	return v
}

// body is the screen's own content, w by h, and the mouse mode it wants.
func (m *Model) body(w, h int) (string, tea.MouseMode) {
	var body string
	mouse := tea.MouseModeNone
	plain := true // Cut to the viewport; graphics keep their escapes whole.
	switch {
	case m.help:
		body = helpText
	case m.screen == screenBrowser:
		body = m.opener.view()
	case len(m.opts.Files) == 0:
		body = m.emptyView(w)
	case m.listing():
		body, plain = m.menuView(), false
		if m.preview != nil && m.previewWidth() > 0 {
			mouse = m.preview.View().MouseMode
		}
		if m.screen == screenGrid {
			mouse = tea.MouseModeCellMotion
		}
	case m.loading:
		body = "Loading " + safe(filepath.Base(m.opts.Files[m.index])) + "…"
	case m.err != nil:
		body = "Cannot open file\n\n" + safe(m.err.Error()) + "\n\nR retry · ] next file · q quit"
	case m.chart != nil:
		v := m.chart.View()
		body, mouse, plain = m.meshFrame(v.Content, w), v.MouseMode, false
	case m.markdown != nil:
		body, mouse, plain = m.markdown.view(), tea.MouseModeCellMotion, false
	case m.sheet != nil:
		body, mouse, plain = m.sheet.view(w, h), tea.MouseModeCellMotion, false
	default:
		body, plain = m.pic.View().Content, false
	}
	if plain {
		lines := strings.Split(body, "\n")
		for i := range lines {
			lines[i] = ansi.Truncate(lines[i], w, "")
		}
		body = strings.Join(lines[:min(len(lines), h)], "\n")
	}
	frame := lipgloss.NewStyle().Width(w).Height(h)
	if len(m.opts.Files) == 0 && !m.help && m.screen != screenBrowser {
		frame = frame.Align(lipgloss.Center, lipgloss.Center)
	}
	return frame.Render(body), mouse
}

// helpText is kept to 22 lines, the room a 24-row terminal leaves.
const helpText = "gloss — a visual pager\n\n" +
	"q / Q          quit / quit, leaving the view in the scrollback\n? / Esc        help / dismiss; Esc: file list\n" +
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

// emptyView is the drop target shown before any file, and what it opens.
func (m *Model) emptyView(w int) string {
	body := "Drop files here to open\n\nDrag them from a file manager, or paste their paths."
	if m.opts.FilesFS != nil {
		// The browser: files come from drops, or from the page around.
		body = "Drop files here to open\n\nDrag them from a file manager onto the terminal."
	}
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
	return body
}

// layered lays the document's layer over its body, where the screen is a
// document's and the help is down.
func (m *Model) layered(body string, w, h int) string {
	if m.help || m.screen != screenDocument {
		return body
	}
	switch m.layer {
	case layerInfo:
		// The mesh view keeps its own title on the first row.
		top := 0
		if m.chart != nil {
			top = 1
		}
		return overlay(body, m.infoBox(w, h-top), w, top)
	case layerColumns:
		if m.sheet != nil && m.sheet.picker != nil {
			return overlay(body, m.sheet.picker.view(m.sheet, w, h), w, 0)
		}
	case layerParts:
		if m.pickingParts() && m.partPicker != nil {
			return overlay(body, m.partsView(w, h-1), w, 1)
		}
	case layerColor:
		if m.pickingColor() && m.colorPicker != nil {
			return overlay(body, m.colorView(w, h-1), w, 1)
		}
	}
	return body
}

// statusLine is the status bar and the keys line for the screen on show.
func (m *Model) statusLine(w, h int) (bar, hint string) {
	switch {
	case m.screen == screenBrowser && !m.help:
		return m.browserStatus(w)
	case m.listing():
		return m.listStatus(w)
	}
	return m.documentStatus(w, h)
}

func (m *Model) browserStatus(w int) (bar, hint string) {
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
	hint = ansi.Truncate(" Enter open · / find · G go to · Tab complete · Ctrl-S sort · Ctrl-T "+greyed+" · Ctrl-X "+text+" · Esc cancel", w, "")
	if m.opener.going {
		hint = ansi.Truncate(" Type a folder's path · Tab complete · Enter go · Esc back", w, "")
	}
	if f := m.opener.find; f != nil {
		bar = lipgloss.NewStyle().Width(w).Render(ansi.Truncate(" Find · "+f.status(), w, "…"))
		hint = ansi.Truncate(" Enter search, then open · ↑/↓ select · Ctrl-A add all · Esc back", w, "")
	}
	return bar, hint
}

func (m *Model) listStatus(w int) (bar, hint string) {
	status := fmt.Sprintf(" Files · %d/%d selected · current %d", m.selection+1, len(m.opts.Files), m.index+1)
	hint = " ↑/↓ select · Enter open · t thumbnails · v preview · Esc cancel · q quit"
	if m.screen == screenGrid && m.grid != nil {
		status = fmt.Sprintf(" Files · grid · %d/%d selected · current %d", m.selection+1, len(m.opts.Files), m.index+1)
		if m.grid.pending() {
			status += " · making thumbnails"
		}
		hint = " ←→↑↓ select · Enter open · t list · Esc cancel · q quit"
	}
	if m.note != "" {
		status += " · " + m.note
	}
	return lipgloss.NewStyle().Width(w).Render(ansi.Truncate(status, w, "")), ansi.Truncate(hint, w, "")
}

// documentStatus names the file and says what is shown of it, and which
// keys work: the document's own, and those of the layer over it.
func (m *Model) documentStatus(w, h int) (bar, hint string) {
	empty := len(m.opts.Files) == 0
	name := "no files"
	if !empty {
		name = safe(filepath.Base(m.opts.Files[m.index]))
		if document.IsStdin(m.opts.Files[m.index]) {
			name = "stdin"
		}
	}
	detail := m.detail(w, h)
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
	bar = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236")).Width(w).Render(ansi.Truncate(status, w, "…"))
	return bar, ansi.Truncate(m.hints(), w, "")
}

// detail says what is shown of the document. How things are drawn, the
// renderer and the transport, is the info box's to say.
func (m *Model) detail(w, h int) string {
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
	case m.markdown != nil:
		mode := "rendered"
		if m.markdown.raw {
			mode = "source"
		}
		format := map[string]string{"docx": "Word", "json": "JSON", "ipynb": "notebook", "html": "HTML", "text": "text"}[m.kind]
		if format == "" {
			format = "Markdown"
		}
		if m.kind == "text" {
			// Source says its language, as the highlighter names it.
			for _, f := range m.fields {
				if f.Label == "Language" && f.Value != "" {
					format = f.Value
				}
			}
		}
		detail = fmt.Sprintf("%s · %s · line %d/%d", format, mode, min(m.markdown.offset+1, len(m.markdown.lines)), len(m.markdown.lines))
	case m.sheet != nil:
		detail = fmt.Sprintf("%s · %s", m.kind, m.sheet.status(w, h))
		if m.kind == "xlsx" {
			detail = fmt.Sprintf("xlsx · sheet %d/%d · %s · %s", m.page, m.pages, safe(m.sheet.sheet.Name), m.sheet.status(w, h))
		}
	case m.kind == "3mf":
		// Too large to draw, or a preview: the picture the file carries.
		detail = fmt.Sprintf("3MF · thumbnail · %dx", 1<<m.zoom)
	}
	return detail
}

// hints names the keys that work over the document.
func (m *Model) hints() string {
	if len(m.opts.Files) == 0 {
		keys := " q quit · ? help · i info"
		if m.canBrowse() {
			keys += " · o browse"
		}
		return keys
	}
	// A layer takes the keys while it is up.
	switch m.layer {
	case layerColumns:
		return " ↑/↓ select · Space show/hide · a all · n none · Esc close"
	case layerParts:
		return " ↑/↓ select · Space show/hide · Enter focus · a all · n only · X all and close · Esc close"
	case layerColor:
		return " Tab mode · ↑/↓ ←/→ choose · Space apply · r file's colors · Enter keep · Esc undo"
	}
	keys := " q quit · ? help · m files · [/] files · n/p pages · +/- zoom · e export · i info"
	switch {
	case m.markdown != nil:
		keys = " q quit · m files · ↑/↓ scroll · Space/b page · s source · g graphics"
	case m.sheet != nil:
		keys = " q quit · m files · ↑/↓ ←/→ move · Space/b page · n/p sheets · c columns · i info"
		if m.kind != "xlsx" {
			keys = " q quit · m files · ↑/↓ ←/→ move · Space/b page · n/p files · c columns · i info"
		}
		if m.sheet.url() != "" && m.opts.Fetch != nil {
			keys = " Enter open address ·" + strings.TrimPrefix(keys, " q quit ·")
		}
	case m.chart != nil && m.mesh != nil:
		keys = " q quit · ? help · m files · [/] files · e export · i info · C color"
		if m.hasParts() {
			keys += " · c parts"
		}
	}
	if m.isFetched() {
		keys = " Esc close ·" + strings.TrimPrefix(keys, " q quit ·")
	}
	if m.canBrowse() {
		keys += " · o browse"
	}
	if what := m.picking(); what != "" {
		// Say what will be sent before it is.
		keys = " Enter send " + what + " · q cancel ·" + strings.TrimPrefix(keys, " q quit ·")
	}
	return keys
}
