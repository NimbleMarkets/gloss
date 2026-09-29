package app

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

var landscape = []document.Field{
	{Label: "File"}, {Label: "Path", Value: "photos/landscape.png"}, {Label: "Size", Value: "5.0 KiB (5,120 bytes)"},
	{Label: "Image"}, {Label: "Format", Value: "PNG"}, {Label: "Dimensions", Value: "640 × 400 (0.3 MP)"},
	{Label: "Photo"}, {Label: "Camera", Value: "Acme\x1b]2;owned\a Snap 3"},
}

func loaded(t *testing.T, files ...string) *Model {
	t.Helper()
	m := viewing(t, Options{Files: files}, "")
	picture := image.NewRGBA(image.Rect(0, 0, 64, 40))
	draw.Draw(picture, picture.Bounds(), image.NewUniform(color.RGBA{R: 200, G: 80, B: 40, A: 255}), image.Point{}, draw.Src)
	m.Update(document.Result{Generation: m.generation, Kind: "png", Page: 1, Pages: 1, Image: picture, Info: landscape})
	return m
}

// rows returns the view as plain text, one string per terminal row.
func rows(m *Model) []string { return strings.Split(ansi.Strip(m.View().Content), "\n") }

func TestInfoKeyOpensABoxInTheCorner(t *testing.T) {
	m := loaded(t, "photos/landscape.png")
	if strings.Contains(m.View().Content, "Dimensions") {
		t.Fatal("the box is open before it was asked for")
	}
	before := rows(m)
	m.Update(press("i"))
	view := m.View().Content
	for _, want := range []string{"File", "photos/landscape.png", "5.0 KiB (5,120 bytes)", "Image", "Dimensions", "640 × 400 (0.3 MP)", "Camera", "Snap 3"} {
		if !strings.Contains(view, want) {
			t.Fatalf("box lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "\x1b]2;") {
		t.Fatal("a value from the file injected a terminal escape")
	}
	after := rows(m)
	if len(after) != len(before) {
		t.Fatalf("the box changed the view from %d to %d rows", len(before), len(after))
	}
	top := after[0]
	if !strings.HasSuffix(strings.TrimRight(top, " "), "╮") || ansi.StringWidth(top) != 100 {
		t.Fatalf("the box is not against the right edge: %q", top)
	}
	box := strings.Index(top, "╭")
	if box <= 0 || strings.TrimSpace(after[1][:box]) == "" || after[1][:box] != before[1][:box] {
		t.Fatalf("the picture beside the box was disturbed:\n%q\n%q", before[1], after[1])
	}
	if last := len(after) - 3; after[last] != before[last] {
		t.Fatalf("the picture below the box was disturbed:\n%q\n%q", before[last], after[last])
	}
	m.Update(press("i"))
	if strings.Contains(m.View().Content, "Dimensions") {
		t.Fatal("i did not close the box")
	}
	m.Update(press("i"))
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if strings.Contains(m.View().Content, "Dimensions") {
		t.Fatal("Esc did not close the box")
	}
}

func TestInfoBoxSitsBelowTheTitleOfAMesh(t *testing.T) {
	m := viewing(t, Options{Files: []string{"part.stl"}}, "stl")
	m.chart = charts.New(100, 28, charts.WithRenderMode(charts.Software))
	m.fields = landscape
	title := rows(m)[0]
	if !strings.Contains(title, "ntcharts3d") {
		t.Fatalf("the mesh view has no title to protect: %q", title)
	}
	m.Update(press("i"))
	after := rows(m)
	if after[0] != title {
		t.Fatalf("the box covers the title:\n%q\n%q", title, after[0])
	}
	if !strings.HasSuffix(strings.TrimRight(after[1], " "), "╮") {
		t.Fatalf("the box does not start on the second row: %q", after[1])
	}
	// One row fewer is left for the box, which must still close.
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 9})
	m.chart.SetSize(100, 7)
	after = rows(m)
	if len(after) != 9 || after[0] != rows(m)[0] || !strings.Contains(after[6], "╰") {
		t.Fatalf("a short terminal:\n%s", strings.Join(after, "\n"))
	}
}

func TestInfoBoxFitsTheTerminal(t *testing.T) {
	m := loaded(t, "photos/landscape.png")
	m.Update(press("i"))
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 40, Height: 12}, {Width: 24, Height: 6}, {Width: 9, Height: 4}, {Width: 1, Height: 1}} {
		m.Update(size)
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) > size.Height {
			t.Fatalf("view height %d exceeds %d", len(lines), size.Height)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size.Width {
				t.Fatalf("line exceeds width %d: %q", size.Width, line)
			}
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 9})
	view := m.View().Content
	if !strings.Contains(view, "Path") || strings.Contains(view, "Camera") || !strings.Contains(view, "…") {
		t.Fatalf("a box taller than the view must be cut short, and say so:\n%s", view)
	}
}

