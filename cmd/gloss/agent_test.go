package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
)

// The headless contract for agents: stdout carries the answer, in the
// form asked for. Paths written, one to a line, unless the bytes were
// asked for; a manifest with --json.

func TestExportPrintsThePathsWritten(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := options{Options: app.Options{Files: []string{"../../examples/shapes.svg"}, Output: filepath.Join(dir, "shapes.png"), MaxEdge: 256, Page: 1, DPI: 72}}
	if err := exportFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != filepath.Join(dir, "shapes.png") {
		t.Fatalf("stdout: %q", stdout.String())
	}
	// A taken name gains a suffix, as the viewer's export does; the path
	// printed is the one written.
	stdout.Reset()
	if err := exportFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != filepath.Join(dir, "shapes-2.png") {
		t.Fatalf("second export: %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "shapes-2.png")); err != nil {
		t.Fatal(err)
	}
}

func TestExportBatchGoesOnPastAFailure(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.png")
	os.WriteFile(bad, []byte("not a png"), 0600)
	var stdout, stderr bytes.Buffer
	opts := options{Options: app.Options{Files: []string{bad, "../../examples/shapes.svg"}, OutputDir: filepath.Join(dir, "out"), MaxEdge: 256, Page: 1, DPI: 72}}
	err := exportFiles(opts, &stdout, &stderr)
	if err == nil {
		t.Fatal("no error for the bad file")
	}
	if !strings.Contains(stdout.String(), "002-shapes.png") || !strings.Contains(stderr.String(), "bad.png") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExportManifest(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := options{Options: app.Options{Files: []string{"../../examples/shapes.svg", "missing.svg"}, OutputDir: dir, MaxEdge: 256, Page: 1, DPI: 72}, JSON: true}
	if err := exportFiles(opts, &stdout, &stderr); err == nil {
		t.Fatal("no error for the missing file")
	}
	var manifest []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil || len(manifest) != 2 {
		t.Fatalf("manifest: %v %s", err, stdout.String())
	}
	first := manifest[0]
	if first["kind"] != "svg" || first["width"] != 256.0 || !strings.HasSuffix(first["output"].(string), "001-shapes.png") || first["error"] != nil {
		t.Fatalf("first: %v", first)
	}
	if manifest[1]["error"] == nil || manifest[1]["output"] != nil {
		t.Fatalf("second: %v", manifest[1])
	}
}

func TestTextTakesTheTextOut(t *testing.T) {
	var stdout, stderr bytes.Buffer
	text := func(file string, more ...string) string {
		stdout.Reset()
		opts := options{Options: app.Options{Files: append([]string{file}, more...), Page: 1, DPI: 72}, Text: true}
		if err := textFiles(opts, &stdout, &stderr); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		return stdout.String()
	}
	if got := text("../../examples/field-notes.docx"); !strings.Contains(got, "# Field notes") || !strings.Contains(got, "| Day |") {
		t.Errorf("docx: %q", got)
	}
	if got := text("../../examples/field-notes.html"); !strings.Contains(got, "# Field notes") || strings.Contains(got, "<h1>") {
		t.Errorf("html: %q", got)
	}
	if got := text("../../examples/analysis.ipynb"); !strings.Contains(got, "```python") {
		t.Errorf("ipynb: %q", got)
	}
	if got := text("../../examples/sales.xlsx"); !strings.HasPrefix(got, "Region,Quarter,Units,Revenue,Booked\n") || !strings.Contains(got, "North,Q1,120,48.5,2026-03-31") {
		t.Errorf("xlsx: %q", got)
	}
	// A Grist table is a sheet: its labels, then its rows as Grist orders them.
	if got := text("../../examples/notes.grist"); !strings.HasPrefix(got, "Species,Site,How many,Weight (kg),Confirmed?,Seen on,Logged at,Tags,Source,Count per kg,Photos\nOtter,Alder Creek,1,8,TRUE,2026-03-09,") {
		t.Errorf("grist: %q", got)
	}
	if got := text("../../examples/sales.csv"); !strings.HasPrefix(got, "region,quarter,units,revenue,booked\n") {
		t.Errorf("csv: %q", got)
	}
	// Text and JSON come out as they are, not fenced for a viewer.
	if got := text("../../examples/notes.txt"); !strings.HasPrefix(got, "Field notes, kept as plain text\n") || strings.Contains(got, "```") {
		t.Errorf("txt: %q", got)
	}
	if got := text("../../examples/batch.jsonl"); !strings.HasPrefix(got, "{\n  \"body\"") || !strings.Contains(got, "\"custom_id\": \"q-1\"") || strings.Contains(got, "```") || strings.Contains(got, "####") {
		t.Errorf("jsonl: %q", got)
	}
	// A picture has no text: say so, and say what to do instead.
	stdout.Reset()
	opts := options{Options: app.Options{Files: []string{"../../examples/shapes.svg"}, Page: 1, DPI: 72}, Text: true}
	if err := textFiles(opts, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "--output") {
		t.Fatalf("svg: %v", err)
	}
	// Several files go to files, or to a manifest; not to one stream.
	stdout.Reset()
	opts.Files = []string{"../../examples/notes.txt", "../../examples/sales.csv"}
	if err := textFiles(opts, &stdout, &stderr); err == nil {
		t.Fatal("two files on one stream")
	}
	dir := t.TempDir()
	opts.OutputDir = dir
	stdout.Reset()
	if err := textFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(stdout.String()), "\n"); len(lines) != 2 || !strings.HasSuffix(lines[0], "001-notes.txt") || !strings.HasSuffix(lines[1], "002-sales.csv") {
		t.Fatalf("paths: %q", stdout.String())
	}
	opts.OutputDir, opts.JSON = "", true
	stdout.Reset()
	if err := textFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var manifest []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil || len(manifest) != 2 || !strings.HasPrefix(manifest[1]["text"].(string), "region,") {
		t.Fatalf("manifest: %v %s", err, stdout.String())
	}
}

func TestPageRanges(t *testing.T) {
	for in, want := range map[string][]int{"1": {1}, "3": {3}, "2-4": {2, 3, 4}, "1,3": {1, 3}, "all": nil} {
		got, err := parsePages(in)
		if err != nil || len(got.pages) != len(want) || got.all != (in == "all") {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"0", "x", "3-1", ""} {
		if _, err := parsePages(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// The viewer takes one page; an export takes a range into a folder.
	if _, _, err := parse([]string{"--page", "2-3", "a.pdf"}, &bytes.Buffer{}); err == nil {
		t.Error("the viewer took a range")
	}
	opts, _, err := parse([]string{"--page", "all", "--output-dir", "out", "a.pdf"}, &bytes.Buffer{})
	if err != nil || !opts.Pages.all {
		t.Fatalf("%+v %v", opts.Pages, err)
	}
	if _, _, err := parse([]string{"--page", "all", "-o", "one.png", "a.pdf"}, &bytes.Buffer{}); err == nil {
		t.Error("a range into one file")
	}
	// Every page of a PDF, and every sheet of a workbook.
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts = options{Options: app.Options{Files: []string{"../../examples/field-guide.pdf"}, OutputDir: dir, MaxEdge: 256, Page: 1, DPI: 36}, Pages: pageRange{all: true}}
	if err := exportFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(stdout.String()), "\n"); len(lines) != 2 || !strings.HasSuffix(lines[1], "001-field-guide-page-2.png") {
		t.Fatalf("pages: %q", stdout.String())
	}
	stdout.Reset()
	opts = options{Options: app.Options{Files: []string{"../../examples/sales.xlsx"}, OutputDir: dir, Page: 1, DPI: 72}, Text: true, Pages: pageRange{all: true}}
	if err := textFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(stdout.String()), "\n"); len(lines) != 2 || !strings.HasSuffix(lines[1], "001-sales-sheet-2.csv") {
		t.Fatalf("sheets: %q", stdout.String())
	}
	stdout.Reset()
	opts = options{Options: app.Options{Files: []string{"../../examples/notes.grist"}, OutputDir: dir, Page: 1, DPI: 72}, Text: true, Pages: pageRange{all: true}}
	if err := textFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(stdout.String()), "\n"); len(lines) != 4 || !strings.HasSuffix(lines[1], "001-notes-table-2.csv") {
		t.Fatalf("tables: %q", stdout.String())
	}
}

func TestTextOfPDFPagesGoesToFilesLikeSheets(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	pages, _ := parsePages("all")
	opts := options{Options: app.Options{Files: []string{"../../examples/field-guide.pdf"}, OutputDir: dir, Page: 1}, Text: true, Pages: pages}
	if err := textFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "001-field-guide-page-1.txt"), filepath.Join(dir, "001-field-guide-page-2.txt")}
	if strings.TrimSpace(stdout.String()) != strings.Join(want, "\n") {
		t.Fatalf("stdout: %q", stdout.String())
	}
	if data, err := os.ReadFile(want[1]); err != nil || !strings.Contains(string(data), "SMALL FILES") {
		t.Fatalf("page 2: %q %v", data, err)
	}
}

