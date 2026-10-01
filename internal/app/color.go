package app

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
)

// colorPicker chooses a paint for the mesh on screen, in one of three
// ways: a palette of swatches, sliders for each channel, or a hex number
// typed in. Each change is painted at once; Esc takes it back.
type colorPicker struct {
	r, g, b int
	mode    int // Palette, sliders, or hex.
	channel int // The slider in hand.
	hex     string
	palette int
	swatch  int
	all     bool // Paint every face, not only those the file left plain.
	wasTint *color.RGBA
	wasAll  bool
	browsed bool        // The swatch or palette has been moved, so Enter means it.
	bg      bool        // Choosing the background, not the mesh's paint.
	wasBg   *color.RGBA // The background before the picker opened.

	// Where the box and its parts were last drawn, for the mouse. Rows count
	// from the first line of text inside the border.
	at   image.Rectangle
	rows struct{ tabs, name, body, reset, scope int }
	drag bool // A slider is being dragged.
}

// The picker's three ways to choose, in the order of its tabs.
var pickerModes = []string{"Palette", "Sliders", "Hex"}

const (
	swatchCells = 5  // A swatch is four cells wide, with one between.
	barCells    = 20 // A slider's bar.
	barAt       = 4  // Where it starts: the marker, the channel's name and a space.
)

const (
	pickPalette = iota
	pickSliders
	pickHex
)

type palette struct {
	name     string
	swatches [16]color.RGBA
}

