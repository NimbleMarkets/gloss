package document

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

// MaxHTMLBytes bounds a page: its Markdown must fit the document limit, and
// a page is mostly markup.
const MaxHTMLBytes = 8 << 20

// Page is an HTML document turned into Markdown.
type Page struct {
	Markdown []byte
	title    string
}

// ReadHTML converts a page's content: scripts and styles are dropped, and
// tables and strikethrough are kept.
func ReadHTML(data []byte) (*Page, error) {
	if len(data) > MaxHTMLBytes {
		return nil, fmt.Errorf("HTML exceeds %d MiB", MaxHTMLBytes>>20)
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("HTML must be UTF-8")
	}
	conv := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		table.NewTablePlugin(),
		strikethrough.NewStrikethroughPlugin(),
	))
	md, err := conv.ConvertReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(md) > MaxMarkdownBytes-4096 {
		md = append(md[:MaxMarkdownBytes-4096], "\n\n*The rest of the page was cut.*\n"...)
	}
	return &Page{Markdown: append(bytes.TrimSpace(md), '\n'), title: htmlTitle(data)}, nil
}

// htmlTitle is the page's <title>, the first one found in the head.
func htmlTitle(data []byte) string {
	tokens := html.NewTokenizer(bytes.NewReader(data))
	for i := 0; i < 1000; i++ {
		switch tokens.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken:
			if name, _ := tokens.TagName(); string(name) == "title" {
				if tokens.Next() == html.TextToken {
					return strings.TrimSpace(string(tokens.Text()))
				}
				return ""
			}
			if name, _ := tokens.TagName(); string(name) == "body" {
				return ""
			}
		}
	}
	return ""
}

// fields describes the page, with the counts of the Markdown made of it.
func (p *Page) fields(md *Markdown) []Field {
	fields := []Field{{"Format", "HTML"}, {"Title", p.title}}
	for _, f := range markdownFields(md) {
		if f.Value != "" && f.Label != "Format" && f.Label != "Title" {
			fields = append(fields, f)
		}
	}
	return section("Page", fields...)
}

// textKind tells HTML, notebooks, and JSON apart by their content: a page
// by its opening tag, a notebook by its cells, and JSON by parsing.
func textKind(path string, data []byte) string {
	head := bytes.ToLower(bytes.TrimSpace(data[:min(len(data), 1024)]))
	if bytes.HasPrefix(head, []byte("<!doctype html")) || bytes.HasPrefix(head, []byte("<html")) {
		return "html"
	}
	if len(head) == 0 || (head[0] != '{' && head[0] != '[') {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	if head[0] == '{' && (ext == ".ipynb" || (bytes.Contains(data, []byte(`"nbformat"`)) && bytes.Contains(data, []byte(`"cells"`)))) {
		var nb struct {
			Cells  json.RawMessage `json:"cells"`
			Format int             `json:"nbformat"`
		}
		if json.Unmarshal(data, &nb) == nil && nb.Format > 0 && len(nb.Cells) > 0 {
			return "ipynb"
		}
	}
	if json.Valid(data) {
		return "json"
	}
	return ""
}
