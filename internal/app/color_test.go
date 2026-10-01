package app

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
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

func TestBackgroundPickerRecolorsTheBackdrop(t *testing.T) {
	m := painted(t)
	defer m.Close()
	if hints := plain(m)[len(plain(m))-1]; !strings.Contains(hints, "B bg-color") {
		t.Fatalf("hints: %s", hints)
	}
	if m.chart.Background() != meshBackground || m.bg != nil {
		t.Fatalf("default: chart=%v bg=%v", m.chart.Background(), m.bg)
	}
	send(m, press("B"))
	if !m.pickingColor() || !strings.Contains(ansi.Strip(m.View().Content), "Background") {
		t.Fatalf("picker not open:\n%s", ansi.Strip(m.View().Content))
	}
	// A swatch applies at once, to the backdrop and not to the mesh.
	meshBefore := shade(m)
	send(m, press("l"), press("l"), press("l"), press("l"), press("l"), press("l"), press("l"), press("j"), press(" "))
	chosen := m.chart.Background()
	if chosen == meshBackground || m.bg == nil || *m.bg != chosen || shade(m) != meshBefore {
		t.Fatalf("swatch: chart=%v bg=%v mesh=%v want %v", chosen, m.bg, shade(m), meshBefore)
	}
	// Esc puts the old backdrop back; Enter keeps the new one.
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.pickingColor() || m.bg != nil || m.chart.Background() != meshBackground {
		t.Fatalf("Esc: picking=%v bg=%v chart=%v", m.pickingColor(), m.bg, m.chart.Background())
	}
	send(m, press("B"), press("l"), press(" "), enter)
	kept := m.chart.Background()
	if m.pickingColor() || kept == meshBackground || m.bg == nil || *m.bg != kept {
		t.Fatalf("Enter: picking=%v bg=%v chart=%v", m.pickingColor(), m.bg, kept)
	}
	// Hex takes a typed color; r restores the default.
	send(m, press("B"), tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyTab})
	send(m, typed("3f3080")...)
	if got := m.chart.Background(); got != (color.RGBA{R: 0x3f, G: 0x30, B: 0x80, A: 255}) {
		t.Fatalf("hex: %v", got)
	}
	send(m, press("r"))
	if m.bg != nil || m.chart.Background() != meshBackground {
		t.Fatalf("r: bg=%v chart=%v", m.bg, m.chart.Background())
	}
	// The mesh picker is unchanged by all this, and the info line names the backdrop.
	send(m, enter, press("C"))
	if !strings.Contains(ansi.Strip(m.View().Content), "Color") {
		t.Fatal("C no longer opens the color picker")
	}
}

