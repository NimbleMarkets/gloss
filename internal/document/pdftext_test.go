package document

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// blankPDF is a one-page PDF with nothing drawn on it: no text layer, as a
// scan has none of its own.
func blankPDF() []byte {
	var b bytes.Buffer
	var offsets []int
	b.WriteString("%PDF-1.4\n")
	for i, body := range []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
	} {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	start := b.Len()
	fmt.Fprintf(&b, "xref\n0 4\n0000000000 65535 f \n")
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", start)
	return b.Bytes()
}

func TestPDFTextLayer(t *testing.T) {
	loader := &Loader{}
	defer loader.Close()
	r := loader.Load(Request{Path: "../../examples/field-guide.pdf", Page: 2, TextOnly: true, Generation: 1})
	text, ext, err := Text(r)
	if err != nil || ext != ".txt" || r.Page != 2 || r.Pages != 2 || r.Image != nil {
		t.Fatalf("page 2: %q %q %v (page %d of %d, image %v)", text, ext, err, r.Page, r.Pages, r.Image != nil)
	}
	if !strings.HasPrefix(string(text), "SMALL FILES. MANY VIEWS.") || !strings.HasSuffix(string(text), "\n") {
		t.Fatalf("text: %q", text)
	}
	// The same loader still draws a page: the renderer is made when wanted.
	if r := loader.Load(Request{Path: "../../examples/field-guide.pdf", Page: 1, MaxEdge: 256, Generation: 2}); r.Err != nil || r.Image == nil {
		t.Fatalf("raster after text: %v", r.Err)
	}
}

func TestPDFPageWithoutTextLayerIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scan.pdf")
	if err := os.WriteFile(path, blankPDF(), 0600); err != nil {
		t.Fatal(err)
	}
	loader := &Loader{}
	defer loader.Close()
	r := loader.Load(Request{Path: path, Page: 1, TextOnly: true, Generation: 1})
	text, _, err := Text(r)
	if err == nil || len(text) != 0 || !strings.Contains(err.Error(), "no text layer") || !strings.Contains(err.Error(), "--output") {
		t.Fatalf("blank page: %q %v", text, err)
	}
	if r.Kind != "pdf" || r.Pages != 1 {
		t.Fatalf("the page still counts: kind %q pages %d", r.Kind, r.Pages)
	}
}

func TestPicturesAndMeshesHaveNoText(t *testing.T) {
	for _, path := range []string{"../../examples/shapes.svg", "../../examples/landscape.png", "../../examples/tetrahedron.stl"} {
		loader := &Loader{}
		_, _, err := Text(loader.Load(Request{Path: path, Page: 1, TextOnly: true, Generation: 1}))
		loader.Close()
		if err == nil || !strings.Contains(err.Error(), "--output") {
			t.Errorf("%s: %v", path, err)
		}
	}
}

// pagesPDF is a PDF whose pages draw what contents say, with Helvetica as /F1,
// a one-pixel image as /Im1, and a form that draws it twice as /Fm1.
func pagesPDF(contents ...string) []byte {
	var b bytes.Buffer
	var offsets []int
	object := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.4\n")
	kids := make([]string, len(contents))
	for i := range contents {
		kids[i] = fmt.Sprintf("%d 0 R", 6+2*i)
	}
	object("<< /Type /Catalog /Pages 2 0 R >>")
	object(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(contents)))
	object("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	object("<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 1 >>\nstream\n\x80\nendstream")
	form := "q /Im1 Do Q q /Im1 Do Q"
	object(fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] /Resources << /XObject << /Im1 4 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form))
	for i, c := range contents {
		object(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 3 0 R >> /XObject << /Im1 4 0 R /Fm1 5 0 R >> >> /Contents %d 0 R >>", 7+2*i))
		object(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c))
	}
	start := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, start)
	return b.Bytes()
}

func TestPDFTextLayerSaysWhenThePageIsAPicture(t *testing.T) {
	long := strings.Repeat("words ", 40)
	path := filepath.Join(t.TempDir(), "mixed.pdf")
	if err := os.WriteFile(path, pagesPDF(
		"BT /F1 12 Tf 10 10 Td (Fig. 1) Tj ET q 100 0 0 100 0 0 cm /Im1 Do Q", // A figure and its caption.
		"BT /F1 12 Tf 10 10 Td ("+long+") Tj ET q /Im1 Do Q",                  // Text, with a picture.
		"BT /F1 12 Tf 10 10 Td (Only words.) Tj ET",                           // Text alone.
		"q /Fm1 Do Q", // A scan: pictures, and no text.
	), 0600); err != nil {
		t.Fatal(err)
	}
	loader := &Loader{}
	defer loader.Close()
	for _, c := range []struct {
		page, chars, images int
		sparse, noText      bool
	}{
		{1, 6, 1, true, false},
		{2, len(strings.TrimSpace(long)), 1, false, false},
		{3, 11, 0, false, false},
		{4, 0, 2, true, true},
	} {
		r := loader.Load(Request{Path: path, Page: c.page, TextOnly: true, Generation: uint64(c.page)})
		if c.noText != errors.Is(r.Err, ErrNoTextLayer) || (!c.noText && r.Err != nil) {
			t.Fatalf("page %d: %v", c.page, r.Err)
		}
		if r.Layer == nil || r.Layer.Images != c.images || r.Layer.Sparse != c.sparse || (!c.noText && r.Layer.Chars != c.chars) {
			t.Errorf("page %d: %+v, want chars %d images %d sparse %v", c.page, r.Layer, c.chars, c.images, c.sparse)
		}
	}
}
