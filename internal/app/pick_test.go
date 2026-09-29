package app

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func picking(t *testing.T, files ...string) *Model {
	t.Helper()
	m := viewing(t, Options{Files: files, Pick: true}, "png")
	send(m, tea.WindowSizeMsg{Width: 240, Height: 30})
	return m
}

func TestPickSendsWhatTheUserAdded(t *testing.T) {
	dir := folder(t)
	m := picking(t)
	if view := m.View().Content; !strings.Contains(view, "Drop a file here to send it") {
		t.Fatalf("the empty screen does not say what a drop will do:\n%s", view)
	}
	send(m, enter)
	if m.Picked() != nil || m.quitting {
		t.Fatal("there was nothing to send")
	}
	send(m, tea.PasteMsg{Content: filepath.Join(dir, "alpha.png") + "\n" + filepath.Join(dir, "beta.svg")})
	if !m.menu {
		t.Fatal("several files open the menu, as ever")
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Enter open") || strings.Contains(view, "Enter send") {
		t.Fatalf("in the menu Enter opens a file:\n%s", view)
	}
	send(m, enter)
	if m.menu || m.Picked() != nil {
		t.Fatalf("menu=%v picked=%q", m.menu, m.Picked())
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Enter send 2 files") {
		t.Fatalf("the viewer does not say what Enter will send:\n%s", view)
	}
	send(m, press("o"))
	send(m, typed("trips/coast")...)
	send(m, enter)
	if m.opener != nil || len(m.opts.Files) != 3 || m.Picked() != nil {
		t.Fatalf("choosing in the browser adds the file; it does not send it: %q", m.opts.Files)
	}
	_, cmd := m.Update(enter)
	want := []string{filepath.Join(dir, "alpha.png"), filepath.Join(dir, "beta.svg"), filepath.Join(dir, "trips", "coast.png")}
	if got := m.Picked(); cmd == nil || strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("picked %q, want %q", got, want)
	}
}

func TestPickAmongFilesOffered(t *testing.T) {
	// Files named by the caller are choices. With nothing added, Enter
	// sends the one on screen.
	dir := folder(t)
	m := picking(t, filepath.Join(dir, "alpha.png"), "beta.svg")
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Enter send alpha.png") {
		t.Fatalf("hint:\n%s", view)
	}
	t.Chdir(dir)
	send(m, press("]"))
	m.Update(enter)
	if got := m.Picked(); len(got) != 1 || got[0] != filepath.Join(dir, "beta.svg") {
		t.Fatalf("picked %q: paths are given in full, wherever gloss was started", got)
	}
}

func TestPickSendsAFileOnce(t *testing.T) {
	dir := folder(t)
	m := picking(t, filepath.Join(dir, "alpha.png"))
	send(m, tea.PasteMsg{Content: filepath.Join(dir, "alpha.png")})
	send(m, tea.PasteMsg{Content: filepath.Join(dir, "beta.svg")})
	send(m, tea.PasteMsg{Content: filepath.Join(dir, "beta.svg")})
	m.Update(enter)
	want := []string{filepath.Join(dir, "alpha.png"), filepath.Join(dir, "beta.svg")}
	if got := m.Picked(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("picked %q, want %q", got, want)
	}
}

func TestPickCancels(t *testing.T) {
	dir := folder(t)
	for _, key := range []tea.KeyPressMsg{press("q"), {Code: 'c', Mod: tea.ModCtrl}} {
		m := picking(t)
		send(m, tea.PasteMsg{Content: filepath.Join(dir, "alpha.png")})
		if _, cmd := m.Update(key); cmd == nil || m.Picked() != nil {
			t.Fatalf("%s: picked %q", key, m.Picked())
		}
	}
}

func TestEnterMeansNothingOutsidePickMode(t *testing.T) {
	m := loaded(t, "photo.png")
	if _, cmd := m.Update(enter); cmd != nil || m.Picked() != nil || strings.Contains(m.View().Content, "Enter send") {
		t.Fatal("Enter sent a file nobody asked for")
	}
}

func TestDropsArriveFromOutside(t *testing.T) {
	// A page serving the viewer hands over what is dropped on it.
	dir := folder(t)
	drops := make(chan []string, 2)
	m := New(Options{Drops: drops, Pick: true, Render: "glyph", Page: 1, DPI: 72})
	t.Cleanup(func() { m.Close() })
	send(m, tea.WindowSizeMsg{Width: 240, Height: 30})
	// Two drops wait; having heard one, the viewer must go on listening.
	drops <- []string{filepath.Join(dir, "alpha.png")}
	drops <- []string{filepath.Join(dir, "beta.svg"), filepath.Join(dir, "notes.dmg")}
	pump(m, m.Init(), 0)
	if len(m.opts.Files) != 2 || len(m.Skipped()) != 1 {
		t.Fatalf("files=%q skipped=%q", m.opts.Files, m.Skipped())
	}
	m.Update(enter)
	if got := m.Picked(); len(got) != 2 {
		t.Fatalf("picked %q", got)
	}
	// The preview pane is a viewer too, but the drops are not meant for it.
	browsing := New(Options{Files: []string{filepath.Join(dir, "alpha.png")}, Drops: drops, Pick: true, Menu: true, Preview: true, Render: "glyph", Page: 1})
	t.Cleanup(func() { browsing.Close() })
	send(browsing, tea.WindowSizeMsg{Width: 240, Height: 30})
	if browsing.preview == nil || browsing.preview.opts.Drops != nil || browsing.preview.opts.Pick {
		t.Fatal("the preview listens for drops")
	}
}
