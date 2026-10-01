package document

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
	"golang.org/x/net/html"
)

// A small file can ask for a great deal: a row of a hundred million commas, a
// table of a few thousand cells that is padded to a square, brackets nested a
// million deep, objects that hold fifty of the next. Each limit here keeps
// what a file may cost in step with what it says.
const (
	maxCSVFields      = 1 << 16   // Values on one line of a CSV.
	maxSheetCells     = 2000000   // Cells held for one Excel sheet, empty ones among them.
	maxWordTableCells = 200000    // Cells of one Word table, padded to its width.
	maxHTMLTableCells = 1000000   // Rows times columns of one HTML table, as it is padded.
	maxQuoteDepth     = 64        // Block quotes within block quotes.
	maxBracketDepth   = 1000      // Unclosed [ in a Markdown document.
	max3MFVisits      = 1 << 20   // Objects a 3MF may place, counting every component.
	maxHTMLColspan    = 1 << 20   // A colspan is a width the converter will pad to.
	zipEndSize        = 22        // The fixed part of a zip archive's end record.
	zipCommentMax     = 0xFFFF    // And the longest comment it may carry.
	zipSixtyFour      = 1 << 30   // What a count of 0xFFFF, "see the zip64 record", stands for.
	maxPDFTreeWalk    = 64        // Parents followed up a PDF's page tree.
	maxPDFStream      = 256 << 20 // What one Flate stream of a PDF may inflate to.
	maxPDFInflated    = 1 << 30   // And all of them together.
)

// parseDeadline is how long a library that cannot be stopped is given.
// It is a variable so that tests can shorten it.
var parseDeadline = 15 * time.Second

// withDeadline runs work, and gives up on it after d. It is for the libraries
// that take a hostile file and never return, and offer no way to stop them: a
// PDF whose page tree lists itself, an SVG of nested uses. The work is
// abandoned, not stopped: it goes on in its goroutine, so this is a way to
// answer, not to save the cost. A panic in it is an error, as it would be in
// the loader's own goroutine.
func withDeadline[T any](d time.Duration, what string, work func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("%s: %v", what, r)}
			}
		}()
		value, err := work()
		done <- result{value, err}
	}()
	select {
	case r := <-done:
		return r.value, r.err
	case <-time.After(d):
		var zero T
		return zero, fmt.Errorf("%s took longer than %v: the file is probably crafted, and was given up", what, d)
	}
}

// renderSVGWithin is renderSVG under the deadline: a few hundred bytes of
// <use> within <use> within <use> is a render that never ends.
func renderSVGWithin(name string, data []byte, edge int) (image.Image, error) {
	return withDeadline(parseDeadline, "drawing the SVG", func() (image.Image, error) {
		return renderSVGUnbounded(name, data, edge)
	})
}

// mediaBox is the size of a page in points, and whether it has one: MediaBox
// is inherited from the nearest ancestor in the page tree. The reader does not
// notice a tree that lists itself, and never returns from it.
func mediaBox(reader *pdf.Reader, page int) (w, h float64, ok bool, err error) {
	type size struct {
		w, h float64
		ok   bool
	}
	s, err := withDeadline(parseDeadline, "reading the PDF's page tree", func() (size, error) {
		for node, depth := reader.Page(page).V, 0; !node.IsNull() && depth < maxPDFTreeWalk; node, depth = node.Key("Parent"), depth+1 {
			box := node.Key("MediaBox")
			if box.Len() != 4 {
				continue
			}
			w, h := math.Abs(box.Index(2).Float64()-box.Index(0).Float64()), math.Abs(box.Index(3).Float64()-box.Index(1).Float64())
			return size{w, h, w > 0 && h > 0 && !math.IsNaN(w) && !math.IsNaN(h) && !math.IsInf(w, 0) && !math.IsInf(h, 0)}, nil
		}
		return size{}, nil
	})
	return s.w, s.h, s.ok, err
}

// pageText is the text layer of a page, under the deadline.
func pageText(reader *pdf.Reader, page int) (string, error) {
	return withDeadline(parseDeadline, "reading the PDF's text", func() (string, error) {
		return reader.Page(page).GetPlainText(nil)
	})
}

// openZip is zip.NewReader after a look at how many entries the archive says
// it has: the reader makes a record of every one before anyone can count them,
// and the central directory of a small archive can name a million.
func openZip(r io.ReaderAt, size int64) (*zip.Reader, error) {
	if n := zipEntries(r, size); n > maxPackageEntries {
		return nil, fmt.Errorf("has more than %d entries", maxPackageEntries)
	}
	return zip.NewReader(r, size)
}

// zipEntries is the number of entries the end record of an archive claims: it
// finds the record as the zip reader does, the last signature whose comment
// fits. Where the count is the zip64 marker the archive is bigger than any we
// take, and the number is correspondingly large.
func zipEntries(r io.ReaderAt, size int64) int {
	tail := min(size, zipEndSize+zipCommentMax)
	if tail < zipEndSize {
		return 0
	}
	buf := make([]byte, tail)
	if n, err := r.ReadAt(buf, size-tail); n < len(buf) && err != nil && err != io.EOF {
		return 0
	}
	for i := len(buf) - zipEndSize; i >= 0; i-- {
		if !bytes.Equal(buf[i:i+4], []byte("PK\x05\x06")) {
			continue
		}
		if comment := int(binary.LittleEndian.Uint16(buf[i+20:])); i+zipEndSize+comment > len(buf) {
			continue
		}
		if count := int(binary.LittleEndian.Uint16(buf[i+10:])); count != 0xFFFF {
			return count
		}
		return zipSixtyFour
	}
	return 0
}