func TestEnterChoosesTheSwatchItIsOn(t *testing.T) {
	m := painted(t)
	defer m.Close()
	before := shade(m)
	// Enter on a picker just opened changes nothing, for either picker.
	send(m, press("C"), enter)
	if m.pickingColor() || shade(m) != before || m.tint != nil {
		t.Fatalf("Enter alone changed the mesh: picking=%v color=%v tint=%v", m.pickingColor(), shade(m), m.tint)
	}
	send(m, press("B"), enter)
	if m.pickingColor() || m.bg != nil || m.chart.Background() != meshBackground {
		t.Fatalf("Enter alone changed the background: bg=%v chart=%v", m.bg, m.chart.Background())
	}
	// Move to a swatch and press Enter: that is a choice.
	send(m, press("C"), press("l"), enter)
	if m.pickingColor() || shade(m) == before {
		t.Fatalf("Enter did not choose the swatch: picking=%v color=%v", m.pickingColor(), shade(m))
	}
	want := palettes[0].swatches[1]
	if got := shade(m); got != want {
		t.Fatalf("chose %v, want %v", got, want)
	}
	// The background picker takes Enter the same way.
	send(m, press("B"), press("l"), press("l"), enter)
	if m.pickingColor() || m.bg == nil || *m.bg != backdrop.swatches[2] {
		t.Fatalf("background: picking=%v bg=%v", m.pickingColor(), m.bg)
	}
	// Esc still takes a choice back, and Enter in the sliders keeps what they set.
	send(m, press("B"), press("l"), tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.bg == nil || *m.bg != backdrop.swatches[2] {
		t.Fatalf("Esc undid too much: %v", m.bg)
	}
}

// at is the screen cell of the picker's text at (x, y): the box's border and
// padding are skipped.
func at(p *colorPicker, x, y int) (int, int) { return p.at.Min.X + 2 + x, p.at.Min.Y + 1 + y }

func click(m *Model, x, y int) {
	send(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func TestColorPickerTakesTheMouse(t *testing.T) {
	m := painted(t)
	defer m.Close()
	camera := m.chart.Camera()
	send(m, press("C"))
	m.View() // The box says where it sits as it is drawn.
	p := m.colorPicker
	if p.at.Empty() {
		t.Fatal("the picker has no place on screen")
	}
	// A click on a swatch chooses it at once, and does not orbit the mesh.
	x, y := at(p, 1*swatchCells+1, p.rows.body)
	click(m, x, y)
	if want := palettes[0].swatches[1]; shade(m) != want || !m.pickingColor() {
		t.Fatalf("swatch click: color=%v want %v picking=%v", shade(m), want, m.pickingColor())
	}
	if m.chart.Camera() != camera {
		t.Fatalf("a click on the picker moved the mesh: %+v", m.chart.Camera())
	}
	// The gap between swatches is nothing.
	x, y = at(p, 1*swatchCells+swatchCells-1, p.rows.body+1)
	click(m, x, y)
	if shade(m) != palettes[0].swatches[1] {
		t.Fatalf("the gap chose a swatch: %v", shade(m))
	}
	// A second click on the swatch chooses it and closes the picker.
	x, y = at(p, 1*swatchCells+1, p.rows.body)
	click(m, x, y)
	if m.pickingColor() || shade(m) != palettes[0].swatches[1] {
		t.Fatalf("second click: picking=%v color=%v", m.pickingColor(), shade(m))
	}

	// The tabs, and a slider dragged along its bar.
	kept := shade(m)
	send(m, press("C"))
	m.View()
	p = m.colorPicker
	x, y = at(p, len("Palette")+3+1, p.rows.tabs)
	click(m, x, y)
	if p.mode != pickSliders {
		t.Fatalf("tab click: mode=%d", p.mode)
	}
	m.View()
	x, y = at(p, barAt+barCells-1, p.rows.body) // The red bar, at its end.
	send(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if p.r != 255 || shade(m).R != 255 {
		t.Fatalf("slider click: r=%d shade=%v", p.r, shade(m))
	}
	x, _ = at(p, barAt, 0)
	send(m, tea.MouseMotionMsg{X: x, Y: y + 5, Button: tea.MouseLeft}, tea.MouseReleaseMsg{X: x, Y: y + 5, Button: tea.MouseLeft})
	if p.r != 0 || p.drag {
		t.Fatalf("slider drag: r=%d drag=%v", p.r, p.drag)
	}
	// Esc gives back the color from when the picker opened, as it does for the keys.
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.pickingColor() || shade(m) != kept {
		t.Fatalf("Esc: picking=%v color=%v want %v", m.pickingColor(), shade(m), kept)
	}

	// The background picker has the same box; clicking in it changes the backdrop.
	send(m, press("B"))
	m.View()
	p = m.colorPicker
	x, y = at(p, 2*swatchCells, p.rows.body)
	click(m, x, y)
	if m.bg == nil || *m.bg != backdrop.swatches[2] {
		t.Fatalf("background click: %v", m.bg)
	}
}

// colored3MF is a model whose file gives its one face a color: red.
func colored3MF(t *testing.T) *Model {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create("3D/3dmodel.model")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(f, `<?xml version="1.0"?><model xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02"><resources>`+
		`<basematerials id="5"><base name="red" displaycolor="#FF0000"/></basematerials>`+
		`<object id="1" pid="5" pindex="0"><mesh><vertices><vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices>`+
		`<triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh></object></resources><build><item objectid="1"/></build></model>`)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "red.3mf")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	res := (&document.Loader{}).Load(document.Request{Path: path, Type: "3mf", Page: 1, DPI: 96})
	if res.Err != nil || res.Mesh == nil {
		t.Fatalf("load: %v", res.Err)
	}
	m := viewing(t, Options{Files: []string{path}, Render3D: "software"}, "")
	send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	res.Generation = m.generation
	send(m, res)
	if m.chart == nil || m.mesh == nil || !m.mesh.HasColors() {
		t.Fatal("no colored mesh on screen")
	}
	return m
}

func TestColoredModelCanBeRecoloredAndReset(t *testing.T) {
	m := colored3MF(t)
	defer m.Close()
	red := color.RGBA{R: 255, A: 255}
	if shade(m) != red {
		t.Fatalf("the file's color: %v", shade(m))
	}
	// The picker paints the whole model, not only faces the file left plain.
	send(m, press("C"))
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Reset") || !strings.Contains(view, "all faces") {
		t.Fatalf("no Reset button or scope:\n%s", view)
	}
	send(m, press("l"), enter)
	if m.pickingColor() || shade(m) == red {
		t.Fatalf("a colored model did not take the color: picking=%v color=%v", m.pickingColor(), shade(m))
	}
	// Reset, by its button, gives the file's color back and keeps the picker open.
	send(m, press("C"))
	m.View()
	p := m.colorPicker
	x, y := at(p, 2, p.rows.reset)
	click(m, x, y)
	if shade(m) != red || m.tint != nil || !m.pickingColor() {
		t.Fatalf("Reset: color=%v tint=%v picking=%v", shade(m), m.tint, m.pickingColor())
	}
	// The scope line is a button too: plain faces only leaves the file's color alone.
	m.View()
	x, y = at(p, 2, p.rows.scope)
	click(m, x, y)
	send(m, press("l"), press(" "))
	if shade(m) != red {
		t.Fatalf("plain faces only repainted the file's color: %v", shade(m))
	}
}
