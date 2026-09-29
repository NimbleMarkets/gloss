package document

import (
	"bytes"
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
	// Compare against the original PNG, allowing for lossy HEIC compression.
	original, _ := examples.Files.ReadFile("landscape.png")
	png, _, _ := image.Decode(bytes.NewReader(original))
	got, err := decodeRaster(data)
	if err != nil {
		t.Fatal(err)
	}
	var total uint64
	for y := 0; y < 400; y += 8 {
		for x := 0; x < 640; x += 8 {
			a, b, c, _ := png.At(x, y).RGBA()
			d, e, f, _ := got.At(x, y).RGBA()
			for _, pair := range [][2]uint32{{a, d}, {b, e}, {c, f}} {
				if pair[0] > pair[1] {
					total += uint64(pair[0] - pair[1])
				} else {
					total += uint64(pair[1] - pair[0])
				}
			}
		}
	}
	if total/(80*50*3) > 2000 {
		t.Fatalf("HEIC color error too large: %d", total)
	}
}
