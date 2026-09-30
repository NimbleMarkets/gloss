package document

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

const sheetNS = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

// workbook packs sheets, in order, with shared strings and styles.
func workbook(t *testing.T, sheets map[string]string, extra map[string]string) []byte {
	t.Helper()
	var names, rels strings.Builder
	parts := map[string]string{}
	i := 0
	for _, name := range sortedKeys(sheets) {
		i++
		fmt.Fprintf(&names, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, name, i, i)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i, i)
		parts[fmt.Sprintf("xl/worksheets/sheet%d.xml", i)] = `<?xml version="1.0"?><worksheet ` + sheetNS + `><sheetData>` + sheets[name] + `</sheetData></worksheet>`
	}
	parts["xl/workbook.xml"] = `<?xml version="1.0"?><workbook ` + sheetNS + `><sheets>` + names.String() + `</sheets></workbook>`
	parts["xl/_rels/workbook.xml.rels"] = `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + rels.String() +
		`<Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>` +
		`<Relationship Id="rId10" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`
	parts["_rels/.rels"] = `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`
	for name, content := range extra {
		parts[name] = content
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, content := range parts {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(f, content)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// sortedKeys keeps sheet order stable: sheets are named 1-, 2-, ... in tests.
func sortedKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func cell(ref, kind, value string) string {
	if kind == "inlineStr" {
		return fmt.Sprintf(`<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, value)
	}
	if kind == "" {
		return fmt.Sprintf(`<c r="%s"><v>%s</v></c>`, ref, value)
	}
	return fmt.Sprintf(`<c r="%s" t="%s"><v>%s</v></c>`, ref, kind, value)
}

func rowsOf(sheet *Sheet) string {
	var lines []string
	for _, row := range sheet.Rows {
		lines = append(lines, strings.Join(row, "|"))
	}
	return strings.Join(lines, "\n")
}

func TestWorkbookSheetsAndCells(t *testing.T) {
	strings_ := `<?xml version="1.0"?><sst ` + sheetNS + ` count="3" uniqueCount="3"><si><t>Name</t></si><si><r><t>Rich</t></r><r><rPr><b/></rPr><t> text</t></r></si><si><t xml:space="preserve"> padded </t></si></sst>`
	book, err := OpenWorkbook(workbook(t, map[string]string{
		"1-Summary": `<row r="1">` + cell("A1", "s", "0") + cell("B1", "inlineStr", "Count") + cell("D1", "s", "1") + `</row>` +
			`<row r="2">` + cell("A2", "inlineStr", "apples") + cell("B2", "", "3") + `<c r="C2"><f>B2*2</f><v>6</v></c>` + cell("D2", "b", "1") + `</row>` +
			`<row r="5">` + cell("A5", "s", "2") + cell("B5", "", "3.5") + cell("C5", "e", "#DIV/0!") + cell("D5", "b", "0") + cell("E5", "str", "=SUM") + `</row>`,
		"2-Empty":   ``,
		"3-Numbers": `<row r="1">` + cell("A1", "", "1234567.891") + cell("B1", "", "0.000001") + cell("C1", "", "-42") + cell("D1", "", "1e21") + `</row>`,
	}, map[string]string{"xl/sharedStrings.xml": strings_}))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(book.Names(), ","); got != "1-Summary,2-Empty,3-Numbers" {
		t.Fatalf("sheets %q", got)
	}
	sheet, err := book.Sheet(0)
	if err != nil {
		t.Fatal(err)
	}
	want := "Name|Count||Rich text\napples|3|6|TRUE\n\n\n padded |3.5|#DIV/0!|FALSE|=SUM"
	if got := rowsOf(sheet); got != want {
		t.Fatalf("rows:\n%s\nwant:\n%s", got, want)
	}
	if sheet.Name != "1-Summary" || sheet.Columns != 5 || sheet.MoreRows != 0 {
		t.Fatalf("%+v", sheet)
	}
	if empty, err := book.Sheet(1); err != nil || len(empty.Rows) != 0 || empty.Columns != 0 {
		t.Fatalf("an empty sheet: %+v %v", empty, err)
	}
	numbers, _ := book.Sheet(2)
	if got := rowsOf(numbers); got != "1234567.891|0.000001|-42|1e+21" {
		t.Fatalf("numbers: %q", got)
	}
	// Once read, a sheet is kept.
	again, _ := book.Sheet(0)
	if again != sheet {
		t.Fatal("the sheet was read twice")
	}
	if _, err := book.Sheet(3); err == nil {
		t.Fatal("a sheet there is not")
	}
}