func TestInfoBoxKeepsTheEndOfALongPath(t *testing.T) {
	m := loaded(t, "deep.png")
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	m.fields = []document.Field{{Label: "File"}, {Label: "Path", Value: "/private/tmp/a/very/long/way/down/into/the/tree/holiday/deep.png"}, {Label: "Producer", Value: "An Extremely Long-Winded Document Production System 12.4.1"}}
	m.Update(press("i"))
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "…") || !strings.Contains(view, "holiday/deep.png") || strings.Contains(view, "/private/tmp") {
		t.Fatalf("path:\n%s", view)
	}
	if !strings.Contains(view, "An Extremely") || strings.Contains(view, "12.4.1") {
		t.Fatalf("other values keep their beginning:\n%s", view)
	}
}

func TestInfoBoxLeavesTheDocumentInUse(t *testing.T) {
	m := loaded(t, "a.pdf", "b.png")
	m.Update(press("i"))
	m.Update(press("+"))
	if m.zoom != 1 || !m.info {
		t.Fatalf("zoom=%d info=%v", m.zoom, m.info)
	}
	m.Update(document.Result{Generation: m.generation, Kind: "pdf", Page: 2, Pages: 3, Info: []document.Field{{Label: "Document"}, {Label: "Page size", Value: "595 × 842 pt"}}})
	if view := m.View().Content; !strings.Contains(view, "595 × 842 pt") || strings.Contains(view, "Dimensions") {
		t.Fatalf("the box shows the previous page:\n%s", view)
	}
	m.Update(press("]"))
	if m.index != 1 || !m.info {
		t.Fatalf("index=%d info=%v", m.index, m.info)
	}
	if view := m.View().Content; strings.Contains(view, "595 × 842 pt") {
		t.Fatalf("the box shows the previous file:\n%s", view)
	}
	m.Update(press("?"))
	if view := m.View().Content; !strings.Contains(view, "a visual pager") || strings.Contains(view, "╭") {
		t.Fatalf("help must not be covered:\n%s", view)
	}
	m.Update(press("?"))
	m.Update(press("m"))
	if view := m.View().Content; !m.menu || strings.Contains(view, "╭") {
		t.Fatalf("the menu must not be covered:\n%s", view)
	}
}

func TestInfoBoxDescribesAFailedFile(t *testing.T) {
	m := viewing(t, Options{Files: []string{"poster.png"}}, "")
	m.Update(document.Result{Generation: m.generation, Err: errors.New("image exceeds 33554432 pixels"), Info: []document.Field{{Label: "Image"}, {Label: "Dimensions", Value: "9000 × 8000 (72.0 MP)"}}})
	m.Update(press("i"))
	if view := m.View().Content; !strings.Contains(view, "9000 × 8000") || !strings.Contains(view, "image exceeds 33554432 pixels") {
		t.Fatalf("view:\n%s", view)
	}
}

func TestInfoKeyNeedsADocument(t *testing.T) {
	empty := viewing(t, Options{}, "")
	empty.Update(press("i"))
	if view := empty.View().Content; !strings.Contains(view, "Drop files here to open") || strings.Contains(view, "╭") {
		t.Fatalf("empty session:\n%s", view)
	}
	menu := viewing(t, Options{Files: []string{"a.png", "b.png"}, Menu: true}, "")
	menu.Update(press("i"))
	if !menu.menu || menu.info {
		t.Fatal("i left the menu")
	}
}

func TestOverlayKeepsKittyCellsWhole(t *testing.T) {
	var grid strings.Builder
	for y := range 3 {
		grid.WriteString("\x1b[38;2;0;0;7m")
		for x := range 12 {
			grid.WriteRune(kitty.Placeholder)
			grid.WriteRune(kitty.Diacritic(y))
			grid.WriteRune(kitty.Diacritic(x))
		}
		grid.WriteString("\x1b[39m\n")
	}
	body := strings.TrimSuffix(grid.String(), "\n")
	out := strings.Split(overlay(body, "abcd\nefgh", 12, 0), "\n")
	if len(out) != 3 || out[2] != strings.Split(body, "\n")[2] {
		t.Fatalf("rows below the box changed: %q", out)
	}
	if lower := strings.Split(overlay(body, "abcd\nefgh", 12, 1), "\n"); lower[0] != strings.Split(body, "\n")[0] || !strings.HasSuffix(lower[1], "abcd") || !strings.HasSuffix(lower[2], "efgh") {
		t.Fatalf("a box moved down a row: %q", lower)
	}
	for y, text := range []string{"abcd", "efgh"} {
		if !strings.HasSuffix(out[y], text) || ansi.StringWidth(out[y]) != 12 {
			t.Fatalf("row %d: %q", y, out[y])
		}
		// Eight image cells remain, each still naming its own row and column.
		for x := range 8 {
			cell := string([]rune{kitty.Placeholder, kitty.Diacritic(y), kitty.Diacritic(x)})
			if strings.Count(out[y], cell) != 1 {
				t.Fatalf("row %d lost cell %d: %q", y, x, out[y])
			}
		}
		if strings.Count(out[y], string(kitty.Placeholder)) != 8 {
			t.Fatalf("row %d shows the image under the box: %q", y, out[y])
		}
	}
	short := overlay("ab\ncd", "XYZ", 8, 0)
	if got := strings.Split(ansi.Strip(short), "\n")[0]; got != "ab   XYZ" {
		t.Fatalf("a short row must be padded out to the box: %q", got)
	}
}