func TestTextOfAScannedPageSaysSoInTheManifest(t *testing.T) {
	dir := t.TempDir()
	scan := filepath.Join(dir, "scan.pdf")
	// A page with nothing on it, then the real guide: the failure does not
	// discard the success.
	os.WriteFile(scan, blankPDF(), 0600)
	var stdout, stderr bytes.Buffer
	opts := options{Options: app.Options{Files: []string{scan, "../../examples/field-guide.pdf"}, Page: 1}, Text: true, JSON: true}
	if err := textFiles(opts, &stdout, &stderr); err == nil {
		t.Fatal("a page without text was a success")
	}
	var manifest []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil || len(manifest) != 2 {
		t.Fatalf("manifest: %v %s", err, stdout.String())
	}
	if e, _ := manifest[0]["error"].(string); !strings.Contains(e, "no text layer") || manifest[0]["text"] != nil {
		t.Fatalf("scan: %v", manifest[0])
	}
	if manifest[1]["error"] != nil || !strings.Contains(manifest[1]["text"].(string), "FIELD GUIDE") {
		t.Fatalf("guide: %v", manifest[1])
	}
}

func TestManifestSaysHowTheSizeWasResolved(t *testing.T) {
	export := func(args ...string) map[string]any {
		t.Helper()
		opts, _, err := parse(append(args, "--json", "-O", t.TempDir(), "../../examples/shapes.svg"), &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		var stdout bytes.Buffer
		if err := exportFiles(opts, &stdout, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		var manifest []map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil || len(manifest) != 1 {
			t.Fatalf("%v %s", err, stdout.String())
		}
		return manifest[0]
	}
	if m := export("--max-edge", "300"); m["max_edge"] != 300.0 || m["vision_profile"] != nil || m["vision_reason"] != nil {
		t.Fatalf("no profile: %v", m)
	}
	if m := export("--vision-profile", "claude-standard"); m["max_edge"] != 1092.0 || m["vision_profile"] != "claude-standard" || m["vision_reason"] == "" {
		t.Fatalf("profile: %v", m)
	}
	// An edge given outright is the caller's.
	if m := export("--vision-profile", "claude-standard", "--max-edge", "300"); m["max_edge"] != 300.0 || m["width"] != 300.0 || m["vision_profile"] != "claude-standard" {
		t.Fatalf("both: %v", m)
	}
}

// blankPDF is a one-page PDF with nothing drawn on it, as a scan has no text.
func blankPDF() []byte {
	var b bytes.Buffer
	var offsets []int
	b.WriteString("%PDF-1.4\n")
	for i, body := range []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>"} {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	start := b.Len()
	b.WriteString("xref\n0 4\n0000000000 65535 f \n")
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", start)
	return b.Bytes()
}

// A page, sheet, or part that is not there is an error, never the last one
// standing in for it; a range says so once, after the pages there are.
func TestExportAndTextRefuseWhatIsNotThere(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		text  bool
		file  string
		page  string
		parts string
		want  string
	}{
		{true, "field-guide.pdf", "3", "", "page 3 is past the end: the PDF has 2 pages"},
		{false, "field-guide.pdf", "9", "", "page 9 is past the end"},
		{true, "sales.xlsx", "9", "", "sheet 9 is past the end: the workbook has 2 sheets"},
		{true, "notes.grist", "5", "", "table 5 is past the end: the Grist document has 4 tables"},
		{false, "lantern.3mf", "1", "7", "no part 7: the model has 3 parts: Plinth, Column, Cap"},
	} {
		pages, err := parsePages(c.page)
		if err != nil {
			t.Fatal(err)
		}
		page, _ := pages.single()
		opts := options{Options: app.Options{Files: []string{"../../examples/" + c.file}, Page: page, DPI: 72, MaxEdge: 64, Output: filepath.Join(dir, c.file+".png")}, Text: c.text, Pages: pages}
		if c.text {
			opts.Output = ""
		}
		if opts.Parts, err = document.ParseParts("", c.parts); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if c.text {
			err = textFiles(opts, &stdout, &stderr)
		} else {
			err = exportFiles(opts, &stdout, &stderr)
		}
		if err == nil || !strings.Contains(err.Error(), c.want) || stdout.Len() != 0 {
			t.Errorf("%s --page %s --partn %q: err=%v stdout=%q", c.file, c.page, c.parts, err, stdout.String())
		}
		// stderr has said it, beside the file: main does not say it again.
		if strings.Count(stderr.String(), c.want) != 1 || !errors.As(err, new(reported)) {
			t.Errorf("%s: stderr=%q", c.file, stderr.String())
		}
	}
	pages, _ := parsePages("1,2-9")
	var stdout, stderr bytes.Buffer
	opts := options{Options: app.Options{Files: []string{"../../examples/field-guide.pdf"}, Page: 1, DPI: 72}, Text: true, JSON: true, Pages: pages}
	if err := textFiles(opts, &stdout, &stderr); err == nil {
		t.Fatal("a range past the end succeeded")
	}
	var manifest []made
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil || len(manifest) != 3 || manifest[1].Text == "" || manifest[2].Page != 3 || !strings.Contains(manifest[2].Error, "past the end") {
		t.Fatalf("manifest: %v %s", err, stdout.String())
	}
}

// A notebook's pictures are inside it: its text, written to a folder, has
// them beside it, linked; on stdout it says they were left out.
func TestTextWritesPackagedPicturesBesideIt(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := options{Options: app.Options{Files: []string{"../../examples/analysis.ipynb"}, Page: 1, DPI: 72, OutputDir: dir}, Text: true}
	if err := textFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "](001-analysis-pictures/cell-3-output-1.png)") || strings.Contains(string(text), "dropped/") {
		t.Fatalf("links: %s", text)
	}
	if _, err := os.Stat(filepath.Join(dir, "001-analysis-pictures", "cell-3-output-1.png")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	opts.OutputDir, opts.JSON = "", true
	if err := textFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var manifest []made
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil || len(manifest) != 1 || !strings.Contains(manifest[0].Note, "1 picture not written") {
		t.Fatalf("manifest: %v %s", err, stdout.String())
	}
}