func TestWorkbookDates(t *testing.T) {
	styles := `<?xml version="1.0"?><styleSheet ` + sheetNS + `><numFmts count="2"><numFmt numFmtId="164" formatCode="yyyy mmm dd hh:mm:ss AM/PM"/><numFmt numFmtId="165" formatCode="[$-409]&quot;Week&quot; 0"/></numFmts>` +
		`<cellXfs count="6"><xf numFmtId="0"/><xf numFmtId="14"/><xf numFmtId="22"/><xf numFmtId="164"/><xf numFmtId="20"/><xf numFmtId="165"/></cellXfs></styleSheet>`
	styled := func(ref, style, value string) string {
		return fmt.Sprintf(`<c r="%s" s="%s"><v>%s</v></c>`, ref, style, value)
	}
	book, err := OpenWorkbook(workbook(t, map[string]string{"1-Dates": `<row r="1">` +
		styled("A1", "1", "45355") + styled("B1", "2", "45355.5625") + styled("C1", "3", "45355.5625") + styled("D1", "4", "0.75") + styled("E1", "5", "12") + styled("F1", "0", "45355") +
		styled("G1", "1", "1") + styled("H1", "1", "60") + styled("I1", "1", "61") + `</row>`}, map[string]string{"xl/styles.xml": styles}))
	if err != nil {
		t.Fatal(err)
	}
	sheet, _ := book.Sheet(0)
	// Dates and times are shown as such where their style says so; a
	// number styled as anything else, or not at all, is a number. Excel's
	// calendar has a 29th of February 1900 that never was.
	want := "2024-03-04|2024-03-04 13:30|2024-03-04 13:30:00|18:00|12|45355|1900-01-01|1900-02-29|1900-03-01"
	if got := rowsOf(sheet); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	from1904, _ := OpenWorkbook(workbook(t, map[string]string{"1-Dates": `<row r="1">` + styled("A1", "1", "0") + `</row>`},
		map[string]string{"xl/styles.xml": styles, "xl/workbook.xml": `<?xml version="1.0"?><workbook ` + sheetNS + `><workbookPr date1904="1"/><sheets><sheet name="1-Dates" sheetId="1" r:id="rId1"/></sheets></workbook>`}))
	if sheet, _ := from1904.Sheet(0); rowsOf(sheet) != "1904-01-01" {
		t.Fatalf("the 1904 calendar: %q", rowsOf(sheet))
	}
}

