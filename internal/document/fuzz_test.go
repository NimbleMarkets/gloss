package document

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/NimbleMarkets/gloss/examples"
)

// The parsers take files from anywhere. Each must refuse or read what it
// is given without panicking, however the bytes are arranged; the loader's
// recover is a net, not a plan. These run as tests on their seeds, and as
// fuzzers with go test -fuzz=Fuzz -run=^$ ./internal/document/.

func FuzzParseSTL(f *testing.F) {
	f.Add([]byte(facetSTL))
	f.Add(make([]byte, 84))
	f.Add([]byte("solid\nfacet normal 0 0 0\nouter loop\nvertex 0 0 0\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if mesh, err := ParseSTL(data); err == nil && mesh == nil {
			t.Fatal("no mesh and no error")
		}
	})
}

func FuzzParse3MF(f *testing.F) {
	f.Add(simple3MF(f))
	f.Add(assembly3MF(f))
	f.Add([]byte("PK\x03\x04 not really a package"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if model, err := Parse3MF(data); err == nil && (model == nil || model.Mesh == nil && model.Thumbnail == nil) {
			t.Fatal("no model and no error")
		}
	})
}

func FuzzReadCSV(f *testing.F) {
	f.Add("a,b,c\n1,2,3\n")
	f.Add("a\tb\n\"quoted, with\ncomma\"\t2\n")
	f.Add("\xff\xfe\x00")
	f.Fuzz(func(t *testing.T, data string) {
		ReadCSV("f.csv", []byte(data))
	})
}

func FuzzReadJSON(f *testing.F) {
	f.Add(`{"a": [1, 2, {"b": null}]}`)
	f.Add("{\"a\":1}\n{\"b\":2}\n")
	f.Add("{not json")
	f.Fuzz(func(t *testing.T, data string) {
		ReadJSON("f.json", []byte(data))
	})
}

func FuzzReadText(f *testing.F) {
	f.Add("plain\ttext\r\n```\n")
	f.Add("\xff")
	f.Fuzz(func(t *testing.T, data string) {
		ReadText("f.txt", []byte(data))
		IsText([]byte(data))
	})
}

func FuzzReadHTML(f *testing.F) {
	f.Add("<html><body><h1>x</h1><table><tr><td>1</td></tr></table></body></html>")
	f.Add("<<<>>> &amp; <a href=x")
	f.Fuzz(func(t *testing.T, data string) {
		ReadHTML([]byte(data))
	})
}

func FuzzReadNotebook(f *testing.F) {
	f.Add(`{"cells":[{"cell_type":"code","source":"x","outputs":[{"output_type":"stream","text":["y"]}]}],"nbformat":4}`)
	f.Add(`{"cells": "no"}`)
	f.Fuzz(func(t *testing.T, data string) {
		ReadNotebook([]byte(data))
	})
}

func FuzzOpenWorkbook(f *testing.F) {
	f.Add(workbook(f, map[string]string{"Sheet1": `<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1"><v>2.5</v></c></row>`}, map[string]string{"xl/sharedStrings.xml": `<sst><si><t>x</t></si></sst>`}))
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if book, err := OpenWorkbook(data); err == nil {
			for i := range book.Names() {
				book.Sheet(i)
			}
		}
	})
}

func FuzzOpenWord(f *testing.F) {
	f.Add(wordDocument(f, `<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`, nil))
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, data []byte) {
		OpenWord(data)
	})
}

func FuzzSharedStrings(f *testing.F) {
	f.Add(`<sst><si><t>a</t></si><si><r><t>b</t></r><rPh><t>c</t></rPh></si></sst>`)
	f.Fuzz(func(t *testing.T, data string) {
		sharedStrings([]byte(data))
	})
}

// seedImage is a few pixels, encoded as each format the standard library writes.
func seedImage(f *testing.F, encode func(*bytes.Buffer, image.Image) error) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(1, 1, color.RGBA{R: 200, A: 255})
	var b bytes.Buffer
	if err := encode(&b, img); err != nil {
		f.Fatal(err)
	}
	return b.Bytes()
}