func rgb(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// Palettes: the colors filament comes in, brighter ones, and softer ones.
var palettes = []palette{
	{"Filament", [16]color.RGBA{rgb(0xf0f0f0), rgb(0x9e9e9e), rgb(0x3a3a3a), rgb(0x1a1a1a), rgb(0xdc322f), rgb(0xff8c00), rgb(0xf0c828), rgb(0x3caa50),
		rgb(0x2aa198), rgb(0x268bd2), rgb(0x6c71c4), rgb(0xd33682), rgb(0xff96be), rgb(0x8c5a32), rgb(0x64b4e6), rgb(0xc8a000)}},
	{"Bright", [16]color.RGBA{rgb(0xffffff), rgb(0xff0000), rgb(0xff7f00), rgb(0xffff00), rgb(0x7fff00), rgb(0x00ff00), rgb(0x00ff7f), rgb(0x00ffff),
		rgb(0x007fff), rgb(0x0000ff), rgb(0x7f00ff), rgb(0xff00ff), rgb(0xff007f), rgb(0xff5555), rgb(0x50fa7b), rgb(0xbd93f9)}},
	{"Soft", [16]color.RGBA{rgb(0xfdf6e3), rgb(0xeee8d5), rgb(0x93a1a1), rgb(0x586e75), rgb(0xf4a6a6), rgb(0xf9c99a), rgb(0xf7e7a1), rgb(0xb9e3b0),
		rgb(0xa8dcd9), rgb(0xa9c9ee), rgb(0xc3b8f0), rgb(0xf0b4d8), rgb(0xd9b99b), rgb(0xb0b0b0), rgb(0x8fb8de), rgb(0xd8c690)}},
}

// meshBackground is the color behind a mesh until another is chosen.
var meshBackground = color.RGBA{R: 24, G: 26, B: 30, A: 255}

// background is the color behind the mesh on screen.
func (m *Model) background() color.RGBA {
	if m.bg != nil {
		return *m.bg
	}
	return meshBackground
}

// setBackground puts the color behind the mesh.
func (m *Model) setBackground(c color.RGBA) tea.Cmd {
	m.bg = &c
	if m.chart == nil {
		return nil
	}
	return m.chart.SetBackground(c)
}

// resetBackground puts the default color back behind the mesh.
func (m *Model) resetBackground() tea.Cmd {
	m.bg = nil
	if m.chart == nil {
		return nil
	}
	return m.chart.SetBackground(meshBackground)
}

// Backdrop is the palette for the background: darks and lights that let a
// mesh stand out, then the brand colors.
var backdrop = palette{"Backdrop", [16]color.RGBA{rgb(0x181a1e), rgb(0x000000), rgb(0x2b2b2b), rgb(0x555555), rgb(0x999999), rgb(0xd0d0d0), rgb(0xf0f0f0), rgb(0xffffff),
	rgb(0x3f3080), rgb(0x655ba7), rgb(0xe24f36), rgb(0x4495aa), rgb(0xa7d7b1), rgb(0xfbf4a5), rgb(0x203040), rgb(0x30503a)}}

// palettesFor lists the palettes the picker offers: the background has its own first.
func (p *colorPicker) palettes() []palette {
	if p.bg {
		return append([]palette{backdrop}, palettes...)
	}
	return palettes
}

// pickingColor says whether the color picker is open over a mesh.

func (m *Model) openColorPicker(bg bool) {
	start := document.DefaultMeshColor()
	if m.tint != nil {
		start = *m.tint
	}
	if bg {
		start = m.background()
	}
	all := m.tintAll
	if !bg && m.tint == nil && m.mesh != nil && m.mesh.HasColors() {
		// A model with colors of its own is meant to be recolored whole;
		// Reset gives its colors back.
		all = true
	}
	p := &colorPicker{r: int(start.R), g: int(start.G), b: int(start.B), all: all, wasTint: m.tint, wasAll: m.tintAll, bg: bg, wasBg: m.bg}
	p.hex = p.hexString()
	m.colorPicker = p
}

func (p *colorPicker) color() color.RGBA {
	return color.RGBA{R: uint8(p.r), G: uint8(p.g), B: uint8(p.b), A: 255}
}

func (p *colorPicker) hexString() string { return fmt.Sprintf("#%02x%02x%02x", p.r, p.g, p.b) }

// paint gives the mesh the color, on the faces the scope says.
func (m *Model) paint(c color.RGBA, all bool) tea.Cmd {
	if m.mesh == nil || m.chart == nil {
		return nil
	}
	m.tint, m.tintAll = &c, all
	m.mesh.RestoreColors()
	m.mesh.Recolor(c, all)
	return m.chart.SetSeries(m.mesh)
}

// unpaint puts the file's own colors back.
func (m *Model) unpaint() tea.Cmd {
	if m.mesh == nil || m.chart == nil {
		return nil
	}
	m.tint, m.tintAll = nil, false
	m.mesh.RestoreColors()
	return m.chart.SetSeries(m.mesh)
}

// resetColor gives back what the picker started from: the file's own colors
// for a mesh, or the default backdrop. The picker stays open.
func (m *Model) resetColor(p *colorPicker) tea.Cmd {
	if p.bg {
		p.r, p.g, p.b = int(meshBackground.R), int(meshBackground.G), int(meshBackground.B)
		p.hex = p.hexString()
		return m.resetBackground()
	}
	p.r, p.g, p.b = int(document.DefaultMeshColor().R), int(document.DefaultMeshColor().G), int(document.DefaultMeshColor().B)
	p.hex = p.hexString()
	return m.unpaint()
}

// toggleScope paints all faces or only the plain ones, for a mesh whose file
// has colors of its own, and paints again with the choice.
func (m *Model) toggleScope(p *colorPicker) tea.Cmd {
	if p.bg || m.mesh == nil || !m.mesh.HasColors() {
		return nil
	}
	p.all = !p.all
	p.hex = p.hexString()
	return m.choose(p)
}

// finishColor closes the picker, keeping its color. In the palette, a swatch
// that has been moved to is chosen first; one never moved to is left alone.
func (m *Model) finishColor(p *colorPicker) tea.Cmd {
	var cmd tea.Cmd
	if p.mode == pickPalette && p.browsed {
		c := p.palettes()[p.palette].swatches[p.swatch]
		p.r, p.g, p.b = int(c.R), int(c.G), int(c.B)
		p.hex = p.hexString()
		cmd = m.choose(p)
	}
	m.colorPicker = nil
	return cmd
}

// colorMouse gives the picker the mouse where it is over the box, so that a
// click there does not orbit the mesh behind it. It reports whether it took
// the message.
func (m *Model) colorMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	p := m.colorPicker
	if p == nil || !m.pickingColor() || m.help {
		return nil, false
	}
	mouse := msg.Mouse()
	inside := image.Pt(mouse.X, mouse.Y).In(p.at)
	lx, ly := mouse.X-(p.at.Min.X+2), mouse.Y-(p.at.Min.Y+1)
	switch msg.(type) {
	case tea.MouseClickMsg:
		if !inside {
			return nil, false
		}
		if mouse.Button != tea.MouseLeft {
			return nil, true
		}
		return m.colorClick(p, lx, ly), true
	case tea.MouseMotionMsg:
		if p.drag {
			return m.setChannel(p, lx-barAt), true
		}
		return nil, inside
	case tea.MouseReleaseMsg:
		dragging := p.drag
		p.drag = false
		return nil, dragging || inside
	case tea.MouseWheelMsg:
		return nil, inside
	}
	return nil, false
}

