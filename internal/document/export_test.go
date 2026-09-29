package document

import (
	"image"
	"image/color"
	"testing"
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
