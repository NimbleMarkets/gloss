package app

import (
	"image"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

func press(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(s)[0], Text: s} }

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
