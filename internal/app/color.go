package app

import (
	"fmt"
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
}

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

// pickingColor says whether the color picker is open over a mesh.

func (m *Model) openColorPicker() {
	start := document.DefaultMeshColor()
	if m.tint != nil {
		start = *m.tint
	}
	p := &colorPicker{r: int(start.R), g: int(start.G), b: int(start.B), all: m.tintAll, wasTint: m.tint, wasAll: m.tintAll}
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

// colorKey handles a key while the picker is open, and reports whether it
// was one of its own. Esc is the viewer's: it closes the picker and takes
// the paint back.
func (m *Model) colorKey(k string) (tea.Cmd, bool) {
	p := m.colorPicker
	apply := func() tea.Cmd { p.hex = p.hexString(); return m.paint(p.color(), p.all) }
	switch k {
	case "enter":
		m.colorPicker = nil
		return nil, true
	case "tab":
		p.mode = (p.mode + 1) % 3
		return nil, true
	case "shift+tab":
		p.mode = (p.mode + 2) % 3
		return nil, true
	case "r":
		p.r, p.g, p.b = int(document.DefaultMeshColor().R), int(document.DefaultMeshColor().G), int(document.DefaultMeshColor().B)
		p.hex = p.hexString()
		return m.unpaint(), true
	case "s":
		if m.mesh != nil && m.mesh.HasColors() {
			p.all = !p.all
			return apply(), true
		}
		return nil, true
	}
	switch p.mode {
	case pickPalette:
		switch k {
		case "j", "down":
			p.swatch = min(15, p.swatch+4)
		case "k", "up":
			p.swatch = max(0, p.swatch-4)
		case "l", "right":
			p.swatch = min(15, p.swatch+1)
		case "h", "left":
			p.swatch = max(0, p.swatch-1)
		case "]", "n":
			p.palette = (p.palette + 1) % len(palettes)
		case "[", "p":
			p.palette = (p.palette + len(palettes) - 1) % len(palettes)
		case "space":
			c := palettes[p.palette].swatches[p.swatch]
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
			return m.paint(c, p.all), true
		}
		return nil, true
	}
}

// colorView draws the picker in a corner box.
func (m *Model) colorView(w, h int) string {
	p := m.colorPicker
	if h < 12 || w < 40 {
		return ""
	}
	c := p.color()
	text, dim := boxText, boxDim
	swatch := lipgloss.NewStyle().Background(c).Foreground(contrast(c)).Padding(0, 1).Render(p.hexString())
	tabs := make([]string, 3)
	for i, name := range []string{"Palette", "Sliders", "Hex"} {
		if i == p.mode {
			tabs[i] = text.Bold(true).Reverse(true).Render(" " + name + " ")
		} else {
			tabs[i] = dim.Render(" " + name + " ")
		}
	}
	lines := []string{
		text.Bold(true).Render("Color") + text.Render("  ") + swatch,
		strings.Join(tabs, text.Render(" ")),
		text.Render(""),
	}
	switch p.mode {
	case pickPalette:
		lines = append(lines, dim.Render("[ ")+text.Render(palettes[p.palette].name)+dim.Render(" ]"))
		for row := 0; row < 4; row++ {
			line := ""
			for col := 0; col < 4; col++ {
				i, s := row*4+col, palettes[p.palette].swatches[row*4+col]
				mark := "    "
				if i == p.swatch {
					mark = " ▪▪ "
				}
				line += lipgloss.NewStyle().Background(s).Foreground(contrast(s)).Render(mark) + text.Render(" ")
			}
			lines = append(lines, line)
		}
	case pickSliders:
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
	foot := "r file's colors"
	if m.mesh != nil && m.mesh.HasColors() {
		foot += " · s plain faces"
		if p.all {
			foot = "r file's colors · s all faces"
		}
	}
	lines = append(lines, text.Render(""), dim.Render(foot))
	return box(lines)
}

// contrast is black or white, whichever reads on c.
func contrast(c color.RGBA) color.Color {
	if (299*int(c.R)+587*int(c.G)+114*int(c.B))/1000 > 128 {
		return lipgloss.Color("0")
	}
	return lipgloss.Color("15")
}
