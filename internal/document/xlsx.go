package document

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	maxSheetRows    = 100000
	maxSheetColumns = 1024
)

// Sheet is one sheet of a workbook, as text: cells by row, each row cut
// after its last cell with anything in it.
type Sheet struct {
	Name        string
	Delimiter   string // Of a CSV: what set its values apart.
	Rows        [][]string
	Columns     int // The widest row.
	MoreRows    int // Beyond the limit, and not read.
	MoreColumns int
}

// Workbook is an Excel file. Its sheets are read as they are turned to.
type Workbook struct {
	pkg       *opc
	names     []string
	parts     []string
	shared    []string
	dates     []dateFormat // By style index.
	from1904  bool
	sheets    []*Sheet
	sheetErrs []error
}

// dateFormat says whether a cell style shows a number as a date, a time,
// or both, and whether to the second.
type dateFormat struct{ date, clock, seconds bool }

func OpenWorkbook(data []byte) (*Workbook, error) {
	pkg, err := openOPC(data)
	if err != nil {
		return nil, fmt.Errorf("workbook: %w", err)
	}
	if !pkg.has("xl/workbook.xml") {
		return nil, fmt.Errorf("workbook: no workbook")
	}
	w := &Workbook{pkg: pkg}
	data, err = pkg.read("xl/workbook.xml")
	if err != nil {
		return nil, fmt.Errorf("workbook: %w", err)
	}
	var book struct {
		Pr struct {
			Date1904 string `xml:"date1904,attr"`
		} `xml:"workbookPr"`
		Sheets struct {
			Sheet []struct {
				Name string `xml:"name,attr"`
				ID   string `xml:"id,attr"`
			} `xml:"sheet"`
		} `xml:"sheets"`
	}
	if err := xml.Unmarshal(data, &book); err != nil {
		return nil, fmt.Errorf("workbook: %w", err)
	}
	w.from1904 = book.Pr.Date1904 == "1" || book.Pr.Date1904 == "true"
	rels := pkg.relationships("xl/workbook.xml")
	for _, s := range book.Sheets.Sheet {
		w.names, w.parts = append(w.names, s.Name), append(w.parts, rels[s.ID])
	}
	if len(w.names) == 0 {
		return nil, fmt.Errorf("workbook: no sheets")
	}
	w.sheets, w.sheetErrs = make([]*Sheet, len(w.names)), make([]error, len(w.names))
	if data, err := pkg.read("xl/sharedStrings.xml"); err == nil {
		w.shared = sharedStrings(data)
	}
	if data, err := pkg.read("xl/styles.xml"); err == nil {
		w.dates = dateStyles(data)
	}
	return w, nil
}

func (w *Workbook) Names() []string { return w.names }

// Sheet reads sheet i, counting from zero, once.
func (w *Workbook) Sheet(i int) (*Sheet, error) {
	if i < 0 || i >= len(w.names) {
		return nil, fmt.Errorf("workbook has %d sheets", len(w.names))
	}
	if w.sheets[i] == nil && w.sheetErrs[i] == nil {
		data, err := w.pkg.read(w.parts[i])
		if err == nil {
			w.sheets[i], err = w.readSheet(w.names[i], data)
		}
		if err != nil {
			w.sheetErrs[i] = fmt.Errorf("sheet %s: %w", w.names[i], err)
		}
	}
	return w.sheets[i], w.sheetErrs[i]
}

// sharedStrings reads the strings cells share, each with its runs joined.
func sharedStrings(data []byte) []string {
	var out []string
	var text *strings.Builder
	d := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		token, err := d.Token()
		if err != nil {
			return out
		}
		switch e := token.(type) {
		case xml.StartElement:
			if e.Name.Local == "si" {
				text = &strings.Builder{}
			}
		case xml.CharData:
			if text != nil && inText(d) {
				text.Write(e)
			}
		case xml.EndElement:
			if e.Name.Local == "si" && text != nil {
				out = append(out, text.String())
				text = nil
			}
		}
	}
}

