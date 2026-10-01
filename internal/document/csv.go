package document

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ReadCSV reads a table of separated values as a sheet, the separator found
// from the file itself: a comma, tab, semicolon, or pipe, whichever the first
// lines agree on. A .tsv is read as tabs.
func ReadCSV(path string, data []byte) (*Sheet, error) {
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if !utf8.Valid(data) {
		data = bytes.ToValidUTF8(data, []byte("�"))
	}
	text := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return '�'
	}, string(data))
	delimiter, name := ',', "comma"
	if strings.EqualFold(filepath.Ext(path), ".tsv") {
		delimiter, name = '\t', "tab"
	} else if r, n := sniffDelimiter(text); n != "" {
		delimiter, name = r, n
	}
	if tooWide(text, delimiter) {
		return nil, fmt.Errorf("a line has more than %d values", maxCSVFields)
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes, r.ReuseRecord = delimiter, -1, true, true
	sheet := &Sheet{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Delimiter: name}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", len(sheet.Rows)+sheet.MoreRows+1, err)
		}
		if len(sheet.Rows) >= maxSheetRows {
			sheet.MoreRows++
			continue
		}
		row := make([]string, 0, len(record))
		for _, cell := range record {
			// A value of several lines is one cell of the grid.
			row = append(row, strings.Join(strings.Fields(cell), " "))
		}
		if len(row) > maxSheetColumns {
			sheet.MoreColumns = max(sheet.MoreColumns, len(row)-maxSheetColumns)
			row = row[:maxSheetColumns]
		}
		for len(row) > 0 && row[len(row)-1] == "" {
			row = row[:len(row)-1]
		}
		sheet.Rows = append(sheet.Rows, row)
		sheet.Columns = max(sheet.Columns, len(row))
	}
	for len(sheet.Rows) > 0 && len(sheet.Rows[len(sheet.Rows)-1]) == 0 {
		sheet.Rows = sheet.Rows[:len(sheet.Rows)-1]
	}
	if len(sheet.Rows) == 0 {
		return nil, fmt.Errorf("no rows")
	}
	return sheet, nil
}

// sniffDelimiter picks the separator that the first lines use most alike.
func sniffDelimiter(text string) (rune, string) {
	lines := strings.SplitN(text, "\n", 21)
	if len(lines) > 20 {
		lines = lines[:20]
	}
	best, bestName, bestScore := ',', "", 0
	for _, c := range []struct {
		r    rune
		name string
	}{{',', "comma"}, {'\t', "tab"}, {';', "semicolon"}, {'|', "pipe"}} {
		// Lines with the separator, weighted by agreement on how many.
		counts := map[int]int{}
		for _, line := range lines {
			if n := strings.Count(line, string(c.r)); n > 0 {
				counts[n]++
			}
		}
		score := 0
		for n, lines := range counts {
			score = max(score, lines*n)
		}
		if score > bestScore {
			best, bestName, bestScore = c.r, c.name, score
		}
	}
	return best, bestName
}

func (s *Sheet) csvFields() []Field {
	format := "Comma-separated values"
	switch s.Delimiter {
	case "tab":
		format = "Tab-separated values"
	case "semicolon":
		format = "Semicolon-separated values"
	case "pipe":
		format = "Pipe-separated values"
	}
	return Section("Table", Field{"Format", format}, Field{"Rows", grouped(len(s.Rows) + s.MoreRows)}, Field{"Columns", grouped(s.Columns + s.MoreColumns)})
}
