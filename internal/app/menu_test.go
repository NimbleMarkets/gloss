package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

func TestMenuSelectionCancelAndOpen(t *testing.T) {
	m := New(Options{Files: []string{"a.pdf", "b.svg", "c.stl"}, Render: "glyph", Page: 1, DPI: 72})
	defer m.Close()
	m.page, m.zoom = 4, 2
	m.Update(press("m"))
	m.menuKey("down")
	if !m.menu || m.selection != 1 || m.index != 0 || m.page != 4 {
		t.Fatal("browsing changed active file")
	}
	m.closeMenu(false)
	if m.index != 0 || m.page != 4 || m.zoom != 2 {
		t.Fatal("cancel lost active view")
	}
	m.Update(press("m"))
	m.menuKey("end")
	m.menuKey("enter")
	if m.menu || m.index != 2 || m.page != 1 || m.zoom != 0 || !m.loading {
		t.Fatal("selection did not open")
	}
}

func TestMenuPreviewIsolation(t *testing.T) {
	m := New(Options{Files: []string{"a.pdf", "b.svg"}, Render: "glyph", Menu: true, Preview: true, Page: 1, DPI: 150})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if m.preview == nil || m.preview.kittyID == m.kittyID {
		t.Fatal("preview is not independent")
	}
	owner := m.preview.kittyID
	m.menuKey("down")
	m.Update(previewResult{owner: owner, result: document.Result{Generation: m.preview.generation, Kind: "svg", Page: 1, Pages: 1}})
	if m.kind != "" || m.index != 0 || m.preview.kind != "svg" {
		t.Fatal("preview result affected active document")
	}
	m.menuKey("v")
	m.Update(previewResult{owner: owner, result: document.Result{Generation: 999, Kind: "stl"}})
	if m.preview != nil || m.kind != "" {
		t.Fatal("stale preview was revived")
	}
}

func TestMenuScrollAndResize(t *testing.T) {
	files := make([]string, 100)
	for i := range files {
		files[i] = fmt.Sprintf("folder/file-%03d.svg", i)
	}
	m := New(Options{Files: files, Menu: true, Render: "glyph", Page: 1})
	defer m.Close()
	for _, sz := range []tea.WindowSizeMsg{{Width: 100, Height: 24}, {Width: 20, Height: 6}, {Width: 2, Height: 3}} {
		m.Update(sz)
		m.menuKey("end")
		view := m.View().Content
		lines := strings.Split(view, "\n")
		if len(lines) > sz.Height {
			t.Fatal("menu overflows height")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > sz.Width {
				t.Fatal("menu overflows width")
			}
		}
		if sz.Width >= 20 && !strings.Contains(m.menuView(), "100") {
			t.Fatal("selection not scrolled into view")
		}
	}
}
