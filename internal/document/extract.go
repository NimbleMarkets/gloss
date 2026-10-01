package document

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// MaxPageTextBytes bounds the text taken from one PDF page.
const MaxPageTextBytes = 16 << 20

// pdfPageText reads the text layer of a page. A page with none, as a scan
// or a figure has, is an error rather than an empty text, so that a caller
// does not take silence for a blank page.
func pdfPageText(reader *pdf.Reader, page int) (string, error) {
	text, err := pageText(reader, page)
	if err != nil {
		return "", fmt.Errorf("page %d: text layer unreadable: %w", page, err)
	}
	if len(text) > MaxPageTextBytes {
		return "", fmt.Errorf("page %d: text exceeds 16 MiB", page)
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("page %d has no text layer (a scan or a figure?): export it as a picture with --output", page)
	}
	return strings.ToValidUTF8(strings.TrimSpace(text), "\ufffd") + "\n", nil
}

// Text is what a document says, for a reader that wants words rather than
// a picture: the Markdown made of a Word document, a page, or a notebook;
// Markdown as it is; text and JSON as they are; a sheet as CSV. It gives
// the text, the extension that suits it, and, for a document that is only
// a picture, an error that says what to do instead.
func Text(r Result) (text []byte, ext string, err error) {
	switch {
	case r.Err != nil:
		return nil, "", r.Err
	case r.Text != "":
		if r.Kind == "json" {
			return []byte(r.Text), ".json", nil
		}
		return []byte(r.Text), ".txt", nil
	case r.Sheet != nil:
		return r.Sheet.CSV(), ".csv", nil
	case r.Markdown != nil:
		return r.Markdown.Source, ".md", nil
	}
	return nil, "", fmt.Errorf("a %s has no text: export it as a picture with --output", r.Kind)
}

// CSV writes the sheet's rows as comma-separated values.
func (s *Sheet) CSV() []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	for _, row := range s.Rows {
		cells := make([]string, s.Columns)
		copy(cells, row)
		w.Write(cells)
	}
	w.Flush()
	return b.Bytes()
}
