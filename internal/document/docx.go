package document

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Word is a Word document, turned into Markdown for the viewer. Headings,
// lists, tables, links, pictures, and bold and italic text are kept; page
// layout, headers and footers, footnotes, and comments are not.
type Word struct {
	pkg                             *opc
	Markdown                        []byte
	Headings, Tables, Images, Links int
	styles                          map[string]wordStyle
	lists                           map[string]map[int]bool // Bullet lists, by number id and level.
	rels                            map[string]string
	counters                        map[string]map[int]int
}

type wordStyle struct {
	name    string // As Word names it, in lower case: "heading 1".
	outline int    // Outline level, counting from zero; -1 for none.
}

// A run of text and how it is set.
type wordRun struct {
	text                 string
	bold, italic, struck bool
	link, raw            string // A link's target; Markdown to be written as it is.
	hardBreak            bool
}

func OpenWord(data []byte) (*Word, error) {
	pkg, err := openOPC(data)
	if err != nil {
		return nil, fmt.Errorf("Word: %w", err)
	}
	if !pkg.has("word/document.xml") {
		return nil, fmt.Errorf("Word: no document")
	}
	w := &Word{pkg: pkg, styles: map[string]wordStyle{}, lists: map[string]map[int]bool{}, counters: map[string]map[int]int{}}
	w.rels = pkg.relationships("word/document.xml")
	if data, err := pkg.read("word/styles.xml"); err == nil {
		w.readStyles(data)
	}
	if data, err := pkg.read("word/numbering.xml"); err == nil {
		w.readNumbering(data)
	}
	data, err = pkg.read("word/document.xml")
	if err != nil {
		return nil, fmt.Errorf("Word: %w", err)
	}
	if err := w.convert(data); err != nil {
		return nil, fmt.Errorf("Word: %w", err)
	}
	return w, nil
}

func (w *Word) readStyles(data []byte) {
	var styles struct {
		Style []struct {
			ID   string `xml:"styleId,attr"`
			Name struct {
				Val string `xml:"val,attr"`
			} `xml:"name"`
			PPr struct {
				Outline *struct {
					Val int `xml:"val,attr"`
				} `xml:"outlineLvl"`
			} `xml:"pPr"`
		} `xml:"style"`
	}
	if xml.Unmarshal(data, &styles) != nil {
		return
	}
	for _, s := range styles.Style {
		style := wordStyle{name: strings.ToLower(s.Name.Val), outline: -1}
		if s.PPr.Outline != nil {
			style.outline = s.PPr.Outline.Val
		}
		w.styles[s.ID] = style
	}
}

func (w *Word) readNumbering(data []byte) {
	var numbering struct {
		Abstract []struct {
			ID  string `xml:"abstractNumId,attr"`
			Lvl []struct {
				Ilvl int `xml:"ilvl,attr"`
				Fmt  struct {
					Val string `xml:"val,attr"`
				} `xml:"numFmt"`
			} `xml:"lvl"`
		} `xml:"abstractNum"`
		Num []struct {
			ID       string `xml:"numId,attr"`
			Abstract struct {
				Val string `xml:"val,attr"`
			} `xml:"abstractNumId"`
		} `xml:"num"`
	}
	if xml.Unmarshal(data, &numbering) != nil {
		return
	}
	bullets := map[string]map[int]bool{}
	for _, a := range numbering.Abstract {
		bullets[a.ID] = map[int]bool{}
		for _, l := range a.Lvl {
			bullets[a.ID][l.Ilvl] = l.Fmt.Val == "bullet" || l.Fmt.Val == "none"
		}
	}
	for _, n := range numbering.Num {
		w.lists[n.ID] = bullets[n.Abstract.Val]
	}
}

// heading is the level of a paragraph style, or zero.
func (w *Word) heading(styleID string, outline int) int {
	style, known := w.styles[styleID]
	name := strings.ToLower(styleID)
	if known {
		name = style.name
	}
	switch {
	case name == "title":
		return 1
	case name == "subtitle":
		return 2
	case strings.HasPrefix(name, "heading"):
		if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(name, "heading"))); err == nil && n >= 1 && n <= 6 {
			return n
		}
	}
	if outline < 0 && known {
		outline = style.outline
	}
	if outline >= 0 && outline < 6 {
		return outline + 1
	}
	return 0
}

var listStart = regexp.MustCompile(`^([-+>]|\d+\.)\s`)

// escape keeps text as text: what Markdown would read as marks is escaped.
func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "`", "\\`", "#", `\#`, "[", `\[`, "]", `\]`).Replace(s)
}

