package document

import (
	"bytes"
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
