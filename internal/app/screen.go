package app

import tea "charm.land/bubbletea/v2"

// screen is what fills the body: one at a time, and said outright rather
// than read off which fields happen to be set. The help page and the
// quitting frame lie over any screen.
type screen int

const (
	screenDocument screen = iota // The file on show, or the drop target when there is none.
	screenList                   // The file list, with a preview beside it.
	screenGrid                   // The file list as thumbnails.
	screenBrowser                // The folder browser.
)

// layer is what floats over a document: at most one.
type layer int

const (
	layerNone layer = iota
	layerInfo
	layerColumns // The sheet's column list.
	layerParts   // The 3MF's part list.
	layerColor   // The mesh's color picker.
)

// listing says whether the file list is on screen, in either style.
func (m *Model) listing() bool { return m.screen == screenList || m.screen == screenGrid }

func (m *Model) browsing() bool { return m.screen == screenBrowser }

func (m *Model) pickingParts() bool { return m.layer == layerParts && m.chart != nil }
func (m *Model) pickingColor() bool { return m.layer == layerColor && m.chart != nil }
func (m *Model) showingInfo() bool  { return m.layer == layerInfo }

// closeLayer takes down whatever floats over the document, undoing a
// color still being chosen.
func (m *Model) closeLayer() tea.Cmd {
	was := m.layer
	m.layer = layerNone
	switch was {
	case layerColumns:
		if m.sheet != nil {
			m.sheet.picker = nil
		}
	case layerParts:
		m.partPicker = nil
	case layerColor:
		p := m.colorPicker
		m.colorPicker = nil
		if p == nil {
			return nil
		}
		if p.bg {
			if p.wasBg == nil {
				return m.resetBackground()
			}
			return m.setBackground(*p.wasBg)
		}
		if p.wasTint == nil {
			return m.unpaint()
		}
		return m.paint(*p.wasTint, p.wasAll)
	}
	return nil
}

// keepLayer takes a layer down without undoing anything: a color
// chosen is kept.
func (m *Model) keepLayer() {
	m.layer, m.colorPicker, m.partPicker = layerNone, nil, nil
	if m.sheet != nil {
		m.sheet.picker = nil
	}
}

// toggleInfo shows the details box, or hides it.
func (m *Model) toggleInfo() tea.Cmd {
	if m.layer == layerInfo {
		m.layer = layerNone
		return nil
	}
	cmd := m.closeLayer()
	m.layer = layerInfo
	return cmd
}

// showDocument returns to the file on show from the list or the browser.
func (m *Model) showDocument() { m.screen, m.opener = screenDocument, nil }
