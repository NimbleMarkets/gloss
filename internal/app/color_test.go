package app

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

func painted(t *testing.T) *Model {
	t.Helper()
	mesh, err := document.ParseSTL([]byte(facet))
	if err != nil {
		t.Fatal(err)
	}
	m := viewing(t, Options{Files: []string{"part.stl"}, Render3D: "software"}, "")
	send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(document.Result{Generation: m.generation, Kind: "stl", Page: 1, Pages: 1, Mesh: mesh})
	if m.chart == nil || m.mesh == nil {
		t.Fatal("no mesh on screen")
	}
	return m
}

func shade(m *Model) color.RGBA {
	g, _ := m.mesh.Geometry(nil)
	return g.Vertices[0].Color
}

func TestColorPickerPaintsTheMesh(t *testing.T) {
	m := painted(t)
	before := shade(m)
	if hints := plain(m)[len(plain(m))-1]; !strings.Contains(hints, "C color") {
		t.Fatalf("hints: %s", hints)
	}
	send(m, press("C"))
	if !m.pickingColor() || !strings.Contains(ansi.Strip(m.View().Content), "Palette") {
		t.Fatalf("picker not open:\n%s", ansi.Strip(m.View().Content))
	}
	// A palette swatch applies at once; Esc puts the old color back.
	send(m, press("l"), press(" "))
	chosen := shade(m)
	if chosen == before {
		t.Fatal("the swatch did not paint the mesh")
	}
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.pickingColor() || shade(m) != before {
		t.Fatalf("Esc: picking=%v color=%v", m.pickingColor(), shade(m))
	}
	// Enter keeps what was chosen.
	send(m, press("C"), press("l"), press(" "), enter)
	if m.pickingColor() || shade(m) != chosen {
		t.Fatalf("Enter: picking=%v color=%v want %v", m.pickingColor(), shade(m), chosen)
	}
	// Hex mode takes a color as typed; e and f are digits there, not keys.
	send(m, press("C"), tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyTab})
	send(m, typed("ff0000")...)
	if got := shade(m); got != (color.RGBA{R: 255, A: 255}) || m.quitting {
		t.Fatalf("hex: %v", got)
	}
	// Sliders, the mode before, move one channel at a time.
	send(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, press("j"), press("l"))
	if got := shade(m); got.G == 0 || got.R != 255 {
		t.Fatalf("slider: %v", got)
	}
	// r restores the file's colors, and the status names the paint.
	send(m, press("r"))
	if shade(m) != before {
		t.Fatalf("r: %v", shade(m))
	}
	send(m, press("l"), enter)
	if !strings.Contains(plain(m)[len(plain(m))-2], "painted") {
		t.Fatalf("status: %s", plain(m)[len(plain(m))-2])
	}
}

func TestColorOptionPaintsOnLoad(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	m := viewing(t, Options{Files: []string{"part.stl"}, Color: &red}, "")
	if q := m.request(false); q.Color == nil || *q.Color != red {
		t.Fatalf("request: %+v", q.Color)
	}
}
