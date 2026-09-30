package app

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	charts "github.com/NimbleMarkets/ntcharts3d"
)

type saved struct {
	names  []string
	images []image.Image
}

func (s *saved) save(t *testing.T) func(string, []byte) (string, error) {
	return func(name string, data []byte) (string, error) {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("export is not a PNG: %v", err)
		}
		s.names, s.images = append(s.names, name), append(s.images, img)
		return name, nil
	}
}

// viewing puts the model in the state a finished load leaves behind.
func viewing(t *testing.T, opts Options, kind string) *Model {
	t.Helper()
	opts.Render, opts.Page, opts.DPI = "glyph", max(1, opts.Page), 72
	m := New(opts)
	t.Cleanup(func() { m.Close() })
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.kind, m.generation, m.pages = kind, 1, 1
	return m
}

func TestExportKeySavesTheCurrentDocument(t *testing.T) {
	var out saved
	m := viewing(t, Options{Files: []string{samples + "shapes.svg"}, MaxEdge: 256, Save: out.save(t)}, "svg")
	m.zoom, m.panX = 2, .1
	_, cmd := m.Update(press("e"))
	if cmd == nil {
		t.Fatal("e did nothing")
	}
	if len(out.names) != 0 {
		t.Fatal("export blocked the update loop")
	}
	m.Update(cmd())
	if len(out.names) != 1 || out.names[0] != "shapes.png" {
		t.Fatalf("names=%q", out.names)
	}
	if b := out.images[0].Bounds(); max(b.Dx(), b.Dy()) != 256 {
		t.Fatalf("exported %v, want the --max-edge of 256", b)
	}
	if m.zoom != 2 || m.panX != .1 || m.loading || m.index != 0 {
		t.Fatal("export disturbed the view")
	}
	if view := m.View().Content; !strings.Contains(view, "saved shapes.png") {
		t.Fatalf("status:\n%s", view)
	}
}

func TestExportFollowsTheCamera(t *testing.T) {
	var out saved
	m := viewing(t, Options{Files: []string{samples + "gloss.stl"}, MaxEdge: 96, Save: out.save(t)}, "stl")
	m.chart = charts.New(100, 28, charts.WithRenderMode(charts.Software))
	for _, beta := range []float64{40, 130} {
		camera := m.chart.Camera()
		camera.Beta = beta
		m.chart.SetCamera(camera)
		_, cmd := m.Update(press("e"))
		if cmd == nil {
			t.Fatal("e did nothing")
		}
		m.Update(cmd())
	}
	if len(out.images) != 2 {
		t.Fatalf("exports=%d note=%q", len(out.images), m.note)
	}
	pixels := func(src image.Image) []byte {
		dst := image.NewRGBA(src.Bounds())
		draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
		return dst.Pix
	}
	if bytes.Equal(pixels(out.images[0]), pixels(out.images[1])) {
		t.Fatal("turning the model did not change the export")
	}
}

func TestExportReportsWhatItCannotSave(t *testing.T) {
	var out saved
	m := viewing(t, Options{Files: []string{samples + "readme.md"}, Save: out.save(t)}, "markdown")
	m.markdown = newMarkdownView(&document.Markdown{}, 9000)
	if _, cmd := m.Update(press("e")); cmd != nil {
		m.Update(cmd())
	}
	if len(out.names) != 0 || !strings.Contains(m.View().Content, "Markdown") {
		t.Fatalf("names=%q note=%q", out.names, m.note)
	}

	m = viewing(t, Options{Files: []string{samples + "shapes.svg"}, Save: func(string, []byte) (string, error) {
		return "", errors.New("disk full\x1b]2;x\a")
	}}, "svg")
	_, cmd := m.Update(press("e"))
	m.Update(cmd())
	if view := m.View().Content; !strings.Contains(view, "export failed") || !strings.Contains(view, "disk full") || strings.Contains(view, "\x1b]2;") {
		t.Fatalf("status:\n%s", view)
	}
}

