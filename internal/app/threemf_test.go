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
	if view := m.View().Content; m.chart == nil || !strings.Contains(view, "3MF · 1 triangles") {
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
	if m.menu || m.chart == nil || m.source != nil || m.kind != "3mf" {
		t.Fatalf("opened: menu=%v chart=%v kind=%q", m.menu, m.chart != nil, m.kind)
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
