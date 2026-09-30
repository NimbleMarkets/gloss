package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

// partPicker is the list of a 3MF's parts, each ticked when shown.
type partPicker struct {
	at, top int // The part under the cursor, and the first listed.
}

// partsResult is a mesh assembled from chosen parts, ready to show.
type partsResult struct {
	generation uint64
	shown      []bool
	mesh       *document.Mesh
	err        error
	focus      bool // Fit the camera to the mesh.
}

// pickingParts says whether the part list is open over a mesh.

// hasParts says whether the mesh on screen is made of parts to choose among.
func (m *Model) hasParts() bool { return m.chart != nil && len(m.parts) > 1 && m.assemble != nil }

// partShown says whether part i is on screen.
func (m *Model) partShown(i int) bool {
	return m.partsShown == nil || (i < len(m.partsShown) && m.partsShown[i])
}

func (m *Model) partsOnScreen() int {
	n := 0
	for i := range m.parts {
		if m.partShown(i) {
			n++
		}
	}
	return n
}

// partsKey handles a key while the part list is open, and reports whether
// it was one of its own. Escape is the viewer's, and closes the list there.
func (m *Model) partsKey(k string) (tea.Cmd, bool) {
	p := m.partPicker
	shown := make([]bool, len(m.parts))
	for i := range shown {
		shown[i] = m.partShown(i)
	}
	switch k {
	case "j", "down":
		p.at++
	case "k", "up":
		p.at--
	case "home", "g":
		p.at = 0
	case "end", "G":
		p.at = len(m.parts) - 1
	case "space":
		if shown[p.at] && m.partsOnScreen() == 1 {
			return nil, true // The last part stays: a mesh must show something.
		}
		shown[p.at] = !shown[p.at]
		return m.showParts(shown, false), true
	case "enter":
		for i := range shown {
			shown[i] = i == p.at
		}
		return m.showParts(shown, true), true
	case "n":
		for i := range shown {
			shown[i] = i == p.at
		}
		return m.showParts(shown, false), true
	case "a":
		return m.showParts(nil, false), true
	case "X":
		m.partPicker = nil
		return m.showParts(nil, false), true
	case "c":
		m.partPicker = nil
		return nil, true
	default:
		return nil, false
	}
	p.at = max(0, min(p.at, len(m.parts)-1))
	return nil, true
}

// showParts assembles the parts marked in shown, all of them when it is
// nil, and puts the mesh on the chart when it is ready.
func (m *Model) showParts(shown []bool, focus bool) tea.Cmd {
	same := shown == nil && m.partsShown == nil
	if shown != nil && m.partsShown != nil {
		same = true
		for i := range shown {
			same = same && shown[i] == m.partShown(i)
		}
	}
	if same && !focus {
		return nil
	}
	assemble, generation := m.assemble, m.generation
	return func() tea.Msg {
		mesh, err := assemble(shown)
		return partsResult{generation: generation, shown: shown, mesh: mesh, err: err, focus: focus}
	}
}

func (m *Model) assembled(r partsResult) tea.Cmd {
	if r.generation != m.generation || m.chart == nil {
		return nil
	}
	if r.err != nil {
		m.note = safe(r.err.Error())
		return nil
	}
	m.partsShown, m.triangles, m.mesh = r.shown, r.mesh.Triangles(), r.mesh
	if m.tint != nil {
		// Painted before the chart sees it, so that one frame shows both.
		r.mesh.Recolor(*m.tint, m.tintAll)
	}
	// The chart draws only through the commands it returns: every one must
	// reach the runtime, or it waits for a frame that never comes.
	shown := m.chart.SetSeries(r.mesh)
	if !r.focus {
		return shown
	}
	// The part fills the picture, seen from where the camera was.
	camera := m.chart.Camera()
	fit := document.View{Alpha: camera.Alpha, Beta: camera.Beta, Projection: camera.Projection}.Camera(r.mesh)
	return tea.Batch(shown, m.chart.SetCamera(fit))
}

// partsView lists the parts in a box no taller than h rows, the cursor
// kept in sight.
func (m *Model) partsView(w, h int) string {
	p := m.partPicker
	rows := h - 2
	if rows < 1 || w < 24 {
		return ""
	}
	p.top = max(0, min(p.top, p.at, len(m.parts)-rows))
	if p.at >= p.top+rows {
		p.top = p.at - rows + 1
	}
	lines := make([]string, 0, rows)
	for i := p.top; i < len(m.parts) && len(lines) < rows; i++ {
		tick := "[x]"
		if !m.partShown(i) {
			tick = "[ ]"
		}
		name := ansi.Truncate(safe(m.parts[i].Name), min(32, w-24), "…")
		line := fmt.Sprintf("%s %d  %s  %s △", tick, i+1, name, grouped(m.parts[i].Triangles))
		style := boxText
		if !m.partShown(i) {
			style = boxDim
		}
		if i == p.at {
			style = style.Reverse(true)
		}
		lines = append(lines, style.Render(strings.TrimRight(line, " ")))
	}
	return box(lines)
}

// meshFrame keeps the chart's picture and replaces the rows it writes
// above and below with gloss's own: the projection, and any trouble, on
// the first; the mouse and keys that work here on the last. How the mesh
// is drawn is the info box's to say.
func (m *Model) meshFrame(content string, w int) string {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return content
	}
	title := " " + m.chart.Camera().Projection.String()
	if err := m.chart.Err(); err != nil {
		title += " · " + safe(err.Error())
	}
	lines[0] = ansi.Truncate(title, w, "…")
	footer := " drag orbit · Shift-drag pan · wheel zoom · 5 projection · r rotate · f fit"
	if m.isPreview {
		footer = " drag orbit · wheel zoom" // Keys are the list's.
	}
	lines[len(lines)-1] = ansi.Truncate(footer, w, "")
	return strings.Join(lines, "\n")
}

// plural counts a noun.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return grouped(n) + " " + noun + "s"
}
