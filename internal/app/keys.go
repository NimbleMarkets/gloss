package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
)

// key handles a key press for the screen on show, and reports whether it
// was taken; a key a mesh may want goes on to it. Each screen has its own
// keys, and its own Esc; the help page lies over any of them.
func (m *Model) key(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	m.note = ""
	k := msg.String()
	if m.screen == screenBrowser {
		return m.browserKey(msg), true
	}
	// Keys over any screen but the browser, whose filter takes letters.
	switch k {
	case "q", "ctrl+c":
		return m.quit(), true
	case "Q":
		// Quit leaving the view where it can be scrolled back to,
		// whatever the launch asked: the choice is best made now.
		m.opts.KeepScreen = true
		return m.quit(), true
	case "?":
		m.help = !m.help
		return nil, true
	}
	if m.help {
		if k == "esc" {
			m.help = false
		}
		return nil, true
	}
	if (k == "o" || k == "O") && m.layer != layerColumns && m.layer != layerParts && m.layer != layerColor && m.layer != layerQR {
		return m.openBrowser(), true
	}
	if m.listing() {
		return m.menuKey(k), true
	}
	return m.documentKey(k)
}

// browserKey gives the folder browser its keys: its filter takes every
// letter, so only a few controls are the viewer's.
func (m *Model) browserKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return m.quit()
	case "esc":
		switch {
		case m.opener.find != nil:
			m.opener.find = nil
		case m.opener.leavePopup():
		case m.opener.going:
			return m.opener.stopGoing()
		default:
			m.showDocument()
		}
		return nil
	case "ctrl+t":
		return m.toggleUnsupported()
	case "ctrl+s":
		return m.reorder()
	case "ctrl+x":
		return m.toggleText()
	}
	return m.browse(msg)
}