func TestExportKeyWaitsForADocument(t *testing.T) {
	var out saved
	files := []string{samples + "shapes.svg"}
	for name, m := range map[string]*Model{
		"empty":   viewing(t, Options{Save: out.save(t)}, ""),
		"loading": viewing(t, Options{Files: files, Save: out.save(t)}, ""),
		"menu":    viewing(t, Options{Files: files, Menu: true, Save: out.save(t)}, "svg"),
		"help":    viewing(t, Options{Files: files, Save: out.save(t)}, "svg"),
		"failed":  viewing(t, Options{Files: files, Save: out.save(t)}, "svg"),
	} {
		switch name {
		case "loading":
			m.loading = true
		case "help":
			m.help = true
		case "failed":
			m.err = errors.New("cannot decode")
		}
		if _, cmd := m.Update(press("e")); cmd != nil {
			m.Update(cmd())
		}
		if len(out.names) != 0 {
			t.Fatalf("%s: exported %q", name, out.names)
		}
	}
}

func TestExportNames(t *testing.T) {
	for _, tt := range []struct {
		path, kind string
		page       int
		want       string
	}{
		{"docs/report.pdf", "pdf", 3, "report-page-3.png"},
		{"photo.heic", "heic", 1, "photo.png"},
		{"photo.png", "png", 1, "photo.png"},
		{"dropped/my photo.png", "png", 1, "my photo.png"},
		{"/tmp/gloss-stdin-123", "svg", 1, "stdin.png"},
		{"archive.tar.stl", "stl", 1, "archive.tar.png"},
		{".hidden", "svg", 1, ".hidden.png"},
	} {
		if got := exportName(tt.path, tt.kind, tt.page); got != tt.want {
			t.Fatalf("exportName(%q, %q, %d) = %q, want %q", tt.path, tt.kind, tt.page, got, tt.want)
		}
	}
}

func TestSaveFileNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(original, []byte("the input"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"photo-2.png", "photo-3.png"} {
		got, err := saveFile(dir, "photo.png", []byte("an export"))
		if err != nil || got != want {
			t.Fatalf("got %q, %v; want %q", got, err, want)
		}
	}
	if data, _ := os.ReadFile(original); string(data) != "the input" {
		t.Fatal("export overwrote the file being viewed")
	}
	if got, err := saveFile(dir, "fresh.png", []byte("x")); err != nil || got != "fresh.png" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := saveFile(filepath.Join(dir, "missing"), "a.png", []byte("x")); err == nil {
		t.Fatal("saving into a missing directory succeeded")
	}
}

func TestExportUsesTheViewersRenderer(t *testing.T) {
	// --3d software means software for what is saved, too.
	for mode, cpu := range map[string]bool{"": false, "auto": false, "software": true, "wireframe": true} {
		m := viewing(t, Options{Files: []string{samples + "gloss.stl"}, Render3D: mode}, "stl")
		if got := m.exportOnCPU(); got != cpu {
			t.Errorf("--3d %q: on the CPU = %v, want %v", mode, got, cpu)
		}
	}
}

func TestViewerStartsFromTheViewAskedFor(t *testing.T) {
	views, err := document.ParseViews("top,front")
	if err != nil {
		t.Fatal(err)
	}
	mesh, err := document.ParseSTL([]byte(facet))
	if err != nil {
		t.Fatal(err)
	}
	m := viewing(t, Options{Files: []string{"part.stl"}, Render3D: "software", Views: views}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "stl", Page: 1, Pages: 1, Mesh: mesh})
	want := views[0].CameraFor(mesh, m.frameAspect())
	if got := m.chart.Camera(); got.Alpha != 89 || got.Beta != -90 || got.Distance != want.Distance {
		t.Fatalf("camera %+v, want the first view asked for, fitted: %+v", got, want)
	}
	// Reset returns there, not to NTCharts3d's own view.
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m.Update(press("f"))
	if got := m.chart.Camera(); got.Alpha != 89 || got.Beta != -90 || got.Distance != want.Distance {
		t.Fatalf("after reset: %+v", got)
	}
	plain := viewing(t, Options{Files: []string{"part.stl"}, Render3D: "software"}, "")
	plain.Update(document.Result{Generation: plain.generation, Kind: "stl", Page: 1, Pages: 1, Mesh: mesh})
	// With no view asked for, the default angles, at a distance fitted to
	// the mesh rather than NTCharts3d's own.
	d := charts.DefaultCamera()
	fitted := document.View{Alpha: d.Alpha, Beta: d.Beta, Projection: d.Projection}.CameraFor(mesh, plain.frameAspect())
	if got := plain.chart.Camera(); got.Alpha != d.Alpha || got.Distance != fitted.Distance || got.Distance == d.Distance {
		t.Fatalf("with no view asked for: %+v, want %+v", got, fitted)
	}
}