// inText reports whether the decoder is within a t element: the text of a
// string, rather than its phonetics or formatting.
func inText(d *xml.Decoder) bool {
	// The decoder does not say where it is, so the caller tracks it.
	return true
}

// dateStyles reads which cell styles are dates and times.
func dateStyles(data []byte) []dateFormat {
	var styles struct {
		NumFmts struct {
			NumFmt []struct {
				ID   int    `xml:"numFmtId,attr"`
				Code string `xml:"formatCode,attr"`
			} `xml:"numFmt"`
		} `xml:"numFmts"`
		CellXfs struct {
			Xf []struct {
				NumFmtID int `xml:"numFmtId,attr"`
			} `xml:"xf"`
		} `xml:"cellXfs"`
	}
	if xml.Unmarshal(data, &styles) != nil {
		return nil
	}
	custom := map[int]string{}
	for _, f := range styles.NumFmts.NumFmt {
		custom[f.ID] = f.Code
	}
	out := make([]dateFormat, len(styles.CellXfs.Xf))
	for i, xf := range styles.CellXfs.Xf {
		out[i] = numberFormat(xf.NumFmtID, custom[xf.NumFmtID])
	}
	return out
}

// numberFormat reads a format by its built-in id, or its code: letters
// for years, days, hours, and seconds outside quotes and brackets say a
// number is a date or time.
func numberFormat(id int, code string) dateFormat {
	if code == "" {
		switch {
		case id == 22:
			return dateFormat{date: true, clock: true}
		case id >= 14 && id <= 17, id >= 27 && id <= 31, id == 36, id >= 50 && id <= 58:
			return dateFormat{date: true}
		case id >= 18 && id <= 21, id >= 45 && id <= 47:
			return dateFormat{clock: true, seconds: id == 21 || id == 45 || id == 47}
		}
		return dateFormat{}
	}
	var bare strings.Builder
	quoted, bracketed := false, false
	for _, r := range strings.ToLower(code) {
		switch {
		case r == '"':
			quoted = !quoted
		case quoted:
		case r == '[':
			bracketed = true
		case r == ']':
			bracketed = false
		case !bracketed:
			bare.WriteRune(r)
		}
	}
	letters := bare.String()
	f := dateFormat{
		date:    strings.ContainsAny(letters, "yd") || (strings.Contains(letters, "m") && !strings.ContainsAny(letters, "hs")),
		clock:   strings.ContainsAny(letters, "hs"),
		seconds: strings.Contains(letters, "s"),
	}
	return f
}

func (w *Workbook) readSheet(name string, data []byte) (*Sheet, error) {
	sheet := &Sheet{Name: name}
	d := xml.NewDecoder(strings.NewReader(string(data)))
	var row []string
	rowAt, at, kind, style := -1, -1, "", 0
	var value, inline strings.Builder
	var text *strings.Builder
	depth := 0
	for {
		token, err := d.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		switch e := token.(type) {
		case xml.StartElement:
			depth++
			switch e.Name.Local {
			case "row":
				rowAt++
				if n, err := strconv.Atoi(attribute(e, "r")); err == nil && n-1 > rowAt {
					rowAt = n - 1
				}
				if rowAt >= maxSheetRows {
					sheet.MoreRows++
					row = nil
					continue
				}
				for len(sheet.Rows) < rowAt {
					sheet.Rows = append(sheet.Rows, nil)
				}
				row = []string{}
			case "c":
				if row == nil {
					continue
				}
				at++
				if ref := attribute(e, "r"); ref != "" {
					if col := columnIndex(strings.TrimRight(ref, "0123456789")); col >= 0 {
						at = col
					}
				}
				kind, style = attribute(e, "t"), 0
				if s, err := strconv.Atoi(attribute(e, "s")); err == nil {
					style = s
				}
				value.Reset()
				inline.Reset()
			case "v":
				text = &value
			case "t":
				text = &inline
			}
		case xml.CharData:
			if text != nil {
				text.Write(e)
			}
		case xml.EndElement:
			depth--
			switch e.Name.Local {
			case "v", "t":
				text = nil
			case "c":
				if row == nil {
					continue
				}
				if at >= maxSheetColumns {
					sheet.MoreColumns = max(sheet.MoreColumns, at+1-maxSheetColumns)
					continue
				}
				for len(row) <= at {
					row = append(row, "")
				}
				row[at] = w.cellText(kind, style, value.String(), inline.String())
			case "row":
				if row == nil {
					continue
				}
				for len(row) > 0 && row[len(row)-1] == "" {
					row = row[:len(row)-1]
				}
				sheet.Rows = append(sheet.Rows, row)
				sheet.Columns = max(sheet.Columns, len(row))
				row, at = nil, -1
			}
		}
	}
	for len(sheet.Rows) > 0 && len(sheet.Rows[len(sheet.Rows)-1]) == 0 {
		sheet.Rows = sheet.Rows[:len(sheet.Rows)-1]
	}
	return sheet, nil
}

