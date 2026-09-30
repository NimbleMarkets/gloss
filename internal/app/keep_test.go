package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func glyphs(view string) bool { return strings.ContainsAny(view, "▄▀") }

func TestMainScreenIsAnOption(t *testing.T) {
	m := loaded(t, "photo.png")
	if !m.View().AltScreen {
		t.Fatal("gloss takes the alternate screen unless told otherwise")
	}
	kept := viewing(t, Options{Files: []string{"photo.png"}, KeepScreen: true}, "")
	if kept.View().AltScreen {
		t.Fatal("the main screen was asked for")
	}
}

func TestQuittingLeavesTheLastViewOnTheMainScreen(t *testing.T) {
	m := loaded(t, "photo.png")
	m.opts.KeepScreen = true
	if view := m.View().Content; !glyphs(view) || !strings.Contains(view, "q quit") {
		t.Fatalf("before quitting:\n%s", view)
	}
	m.Update(press("?"))
	_, cmd := m.Update(press("q"))
	if cmd == nil {
		t.Fatal("q did not quit")
	}
	view := m.View()
	if view.AltScreen || !glyphs(view.Content) || !strings.Contains(view.Content, "photo.png") {
		t.Fatalf("the picture did not stay:\n%s", view.Content)
	}
	// What is left behind is a picture, not a program: no keys are offered,
	// and the mouse is given back.
	if strings.Contains(view.Content, "q quit") || strings.Contains(view.Content, "a visual pager") || view.MouseMode != tea.MouseModeNone {
		t.Fatalf("the last view still offers keys:\n%s", view.Content)
	}
	// Bubble Tea erases the last row of the frame on its way out, and draws
	// a frame of another height below the old one rather than over it. So
	// the last view is as tall as those before it, and ends on an empty
	// row, which the prompt takes.
	lines := strings.Split(view.Content, "\n")
	if len(lines) != 30 || lines[29] != "" || !strings.Contains(lines[28], "photo.png") {
		t.Fatalf("%d rows of 30, ending %q, %q", len(lines), lines[len(lines)-2], lines[len(lines)-1])
	}
}

func TestQuittingClearsTheAlternateScreenAsBefore(t *testing.T) {
	m := loaded(t, "photo.png")
	m.Update(press("q"))
	if view := m.View(); !view.AltScreen || glyphs(view.Content) {
		t.Fatalf("graphics must be cleared before the screen is given back:\n%s", view.Content)
	}
}

func TestQuittingFromTheBrowserLeavesTheDocument(t *testing.T) {
	m := browsing(t, folder(t))
	m.opts.KeepScreen = true
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if view := m.View().Content; m.opener != nil || strings.Contains(view, "Filter:") || !strings.Contains(view, "alpha.png") {
		t.Fatalf("last view:\n%s", view)
	}
}

func TestKeptPicturesAreNotDrawnOverByTheNextGloss(t *testing.T) {
	// A Kitty picture left in the scrollback belongs to the terminal, under
	// its number. Each run starts its numbers somewhere new, since Q may
	// leave a picture behind from the alternate screen too.
	seen := map[int]bool{}
	for range 8 {
		m := New(Options{Files: []string{"a.png", "b.png"}, KeepScreen: true, Menu: true, Preview: true, Render: "glyph", Page: 1})
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		if m.preview == nil {
			t.Fatal("no preview")
		}
		for _, id := range []int{m.kittyID, m.preview.kittyID} {
			// Placeholder cells carry the number in a 24-bit color.
			if id < 100 || id >= 1<<24 {
				t.Fatalf("picture number %d cannot be drawn", id)
			}
		}
		if m.preview.kittyID <= m.kittyID {
			t.Fatalf("the preview started over: %d after %d", m.preview.kittyID, m.kittyID)
		}
		seen[m.kittyID] = true
		m.Close()
	}
	if len(seen) < 2 {
		t.Fatalf("every run began at the same number: %v", seen)
	}
	// Previews within a run follow on from its start, and never start over.
	nextModelID.Store(41)
	m := New(Options{Files: []string{"a.png"}, Render: "glyph", Page: 1, keptScreen: true})
	defer m.Close()
	if m.kittyID != 100+42*1000 {
		t.Fatalf("a run already placed moved its numbers: %d", m.kittyID)
	}
}

func TestShiftQKeepsTheViewWhateverWasLaunched(t *testing.T) {
	// From the alternate screen, Q leaves the last view on the main screen,
	// as -X would have; q clears it as ever.
	m := loaded(t, "photo.png")
	if !m.View().AltScreen {
		t.Fatal("not on the alternate screen")
	}
	_, cmd := m.Update(press("Q"))
	if cmd == nil {
		t.Fatal("Q did not quit")
	}
	view := m.View()
	if view.AltScreen || !glyphs(view.Content) || !strings.Contains(view.Content, "photo.png") || strings.Contains(view.Content, "q quit") {
		t.Fatalf("the picture did not stay:\n%s", view.Content)
	}
	// With -X, Q and q are the same: the view stays.
	kept := loaded(t, "photo.png")
	kept.opts.KeepScreen = true
	kept.Update(press("Q"))
	if view := kept.View(); view.AltScreen || !glyphs(view.Content) {
		t.Fatalf("with -X:\n%s", view.Content)
	}
	// Every run starts its picture numbers somewhere new, so the pictures a
	// Q left in the scrollback are not drawn over by the next gloss.
	seen := map[int]bool{}
	for range 8 {
		m := New(Options{Files: []string{"a.png"}, Render: "glyph", Page: 1})
		seen[m.kittyID] = true
		m.Close()
	}
	if len(seen) < 2 {
		t.Fatalf("every run began at the same number: %v", seen)
	}
}