func TestWorkbookLimits(t *testing.T) {
	var rows strings.Builder
	for i := 1; i <= maxSheetRows+5; i++ {
		fmt.Fprintf(&rows, `<row r="%d">`+cell(fmt.Sprintf("A%d", i), "", "1")+`</row>`, i)
	}
	book, err := OpenWorkbook(workbook(t, map[string]string{"1-Long": rows.String()}, nil))
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := book.Sheet(0)
	if err != nil || len(sheet.Rows) != maxSheetRows || sheet.MoreRows != 5 {
		t.Fatalf("rows=%d more=%d err=%v", len(sheet.Rows), sheet.MoreRows, err)
	}
	var wide strings.Builder
	wide.WriteString(`<row r="1">`)
	for i := range maxSheetColumns + 3 {
		wide.WriteString(cell(ColumnName(i)+"1", "", "1"))
	}
	wide.WriteString(`</row>`)
	book, _ = OpenWorkbook(workbook(t, map[string]string{"1-Wide": wide.String()}, nil))
	if sheet, _ := book.Sheet(0); sheet.Columns != maxSheetColumns || sheet.MoreColumns != 3 || len(sheet.Rows[0]) != maxSheetColumns {
		t.Fatalf("columns=%d more=%d", sheet.Columns, sheet.MoreColumns)
	}
	for name, data := range map[string][]byte{
		"not an archive": []byte("PK\x03\x04 but not really"),
		"no workbook":    archive3MF(t, map[string]string{"xl/readme.txt": "hello"}),
		"no sheets":      workbook(t, map[string]string{}, nil),
		"sheet missing":  workbook(t, map[string]string{"1-Gone": ""}, map[string]string{"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/gone.xml"/></Relationships>`}),
		"malformed":      workbook(t, map[string]string{"1-Broken": `<row r="1"><c r="A1"><v>1`}, nil),
	} {
		book, err := OpenWorkbook(data)
		if err == nil {
			_, err = book.Sheet(0)
		}
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 701: "ZZ", 702: "AAA", 16383: "XFD"} {
		if got := ColumnName(i); got != want {
			t.Errorf("ColumnName(%d) = %q, want %q", i, got, want)
		}
		if got := columnIndex(want); got != i {
			t.Errorf("columnIndex(%q) = %d, want %d", want, got, i)
		}
	}
}

func TestWorkbookInfo(t *testing.T) {
	core := `<?xml version="1.0"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>Fruit</dc:title><dc:creator>R. Grocer</dc:creator><cp:lastModifiedBy>S. Grocer</cp:lastModifiedBy><dcterms:created xsi:type="dcterms:W3CDTF">2026-03-04T05:06:07Z</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">2026-04-05T06:07:08Z</dcterms:modified></cp:coreProperties>`
	app := `<?xml version="1.0"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Numbers 14.2</Application><Company>Acme</Company></Properties>`
	data := workbook(t, map[string]string{
		"1-Fruit": `<row r="1">` + cell("A1", "inlineStr", "apples") + cell("B1", "", "3") + `</row><row r="2">` + cell("A2", "inlineStr", "pears") + `</row>`,
		"2-Notes": `<row r="1">` + cell("A1", "inlineStr", "hi") + `</row>`,
	}, map[string]string{"docProps/core.xml": core, "docProps/app.xml": app})
	l := &Loader{}
	defer l.Close()
	r := l.Load(Request{Path: write(t, "fruit.xlsx", data), Page: 2, DPI: 72, Generation: 1})
	if r.Err != nil || r.Kind != "xlsx" || r.Sheet == nil || r.Page != 2 || r.Pages != 2 || r.Sheet.Name != "2-Notes" {
		t.Fatalf("err=%v kind=%q page=%d/%d sheet=%v", r.Err, r.Kind, r.Page, r.Pages, r.Sheet)
	}
	expect(t, r.Info, map[string]string{
		"Format": "Excel workbook", "Sheets": "1-Fruit (2 rows × 2 columns), 2-Notes (1 row × 1 column)", "Title": "Fruit", "Author": "R. Grocer",
		"Changed by": "S. Grocer", "Created": "2026-03-04 05:06", "Changed": "2026-04-05 06:07", "Application": "Numbers 14.2", "Company": "Acme",
	})
	if field(r.Info, "Size") == "" {
		t.Errorf("%+v", r.Info)
	}
	// Turning to a sheet out of range lands on the nearest, as with pages.
	if r := l.Load(Request{Path: write(t, "fruit.xlsx", data), Page: 9, DPI: 72, Generation: 2}); r.Err != nil || r.Page != 2 {
		t.Fatalf("page 9: %v %d", r.Err, r.Page)
	}
	if kind, err := Detect("book", data, ""); err != nil || kind != "xlsx" {
		t.Fatalf("Detect = %q, %v", kind, err)
	}
	if kind, err := Probe(write(t, "book.xlsm", data), ""); err != nil || kind != "xlsx" {
		t.Fatalf("Probe = %q, %v", kind, err)
	}
}
