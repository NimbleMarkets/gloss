package app

import (
	"image"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownLayoutAndScroll(t *testing.T) {
	doc := &document.Markdown{Source: []byte("# Heading\n\nSome **bold** text.\n\n![picture](local.png)\n\n" + strings.Repeat("another paragraph\n\n", 30)), Images: []document.MarkdownImage{{Alt: "picture", Image: image.NewRGBA(image.Rect(0, 0, 40, 20))}}}
	m := newMarkdownView(doc, 500)
	m.layout(60, 12, 8, 16)
	var all strings.Builder
	rows := 0
	for _, line := range m.lines {
		all.WriteString(line.text)
		if line.image >= 0 {
			rows++
		}
	}
	plain := ansi.Strip(all.String())
	if !strings.Contains(plain, "Heading") || !strings.Contains(plain, "[image: picture]") || rows == 0 {
		t.Fatalf("missing rendered content: %s; image rows %d", plain, rows)
	}
	if !m.key("end") || m.offset != len(m.lines)-m.height {
		t.Fatal("end did not scroll to bottom")
	}
	m.key("home")
	if m.offset != 0 {
		t.Fatal("home did not reset")
	}
	m.raw = true
	m.layout(60, 12, 8, 16)
	if !strings.Contains(m.view(), "![picture](local.png)") {
		t.Fatal("source mode lost Markdown syntax")
	}
	for _, line := range m.lines {
		if line.image >= 0 {
			t.Fatal("source mode contains rendered images")
		}
	}
}
