package document

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/NimbleMarkets/gloss/examples"
)

func TestEmbeddedGallery(t *testing.T) {
	for _, name := range examples.Names {
		t.Run(name, func(t *testing.T) {
			l := &Loader{Files: examples.Files}
			defer l.Close()
			r := l.Load(Request{Path: name, Page: 1, DPI: 72, Generation: 1})
			if r.Err != nil {
				t.Fatal(r.Err)
			}
			switch r.Kind {
			case "markdown":
				if len(r.Markdown.Images) != 3 {
					t.Fatalf("images: %d", len(r.Markdown.Images))
				}
				for _, asset := range r.Markdown.Images {
					if asset.Err != nil || asset.Image == nil {
						t.Fatalf("asset: %+v", asset)
					}
				}
			case "stl":
				if r.Mesh == nil {
					t.Fatal("no mesh")
				}
			default:
				if r.Image == nil || r.Image.Bounds().Empty() {
					t.Fatal("no image")
				}
			}
			if r.Kind == "pdf" {
				r = l.Load(Request{Path: name, Page: 2, DPI: 72, Generation: 2})
				if r.Err != nil || r.Pages != 2 || r.Page != 2 {
					t.Fatalf("PDF page 2: %+v", r)
				}
			}
		})
	}
}

func TestHEICDetectionAndExport(t *testing.T) {
	data, err := examples.Files.ReadFile("landscape.heic")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"photo.heic", "photo.HEIF", "undecorated"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if kind, err := Probe(path, ""); err != nil || kind != "heic" {
			t.Fatalf("probe %s: %s %v", name, kind, err)
		}
		l := &Loader{}
		r := l.Load(Request{Path: path, Generation: 1})
		l.Close()
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		if r.Image.Bounds() != image.Rect(0, 0, 640, 400) {
			t.Fatal(r.Image.Bounds())
		}
		out, err := ExportForVision(r, 320, "")
		if err != nil || out.Bounds().Dx() != 320 {
			t.Fatalf("export: %v", err)
		}
	}
	if _, err := decodeRaster(data[:100]); err == nil {
		t.Fatal("truncated HEIC accepted")
	}
	// Check known colors in the desert fixture, allowing for lossy compression.
	got, err := decodeRaster(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		x, y int
		rgb  [3]uint32
	}{
		{40, 40, [3]uint32{35, 155, 163}},    // turquoise sky
		{155, 115, [3]uint32{255, 226, 158}}, // sun
		{470, 280, [3]uint32{23, 70, 64}},    // cactus
	} {
		r, g, b, _ := got.At(sample.x, sample.y).RGBA()
		for i, value := range []uint32{r, g, b} {
			delta := int(value>>8) - int(sample.rgb[i])
			if delta < -12 || delta > 12 {
				t.Fatalf("HEIC color at (%d,%d): got %v", sample.x, sample.y, []uint32{r >> 8, g >> 8, b >> 8})
			}
		}
	}
}
