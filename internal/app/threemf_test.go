package app

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/charmbracelet/x/ansi"
)

const facet = "solid t\nfacet normal 0 0 0\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nvertex 0 1 0\nendloop\nendfacet\nendsolid t\n"

// write3MF makes a one-triangle model that carries a picture of itself.
func write3MF(t *testing.T, path string) string {
	t.Helper()
	var picture, b bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(&b)
	for name, content := range map[string]string{
		"Metadata/thumbnail.png": picture.String(),
		"3D/3dmodel.model": `<?xml version="1.0"?><model xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02"><resources><object id="1"><mesh>` +
			`<vertices><vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices>` +
			`<triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh></object></resources><build><item objectid="1"/></build></model>`,
	} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(f, content)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func Test3MFStatus(t *testing.T) {
	mesh, err := document.ParseSTL([]byte(facet))
	if err != nil {
		t.Fatal(err)
	}
	m := viewing(t, Options{Files: []string{"part.3mf"}, Render3D: "software"}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "3mf", Page: 1, Pages: 1, Mesh: mesh})
	if view := m.View().Content; m.chart == nil || !strings.Contains(view, "3MF · 1 triangle") {
		t.Fatalf("a 3MF mesh:\n%s", view)
	}
	m = viewing(t, Options{Files: []string{"plate.3mf"}}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "3mf", Page: 1, Pages: 1, Image: image.NewRGBA(image.Rect(0, 0, 8, 6))})
	if view := m.View().Content; m.chart != nil || !strings.Contains(view, "3MF · thumbnail") {
		t.Fatalf("a 3MF shown by its thumbnail:\n%s", view)
	}
}