// paragraph writes runs as one Markdown paragraph, its marks around words
// rather than around the spaces beside them.
func paragraph(runs []wordRun) string {
	var out strings.Builder
	for i := 0; i < len(runs); i++ {
		r := runs[i]
		if r.raw != "" {
			out.WriteString(r.raw)
			continue
		}
		if r.hardBreak {
			out.WriteString("  \n")
			continue
		}
		// Runs set alike are written together.
		text := r.text
		for i+1 < len(runs) && runs[i+1].raw == "" && !runs[i+1].hardBreak && runs[i+1].bold == r.bold && runs[i+1].italic == r.italic && runs[i+1].struck == r.struck && runs[i+1].link == r.link {
			i++
			text += runs[i].text
		}
		lead := text[:len(text)-len(strings.TrimLeft(text, " \t"))]
		trail := text[len(strings.TrimRight(text, " \t")):]
		text = strings.TrimSpace(text)
		if text == "" {
			out.WriteString(lead)
			continue
		}
		marks := ""
		if r.bold {
			marks += "**"
		}
		if r.italic {
			marks += "*"
		}
		if r.struck {
			marks += "~~"
		}
		word := marks + escape(text) + reverse(marks)
		if r.link != "" {
			word = "[" + word + "](" + r.link + ")"
		}
		out.WriteString(lead + word + trail)
	}
	return out.String()
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// convert reads the document as a stream: a long document is many small
// elements.
func (w *Word) convert(data []byte) error {
	d := xml.NewDecoder(strings.NewReader(string(data)))
	var out strings.Builder
	// Blocks are set apart by a blank line, except the items of one list.
	blocks, lastList := 0, ""
	add := func(block, list string) {
		if block == "" {
			return
		}
		if blocks > 0 {
			if list != "" && list == lastList {
				out.WriteString("\n")
			} else {
				out.WriteString("\n\n")
			}
		}
		out.WriteString(block)
		blocks++
		lastList = list
	}
	var runs []wordRun
	var run wordRun
	var text *strings.Builder
	style, outline, numID, level := "", -1, "", 0
	inParagraph, inTable := false, 0
	var table [][]string
	var row []string
	var cell []string
	link, alt := "", ""
	depth := 0
	for {
		token, err := d.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}
		switch e := token.(type) {
		case xml.StartElement:
			depth++
			switch e.Name.Local {
			case "p":
				inParagraph, runs, style, outline, numID, level = true, nil, "", -1, "", 0
			case "pStyle":
				style = attribute(e, "val")
			case "outlineLvl":
				if n, err := strconv.Atoi(attribute(e, "val")); err == nil {
					outline = n
				}
			case "numId":
				numID = attribute(e, "val")
			case "ilvl":
				if n, err := strconv.Atoi(attribute(e, "val")); err == nil {
					level = n
				}
			case "r":
				run = wordRun{link: link}
			case "b", "i", "strike":
				if val := attribute(e, "val"); val == "0" || val == "false" {
					continue
				}
				switch e.Name.Local {
				case "b":
					run.bold = true
				case "i":
					run.italic = true
				case "strike":
					run.struck = true
				}
			case "t":
				text = &strings.Builder{}
			case "tab":
				runs = append(runs, wordRun{text: "\t", link: link})
			case "br":
				runs = append(runs, wordRun{hardBreak: true})
			case "hyperlink":
				if target := w.rels[attribute(e, "id")]; strings.Contains(target, "://") {
					link = target
					w.Links++
				}
			case "docPr":
				alt = attribute(e, "descr")
				if alt == "" {
					alt = attribute(e, "name")
				}
			case "blip":
				if target := w.rels[attribute(e, "embed")]; target != "" {
					runs = append(runs, wordRun{raw: "![" + escape(alt) + "](" + strings.TrimPrefix(target, "word/") + ")"})
					w.Images++
				}
			case "tbl":
				inTable++
				table, row, cell = nil, nil, nil
			case "tr":
				row = nil
			case "tc":
				cell = nil
			}
		case xml.CharData:
			if text != nil {
				text.Write(e)
			}
		case xml.EndElement:
			depth--
			switch e.Name.Local {
			case "t":
				if text != nil {
					r := run
					r.text = text.String()
					runs = append(runs, r)
					text = nil
				}
			case "hyperlink":
				link = ""
			case "p":
				inParagraph = false
				body := strings.TrimSpace(paragraph(runs))
				if inTable > 0 {
					if body != "" {
						cell = append(cell, body)
					}
					continue
				}
				if body == "" {
					continue
				}
				list := ""
				switch h := w.heading(style, outline); {
				case h > 0:
					body = strings.Repeat("#", h) + " " + body
					w.Headings++
				case numID != "" && numID != "0":
					list = numID
					marker := "- "
					if !w.lists[numID][level] {
						if w.counters[numID] == nil {
							w.counters[numID] = map[int]int{}
						}
						w.counters[numID][level]++
						for deeper := range w.counters[numID] {
							if deeper > level {
								delete(w.counters[numID], deeper)
							}
						}
						marker = strconv.Itoa(w.counters[numID][level]) + ". "
					}
					body = strings.Repeat("  ", level) + marker + body
				case strings.Contains(strings.ToLower(w.styles[style].name+style), "quote"):
					body = "> " + body
				default:
					if listStart.MatchString(body) {
						body = `\` + body
					}
				}
				add(body, list)
			case "tc":
				row = append(row, strings.ReplaceAll(strings.Join(cell, " "), "|", `\|`))
			case "tr":
				table = append(table, row)
			case "tbl":
				if inTable--; inTable > 0 || len(table) == 0 {
					continue
				}
				width := 0
				for _, r := range table {
					width = max(width, len(r))
				}
				var lines []string
				for i, r := range table {
					for len(r) < width {
						r = append(r, "")
					}
					lines = append(lines, "| "+strings.Join(r, " | ")+" |")
					if i == 0 {
						lines = append(lines, "|"+strings.Repeat(" --- |", width))
					}
				}
				add(strings.Join(lines, "\n"), "")
				w.Tables++
			}
		}
		if out.Len() > MaxMarkdownBytes-256 {
			out.WriteString("\n\nThe document is cut here.")
			break
		}
	}
	_ = inParagraph
	out.WriteString("\n")
	w.Markdown = []byte(out.String())
	return nil
}

func (w *Word) fields() []Field {
	count := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	office := w.pkg.officeFields()
	for i := range office {
		if office[i].Label == "Words" || office[i].Label == "Pages" {
			if n, err := strconv.Atoi(strings.TrimSpace(office[i].Value)); err == nil {
				office[i].Value = grouped(n)
			}
		}
	}
	return Section("Document", append(append([]Field{{"Format", "Word document"}}, office...),
		Field{"Headings", count(w.Headings)}, Field{"Tables", count(w.Tables)}, Field{"Images", count(w.Images)}, Field{"Links", count(w.Links)})...)
}
