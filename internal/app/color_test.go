package app

import (
	"image/color"
	"slices"
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
	send(m, document.Result{Generation: m.generation, Kind: "stl", Page: 1, Pages: 1, Mesh: mesh})
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

func TestExportCarriesWhatIsOnScreen(t *testing.T) {
	m, _ := assembled(t)
	send(m, press("c"), press("j"), press(" "), tea.KeyPressMsg{Code: tea.KeyEscape})
	send(m, press("C"), press("l"), press(" "), enter)
	q := m.exportRequest()
	if !slices.Equal(q.Shown, []bool{true, false, true}) || q.Color == nil || q.PaintAll {
		t.Fatalf("export request: shown=%v color=%v all=%v", q.Shown, q.Color, q.PaintAll)
	}
	// Paint on every face, where a mesh has colors of its own, goes too.
	m.tintAll = true
	if q := m.exportRequest(); !q.PaintAll {
		t.Fatal("the export does not paint all faces")
	}
	m.tintAll = false
	// A reload keeps the choice; the next file starts afresh.
	if q := m.request(true); !slices.Equal(q.Shown, []bool{true, false, true}) {
		t.Fatalf("reload request: shown=%v", q.Shown)
	}
	m.opts.Files = append(m.opts.Files, "other.3mf")
	m.switchFile(1)
	if q := m.request(false); q.Shown != nil || q.Color == nil {
		t.Fatalf("next file: shown=%v color=%v", q.Shown, q.Color)
	}
}

func TestPartsShownOnLoadReachTheViewer(t *testing.T) {
	m, _ := assembled(t)
	mesh, _ := document.ParseSTL([]byte(facet))
	send(m, document.Result{Generation: m.generation, Kind: "3mf", Page: 1, Pages: 1, Mesh: mesh, Shown: []bool{false, true, false},
		Parts: []document.Part{{Name: "Base"}, {Name: "Lid"}, {Name: "Hinge"}}, Assemble: func([]bool) (*document.Mesh, error) { return mesh, nil }})
	if !strings.Contains(plain(m)[len(plain(m))-2], "parts 1/3") {
		t.Fatalf("status: %s", plain(m)[len(plain(m))-2])
	}
}