// colorClick answers a left click at (lx, ly) in the box's text.
func (m *Model) colorClick(p *colorPicker, lx, ly int) tea.Cmd {
	if ly == p.rows.reset {
		return m.resetColor(p)
	}
	if ly == p.rows.scope && p.rows.scope >= 0 {
		return m.toggleScope(p)
	}
	if ly == p.rows.tabs {
		x := 0
		for i, name := range pickerModes {
			if lx >= x && lx < x+len(name)+2 {
				p.mode = i
				break
			}
			x += len(name) + 3
		}
		return nil
	}
	switch p.mode {
	case pickPalette:
		if ly == p.rows.name {
			n, w := len(p.palettes()), len(p.palettes()[p.palette].name)+4
			switch {
			case lx >= 0 && lx < 2:
				p.palette, p.browsed = (p.palette+n-1)%n, true
			case lx >= w-2 && lx < w:
				p.palette, p.browsed = (p.palette+1)%n, true
			}
			return nil
		}
		row, col := ly-p.rows.body, lx/swatchCells
		if row < 0 || row > 3 || lx < 0 || col > 3 || lx%swatchCells == swatchCells-1 {
			return nil
		}
		i := row*4 + col
		if p.browsed && i == p.swatch {
			return m.finishColor(p) // A second click on the swatch chooses it.
		}
		p.swatch, p.browsed = i, true
		c := p.palettes()[p.palette].swatches[i]
		p.r, p.g, p.b = int(c.R), int(c.G), int(c.B)
		p.hex = p.hexString()
		return m.choose(p)
	case pickSliders:
		if row := ly - p.rows.body; row >= 0 && row < 3 {
			p.channel = row
			if lx >= barAt && lx < barAt+barCells {
				p.drag = true
				return m.setChannel(p, lx-barAt)
			}
		}
	}
	return nil
}

// setChannel sets the slider in hand to where cell k of its bar is.
func (m *Model) setChannel(p *colorPicker, k int) tea.Cmd {
	v := max(0, min(barCells-1, k)) * 255 / (barCells - 1)
	*[]*int{&p.r, &p.g, &p.b}[p.channel] = v
	p.hex = p.hexString()
	return m.choose(p)
}

// choose applies the picker's color to what it is choosing: the mesh or its background.
func (m *Model) choose(p *colorPicker) tea.Cmd {
	if p.bg {
		return m.setBackground(p.color())
	}
	return m.paint(p.color(), p.all)
}

// colorKey handles a key while the picker is open, and reports whether it
// was one of its own. Esc is the viewer's: it closes the picker and takes
// the paint back.
func (m *Model) colorKey(k string) (tea.Cmd, bool) {
	p := m.colorPicker
	apply := func() tea.Cmd { p.hex = p.hexString(); return m.choose(p) }
	switch k {
	case "enter":
		// In the palette, Enter chooses the swatch it is on, as Space does,
		// and then closes; the sliders and hex have applied theirs already.
		// Until the highlight has been moved, it is only where it started,
		// and Enter leaves the color alone.
		return m.finishColor(p), true
	case "tab":
		p.mode = (p.mode + 1) % 3
		return nil, true
	case "shift+tab":
		p.mode = (p.mode + 2) % 3
		return nil, true
	case "r":
		return m.resetColor(p), true
	case "s":
		return m.toggleScope(p), true
	}
	switch p.mode {
	case pickPalette:
		switch k {
		case "j", "down":
			p.swatch, p.browsed = min(15, p.swatch+4), true
		case "k", "up":
			p.swatch, p.browsed = max(0, p.swatch-4), true
		case "l", "right":
			p.swatch, p.browsed = min(15, p.swatch+1), true
		case "h", "left":
			p.swatch, p.browsed = max(0, p.swatch-1), true
		case "]", "n":
			p.palette, p.browsed = (p.palette+1)%len(p.palettes()), true
		case "[", "p":
			p.palette, p.browsed = (p.palette+len(p.palettes())-1)%len(p.palettes()), true
		case "space":
			c := p.palettes()[p.palette].swatches[p.swatch]
			p.r, p.g, p.b = int(c.R), int(c.G), int(c.B)
			return apply(), true
		default:
			return nil, true
		}
		return nil, true
	case pickSliders:
		step := 0
		switch k {
		case "j", "down":
			p.channel = min(2, p.channel+1)
		case "k", "up":
			p.channel = max(0, p.channel-1)
		case "l", "right":
			step = 13
		case "h", "left":
			step = -13
		case "L", "shift+right":
			step = 1
		case "H", "shift+left":
			step = -1
		default:
			if len(k) == 1 && k[0] >= '0' && k[0] <= '9' {
				v := int(k[0]-'0') * 255 / 9
				*[]*int{&p.r, &p.g, &p.b}[p.channel] = v
				return apply(), true
			}
			return nil, true
		}
		if step != 0 {
			at := []*int{&p.r, &p.g, &p.b}[p.channel]
			*at = max(0, min(255, *at+step))
			return apply(), true
		}
		return nil, true
	default:
		switch {
		case k == "backspace":
			if len(p.hex) > 1 {
				p.hex = p.hex[:len(p.hex)-1]
			}
		case len(k) == 1 && strings.ContainsRune("0123456789abcdefABCDEF", rune(k[0])):
			if len(p.hex) >= 7 {
				p.hex = "#" // A full number typed over starts afresh.
			}
			p.hex += strings.ToLower(k)
		default:
			return nil, true
		}
		if c, err := document.ParseColor(p.hex); err == nil && len(p.hex) == 7 {
			p.r, p.g, p.b = int(c.R), int(c.G), int(c.B)
			return m.choose(p), true
		}
		return nil, true
	}
}

