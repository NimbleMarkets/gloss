package app

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	gam "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type markdownLine struct {
	text  string
	image int // -1 denotes text
	row   int
}

type markdownView struct {
	doc                   *document.Markdown
	pictures              []picture.Model
	lines                 []markdownLine
	width, height, offset int
	raw                   bool
}

func newMarkdownView(doc *document.Markdown, id int) *markdownView {
	m := &markdownView{doc: doc}
	for i := range doc.Images {
		m.pictures = append(m.pictures, picture.NewWithConfig(picture.Config{KittyID: id + i, KittyZ: -1}))
	}
	return m
}

func (m *markdownView) close() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.pictures {
		cmds = append(cmds, m.pictures[i].SetImage(nil))
	}
	return tea.Batch(cmds...)
}

func (m *markdownView) update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.pictures {
		cmds = append(cmds, m.pictures[i].Update(msg))
	}
	return tea.Batch(cmds...)
}

func (m *markdownView) setKitty(kitty bool) tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.pictures {
		if (m.pictures[i].Mode() == picture.PictureKitty) != kitty {
			cmds = append(cmds, m.pictures[i].Toggle())
		}
	}
	return tea.Batch(cmds...)
}

func (m *markdownView) layout(width, height, cw, ch int) tea.Cmd {
	m.width, m.height = max(1, width), max(1, height)
	m.lines = nil
	addText := func(s string) {
		m.lines = append(m.lines, markdownLine{text: ansi.Truncate(s, m.width, ""), image: -1})
	}
	if m.raw {
		for _, line := range strings.Split(ansi.Hardwrap(strings.ReplaceAll(string(m.doc.Source), "\t", "    "), m.width, true), "\n") {
			addText(line)
		}
		m.scroll(0)
		return nil
	}
	source := bytes.Clone(m.doc.Source)
	root := document.MarkdownTree(source)
	var images []*ast.Image
	_ = ast.Walk(root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if img, ok := n.(*ast.Image); ok && enter {
			images = append(images, img)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	markers := map[rune]int{}
	marker := rune(0xe000)
	for i, img := range images {
		value := "[image limit reached]"
		if i < len(m.doc.Images) {
			for strings.ContainsRune(string(m.doc.Source), marker) {
				marker++
			}
			value = string(marker)
			markers[marker] = i
			marker++
		}
		start := len(source)
		source = append(source, []byte(value)...)
		// Text segments can refer to appended source bytes without reparsing the
		// document. Glamour keeps links, lists, tables, and references intact.
		img.Parent().ReplaceChild(img.Parent(), img, ast.NewTextSegment(text.NewSegment(start, len(source))))
	}
	style := styles.DarkStyleConfig
	zero := uint(0)
	style.Document.Margin = &zero
	wrap := true
	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(gam.NewRenderer(gam.Options{WordWrap: m.width, Styles: style, TableWrap: &wrap, InlineTableLinks: true}), 100)))
	var out bytes.Buffer
	if err := r.Render(&out, source, root); err != nil {
		addText("Markdown render error: " + safe(err.Error()))
		m.scroll(0)
		return nil
	}
	var cmds []tea.Cmd
	imageRows := make([]int, len(m.pictures))
	for i, asset := range m.doc.Images {
		if asset.Image == nil {
			continue
		}
		b := asset.Image.Bounds()
		rows := int(math.Ceil(float64(b.Dy()) * float64(m.width*max(1, cw)) / float64(b.Dx()*max(1, ch))))
		rows = max(1, min(rows, max(3, min(20, m.height-2))))
		imageRows[i] = rows
		cmds = append(cmds, m.pictures[i].SetCellPixelSize(cw, ch), m.pictures[i].SetSize(m.width, rows), m.pictures[i].SetImage(asset.Image))
	}
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		var inline []int
		clean := strings.Map(func(r rune) rune {
			if i, ok := markers[r]; ok {
				inline = append(inline, i)
				return '▧'
			}
			return r
		}, line)
		addText(clean)
		for _, i := range inline {
			asset := m.doc.Images[i]
			label := asset.Alt
			if label == "" {
				label = asset.Destination
			}
			if asset.Err != nil {
				addText(fmt.Sprintf("[image: %s — %s]", safe(label), safe(asset.Err.Error())))
				continue
			}
			addText("[image: " + safe(label) + "]")
			for row := 0; row < imageRows[i]; row++ {
				m.lines = append(m.lines, markdownLine{image: i, row: row})
			}
		}
	}
	m.scroll(0)
	return tea.Batch(cmds...)
}

func (m *markdownView) scroll(delta int) {
	m.offset = max(0, min(max(0, len(m.lines)-m.height), m.offset+delta))
}

func (m *markdownView) key(k string) bool {
	switch k {
	case "j", "down":
		m.scroll(1)
	case "k", "up":
		m.scroll(-1)
	case "space", "pgdown":
		m.scroll(max(1, m.height-1))
	case "b", "pgup":
		m.scroll(-max(1, m.height-1))
	case "ctrl+d":
		m.scroll(max(1, m.height/2))
	case "ctrl+u":
		m.scroll(-max(1, m.height/2))
	case "home", "0":
		m.offset = 0
	case "end", "G":
		m.scroll(len(m.lines))
	default:
		return false
	}
	return true
}

func (m *markdownView) view() string {
	cache := map[int][]string{}
	var lines []string
	for i := m.offset; i < min(len(m.lines), m.offset+m.height); i++ {
		line := m.lines[i]
		if line.image < 0 {
			lines = append(lines, line.text)
			continue
		}
		rows, ok := cache[line.image]
		if !ok {
			rows = strings.Split(m.pictures[line.image].View().Content, "\n")
			cache[line.image] = rows
		}
		if line.row < len(rows) {
			lines = append(lines, rows[line.row])
		} else {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}
