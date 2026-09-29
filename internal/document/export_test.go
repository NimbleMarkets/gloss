package document

import (
	"image"
	"image/color"
	"os"
	"testing"

	charts "github.com/NimbleMarkets/ntcharts3d"
)

func TestExportSizeAndAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 210, 120))
	src.SetNRGBA(10, 20, color.NRGBA{R: 255, A: 255})
	for _, edge := range []int{100, 400} {
		img, err := ExportImage(Result{Image: src}, edge)
		if err != nil {
			t.Fatal(err)
		}
		wantW := min(edge, 200)
		if img.Bounds().Dx() != wantW || img.Bounds().Dy() != wantW/2 {
			t.Fatal(img.Bounds())
		}
		r, g, b, a := img.At(img.Bounds().Max.X-1, img.Bounds().Max.Y-1).RGBA()
		if r != 65535 || g != 65535 || b != 65535 || a != 65535 {
			t.Fatal("transparency not flattened onto white")
		}
	}
}

func TestExportPDFAtTargetSize(t *testing.T) {
	l := &Loader{}
	defer l.Close()
	r := l.Load(Request{Path: writeFixture(t, "pages.pdf", samplePDF()), Page: 2, DPI: 72, MaxEdge: 256, Generation: 1})
	img, err := ExportImage(r, 256)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatal(img.Bounds())
	}
	red, _, blue, _ := img.At(128, 128).RGBA()
	if blue <= red {
		t.Fatal("wrong PDF page exported")
	}
}

func TestExportMesh(t *testing.T) {
	m, err := ParseSTL([]byte(triangleSTL))
	if err != nil {
		t.Fatal(err)
	}
	img, err := ExportImage(Result{Mesh: m}, 128)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 128, 128) {
		t.Fatal(img.Bounds())
	}
	colored := 0
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r != g || g != b {
				colored++
			}
		}
	}
	if colored < 100 {
		t.Fatalf("mesh not visible: %d colored pixels", colored)
	}
}

// drawing replaces the GPU for a test, and says how often it was asked.
func drawing(t *testing.T, img image.Image) *int {
	t.Helper()
	asked, real := 0, gpuSnapshot
	gpuSnapshot = func(*Mesh, int, charts.Camera) (image.Image, bool) {
		asked++
		return img, img != nil
	}
	t.Cleanup(func() { gpuSnapshot = real })
	return &asked
}

func TestMeshExportPrefersTheGPU(t *testing.T) {
	mesh, err := ParseSTL([]byte(triangleSTL))
	if err != nil {
		t.Fatal(err)
	}
	picture := image.NewRGBA(image.Rect(0, 0, 128, 128))
	asked := drawing(t, picture)
	img, err := ExportImage(Result{Mesh: mesh}, 128)
	if err != nil || img != image.Image(picture) || *asked != 1 {
		t.Fatalf("img=%v err=%v asked=%d", img.Bounds(), err, *asked)
	}
	// Software asked for by name is what is used.
	img, err = ExportImage(Result{Mesh: mesh, CPU: true}, 128)
	if err != nil || img == image.Image(picture) || *asked != 1 || img.Bounds() != image.Rect(0, 0, 128, 128) {
		t.Fatalf("img=%v err=%v asked=%d", img.Bounds(), err, *asked)
	}
}

func TestMeshExportFallsBackToTheCPU(t *testing.T) {
	mesh, err := ParseSTL([]byte(triangleSTL))
	if err != nil {
		t.Fatal(err)
	}
	asked := drawing(t, nil)
	img, err := ExportImage(Result{Mesh: mesh}, 128)
	if err != nil || *asked != 1 || img.Bounds() != image.Rect(0, 0, 128, 128) {
		t.Fatalf("err=%v asked=%d", err, *asked)
	}
	colored := 0
	for y := range 128 {
		for x := range 128 {
			if r, g, b, _ := img.At(x, y).RGBA(); r != g || g != b {
				colored++
			}
		}
	}
	if colored < 100 {
		t.Fatalf("mesh not visible: %d colored pixels", colored)
	}
}

func TestGPUDrawsTheMesh(t *testing.T) {
	data, err := os.ReadFile("../../examples/gloss.stl")
	if err != nil {
		t.Fatal(err)
	}
	mesh, err := ParseSTL(data)
	if err != nil {
		t.Fatal(err)
	}
	camera := charts.DefaultCamera()
	img, ok := gpuSnapshot(mesh, 256, camera)
	if !ok {
		t.Skip("no GPU here; the CPU draws exports")
	}
	if img.Bounds() != image.Rect(0, 0, 256, 256) {
		t.Fatal(img.Bounds())
	}
	colored, white := 0, 0
	for y := range 256 {
		for x := range 256 {
			r, g, b, a := img.At(x, y).RGBA()
			if a != 0xffff {
				t.Fatalf("the export is not opaque at %d,%d", x, y)
			}
			switch {
			case r == 0xffff && g == 0xffff && b == 0xffff:
				white++
			case r != g || g != b:
				colored++
			}
		}
	}
	// The model, on white, and nothing else: no axes, grid, or legend.
	if colored < 2000 || white < 256*256/3 || white+colored < 256*256*95/100 {
		t.Fatalf("colored=%d white=%d of %d", colored, white, 256*256)
	}
	if img.At(2, 2) != img.At(253, 253) {
		t.Fatal("the corners differ: something is drawn at the edge")
	}
	camera.Beta += 90
	turned, ok := gpuSnapshot(mesh, 256, camera)
	if !ok {
		t.Fatal("the GPU was lost")
	}
	differing := 0
	for y := range 256 {
		for x := range 256 {
			if img.At(x, y) != turned.At(x, y) {
				differing++
			}
		}
	}
	if differing < 500 {
		t.Fatalf("turning the camera changed %d pixels", differing)
	}
}