// colorView draws the picker in a corner box.
func (m *Model) colorView(w, h int) string {
	p := m.colorPicker
	if h < 12 || w < 40 {
		p.at = image.Rectangle{}
		return ""
	}
	c := p.color()
	title := "Color"
	if p.bg {
		title = "Background"
	}
	text, dim := boxText, boxDim
	swatch := lipgloss.NewStyle().Background(c).Foreground(contrast(c)).Padding(0, 1).Render(p.hexString())
	tabs := make([]string, 3)
	for i, name := range pickerModes {
		if i == p.mode {
			tabs[i] = text.Bold(true).Reverse(true).Render(" " + name + " ")
		} else {
			tabs[i] = dim.Render(" " + name + " ")
		}
	}
	lines := []string{
		text.Bold(true).Render(title) + text.Render("  ") + swatch,
		strings.Join(tabs, text.Render(" ")),
		text.Render(""),
	}
	p.rows.tabs = 1
	switch p.mode {
	case pickPalette:
		p.rows.name, p.rows.body = len(lines), len(lines)+1
		lines = append(lines, dim.Render("[ ")+text.Render(p.palettes()[p.palette].name)+dim.Render(" ]"))
		for row := 0; row < 4; row++ {
			line := ""
			for col := 0; col < 4; col++ {
				i, s := row*4+col, p.palettes()[p.palette].swatches[row*4+col]
				mark := "    "
				if i == p.swatch {
					mark = " ▪▪ "
				}
				line += lipgloss.NewStyle().Background(s).Foreground(contrast(s)).Render(mark) + text.Render(" ")
			}
			lines = append(lines, line)
		}
	case pickSliders:
		p.rows.body = len(lines)
		for i, name := range []string{"R", "G", "B"} {
			v := []int{p.r, p.g, p.b}[i]
			var fill color.RGBA
			switch i {
			case 0:
				fill = color.RGBA{R: uint8(v), A: 255}
			case 1:
				fill = color.RGBA{G: uint8(v), A: 255}
			default:
				fill = color.RGBA{B: uint8(v), A: 255}
			}
			filled := v * 20 / 255
			bar := lipgloss.NewStyle().Background(fill).Render(strings.Repeat(" ", filled)) + lipgloss.NewStyle().Background(lipgloss.Color("238")).Render(strings.Repeat(" ", 20-filled))
			mark := "  "
			if i == p.channel {
				mark = "▸ "
			}
			lines = append(lines, text.Render(mark+name+" ")+bar+text.Render(fmt.Sprintf(" %3d", v)))
		}
	default:
		lines = append(lines, text.Render("Type a color: ")+text.Reverse(true).Render(p.hex+strings.Repeat(" ", 7-len(p.hex))))
		if len(p.hex) < 7 {
			lines = append(lines, dim.Render("#rrggbb"))
		}
	}
	what := "file's colors"
	if p.bg {
		what = "default"
	}
	lines = append(lines, text.Render(""))
	p.rows.reset, p.rows.scope = len(lines), -1
	lines = append(lines, text.Bold(true).Reverse(true).Render(" Reset ")+dim.Render("  r  ")+text.Render(what))
	if !p.bg && m.mesh != nil && m.mesh.HasColors() {
		scope := "plain faces only"
		if p.all {
			scope = "all faces"
		}
		p.rows.scope = len(lines)
		lines = append(lines, dim.Render("s  paint ")+text.Render(scope))
	}
	b := box(lines)
	p.at = image.Rect(w-lipgloss.Width(b), 1, w, 1+lipgloss.Height(b))
	return b
}

// contrast is black or white, whichever reads on c.
func contrast(c color.RGBA) color.Color {
	if (299*int(c.R)+587*int(c.G)+114*int(c.B))/1000 > 128 {
		return lipgloss.Color("0")
	}
	return lipgloss.Color("15")
}
