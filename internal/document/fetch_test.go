package document

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSheetLinks(t *testing.T) {
	rels := `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/photo.png" TargetMode="External"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="mailto:someone@example.com" TargetMode="External"/></Relationships>`
	sheet := `<row r="1">` + cell("A1", "inlineStr", "Photo") + cell("B1", "inlineStr", "Write") + cell("C1", "inlineStr", "https://example.com/plain.jpg") + cell("D1", "inlineStr", "ftp://example.com/x") + cell("E1", "inlineStr", "Here") + `</row>`
	data := workbook(t, map[string]string{"1-Links": sheet}, map[string]string{"xl/worksheets/_rels/sheet1.xml.rels": rels})
	// The workbook builder wraps sheetData; hyperlinks follow it.
	book, err := OpenWorkbook(rezip(t, data, "xl/worksheets/sheet1.xml", `</sheetData><hyperlinks><hyperlink ref="A1" r:id="rId1"/><hyperlink ref="B1" r:id="rId2"/><hyperlink ref="E1" location="Sheet2!A1"/></hyperlinks></worksheet>`))
	if err != nil {
		t.Fatal(err)
	}
	s, err := book.Sheet(0)
	if err != nil {
		t.Fatal(err)
	}
	for col, want := range []string{"https://example.com/photo.png", "", "https://example.com/plain.jpg", "", ""} {
		if got := s.URL(0, col); got != want {
			t.Errorf("column %d: URL %q, want %q", col, got, want)
		}
	}
	if s.URL(5, 0) != "" || s.URL(0, 9) != "" {
		t.Error("a cell there is not has a URL")
	}
	csv, _ := ReadCSV("t.csv", []byte("name,picture\nfern,http://example.com/fern.jpg\nmoss,not a link\n"))
	if csv.URL(1, 1) != "http://example.com/fern.jpg" || csv.URL(2, 1) != "" || csv.URL(0, 1) != "" {
		t.Errorf("CSV links: %q %q", csv.URL(1, 1), csv.URL(2, 1))
	}
}

// rezip replaces the tail of one part of an archive.
func rezip(t *testing.T, data []byte, part, tail string) []byte {
	t.Helper()
	p, err := openOPC(data)
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for name := range p.files {
		content, _ := p.read(name)
		parts[name] = string(content)
	}
	parts[part] = strings.Replace(parts[part], "</sheetData></worksheet>", tail, 1)
	return archive3MF(t, parts)
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestFetch(t *testing.T) {
	served := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/photos/holiday.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes(t))
		case "/photo":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes(t))
		case "/readme":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("# Hello\n\nA readme.\n"))
		case "/moved":
			http.Redirect(w, r, "/photos/holiday.png", http.StatusFound)
		case "/away":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		case "/huge":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes(t))
			w.Write(make([]byte, MaxFileBytes))
		case "/slow":
			time.Sleep(2 * time.Second)
		case "/blob":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("not a picture, nor anything else gloss shows"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer served.Close()
	dir := t.TempDir()
	fetch := func(url string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return Fetch(ctx, url, dir)
	}
	path, err := fetch(served.URL + "/photos/holiday.png")
	if err != nil || filepath.Base(path) != "holiday.png" || filepath.Dir(path) != dir {
		t.Fatalf("path=%q err=%v", path, err)
	}
	if kind, err := Probe(path, ""); err != nil || kind != "png" {
		t.Fatalf("what was fetched: %q %v", kind, err)
	}
	// A name is taken from the content where the address has none.
	if path, err := fetch(served.URL + "/photo"); err != nil || filepath.Base(path) != "photo.png" {
		t.Errorf("path=%q err=%v", path, err)
	}
	if path, err := fetch(served.URL + "/moved"); err != nil || filepath.Base(path) != "holiday-2.png" {
		t.Errorf("after a redirect: path=%q err=%v", path, err)
	}
	// Anything gloss shows may be fetched, not only pictures.
	if path, err := fetch(served.URL + "/readme"); err != nil || filepath.Ext(path) != ".md" {
		t.Errorf("readme: path=%q err=%v", path, err)
	}
	for name, url := range map[string]string{
		"not a picture":     served.URL + "/blob",
		"missing":           served.URL + "/gone",
		"too large":         served.URL + "/huge",
		"too slow":          served.URL + "/slow",
		"away from the web": served.URL + "/away",
		"a file":            "file:///etc/passwd",
		"mail":              "mailto:someone@example.com",
		"nothing":           "",
	} {
		if path, err := fetch(url); err == nil {
			t.Errorf("%s: fetched to %q", name, path)
		} else if strings.Contains(err.Error(), "\x1b") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// What could not be used was not kept.
	left, _ := os.ReadDir(dir)
	var names []string
	for _, f := range left {
		names = append(names, f.Name())
	}
	if want := "holiday-2.png holiday.png photo.png readme.md"; strings.Join(names, " ") != want {
		t.Errorf("left %q, want %q", names, want)
	}
	fmt.Fprint(os.Stderr)
}
