package app

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"github.com/charmbracelet/x/ansi"
)

// infoBox draws the details of the current file, no larger than w by h cells.
// It returns "" when the terminal has no room for a box.
func (m *Model) infoBox(w, h int) string {
	inner, rows := min(56, w-8), h-2
	if inner < 16 || rows < 2 {
		return ""
	}
	fields := append(append([]document.Field(nil), m.fields...), m.terminalFields()...)
	labels := 0
	for _, f := range fields {
		if f.Value != "" {
			labels = max(labels, lipgloss.Width(f.Label))
		}
	}
	text := boxText
	heading, label := text.Bold(true).Foreground(lipgloss.Color(colorNimbleYellow)), boxDim
	var lines []string
	for _, f := range fields {
		if f.Value == "" {
			lines = append(lines, heading.Render(ansi.Truncate(safe(f.Label), inner, "…")))
			continue
		}
		name, value := safe(f.Label), safe(f.Value)
		room := max(1, inner-labels-3)
		if over := ansi.StringWidth(value) - room; over > 0 && f.Label == "Path" {
			// The name at the end of a path says more than the root at its start.
			value = "…" + ansi.TruncateLeft(value, over+1, "")
		}
		value = ansi.Truncate(value, room, "…")
		lines = append(lines, label.Render(" "+name+strings.Repeat(" ", labels-ansi.StringWidth(name)+2))+text.Render(value))
	}
	if len(lines) == 0 {
		lines = []string{text.Render("No details yet.")}
	}
	if len(lines) > rows {
		lines = append(lines[:rows-1], text.Render("…"))
	}
	return box(lines)
}

// terminalFields describes the terminal and how gloss draws on it: what
// the box says when there is no file, and what it ends with when there is.
func (m *Model) terminalFields() []document.Field {
	size := fmt.Sprintf("%d × %d cells", m.width, m.height)
	if cw, ch := m.pic.CellPixelSize(); cw > 1 && ch > 1 {
		size += fmt.Sprintf(", %d × %d pixels each", cw, ch)
	}
	pictures := "glyphs"
	if m.pic.Mode() == picture.PictureKitty {
		pictures = "Kitty graphics"
	}
	if m.opts.Render == "glyph" {
		pictures += " (asked for)"
	}
	kitty := map[picture.KittyCapability]string{
		picture.KittyCapabilitySupported:   "supported",
		picture.KittyCapabilityUnsupported: "not supported",
	}[m.pic.KittySupported()]
	if kitty == "" {
		kitty = "not yet known"
	}
	meshes := map[string]string{"software": "software (asked for)", "wireframe": "wireframe (asked for)"}[m.opts.Render3D]
	if meshes == "" {
		meshes = "WebGPU, or software without it"
	}
	if m.chart != nil {
		meshes = m.chart.RenderMode().String()
	}
	program := os.Getenv("TERM_PROGRAM")
	if program != "" {
		program = strings.TrimSpace(program + " " + os.Getenv("TERM_PROGRAM_VERSION"))
	} else {
		program = os.Getenv("TERM")
	}
	multiplexer := ""
	if os.Getenv("TMUX") != "" {
		multiplexer = "tmux"
	}
	screen := "alternate"
	if m.opts.KeepScreen {
		screen = "main, kept after quitting"
	}
	return document.Section("Terminal",
		document.Field{Label: "Size", Value: size},
		document.Field{Label: "Pictures", Value: pictures},
		document.Field{Label: "Kitty graphics", Value: kitty},
		document.Field{Label: "Meshes", Value: meshes},
		document.Field{Label: "Program", Value: program},
		document.Field{Label: "Multiplexer", Value: multiplexer},
		document.Field{Label: "Screen", Value: screen},
	)
}

// The corner boxes' colours: a shade, plain text on it, and dim text.
var (
	boxShade = lipgloss.Color(colorNimblePurpleDark)
	boxText  = lipgloss.NewStyle().Background(boxShade).Foreground(lipgloss.Color(colorNimbleText))
	boxDim   = boxText.Foreground(lipgloss.Color(colorNimbleDim))
)

// box frames styled lines as a shaded corner box, each padded to the widest.
func box(lines []string) string {
	return boxWithPadding(lines, 1)
}

func boxWithPadding(lines []string, padding int) string {
	shade, text := boxShade, boxText
	width := 0
	for _, line := range lines {
		width = max(width, lipgloss.Width(line))
	}
	for i, line := range lines {
		lines[i] = line + text.Render(strings.Repeat(" ", width-lipgloss.Width(line)))
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(frameColor).BorderBackground(shade).
		Background(shade).Padding(0, padding).Render(strings.Join(lines, "\n"))
}

// overlay lays box over the right of body, which is w cells wide, top rows
// below its first.
// Rows are cut between cells, so a Kitty image keeps the placeholders that
// remain visible and gives up only those the box covers.
func overlay(body, box string, w, top int) string {
	if box == "" {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, row := range strings.Split(box, "\n") {
		i += top
		if i >= len(lines) {
			break
		}
		x := max(0, w-ansi.StringWidth(row))
		left := ansi.Truncate(lines[i], x, "")
		lines[i] = left + "\x1b[m" + strings.Repeat(" ", max(0, x-ansi.StringWidth(left))) + row
	}
	return strings.Join(lines, "\n")
}