func FuzzDecodeRaster(f *testing.F) {
	f.Add(seedImage(f, func(b *bytes.Buffer, i image.Image) error { return png.Encode(b, i) }))
	f.Add(seedImage(f, func(b *bytes.Buffer, i image.Image) error { return jpeg.Encode(b, i, nil) }))
	f.Add(seedImage(f, func(b *bytes.Buffer, i image.Image) error { return gif.Encode(b, i, nil) }))
	for _, name := range []string{"landscape.png", "landscape.heic"} {
		if data, err := examples.Files.ReadFile(name); err == nil {
			f.Add(data)
		}
	}
	f.Add([]byte("BM"))
	f.Add([]byte("II*\x00"))
	f.Add([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "))
	f.Fuzz(func(t *testing.T, data []byte) {
		// Dimensions are read before pixels are allocated, so whatever the
		// header claims, the decode stays inside the pixel budget.
		if img, err := decodeRaster(data); err == nil {
			if b := img.Bounds(); b.Dx() <= 0 || b.Dy() <= 0 || b.Dx() > MaxPixels/b.Dy() {
				t.Fatalf("decoded %v, over the %d-pixel budget", b, MaxPixels)
			}
		}
	})
}

func FuzzRenderSVG(f *testing.F) {
	if data, err := examples.Files.ReadFile("shapes.svg"); err == nil {
		f.Add(data)
	}
	f.Add([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1e9" height="1e9"><rect width="5" height="5"/></svg>`))
	f.Add([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 0"><use href="#a"/></svg>`))
	f.Add([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><g id="a"><use href="#a"/></g></svg>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		renderSVG("f.svg", data, 32)
		svgFields(data)
	})
}

func FuzzMarkdown(f *testing.F) {
	f.Add([]byte("# Title\n\n![a](x.png)\n\n[ref]: y.svg\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"))
	f.Add([]byte("![](../../../../etc/passwd)\n![](/dev/zero)\n![](https://example.com/x.png)\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		loadMarkdown("f.md", data, t.TempDir())
	})
}

// The option parsers read words a person or an agent typed; none may panic,
// and what they accept must be inside the bounds they promise.
func FuzzParseOptionWords(f *testing.F) {
	f.Add("#ff8800", "name,other", "2,4-6", "front,top", "20,-120,3")
	f.Add("orange", "", "1-", "all", "90")
	f.Add("", ",,,", "-1,0,999999999999999999999", "iso,,", "1e309,nan,inf")
	f.Fuzz(func(t *testing.T, colour, names, indexes, views, camera string) {
		ParseColor(colour)
		ParseColumns(names, indexes)
		ParseParts(names, indexes)
		ParseViews(views)
		ParseCamera(camera)
		ParseDrop(names + "\n" + colour)
	})
}

// prettyJSON is json.Indent with a ceiling: given room, it must lay out every
// valid value exactly as json.Indent does, and given little, it must keep to it.
func FuzzPrettyJSON(f *testing.F) {
	for _, seed := range []string{`{"a":[1,2,{"b":null}],"c":"x\"y,]"}`, `[]`, `{}`, `[[],{}]`, `  [ 1 , 2 ]  `, `"a\\"`, `-1.5e+3`, `[{"k":[[],[{}]]}]`} {
		f.Add(seed, 7)
	}
	f.Fuzz(func(t *testing.T, doc string, limit int) {
		if !json.Valid([]byte(doc)) {
			return
		}
		src := bytes.TrimSpace([]byte(doc))
		var want bytes.Buffer
		if err := json.Indent(&want, src, "", "  "); err != nil {
			return
		}
		got, lines, cut := prettyJSON(src, want.Len()+16)
		if cut || !bytes.Equal(got, want.Bytes()) || lines != bytes.Count(want.Bytes(), []byte("\n"))+1 {
			t.Fatalf("not json.Indent:\ndoc=%q\ngot=%q (cut=%v lines=%d)\nwant=%q", doc, got, cut, lines, want.Bytes())
		}
		limit = max(0, min(limit, want.Len()))
		small, _, tooBig := prettyJSON(src, limit)
		if len(small) > limit || (limit < want.Len()) != tooBig || !bytes.HasPrefix(want.Bytes(), small) {
			t.Fatalf("limit %d: kept %d bytes, cut=%v, of %d", limit, len(small), tooBig, want.Len())
		}
	})
}