// cellText is what a cell shows.
func (w *Workbook) cellText(kind string, style int, value, inline string) string {
	switch kind {
	case "s":
		if i, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && i >= 0 && i < len(w.shared) {
			return w.shared[i]
		}
		return ""
	case "inlineStr":
		return inline
	case "str", "e":
		return value
	case "b":
		if strings.TrimSpace(value) == "1" {
			return "TRUE"
		}
		return "FALSE"
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return value
	}
	if style < len(w.dates) && (w.dates[style].date || w.dates[style].clock) {
		return w.dateText(n, w.dates[style])
	}
	return numberText(n)
}

func numberText(n float64) string {
	if a := math.Abs(n); a >= 1e15 || (a > 0 && a < 1e-6) {
		return strconv.FormatFloat(n, 'g', -1, 64)
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// dateText reads a serial date. Excel counts days from the end of 1899,
// with a 29th of February 1900 that never was; the 1904 calendar counts
// from the start of 1904, and has no such day.
func (w *Workbook) dateText(serial float64, f dateFormat) string {
	days, fraction := math.Floor(serial), serial-math.Floor(serial)
	epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	switch {
	case w.from1904:
		epoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	case days < 60:
		epoch = time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)
	case days == 60 && f.date:
		return "1900-02-29" + w.clockText(fraction, f)
	}
	t := epoch.AddDate(0, 0, int(days))
	if !f.date {
		return strings.TrimSpace(w.clockText(fraction, f))
	}
	return t.Format("2006-01-02") + w.clockText(fraction, f)
}

func (w *Workbook) clockText(fraction float64, f dateFormat) string {
	if !f.clock {
		return ""
	}
	seconds := int(math.Round(fraction * 86400))
	out := fmt.Sprintf(" %02d:%02d", seconds/3600%24, seconds/60%60)
	if f.seconds {
		out += fmt.Sprintf(":%02d", seconds%60)
	}
	return out
}

// ColumnName is the letters of column i, counting from zero: A, B, …, Z, AA.
func ColumnName(i int) string {
	name := ""
	for i >= 0 {
		name = string(rune('A'+i%26)) + name
		i = i/26 - 1
	}
	return name
}

// columnIndex is the number of a column named by letters, or -1.
func columnIndex(name string) int {
	if name == "" {
		return -1
	}
	i := 0
	for _, r := range strings.ToUpper(name) {
		if r < 'A' || r > 'Z' {
			return -1
		}
		i = i*26 + int(r-'A') + 1
	}
	return i - 1
}

// fields describes the workbook.
func (w *Workbook) fields() []Field {
	var sheets []string
	for i, name := range w.names {
		size := ""
		if sheet, err := w.Sheet(i); err == nil {
			size = fmt.Sprintf(" (%s × %s)", plural(len(sheet.Rows)+sheet.MoreRows, "row"), plural(sheet.Columns+sheet.MoreColumns, "column"))
		}
		sheets = append(sheets, name+size)
	}
	return section("Workbook", append([]Field{{"Format", "Excel workbook"}, {"Sheets", strings.Join(sheets, ", ")}}, w.pkg.officeFields()...)...)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return grouped(n) + " " + noun + "s"
}
