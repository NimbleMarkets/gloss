package document

import (
	"strings"
	"testing"
)

func TestHTMLBecomesMarkdown(t *testing.T) {
	page, err := ReadHTML([]byte(`<!doctype html><html><head><title>A Page</title><style>b{}</style><script>alert(1)</script></head>
<body><h1>Hi</h1><p>Some <b>bold</b> and <a href="x.html">link</a>.</p><img src="pic.png" alt="a pic">
<table><tr><th>a</th><th>b</th></tr><tr><td>1</td><td>2</td></tr></table><pre><code class="language-go">x := 1</code></pre>
<p>Text with <span style="color:red">an escape \x1b[31m</span></p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	md := string(page.Markdown)
	for _, want := range []string{"# Hi\n", "**bold**", "[link](x.html)", "![a pic](pic.png)", "| a | b |", "| 1 | 2 |", "```go\nx := 1\n```"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "alert") || strings.Contains(md, "b{}") {
		t.Errorf("script or style kept:\n%s", md)
	}
	md2, _ := loadMarkdownFrom("page.html", page.Markdown, t.TempDir(), nil)
	fields := page.fields(md2)
	if field(fields, "Title") != "A Page" || field(fields, "Format") != "HTML" || field(fields, "Headings") != "1" || field(fields, "Links") != "1" {
		t.Errorf("fields: %v", fields)
	}
}

func TestHTMLLimits(t *testing.T) {
	if _, err := ReadHTML([]byte(strings.Repeat("<p>x</p>", MaxHTMLBytes/8+1))); err == nil || !strings.Contains(err.Error(), "MiB") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReadHTML([]byte("\xff\xfe not text")); err == nil {
		t.Fatal("accepted what is not UTF-8")
	}
}

func TestHTMLIsDetected(t *testing.T) {
	for _, tt := range []struct{ path, data, kind string }{
		{"page.html", "<p>x</p>", "html"},
		{"page.htm", "", "html"},
		{"page", "<!DOCTYPE html><html>", "html"},
		{"page", "  <html lang=en>", "html"},
		{"data.json", `{"a":1}`, "json"},
		{"data.jsonl", "", "json"},
		{"data.ndjson", "", "json"},
		{"data", `{"a": [1, 2]}`, "json"},
		{"nb.ipynb", "", "ipynb"},
		{"nb", `{"cells": [], "nbformat": 4}`, "ipynb"},
		{"data", "{not json", ""},
	} {
		kind, err := Detect(tt.path, []byte(tt.data), "")
		if kind != tt.kind || (err == nil) != (tt.kind != "") {
			t.Errorf("%s %q: %q %v", tt.path, tt.data, kind, err)
		}
	}
}