// checkMarkdownNesting refuses what the parser takes longer for the deeper it
// goes: a million "> " at the start of a line, or a million "[" in a row, cost
// it minutes. No document nests so far.
func checkMarkdownNesting(source string) error {
	quotes, brackets := 0, 0
	atStart, escaped := true, false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\n' {
			atStart, quotes, escaped = true, 0, false
			continue
		}
		if atStart {
			switch c {
			case '>':
				if quotes++; quotes > maxQuoteDepth {
					return fmt.Errorf("Markdown quotes nest more than %d deep", maxQuoteDepth)
				}
				continue
			case ' ', '\t':
				continue
			}
			atStart = false
		}
		switch {
		case escaped:
			escaped = false
		case c == '\\':
			escaped = true
		case c == '[':
			if brackets++; brackets > maxBracketDepth {
				return fmt.Errorf("Markdown has more than %d unclosed brackets", maxBracketDepth)
			}
		case c == ']' && brackets > 0:
			brackets--
		}
	}
	return nil
}

// checkHTMLTables refuses a page with a table whose padded size is more than
// the converter should be given: it fills every row out to the widest, and a
// colspan to its width, so a row of six thousand cells and six thousand rows
// of one are thirty-six million.
func checkHTMLTables(data []byte) error {
	type table struct{ rows, cols, row int }
	var stack []table
	tooBig := func(t table) error {
		if int64(t.rows)*int64(t.cols) > maxHTMLTableCells {
			return fmt.Errorf("HTML has a table of %d rows by %d columns: more than %d cells", t.rows, t.cols, maxHTMLTableCells)
		}
		return nil
	}
	tokens := html.NewTokenizer(bytes.NewReader(data))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			for _, t := range stack {
				if err := tooBig(t); err != nil {
					return err
				}
			}
			return nil
		case html.StartTagToken, html.SelfClosingTagToken:
			tag, hasAttr := tokens.TagName()
			name := string(tag)
			n := len(stack)
			switch {
			case name == "table":
				stack = append(stack, table{})
			case name == "tr" && n > 0:
				stack[n-1].rows++
				stack[n-1].row = 0
			case (name == "td" || name == "th") && n > 0:
				span := 1
				for hasAttr {
					var key, value []byte
					key, value, hasAttr = tokens.TagAttr()
					if string(key) == "colspan" {
						if s, err := strconv.Atoi(strings.TrimSpace(string(value))); err == nil && s > 1 {
							span = min(s, maxHTMLColspan)
						}
					}
				}
				stack[n-1].row += span
				stack[n-1].cols = max(stack[n-1].cols, stack[n-1].row)
			}
		case html.EndTagToken:
			if tag, _ := tokens.TagName(); string(tag) == "table" && len(stack) > 0 {
				t := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if err := tooBig(t); err != nil {
					return err
				}
			}
		}
	}
}

// wideLine reports whether a line of text has more than max of the separator.
func tooWide(text string, separator rune) bool {
	sep := string(separator)
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Count(line, sep) > maxCSVFields {
			return true
		}
	}
	return false
}

// checkPDFStreams refuses a PDF whose compressed streams inflate to more than
// they should: a megabyte of Flate can say a gigabyte, and both the Go reader
// and PDFium make the whole of it before anyone can say no. It inflates each
// stream in turn into nothing, stopping at the limits, so what it costs is
// time, not memory. A stream that is not Flate is passed over.
func checkPDFStreams(data []byte) error {
	var total int64
	rest := data
	for {
		i := bytes.Index(rest, []byte("stream"))
		if i < 0 {
			return nil
		}
		start := i + len("stream")
		if bytes.HasSuffix(rest[:i], []byte("end")) {
			rest = rest[start:] // The end of one, not the start of the next.
			continue
		}
		switch {
		case bytes.HasPrefix(rest[start:], []byte("\r\n")):
			start += 2
		case bytes.HasPrefix(rest[start:], []byte("\n")):
			start++
		default:
			rest = rest[start:]
			continue
		}
		end := bytes.Index(rest[start:], []byte("endstream"))
		if end < 0 {
			return nil // Cut short: the readers will say so.
		}
		n := inflatedSize(rest[start:start+end], maxPDFStream+1)
		if total += n; n > maxPDFStream || total > maxPDFInflated {
			return fmt.Errorf("PDF has streams that inflate to more than %d MiB: a compression bomb, or too large to open", maxPDFInflated>>20)
		}
		rest = rest[start+end+len("endstream"):]
	}
}

// inflatedSize is how many bytes a Flate stream makes, counted up to limit.
func inflatedSize(raw []byte, limit int64) int64 {
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return 0
	}
	defer zr.Close()
	n, _ := io.Copy(io.Discard, io.LimitReader(zr, limit))
	return n
}