func Test3MFPreviewsByThumbnailAndOpensAsAMesh(t *testing.T) {
	dir := t.TempDir()
	model := write3MF(t, filepath.Join(dir, "part.3mf"))
	m := New(Options{Files: []string{model}, Menu: true, Preview: true, Render: "glyph", Render3D: "software", Page: 1, DPI: 72})
	t.Cleanup(func() { m.Close() })
	send(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	pump(m, m.Init(), 0)
	if m.preview == nil || m.preview.source == nil || m.preview.chart != nil || m.preview.source.Bounds().Dx() != 8 {
		t.Fatalf("the preview is not the thumbnail: %+v", m.preview)
	}
	send(m, enter)
	if m.listing() || m.chart == nil || m.source != nil || m.kind != "3mf" {
		t.Fatalf("opened: menu=%v chart=%v kind=%q", m.listing(), m.chart != nil, m.kind)
	}
}

func Test3MFExportsFromTheCamera(t *testing.T) {
	var out saved
	m := viewing(t, Options{Files: []string{write3MF(t, filepath.Join(t.TempDir(), "part.3mf"))}, MaxEdge: 64, Render3D: "software", Save: out.save(t)}, "")
	pump(m, m.load(false), 0)
	if m.chart == nil {
		t.Fatalf("not loaded: %v", m.err)
	}
	send(m, press("e"))
	if len(out.names) != 1 || out.names[0] != "part.png" || out.images[0].Bounds().Dx() != 64 || out.images[0].Bounds().Dy() != 64 {
		t.Fatalf("names=%q note=%q", out.names, m.note)
	}
}

// assembled shows a three-part model whose parts are one triangle each.
func assembled(t *testing.T) (*Model, *[]bool) {
	t.Helper()
	var last []bool
	assemble := func(shown []bool) (*document.Mesh, error) {
		last = shown
		n := 3
		if shown != nil {
			n = 0
			for _, s := range shown {
				if s {
					n++
				}
			}
		}
		if n == 0 {
			return nil, fmt.Errorf("no parts shown")
		}
		face := "facet normal 0 0 0\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nvertex 0 1 0\nendloop\nendfacet\n"
		return document.ParseSTL([]byte("solid t\n" + strings.Repeat(face, n) + "endsolid t\n"))
	}
	mesh, _ := assemble(nil)
	m := viewing(t, Options{Files: []string{"assembly.3mf"}, Render3D: "software"}, "")
	send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	send(m, document.Result{Generation: m.generation, Kind: "3mf", Page: 1, Pages: 1, Mesh: mesh,
		Parts: []document.Part{{Name: "Base", Triangles: 1}, {Name: "Lid", Triangles: 1}, {Name: "Hinge", Triangles: 1}}, Assemble: assemble})
	if m.chart == nil {
		t.Fatalf("no chart: err=%v generation=%d loading=%v kind=%q", m.err, m.generation, m.loading, m.kind)
	}
	return m, &last
}

func TestPartsOf3MFCanBeHiddenAndFocused(t *testing.T) {
	m, last := assembled(t)
	if hints := plain(m)[len(plain(m))-1]; !strings.Contains(hints, "c parts") {
		t.Fatalf("hints: %s", hints)
	}
	send(m, press("c"))
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"[x] 1  Base", "[x] 2  Lid", "[x] 3  Hinge", "1 △"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in the picker:\n%s", want, view)
		}
	}
	// Untick the lid: the mesh is rebuilt without it.
	send(m, press("j"), press(" "))
	if *last == nil || (*last)[1] || m.triangles != 2 || !strings.Contains(ansi.Strip(m.View().Content), "[ ] 2  Lid") {
		t.Fatalf("after hiding the lid: shown=%v triangles=%d", *last, m.triangles)
	}
	if !strings.Contains(plain(m)[len(plain(m))-2], "parts 2/3") {
		t.Fatalf("status: %s", plain(m)[len(plain(m))-2])
	}
	// Enter focuses the part under the cursor, alone.
	send(m, press("j"), enter)
	if *last == nil || (*last)[0] || (*last)[1] || !(*last)[2] || m.triangles != 1 {
		t.Fatalf("after focusing the hinge: shown=%v triangles=%d", *last, m.triangles)
	}
	// The last part cannot be hidden; X brings them all back and closes.
	send(m, press(" "))
	if m.triangles != 1 {
		t.Fatal("the last part was hidden")
	}
	send(m, press("X"))
	if m.triangles != 3 || m.pickingParts() {
		t.Fatalf("after X: triangles=%d picking=%v", m.triangles, m.pickingParts())
	}
	send(m, press("c"), tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.pickingParts() {
		t.Fatal("Esc did not close the picker")
	}
}

func TestPartsOptionAppliesToAMeshOnLoad(t *testing.T) {
	filter, _ := document.ParseParts("hinge", "")
	m := viewing(t, Options{Files: []string{"assembly.3mf"}, Parts: filter, Render3D: "software"}, "")
	q := m.request(false)
	if len(q.Parts.Names) != 1 || q.Parts.Names[0] != "hinge" {
		t.Fatalf("request: %+v", q.Parts)
	}
}

// A chart schedules its render through the command it returns; a command
// dropped leaves it waiting for a frame that never comes, and it draws
// nothing more. Every change to the parts must keep the chart drawing.
func TestPartsChangesKeepTheChartDrawing(t *testing.T) {
	m, _ := assembled(t)
	drawing := func(step string) {
		t.Helper()
		camera := m.chart.Camera()
		camera.Alpha += 5
		cmd := m.chart.SetCamera(camera)
		if cmd == nil {
			t.Fatalf("after %s, the chart no longer draws", step)
		}
		pump(m, cmd, 0)
	}
	drawing("opening")
	send(m, press("c"), press("j"), press(" "))
	drawing("hiding a part")
	send(m, press("j"), enter)
	drawing("focusing a part")
	send(m, press("X"))
	drawing("showing all")
	send(m, press("C"), press("l"), press(" "))
	drawing("painting")
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	drawing("undoing the paint")
}