// documentKey handles a key over the document: the layer on it first,
// then the document's own kind, then what every document answers to. A
// key none of them takes is left to a mesh, which has keys of its own.
func (m *Model) documentKey(k string) (tea.Cmd, bool) {
	empty := len(m.opts.Files) == 0
	if empty {
		if k == "i" || (k == "esc" && m.layer == layerInfo) {
			return m.toggleInfo(), true
		}
		return nil, true
	}
	if cmd, handled := m.layerKey(k); handled {
		return cmd, true
	}
	if k == "esc" {
		if cmd, ok := m.closeFetched(); ok {
			return cmd, true
		}
		if !m.isPreview {
			// Back to the list, with previews, from wherever the viewer is.
			m.opts.Preview = true
			return m.openMenu(m.index), true
		}
		return nil, true
	}
	if a := m.animation; a != nil {
		switch k {
		case "space":
			return m.toggleAnimation(), true
		case "<":
			a.setSpeed(a.speed - 1)
			return nil, true
		case ">":
			a.setSpeed(a.speed + 1)
			return nil, true
		case "backspace":
			a.setSpeed(0)
			return nil, true
		}
	}
	if m.chart != nil && m.mesh != nil && (k == "C" || k == "B") {
		m.openColorPicker(k == "B")
		m.layer = layerColor
		return nil, true
	}
	if m.hasParts() {
		switch k {
		case "c":
			m.partPicker = &partPicker{}
			m.layer = layerParts
			return nil, true
		case "X":
			return m.showParts(nil, false), true
		}
	}
	if m.sheet != nil {
		if k == "u" && m.sheet.url() != "" {
			return m.openQR(m.sheet.url()), true
		}
		if k == "c" {
			m.sheet.picker = &columnPicker{at: m.sheet.column()}
			m.layer = layerColumns
			return nil, true
		}
		if m.sheet.key(k, m.width, m.bodyHeight()-1) {
			return nil, true
		}
		if k == "enter" && m.sheet.url() != "" {
			return m.open(m.sheet.url()), true
		}
	}
	if m.markdown != nil {
		if m.markdown.key(k) {
			return nil, true
		}
		if k == "s" {
			m.markdown.raw = !m.markdown.raw
			m.markdown.offset = 0
			return m.layoutMarkdown(), true
		}
	}
	switch k {
	case "m":
		return m.openMenu(m.index), true
	case "]", "tab":
		return m.switchFile(1), true
	case "[", "shift+tab":
		return m.switchFile(-1), true
	case "R":
		return m.load(true), true
	case "e":
		return m.export(), true
	case "enter":
		if m.opts.Pick {
			return m.pick(), true
		}
		return nil, true
	case "i":
		return m.toggleInfo(), true
	case "5":
		// NTCharts3d binds the projection to o, which opens the browser here.
		if m.chart != nil {
			_, cmd := m.chart.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
			return cmd, true
		}
		return nil, true
	case "n", "space", "pgdown":
		if m.paged() {
			return m.movePage(m.page + 1), true
		}
		return m.switchFile(1), true
	case "p", "b", "pgup":
		if m.paged() {
			return m.movePage(m.page - 1), true
		}
		return m.switchFile(-1), true
	case "home":
		return m.movePage(1), true
	case "end", "G":
		return m.movePage(m.pages), true
	case "g":
		m.autoKitty = false
		if m.chart == nil {
			old := m.pic
			cmd := m.pic.Toggle()
			if m.animation != nil {
				if m.pic.Mode() == picture.PictureKitty {
					return m.refreshImage(), true
				}
				cleanup := old.SetImage(nil)
				m.retireAnimationPicture(cleanup)
				return tea.Batch(cmd, cleanup), true
			}
			if m.markdown != nil {
				cmd = tea.Batch(cmd, m.markdown.setKitty(m.pic.Mode() == picture.PictureKitty))
			}
			return cmd, true
		}
		return nil, false // The mesh has a glyph toggle of its own.
	case "r":
		if m.chart != nil {
			// NTCharts toggles rotation on r too, but it counts the key as
			// input and waits out its idle delay before turning, so the key
			// seems to do nothing. Set the camera instead: rotation starts
			// at once.
			cam := m.chart.Camera()
			cam.AutoRotate = !cam.AutoRotate
			return m.chart.SetCamera(cam), true
		}
		return m.pictureKey(k), true
	case "0", "f":
		if m.chart != nil {
			return m.chart.SetCamera(m.home), true
		}
		m.zoom, m.panX, m.panY = 0, 0, 0
		return m.refreshImage(), true
	}
	if m.chart != nil {
		return nil, false // Orbit, rotate, and the rest are the mesh's.
	}
	return m.pictureKey(k), true
}

// layerKey gives a layer over the document its keys, and reports whether
// it took the key. Esc closes it.
func (m *Model) layerKey(k string) (tea.Cmd, bool) {
	switch m.layer {
	case layerQR:
		switch k {
		case "esc", "u":
			return m.closeLayer(), true
		case "g":
			m.autoKitty = false
			return m.pic.Toggle(), true
		case "e":
			return m.exportQR(), true
		}
		return nil, true
	case layerNone:
		return nil, false
	case layerInfo:
		if k == "esc" {
			return m.closeLayer(), true
		}
		return nil, false // The document beneath goes on answering.
	case layerColumns:
		if m.sheet == nil {
			m.layer = layerNone
			return nil, false
		}
		if k == "esc" || k == "enter" || k == "c" {
			return m.closeLayer(), true
		}
		if m.sheet.picker != nil && m.sheet.picker.key(m.sheet, k) {
			return nil, true
		}
		return nil, true
	case layerParts:
		if k == "esc" {
			return m.closeLayer(), true
		}
		cmd, ok := m.partsKey(k)
		if m.partPicker == nil {
			m.layer = layerNone
		}
		return cmd, ok
	case layerColor:
		if k == "esc" {
			return m.closeLayer(), true
		}
		cmd, ok := m.colorKey(k)
		if m.colorPicker == nil {
			m.layer = layerNone
		}
		return cmd, ok
	}
	return nil, false
}

// pictureKey zooms and pans a picture, or reloads it.
func (m *Model) pictureKey(k string) tea.Cmd {
	switch k {
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
		return m.load(true)
	default:
		return nil
	}
	return m.refreshImage()
}
