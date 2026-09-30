package document

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	charts "github.com/NimbleMarkets/ntcharts3d"
)

const triangleSTL = `solid triangle
facet normal 0 0 0
outer loop
vertex 0 0 0
vertex 1 0 0
vertex 0 1 0
endloop
endfacet
endsolid triangle
`

func binarySTL() []byte {
	b := make([]byte, 134)
	copy(b, "solid binary header")
	binary.LittleEndian.PutUint32(b[80:], 1)
	binary.LittleEndian.PutUint32(b[108:], math.Float32bits(1))
	binary.LittleEndian.PutUint32(b[124:], math.Float32bits(1))
	return b
}

func TestSTLFormatsAndNormals(t *testing.T) {
	for _, data := range [][]byte{[]byte(triangleSTL), binarySTL()} {
		mesh, err := ParseSTL(data)
		if err != nil {
			t.Fatal(err)
		}
		g, _ := mesh.Geometry(nil)
		if mesh.Triangles() != 1 || g.Vertices[0].Normal.Z != 1 || g.Bounds.Max.X != 1 || g.Bounds.Max.Y != 1 {
			t.Fatalf("unexpected geometry: %+v", g)
		}
		kind, err := Detect("extensionless", data, "")
		if err != nil || kind != "stl" {
			t.Fatalf("detect = %q, %v", kind, err)
		}
	}
}

func TestSTLRejectsMalformed(t *testing.T) {
	nan := binarySTL()
	binary.LittleEndian.PutUint32(nan[96:], math.Float32bits(float32(math.NaN())))
	for name, data := range map[string][]byte{
		"empty": nil, "truncated": binarySTL()[:133], "nan": nan,
		"incomplete":   []byte(strings.ReplaceAll(triangleSTL, "endfacet", "")),
		"no end":       []byte(strings.ReplaceAll(triangleSTL, "endsolid triangle", "")),
		"no triangles": []byte("solid empty\nendsolid empty\n"),
		"trailing":     []byte(triangleSTL + "junk"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSTL(data); err == nil {
				t.Fatal("accepted malformed STL")
			}
		})
	}
}

