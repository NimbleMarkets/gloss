package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

var landscape = []document.Field{
	{Label: "File"}, {Label: "Path", Value: "photos/landscape.png"}, {Label: "Size", Value: "5.0 KiB (5,120 bytes)"},
	{Label: "Image"}, {Label: "Format", Value: "PNG"}, {Label: "Dimensions", Value: "640 × 400 (0.3 MP)"},
	{Label: "Photo"}, {Label: "Camera", Value: "Acme\x1b]2;owned\a Snap 3"},
}

func loaded(t *testing.T, files ...string) *Model {
	t.Helper()
	m := viewing(t, Options{Files: files}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "png", Page: 1, Pages: 1, Info: landscape})
	return m
}

func TestInfoKeyShowsWhatTheFileSays(t *testing.T) {
	m := loaded(t, "photos/landscape.png")
	if strings.Contains(m.View().Content, "Dimensions") {
		t.Fatal("the panel is open before it was asked for")
	}
	m.Update(press("i"))
	view := m.View().Content
	for _, want := range []string{"landscape.png", "File", "photos/landscape.png", "5.0 KiB (5,120 bytes)", "Image", "Dimensions", "640 × 400 (0.3 MP)", "Camera", "Snap 3"} {
		if !strings.Contains(view, want) {
			t.Fatalf("panel lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "\x1b]2;") {
		t.Fatal("a value from the file injected a terminal escape")
	}
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("the panel must not capture the mouse")
	}
	m.Update(press("i"))
	if strings.Contains(m.View().Content, "Dimensions") {
		t.Fatal("i did not close the panel")
	}
	m.Update(press("i"))
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if strings.Contains(m.View().Content, "Dimensions") {
		t.Fatal("Esc did not close the panel")
	}
}

func TestInfoPanelFitsTheTerminal(t *testing.T) {
	m := loaded(t, "photos/landscape.png")
	m.Update(press("i"))
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 24, Height: 6}, {Width: 9, Height: 4}, {Width: 1, Height: 1}} {
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
}

func TestInfoPanelScrollsWhenItIsTall(t *testing.T) {
	m := loaded(t, "photos/landscape.png")
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 7})
	m.Update(press("i"))
	if view := m.View().Content; !strings.Contains(view, "Path") || strings.Contains(view, "Camera") {
		t.Fatalf("top of the panel:\n%s", view)
	}
	for range 20 {
		m.Update(press("j"))
	}
	if view := m.View().Content; !strings.Contains(view, "Camera") || strings.Contains(view, "Path") {
		t.Fatalf("bottom of the panel:\n%s", view)
	}
	for range 20 {
		m.Update(press("k"))
	}
	if view := m.View().Content; !strings.Contains(view, "Path") {
		t.Fatalf("back at the top:\n%s", view)
	}
}

func TestInfoPanelHoldsTheDocumentStill(t *testing.T) {
	m := loaded(t, "a.png", "b.png")
	m.zoom = 2
	m.Update(press("i"))
	for _, key := range []string{"]", "n", "+", "m", "e", "R"} {
		m.Update(press(key))
	}
	if m.index != 0 || m.zoom != 2 || m.menu || m.loading {
		t.Fatalf("keys reached the document: index=%d zoom=%d menu=%v loading=%v", m.index, m.zoom, m.menu, m.loading)
	}
	m.Update(press("?"))
	if view := m.View().Content; !strings.Contains(view, "a visual pager") {
		t.Fatal("help is unavailable from the panel")
	}
}

func TestInfoFollowsTheDocument(t *testing.T) {
	m := loaded(t, "a.pdf", "b.png")
	m.Update(document.Result{Generation: m.generation, Kind: "pdf", Page: 2, Pages: 3, Info: []document.Field{{Label: "Document"}, {Label: "Page size", Value: "595 × 842 pt"}}})
	m.Update(press("i"))
	if view := m.View().Content; !strings.Contains(view, "595 × 842 pt") || strings.Contains(view, "Dimensions") {
		t.Fatalf("panel shows the previous page:\n%s", view)
	}
	m.Update(press("i"))
	m.Update(press("]"))
	m.Update(press("i"))
	if view := m.View().Content; strings.Contains(view, "595 × 842 pt") {
		t.Fatalf("panel shows the previous file:\n%s", view)
	}
}

func TestInfoPanelExplainsAFailedFile(t *testing.T) {
	m := viewing(t, Options{Files: []string{"poster.png"}}, "")
	m.Update(document.Result{Generation: m.generation, Err: errors.New("image exceeds 33554432 pixels"), Info: []document.Field{{Label: "Image"}, {Label: "Dimensions", Value: "9000 × 8000 (72.0 MP)"}}})
	m.Update(press("i"))
	if view := m.View().Content; !strings.Contains(view, "9000 × 8000") || !strings.Contains(view, "image exceeds 33554432 pixels") {
		t.Fatalf("panel:\n%s", view)
	}
	m = viewing(t, Options{Files: []string{"gone.png"}}, "")
	m.Update(document.Result{Generation: m.generation, Err: errors.New("no such file\x1b]2;x\a")})
	m.Update(press("i"))
	if view := m.View().Content; !strings.Contains(view, "no such file") || strings.Contains(view, "No details yet") || strings.Contains(view, "\x1b]2;") {
		t.Fatalf("panel:\n%s", view)
	}
}

func TestInfoKeyNeedsADocument(t *testing.T) {
	empty := viewing(t, Options{}, "")
	empty.Update(press("i"))
	if view := empty.View().Content; !strings.Contains(view, "Drop files here to open") {
		t.Fatalf("empty session:\n%s", view)
	}
	menu := viewing(t, Options{Files: []string{"a.png", "b.png"}, Menu: true}, "")
	menu.Update(press("i"))
	if !menu.menu || menu.info {
		t.Fatal("i left the menu")
	}
	dropped := loaded(t, samples+"shapes.svg")
	dropped.Update(press("i"))
	if !deliver(dropped, DropMsg{Paths: []string{samples + "readme.md"}}) || dropped.info {
		t.Fatal("a drop must bring the new file into view")
	}
}
