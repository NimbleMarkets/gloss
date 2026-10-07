package app

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	gam "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Logical text is immutable and independent of the viewport. Screen spans map
// its UTF-8 byte offsets into a row; generated indentation/padding has no span.
type textUnit struct{ styled, plain string }
type textSpan struct{ unit, start, end, offset int }
type textBlock struct {
	unit, image int
	table       *textTable
	prefix      string
}
type textTable struct {
	rows       [][]int
	alignments []east.Alignment
}
type markdownContent struct {
	units  []textUnit
	blocks []textBlock
}

func (c *markdownContent) unit(s string) int {
	i := len(c.units)
	c.units = append(c.units, textUnit{styled: s, plain: ansi.Strip(s)})
	return i
}

func (c *markdownContent) line(s string) {
	c.blocks = append(c.blocks, textBlock{unit: c.unit(s), image: -1})
}

func sourceContent(source []byte) *markdownContent {
	c := &markdownContent{}
	for _, line := range strings.Split(strings.ReplaceAll(string(source), "\t", "    "), "\n") {
		c.line(line)
	}
	return c
}

// Width zero disables Glamour's wrapping and padding. Tables are extracted
// before rendering: naturally sized tables can otherwise allocate rows times
// the widest cell. Their cells are rendered independently and laid out within
// the viewport below, without making table borders part of searchable text.
func logicalRenderer() func([]byte, ast.Node) string {
	style := styles.DarkStyleConfig
	zero := uint(0)
	style.Document.Margin = &zero
	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(gam.NewRenderer(gam.Options{WordWrap: 0, Styles: style, InlineTableLinks: true}), 100)))
	return func(source []byte, root ast.Node) string {
		var out bytes.Buffer
		if err := r.Render(&out, source, root); err != nil {
			return "Markdown render error: " + safe(err.Error())
		}
		return strings.TrimSuffix(out.String(), "\n")
	}
}

func renderedContent(doc *document.Markdown) *markdownContent {
	c := &markdownContent{}
	renderLogical := logicalRenderer()
	source := bytes.Clone(doc.Source)
	root := document.MarkdownTree(source)
	var images []*ast.Image
	var tables []*east.Table
	_ = ast.Walk(root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Image:
			images = append(images, n)
			return ast.WalkSkipChildren, nil
		case *east.Table:
			tables = append(tables, n)
		}
		return ast.WalkContinue, nil
	})
	imageMarkers := map[rune]int{}
	tableMarkers := map[rune]*east.Table{}
	marker := rune(0xe000)
	usedMarkers := map[rune]bool{}
	for _, r := range string(doc.Source) {
		if r >= marker {
			usedMarkers[r] = true
		}
	}
	nextMarker := func() rune {
		for usedMarkers[marker] {
			marker++
		}
		r := marker
		marker++
		return r
	}
	segment := func(value string) *ast.Text {
		start := len(source)
		source = append(source, value...)
		return ast.NewTextSegment(text.NewSegment(start, len(source)))
	}
	for i, img := range images {
		value := "[image limit reached]"
		if i < len(doc.Images) {
			r := nextMarker()
			value = string(r)
			imageMarkers[r] = i
		}
		img.Parent().ReplaceChild(img.Parent(), img, segment(value))
	}
	for _, table := range tables {
		r := nextMarker()
		tableMarkers[r] = table
		p := ast.NewParagraph()
		p.AppendChild(p, segment(string(r)))
		table.Parent().ReplaceChild(table.Parent(), table, p)
	}
	for _, line := range strings.Split(renderLogical(source, root), "\n") {
		var table *east.Table
		var inline []int
		clean := strings.Map(func(r rune) rune {
			if t, ok := tableMarkers[r]; ok {
				table = t
				return -1
			}
			if i, ok := imageMarkers[r]; ok {
				inline = append(inline, i)
				return '▧'
			}
			return r
		}, line)
		if table != nil {
			t := &textTable{alignments: table.Alignments}
			for row := table.FirstChild(); row != nil; row = row.NextSibling() {
				var cells []int
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					d, p := ast.NewDocument(), ast.NewParagraph()
					d.AppendChild(d, p)
					for cell.FirstChild() != nil {
						p.AppendChild(p, cell.FirstChild())
					}
					styled := strings.Map(func(r rune) rune {
						if i, ok := imageMarkers[r]; ok {
							inline = append(inline, i)
							return '▧'
						}
						return r
					}, strings.Trim(renderLogical(source, d), "\n"))
					cells = append(cells, c.unit(styled))
				}
				t.rows = append(t.rows, cells)
			}
			c.blocks = append(c.blocks, textBlock{table: t, image: -1, prefix: ansi.Strip(clean)})
		} else {
			c.line(clean)
		}
		for _, i := range inline {
			asset := doc.Images[i]
			label := asset.Alt
			if label == "" {
				label = asset.Destination
			}
			if asset.Err != nil {
				c.line(fmt.Sprintf("[image: %s — %s]", safe(label), safe(asset.Err.Error())))
				continue
			}
			c.line("[image: " + safe(label) + "]")
			c.blocks = append(c.blocks, textBlock{image: i})
		}
	}
	return c
}

