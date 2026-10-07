package app

import (
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"github.com/charmbracelet/x/ansi"
)

type markdownLine struct {
	spans []textSpan
	text  string
	image int // -1 denotes text
	row   int
}

type markdownView struct {
	content, sourceText, renderedText *markdownContent
	highlights                        map[int][][2]int
	layoutVersion                     uint64
	doc                               *document.Markdown
	pictures                          []picture.Model
	lines                             []markdownLine
	width, height, offset             int
	raw                               bool
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
	m.layoutVersion++
	m.highlights = nil
	if m.raw {
		if m.sourceText == nil {
			m.sourceText = sourceContent(m.doc.Source)
		}
		m.content = m.sourceText
	} else {
		if m.renderedText == nil {
			m.renderedText = renderedContent(m.doc)
		}
		m.content = m.renderedText
	}
	var cmds []tea.Cmd
	for _, block := range m.content.blocks {
		switch {
		case block.table != nil:
			prefix := block.prefix
			if ansi.StringWidth(prefix) >= m.width {
				prefix = ""
			}
			for _, line := range layoutTextTable(block.table, m.content.units, m.width-ansi.StringWidth(prefix)) {
				line.text = prefix + line.text
				for i := range line.spans {
					line.spans[i].offset += len(prefix)
				}
				m.lines = append(m.lines, line)
			}
		case block.image >= 0:
			i := block.image
			asset := m.doc.Images[i]
			if asset.Image == nil {
				continue
			}
			b := asset.Image.Bounds()
			rows := int(math.Ceil(float64(b.Dy()) * float64(m.width*max(1, cw)) / float64(b.Dx()*max(1, ch))))
			rows = max(1, min(rows, max(3, min(20, m.height-2))))
			cmds = append(cmds, m.pictures[i].SetCellPixelSize(cw, ch), m.pictures[i].SetSize(m.width, rows), m.pictures[i].SetImage(asset.Image))
			for row := 0; row < rows; row++ {
				m.lines = append(m.lines, markdownLine{image: i, row: row})
			}
		default:
			m.lines = append(m.lines, wrapTextUnit(m.content.units[block.unit], block.unit, m.width, m.raw)...)
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
			lines = append(lines, highlightRanges(line.text, m.highlights[i]))
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
