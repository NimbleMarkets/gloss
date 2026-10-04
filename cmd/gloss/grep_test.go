package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/app"
)

// figurePDF writes a PDF whose pages draw what contents say, with Helvetica
// as /F1 and a one-pixel image as /Im1.
func figurePDF(t *testing.T, contents ...string) string {
	t.Helper()
	var b bytes.Buffer
	var offsets []int
	object := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.4\n")
	kids := make([]string, len(contents))
	for i := range contents {
		kids[i] = fmt.Sprintf("%d 0 R", 5+2*i)
	}
	object("<< /Type /Catalog /Pages 2 0 R >>")
	object(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(contents)))
	object("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	object("<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 1 >>\nstream\n\x80\nendstream")
	for i, c := range contents {
		object(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 3 0 R >> /XObject << /Im1 4 0 R >> >> /Contents %d 0 R >>", 6+2*i))
		object(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c))
	}
	start := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, start)
	path := filepath.Join(t.TempDir(), "figures.pdf")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTextManifestSaysWhenAPageIsAPicture(t *testing.T) {
	path := figurePDF(t,
		"BT /F1 12 Tf 10 10 Td (Fig. 1) Tj ET q /Im1 Do Q",
		"BT /F1 12 Tf 10 10 Td (Plain words on a page.) Tj ET",
		"q /Im1 Do Q",
	)
	var stdout, stderr bytes.Buffer
	opts := options{Options: app.Options{Files: []string{path}, DPI: 72}, Text: true, JSON: true, Pages: pageRange{all: true}}
	err := textFiles(opts, &stdout, &stderr)
	if exitCode(err) != 1 {
		t.Fatalf("a page with no text layer must fail the run: %v", err)
	}
	var got []made
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || len(got) != 3 {
		t.Fatalf("manifest: %v %s", err, stdout.String())
	}
	figure, words, scan := got[0], got[1], got[2]
	if figure.Error != "" || !figure.Sparse || figure.Chars == nil || *figure.Chars != 6 || figure.Images == nil || *figure.Images != 1 || figure.Text == "" {
		t.Errorf("figure page: %+v", figure)
	}
	if words.Error != "" || words.Sparse || *words.Chars != 22 || *words.Images != 0 {
		t.Errorf("text page: %+v", words)
	}
	if scan.Error == "" || scan.Text != "" || scan.Sparse || scan.Chars != nil || scan.Images == nil || *scan.Images != 1 {
		t.Errorf("scan page: %+v", scan)
	}
	if !strings.Contains(stderr.String(), "page 1 has 1 image") {
		t.Errorf("stderr: %q", stderr.String())
	}
}

func TestGrepPrintsMatchingPagesOnly(t *testing.T) {
	path := figurePDF(t,
		"BT /F1 12 Tf 10 10 Td (The invoice total is 42.) Tj ET",
		"BT /F1 12 Tf 10 10 Td (Nothing here.) Tj ET",
		"q /Im1 Do Q",
		"BT /F1 12 Tf 10 10 Td (Another invoice, and another invoice.) Tj ET",
	)
	run := func(opts options) (string, string, error) {
		var stdout, stderr bytes.Buffer
		err := grepFiles(opts, &stdout, &stderr)
		return stdout.String(), stderr.String(), err
	}
	opts := options{Options: app.Options{Files: []string{path}, DPI: 72}, Grep: regexp.MustCompile(`invoice`), Pages: pageRange{all: true}}
	out, errs, err := run(opts)
	if err != nil {
		t.Fatalf("a page without text is not a failure of a search: %v", err)
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 3 || lines[0] != "1: The invoice total is 42." || !strings.HasPrefix(lines[1], "4: ") || !strings.HasPrefix(lines[2], "4: ") {
		t.Fatalf("stdout: %q", out)
	}
	if !strings.Contains(errs, "page 3 has no text layer") {
		t.Errorf("stderr: %q", errs)
	}
	opts.JSON = true
	out, _, _ = run(opts)
	var hits []made
	if err := json.Unmarshal([]byte(out), &hits); err != nil || len(hits) != 3 || hits[0].Page != 1 || hits[0].Text != "" || !strings.Contains(hits[0].Excerpt, "invoice") {
		t.Fatalf("json: %v %s", err, out)
	}
	// No match: an empty answer, and not a failure.
	opts.Grep = regexp.MustCompile(`zebra`)
	if out, _, err := run(opts); err != nil || strings.TrimSpace(out) != "[]" {
		t.Fatalf("no match: %q %v", out, err)
	}
	// A picture or a plain file is an error among the others; the PDF is still searched.
	opts.Grep = regexp.MustCompile(`total`)
	opts.Files = []string{"../../examples/shapes.svg", path, "../../examples/notes.txt"}
	out, _, err = run(opts)
	hits = nil
	if exitCode(err) != 1 || json.Unmarshal([]byte(out), &hits) != nil || len(hits) != 3 ||
		!strings.Contains(hits[0].Error, "--output") || hits[1].Page != 1 || hits[1].Error != "" || !strings.Contains(hits[2].Error, "--text") {
		t.Fatalf("mixed: %v %s", err, out)
	}
}

func TestGrepStopsAtItsCap(t *testing.T) {
	path := figurePDF(t, "BT /F1 12 Tf 10 10 Td ("+strings.Repeat("a ", maxGrepHits+50)+") Tj ET")
	var stdout, stderr bytes.Buffer
	opts := options{Options: app.Options{Files: []string{path}, DPI: 72}, Grep: regexp.MustCompile(`a`), JSON: true, Pages: pageRange{all: true}}
	if err := grepFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var hits []made
	if err := json.Unmarshal(stdout.Bytes(), &hits); err != nil || len(hits) != maxGrepHits || !strings.Contains(hits[len(hits)-1].Note, "stopped") {
		t.Fatalf("%d hits, %v", len(hits), err)
	}
	if !strings.Contains(stderr.String(), "stopped at") {
		t.Errorf("stderr: %q", stderr.String())
	}
}

func TestExcerptIsOneBoundedLine(t *testing.T) {
	text := strings.Repeat("x", 500) + "\n\tneedle\x1b[31m here\n" + strings.Repeat("y", 500)
	at := strings.Index(text, "needle")
	got := excerpt(text, at, at+len("needle"))
	if strings.ContainsAny(got, "\n\t\x1b") || !strings.Contains(got, "needle") || !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("excerpt: %q", got)
	}
	if n := len([]rune(got)); n > 2*excerptContext+excerptMatch+4 {
		t.Fatalf("excerpt of %d runes", n)
	}
	if got := excerpt("short", 0, 5); got != "short" {
		t.Fatalf("whole: %q", got)
	}
}

func TestGrepFlags(t *testing.T) {
	opts, _, err := parse([]string{"--grep", "total", "--json", "a.pdf"}, &bytes.Buffer{})
	if err != nil || opts.Grep == nil || !opts.Pages.all {
		t.Fatalf("%+v %v", opts.Pages, err)
	}
	if opts, _, err := parse([]string{"--grep", "total", "-p", "2-3", "a.pdf"}, &bytes.Buffer{}); err != nil || opts.Pages.all {
		t.Fatalf("a range without --json: %v", err)
	}
	for _, args := range [][]string{{"--grep", "", "a.pdf"}, {"--grep", "(", "a.pdf"}, {"--grep", "x", "-o", "a.png", "a.pdf"}, {"--grep", "x", "--info", "a.pdf"}, {"--grep", "x", "--serve"}} {
		if _, _, err := parse(args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func grepPattern(p string) *regexp.Regexp { return regexp.MustCompile(p) }