var continuationIndent = regexp.MustCompile(`^(?:[ \t]*│ ?)*[ \t]*(?:[•▸] |[0-9]+\. )?`)

func wrapTextUnit(unit textUnit, id, width int, raw bool) []markdownLine {
	var result []markdownLine
	base := 0
	for _, logical := range strings.Split(unit.styled, "\n") {
		plain := ansi.Strip(logical)
		indent := ""
		if !raw {
			indent = continuationIndent.FindString(plain)
		}
		indentWidth := ansi.StringWidth(indent)
		if indentWidth >= width {
			indent, indentWidth = "", 0
		}
		firstPrefix := ansi.Cut(logical, 0, indentWidth)
		continuation := strings.Map(func(r rune) rune {
			if r == '│' || unicode.IsSpace(r) {
				return r
			}
			return ' '
		}, indent)
		body := ansi.Cut(logical, indentWidth, ansi.StringWidth(logical))
		wrapped := lipgloss.Wrap(body, max(1, width-indentWidth), "")
		if raw {
			wrapped = ansi.Hardwrap(logical, max(1, width), true)
		}
		pos := len(indent)
		for i, part := range strings.Split(wrapped, "\n") {
			partPlain := ansi.Strip(part)
			// Word wrapping may discard boundary spaces. Match against the
			// logical suffix; never concatenate unrelated physical lines.
			if skip := strings.Index(plain[pos:], partPlain); skip >= 0 {
				pos += skip
			}
			prefix := continuation
			if i == 0 {
				prefix = firstPrefix
			}
			if raw {
				prefix = ""
			}
			line := markdownLine{text: ansi.Truncate(prefix+part, width, ""), image: -1}
			visible := max(0, len(ansi.Strip(line.text))-len(ansi.Strip(prefix)))
			line.spans = []textSpan{{unit: id, start: base + pos, end: base + pos + visible, offset: len(ansi.Strip(prefix))}}
			result = append(result, line)
			pos += len(partPlain)
		}
		base += len(plain) + 1
	}
	return result
}

// Keep cells as independent logical units, even when several share a screen
// row. A narrow terminal stacks cells instead of discarding right-hand columns.
func layoutTextTable(table *textTable, units []textUnit, width int) []markdownLine {
	cols := 0
	for _, row := range table.rows {
		cols = max(cols, len(row))
	}
	if cols == 0 {
		return nil
	}
	var out []markdownLine
	if width < 3*cols-1 {
		for _, row := range table.rows {
			for _, id := range row {
				out = append(out, wrapTextUnit(units[id], id, width, false)...)
			}
			out = append(out, markdownLine{image: -1})
		}
		return out
	}
	available := width - (cols-1)*3
	wants, widths := make([]int, cols), make([]int, cols)
	for c := range widths {
		widths[c] = 1
	}
	for _, row := range table.rows {
		for c, id := range row {
			for _, line := range strings.Split(units[id].plain, "\n") {
				wants[c] = max(wants[c], min(available, ansi.StringWidth(line)))
			}
		}
	}
	for left := available - cols; left > 0; {
		changed := false
		for c := range widths {
			if left > 0 && widths[c] < wants[c] {
				widths[c]++
				left--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	for r, row := range table.rows {
		cells := make([][]markdownLine, len(row))
		height := 1
		for c, id := range row {
			cells[c] = wrapTextUnit(units[id], id, widths[c], false)
			height = max(height, len(cells[c]))
		}
		for y := 0; y < height; y++ {
			line := markdownLine{image: -1}
			for c := range widths {
				if c > 0 {
					line.text += " │ "
				}
				part := ""
				var spans []textSpan
				if c < len(cells) && y < len(cells[c]) {
					cell := cells[c][y]
					spans = cell.spans
					part = cell.text
				}
				padding := max(0, widths[c]-ansi.StringWidth(part))
				left := 0
				if c < len(table.alignments) {
					switch table.alignments[c] {
					case east.AlignRight:
						left = padding
					case east.AlignCenter:
						left = padding / 2
					}
				}
				for _, span := range spans {
					span.offset += len(ansi.Strip(line.text)) + left
					line.spans = append(line.spans, span)
				}
				line.text += strings.Repeat(" ", left) + part + strings.Repeat(" ", padding-left)
			}
			out = append(out, line)
		}
		if r == 0 {
			var parts []string
			for _, w := range widths {
				parts = append(parts, strings.Repeat("─", w))
			}
			out = append(out, markdownLine{text: strings.Join(parts, "─┼─"), image: -1})
		}
	}
	return out
}
