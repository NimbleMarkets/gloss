package document

import (
	"bytes"
	"cmp"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2/lexers"
)

// TextDoc is a plain text file, fenced as Markdown so that it is shown as
// it is: no line of it is read as a heading, a list, or emphasis.
type TextDoc struct {
	Markdown   []byte
	language   string // As chroma names it, for highlighting; empty for prose.
	lines      int
	words      int
	characters int
	shown      int
}

// ReadText takes text as it comes: returns are dropped, a byte order mark
// too, and bytes that are not UTF-8 stand as U+FFFD. Source is fenced in
// its language, told by the file's name or its first line, so that the
// viewer highlights it.
func ReadText(path string, data []byte) (*TextDoc, error) {
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	text := strings.ReplaceAll(strings.ToValidUTF8(string(data), "\ufffd"), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimRight(text, "\n")
	doc := &TextDoc{characters: utf8.RuneCountInString(text), words: len(strings.Fields(text)), language: language(path, text)}
	lines := strings.Split(text, "\n")
	doc.lines = len(lines)
	if text == "" {
		doc.lines = 0
	}
	// Cut on a line, with room for the fence and the note.
	budget := MaxMarkdownBytes - 4096
	if len(text) > budget {
		text = text[:strings.LastIndexByte(text[:budget], '\n')+1]
		doc.shown = strings.Count(text, "\n")
		text = strings.TrimRight(text, "\n")
	}
	var md bytes.Buffer
	fenced(&md, cmp.Or(doc.language, "text"), text)
	if doc.shown > 0 {
		md.WriteString("*First " + grouped(doc.shown) + " of " + grouped(doc.lines) + " lines shown.*\n")
	}
	doc.Markdown = append(bytes.TrimRight(md.Bytes(), "\n"), '\n')
	return doc, nil
}

// language is the name chroma gives the language a file is in, by its
// name, or by its first line when that says. Prose has none.
func language(path, text string) string {
	lexer := lexers.Match(filepath.Base(path))
	if lexer == nil && strings.HasPrefix(text, "#!") {
		lexer = lexers.Analyse(text)
	}
	if lexer == nil {
		return ""
	}
	name := strings.ToLower(lexer.Config().Name)
	if name == "plaintext" || name == "text" {
		return ""
	}
	// The fence takes an alias where the name has spaces or capitals.
	for _, alias := range lexer.Config().Aliases {
		if !strings.ContainsAny(alias, " +#") {
			return alias
		}
	}
	return name
}

// TextSniff is how much of a file says whether it is text.
const TextSniff = 8 << 10

// IsText says whether data reads as text rather than binary: no NUL byte,
// UTF-8 throughout, but for a rune cut short at the end, and few control
// characters beyond tabs, line ends, and escapes. Nothing is not text.
func IsText(data []byte) bool {
	cut := len(data) > TextSniff
	if cut {
		data = data[:TextSniff]
	}
	if len(data) == 0 || bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	// A rune cut by the limit is whole in the file.
	for i := 0; cut && i < utf8.UTFMax && !utf8.Valid(data); i++ {
		data = data[:len(data)-1]
	}
	if !utf8.Valid(data) {
		return false
	}
	controls, runes := 0, 0
	for _, r := range string(data) {
		runes++
		if r < ' ' && r != '\t' && r != '\n' && r != '\r' && r != '\f' && r != 0x1b {
			controls++
		}
	}
	return controls*10 <= runes
}

func (doc *TextDoc) fields() []Field {
	fields := []Field{{"Format", "Plain text"}, {"Language", doc.language}, {"Lines", grouped(doc.lines)}}
	if doc.shown > 0 {
		fields = append(fields, Field{"Shown", grouped(doc.shown) + " lines"})
	}
	return Section("Text", append(fields, Field{"Words", grouped(doc.words)}, Field{"Characters", grouped(doc.characters)})...)
}
