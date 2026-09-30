package document

import (
	"strings"
	"testing"
)

func TestPlainTextIsShownAsItIs(t *testing.T) {
	doc, err := ReadText([]byte("\ufeffline one\r\n\tindented *not emphasis*\n# not a heading\n``` not a fence\n\xff\n"))
	if err != nil {
		t.Fatal(err)
	}
	md := string(doc.Markdown)
	if !strings.HasPrefix(md, "````text\n") || !strings.HasSuffix(md, "\n````\n") {
		t.Fatalf("not fenced past its own backticks:\n%s", md)
	}
	for _, want := range []string{"line one\n\tindented *not emphasis*\n# not a heading\n``` not a fence\n\ufffd\n"} {
		if !strings.Contains(md, want) {
			t.Fatalf("text changed:\n%s", md)
		}
	}
	if strings.Contains(md, "\r") || strings.Contains(md, "\ufeff") {
		t.Fatalf("returns or the byte order mark kept:\n%q", md)
	}
	fields := doc.fields()
	for label, want := range map[string]string{"Format": "Plain text", "Lines": "5", "Words": "14", "Characters": "67"} {
		if got := field(fields, label); got != want {
			t.Errorf("%s = %q, want %q", label, got, want)
		}
	}
}

func TestLongTextIsCut(t *testing.T) {
	doc, err := ReadText([]byte(strings.Repeat("a line of text\n", 200000)))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Markdown) > MaxMarkdownBytes || !strings.Contains(string(doc.Markdown), "lines shown") || field(doc.fields(), "Lines") != "200,000" {
		t.Fatalf("len=%d fields=%v", len(doc.Markdown), doc.fields())
	}
}

func TestTextIsDetectedByName(t *testing.T) {
	for _, tt := range []struct{ path, data, kind string }{
		{"notes.txt", "hello", "text"},
		{"build.log", "", "text"},
		{"a.text", "", "text"},
		{"notes", "hello", "text"},
		{"notes", "\x00", ""},
	} {
		kind, err := Detect(tt.path, []byte(tt.data), "")
		if kind != tt.kind || (err == nil) != (tt.kind != "") {
			t.Errorf("%s: %q %v", tt.path, kind, err)
		}
	}
}

func TestTextIsToldFromBinary(t *testing.T) {
	for _, tt := range []struct {
		name string
		data string
		text bool
	}{
		{"plain", "hello, world\n", true},
		{"source", "package main\n\nfunc main() {}\n", true},
		{"tabs and returns", "a\tb\r\nc\n", true},
		{"unicode", "héllo — “quoted” 日本語\n", true},
		{"escape codes", "\x1b[31mred\x1b[0m\n", true},
		{"empty", "", false},
		{"nul", "hello\x00world", false},
		{"png", "\x89PNG\r\n\x1a\n\x00\x00", false},
		{"latin1", "caf\xe9 au lait", false},
		{"controls", "\x01\x02\x03\x04\x05\x06\x07 text", false},
	} {
		if got := IsText([]byte(tt.data)); got != tt.text {
			t.Errorf("%s: IsText = %v", tt.name, got)
		}
	}
	// A rune cut by the sniffing limit does not make a file binary.
	long := strings.Repeat("é", 5000)
	if !IsText([]byte(long)[:8193]) {
		t.Error("a rune cut at the limit read as binary")
	}
	// Detection falls back to text for what is not binary, by any name.
	for _, tt := range []struct{ path, data, kind string }{
		{"notes", "hello", "text"},
		{"main.go", "package main", "text"},
		{"Makefile", "all:\n\tgo build\n", "text"},
		{"file.rtf", "{\\rtf1 hi}", "text"},
		{"blob", "\x00\x01\x02", ""},
		{"file.bin", "hello\x00", ""},
	} {
		kind, err := Detect(tt.path, []byte(tt.data), "")
		if kind != tt.kind || (err == nil) != (tt.kind != "") {
			t.Errorf("%s: %q %v", tt.path, kind, err)
		}
	}
}
