package app

import (
	"image"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/charmbracelet/x/ansi"
)

func press(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(s)[0], Text: s} }

func TestMouseOrbitDirection(t *testing.T) {
	m := New(Options{Files: []string{"model.stl"}, Render: "glyph"})
	m.width, m.height = 80, 30
	m.chart = charts.New(80, 28, charts.WithRenderMode(charts.Software))
	defer m.Close()
	m.chart.View() // NTCharts registers its mouse zone asynchronously.
	before := m.chart.Camera()
	for i := 0; i < 100; i++ {
		m.Update(tea.MouseClickMsg{X: 20, Y: 10, Button: tea.MouseLeft})
		m.Update(tea.MouseMotionMsg{X: 23, Y: 12, Button: tea.MouseLeft})
		m.Update(tea.MouseReleaseMsg{X: 23, Y: 12, Button: tea.MouseLeft})
		if m.chart.Camera().Beta != before.Beta {
			break
		}
		time.Sleep(time.Millisecond)
	}
	after := m.chart.Camera()
	if after.Beta != before.Beta-6 || after.Alpha != before.Alpha+4 {
		t.Fatalf("orbit: before=%+v after=%+v", before, after)
	}
	m.Update(tea.MouseClickMsg{X: 20, Y: 10, Button: tea.MouseLeft, Mod: tea.ModShift})
	m.Update(tea.MouseMotionMsg{X: 23, Y: 10, Button: tea.MouseLeft, Mod: tea.ModShift})
	m.Update(tea.MouseReleaseMsg{X: 23, Y: 10, Button: tea.MouseLeft, Mod: tea.ModShift})
	pan := m.chart.Camera()
	if pan.Beta != after.Beta || pan.Target == after.Target {
		t.Fatal("shift-drag must pan without orbiting")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if m.chart.Camera().Beta != pan.Beta+5 {
		t.Fatal("keyboard orbit direction changed")
	}
}

func TestCropPanningAndNonzeroOrigin(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 20, 110, 100))
	src.Set(10, 20, color.RGBA{R: 255, A: 255})
	src.Set(109, 99, color.RGBA{B: 255, A: 255})
	left := crop(src, 1, -100, -100)
	if left.Bounds().Dx() != 50 || left.Bounds().Dy() != 40 {
		t.Fatal(left.Bounds())
	}
	r, _, _, _ := left.At(0, 0).RGBA()
	if r != 65535 {
		t.Fatal("pan did not clamp to upper left")
	}
	right := crop(src, 6, 100, 100)
	_, _, b, _ := right.At(0, 0).RGBA()
	if b != 65535 {
		t.Fatal("pan did not clamp to lower right")
	}
}

func TestNavigationAndStaleResults(t *testing.T) {
	m := New(Options{Files: []string{"a.pdf", "b.svg"}, Page: 1, DPI: 72, Render: "glyph"})
	defer m.Close()
	m.kind, m.pages, m.generation = "pdf", 3, 1
	m.Update(press("n"))
	if m.page != 2 || !m.loading {
		t.Fatal("next page not queued")
	}
	m.Update(document.Result{Generation: 1, Kind: "png", Page: 1, Pages: 1})
	if m.kind != "pdf" || m.page != 2 {
		t.Fatal("stale result overwrote current page")
	}
	m.Update(press("]"))
	if m.index != 1 || m.page != 1 || m.kind != "" {
		t.Fatal("file navigation did not reset state")
	}
	gen := m.generation
	m.Update(press("]"))
	if m.generation != gen {
		t.Fatal("advanced past final file")
	}
	m.Update(press("["))
	if m.index != 0 {
		t.Fatal("previous file")
	}
}

func TestViewFitsAndSanitizes(t *testing.T) {
	m := New(Options{Files: []string{"evil\x1b]2;title\a.png"}, Render: "glyph", Page: 1})
	defer m.Close()
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 10, Height: 4}, {Width: 1, Height: 1}} {
		m.Update(size)
		m.help = true
		view := m.View()
		lines := strings.Split(view.Content, "\n")
		if len(lines) > size.Height {
			t.Fatalf("view height %d exceeds %d", len(lines), size.Height)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size.Width {
				t.Fatalf("line exceeds width: %q", line)
			}
		}
		if strings.Contains(view.Content, "\x1b]2;") {
			t.Fatal("filename injected terminal escape")
		}
	}
}

func TestHelpFitsAStandardTerminal(t *testing.T) {
	m := New(Options{Files: []string{"a.png"}, Render: "glyph", Page: 1})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(press("?"))
	view := m.View().Content
	for _, want := range []string{"quit", "file menu", "browse", "Drop files", "PDF page", "zoom", "orbit", "fit", "Kitty", "reload", "export", "details", "Shift-drag", "auto-rotate", "orthographic", "source", "gloss --help"} {
		if !strings.Contains(view, want) {
			t.Errorf("help cut off before %q", want)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 80 {
			t.Errorf("line exceeds the terminal: %q", line)
		}
	}
}

func TestRTogglesMeshRotationAtOnce(t *testing.T) {
	m := painted(t)
	defer m.Close()
	if m.chart.Camera().AutoRotate {
		t.Fatal("a mesh starts still")
	}
	_, cmd := m.Update(press("r"))
	if !m.chart.Camera().AutoRotate || cmd == nil {
		t.Fatalf("r did not start rotation: %+v cmd=%v", m.chart.Camera(), cmd != nil)
	}
	m.Update(press("r"))
	if m.chart.Camera().AutoRotate {
		t.Fatal("a second r did not stop rotation")
	}
}

// The help ends on the address of the documentation, as a hyperlink.
func TestHelpLinksToTheDocs(t *testing.T) {
	m := New(Options{Files: []string{"a.png"}, Render: "glyph", Page: 1})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(press("?"))
	view := m.View().Content
	link := ansi.SetHyperlink(DocsURL) + DocsURL + ansi.ResetHyperlink()
	if !strings.Contains(view, link) {
		t.Fatalf("no hyperlink to %s in the help:\n%q", DocsURL, view)
	}
	// It is a line of the help itself, above the status bar, and the screen still fits.
	lines := strings.Split(ansi.Strip(view), "\n")
	at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, DocsURL) })
	if at < 0 || at >= len(lines)-2 || len(lines) > 24 {
		t.Fatalf("the docs are on line %d of %d:\n%s", at, len(lines), strings.Join(lines, "\n"))
	}
}
