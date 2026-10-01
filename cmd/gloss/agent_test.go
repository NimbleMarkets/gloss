package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/app"
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
