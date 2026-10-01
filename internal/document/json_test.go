package document

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestJSONIsPrettyPrintedAsMarkdown(t *testing.T) {
	doc, err := ReadJSON("data.json", []byte(`{"name":"gloss","tags":["tui","go"],"size":3}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "```json\n{\n  \"name\": \"gloss\",\n  \"tags\": [\n    \"tui\",\n    \"go\"\n  ],\n  \"size\": 3\n}\n```\n"
	if string(doc.Markdown) != want {
		t.Fatalf("markdown:\n%s", doc.Markdown)
	}
	if doc.Records != 0 || doc.Lines {
		t.Fatalf("%+v", doc)
	}
	fields := doc.fields()
	if !hasField(fields, "Format", "JSON") || !hasField(fields, "Top level", "object with 3 keys") || !hasField(fields, "Keys", "name, tags, size") {
		t.Fatalf("fields: %v", fields)
	}
}

func TestJSONLinesAreNumberedRecords(t *testing.T) {
	doc, err := ReadJSON("batch.jsonl", []byte("{\"id\":1}\n\n{\"id\":2,\"ok\":true}\n"))
	if err != nil {
		t.Fatal(err)
	}
	md := string(doc.Markdown)
	if !doc.Lines || doc.Records != 2 || strings.Count(md, "```json\n") != 2 || !strings.Contains(md, "#### 1\n") || !strings.Contains(md, "#### 2\n") {
		t.Fatalf("%+v\n%s", doc, md)
	}
	if !hasField(doc.fields(), "Format", "JSON Lines") || !hasField(doc.fields(), "Records", "2") {
		t.Fatalf("fields: %v", doc.fields())
	}
	// Lines are read as records even from a .json file.
	if doc, err := ReadJSON("data.json", []byte("[1]\n[2]\n")); err != nil || doc.Records != 2 {
		t.Fatalf("%+v %v", doc, err)
	}
}

func TestJSONThatIsNotSaysWhere(t *testing.T) {
	_, err := ReadJSON("bad.json", []byte("{\"a\": 1,\n\"b\": }\n"))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReadJSON("bad.jsonl", []byte("{\"a\":1}\nnope\n")); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestLongJSONIsCut(t *testing.T) {
	var lines []string
	for i := 0; i < 200000; i++ {
		lines = append(lines, `{"i":`+strings.Repeat("9", 20)+`}`)
	}
	doc, err := ReadJSON("big.jsonl", []byte(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Markdown) > MaxMarkdownBytes || doc.Records != 200000 || doc.Shown >= 200000 || !strings.Contains(string(doc.Markdown), "records shown") {
		t.Fatalf("len=%d records=%d shown=%d", len(doc.Markdown), doc.Records, doc.Shown)
	}
}

func hasField(fields []Field, label, value string) bool { return field(fields, label) == value }

// A few kilobytes of nested brackets must not be given gigabytes of indent.
func TestNestedJSONIsBounded(t *testing.T) {
	deep := strings.Repeat("[", 9000) + strings.Repeat("]", 9000)
	bomb := []byte("[" + strings.TrimSuffix(strings.Repeat(deep+",", 6), ",") + "]") // ~108 KB
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	doc, err := ReadJSON("bomb.json", bomb)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 64<<20 {
		t.Fatalf("a %d-byte file allocated %d MiB", len(bomb), grew>>20)
	}
	if len(doc.Markdown) > MaxMarkdownBytes || !doc.cut {
		t.Fatalf("markdown %d bytes, cut=%v", len(doc.Markdown), doc.cut)
	}
	// The same, one value to a line.
	lines := bytes.Repeat(append([]byte(deep), '\n'), 20)
	rec, err := ReadJSON("bomb.jsonl", lines)
	if err != nil || len(rec.Markdown) > MaxMarkdownBytes {
		t.Fatalf("records: %v, %d bytes", err, len(rec.Markdown))
	}
}