func writeFixture(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRasterSVGAndSTL(t *testing.T) {
	var pngData bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 16, 8))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, kind string
		data       []byte
	}{
		{"no-extension", "png", pngData.Bytes()},
		{"vector", "svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 10"><rect width="20" height="10" fill="red"/></svg>`)},
		{"mesh", "stl", binarySTL()},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			l := &Loader{}
			defer l.Close()
			r := l.Load(Request{Path: writeFixture(t, tc.name, tc.data), Generation: 1, Page: 1, DPI: 72})
			if r.Err != nil {
				t.Fatal(r.Err)
			}
			if r.Kind != tc.kind {
				t.Fatalf("kind = %q", r.Kind)
			}
			if tc.kind != "stl" && (r.Image == nil || r.Image.Bounds().Empty()) {
				t.Fatal("no rasterized image")
			}
		})
	}
}

// A self-contained PDF fixture: two pages containing red and blue rectangles.
func samplePDF() []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] /Resources << >> /Contents 5 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] /Resources << >> /Contents 6 0 R >>",
	}
	for _, c := range []string{"1 0 0", "0 0 1"} {
		stream := c + " rg 0 0 72 72 re f\n"
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream))
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return b.Bytes()
}

func TestPDFRenderingNavigationAndClose(t *testing.T) {
	l := &Loader{}
	defer l.Close()
	path := writeFixture(t, "pages.pdf", samplePDF())
	for i, page := range []int{1, 2, 99, 0} {
		r := l.Load(Request{Path: path, Page: page, DPI: 72, Generation: uint64(i + 1)})
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		if r.Pages != 2 || r.Page != max(1, min(page, 2)) {
			t.Fatalf("bad page metadata: %+v", r)
		}
		red, _, blue, _ := r.Image.At(36, 36).RGBA()
		if (r.Page == 1 && red <= blue) || (r.Page == 2 && blue <= red) {
			t.Fatalf("wrong page color: red=%d blue=%d", red, blue)
		}
	}
	if got := l.Load(Request{Path: "missing", Generation: 1}); got.Err != nil || got.Image != nil {
		t.Fatal("stale request was executed")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if got := l.Load(Request{Path: path, Generation: 9}); got.Image != nil {
		t.Fatal("load after close")
	}
}

func TestInvalidInputAndTerminalSanity(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("\x00\x01"), []byte("caf\xe9"), []byte("\x89PNG\r\n\x1a\n\x00")} {
		if _, err := Detect("unknown", data, ""); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
	// What is not binary is text, whatever else it fails to be.
	for _, data := range [][]byte{[]byte("hello"), []byte("<div/>"), []byte("{not json")} {
		if kind, err := Detect("unknown", data, ""); err != nil || kind != "text" {
			t.Fatalf("%q: %q %v", data, kind, err)
		}
	}
	if _, err := ReadFile(t.TempDir()); err == nil {
		t.Fatal("accepted directory")
	}
	if _, err := ReadLimited(strings.NewReader("")); err == nil {
		t.Fatal("accepted empty input")
	}
	l := &Loader{}
	defer l.Close()
	r := l.Load(Request{Path: writeFixture(t, "bad.pdf", []byte("%PDF-broken")), Generation: 1})
	if r.Err == nil {
		t.Fatal("accepted malformed PDF")
	}
}

func TestProbeExtensionless(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	// Larger than the sniffing prefix: binary STL needs total file size.
	mesh := make([]byte, 84+50*1400)
	binary.LittleEndian.PutUint32(mesh[80:84], 1400)
	for kind, data := range map[string][]byte{"png": pngData.Bytes(), "svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "pdf": []byte("%PDF-1.7\n"), "stl": mesh} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "undecorated")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := Probe(path, "")
			if err != nil || got != kind {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
	for _, suffix := range []string{".dmg", ".zip", ".xyz", ""} {
		path := filepath.Join(t.TempDir(), "unsupported"+suffix)
		if err := os.WriteFile(path, []byte("unrecognized\x00content"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Probe(path, ""); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s: %v", suffix, err)
		}
	}
}

// manySTL is a binary STL of n triangles.
func manySTL(n int) []byte {
	b := make([]byte, 84+50*n)
	binary.LittleEndian.PutUint32(b[80:], uint32(n))
	for i := range n {
		at := b[84+50*i+12:]
		x := float32(i % 1000)
		for j, corner := range [][3]float32{{x, float32(i / 1000), 0}, {x + 1, float32(i / 1000), 0}, {x, float32(i/1000) + 1, 0}} {
			for k, v := range corner {
				binary.LittleEndian.PutUint32(at[12*j+4*k:], math.Float32bits(v))
			}
		}
	}
	return b
}

func TestMeshLimitIsTheGPUs(t *testing.T) {
	if MaxTriangles != charts.MaxMeshTriangles || MaxTriangles < 900000 {
		t.Fatalf("MaxTriangles = %d, NTCharts3d draws %d", MaxTriangles, charts.MaxMeshTriangles)
	}
	// More than the 87,381 that a chart's limits once allowed.
	mesh, err := ParseSTL(manySTL(200000))
	if err != nil || mesh.Triangles() != 200000 {
		t.Fatalf("200,000 triangles: %v", err)
	}
	if _, err := mesh.Geometry(nil); err != nil {
		t.Fatal(err)
	}
	// What NTCharts3d is given, it accepts.
	chart := charts.New(1, 3, charts.WithRenderMode(charts.Software))
	defer chart.Close()
	if chart.SetSeries(mesh); chart.Err() != nil {
		t.Fatalf("NTCharts3d refused the mesh: %v", chart.Err())
	}
	img, err := ExportImage(Result{Mesh: mesh, CPU: true}, 256)
	if err != nil || img.Bounds().Dx() != 256 {
		t.Fatalf("export: %v", err)
	}
}

func TestMeshesOverTheLimitAreRefused(t *testing.T) {
	limit(t, 1000)
	if _, err := ParseSTL(manySTL(1000)); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	if _, err := ParseSTL(manySTL(1001)); err == nil || !strings.Contains(err.Error(), "1,000") {
		t.Fatalf("binary, over the limit: %v", err)
	}
	var text strings.Builder
	text.WriteString("solid many\n")
	for range 1001 {
		text.WriteString("facet normal 0 0 1\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nvertex 0 1 0\nendloop\nendfacet\n")
	}
	text.WriteString("endsolid many\n")
	if _, err := ParseSTL([]byte(text.String())); err == nil || !strings.Contains(err.Error(), "1,000") {
		t.Fatalf("ASCII, over the limit: %v", err)
	}
}