func TestMeshViewKeepsRendererDetailsForTheInfoBox(t *testing.T) {
	m := painted(t)
	lines := plain(m)
	title, footer, status := lines[0], lines[len(lines)-3], lines[len(lines)-2]
	for _, line := range []string{title, footer, status} {
		for _, word := range []string{"WebGPU", "software", "Kitty", "kitty", "glyph", "ntcharts3d", "legend", "data"} {
			if strings.Contains(line, word) {
				t.Errorf("%q in %q", word, line)
			}
		}
	}
	if !strings.Contains(title, m.chart.Camera().Projection.String()) || !strings.Contains(footer, "drag orbit") || !strings.Contains(footer, "5 projection") {
		t.Errorf("title %q footer %q", title, footer)
	}
	if !strings.Contains(status, "STL · 1 triangle") {
		t.Errorf("status %q", status)
	}
	// Pictures say no more of how they are drawn either.
	m = loaded(t, "photos/landscape.png")
	if status := plain(m)[len(plain(m))-2]; strings.Contains(status, "glyph") || strings.Contains(status, "kitty") || !strings.Contains(status, "png · 1x") {
		t.Errorf("picture status %q", status)
	}
}

func TestMeshStartsFittedToTheFrame(t *testing.T) {
	// A tall, flat model needs the camera further off than the default to
	// be seen whole: the start is fitted, as an export's view is.
	tall := "solid t\n" + strings.Repeat("facet normal 0 0 0\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nvertex 0 1 20\nendloop\nendfacet\n", 1) + "endsolid t\n"
	mesh, err := document.ParseSTL([]byte(tall))
	if err != nil {
		t.Fatal(err)
	}
	m := viewing(t, Options{Files: []string{"tall.stl"}, Render3D: "software"}, "")
	send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	send(m, document.Result{Generation: m.generation, Kind: "stl", Page: 1, Pages: 1, Mesh: mesh})
	d := charts.DefaultCamera()
	want := document.View{Alpha: d.Alpha, Beta: d.Beta, Projection: d.Projection}.CameraFor(mesh, m.frameAspect())
	if got := m.chart.Camera(); got.Distance != want.Distance || got.Alpha != d.Alpha {
		t.Fatalf("start camera %+v, want fitted %+v", got, want)
	}
	// f goes back to that fitted start.
	c := m.chart.Camera()
	c.Distance *= 3
	send(m, m.chart.SetCamera(c))
	send(m, press("f"))
	if m.chart.Camera().Distance != want.Distance {
		t.Fatalf("f did not return to the fitted view: %+v", m.chart.Camera())
	}
}

func TestFitFollowsTheFrameUntilTheCameraMoves(t *testing.T) {
	// In the browser the mesh may load before the terminal says its size:
	// the fit is made again for the frame that comes, and the home with
	// it. Once the camera has been moved, a resize leaves it alone.
	mesh, _ := document.ParseSTL([]byte(facet))
	m := viewing(t, Options{Files: []string{"part.stl"}, Render3D: "software"}, "")
	send(m, document.Result{Generation: m.generation, Kind: "stl", Page: 1, Pages: 1, Mesh: mesh})
	send(m, tea.WindowSizeMsg{Width: 160, Height: 30})
	d := charts.DefaultCamera()
	want := document.View{Alpha: d.Alpha, Beta: d.Beta, Projection: d.Projection}.CameraFor(mesh, m.frameAspect())
	if got := m.chart.Camera(); got.Distance != want.Distance || m.home.Distance != want.Distance {
		t.Fatalf("after the size came: camera %v home %v, want %v", got.Distance, m.home.Distance, want.Distance)
	}
	moved := m.chart.Camera()
	moved.Alpha += 10
	send(m, m.chart.SetCamera(moved))
	send(m, tea.WindowSizeMsg{Width: 60, Height: 40})
	if got := m.chart.Camera(); got.Alpha != moved.Alpha || got.Distance != moved.Distance {
		t.Fatalf("a resize moved the camera the user had placed: %+v", got)
	}
	if m.home.Distance == want.Distance {
		t.Fatal("the home did not follow the new frame")
	}
}
