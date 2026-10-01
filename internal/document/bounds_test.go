package document

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

// within fails the test when f has not returned in d, so that a hang is a
// failure, and says how much memory f asked for, so that a blowup is one too.
func within(t *testing.T, d time.Duration, maxAlloc uint64, f func()) {
	t.Helper()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	done := make(chan struct{})
	go func() { defer close(done); f() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("did not return in %v", d)
	}
	runtime.ReadMemStats(&after)
	if grew := after.TotalAlloc - before.TotalAlloc; grew > maxAlloc {
		t.Fatalf("allocated %d MiB, over the %d MiB allowed", grew>>20, maxAlloc>>20)
	}
}

func TestWithDeadlineGivesUpOnWhatNeverReturns(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	began := time.Now()
	_, err := withDeadline(100*time.Millisecond, "the test", func() (int, error) { <-stop; return 1, nil })
	if err == nil || !strings.Contains(err.Error(), "took longer") || time.Since(began) > 2*time.Second {
		t.Fatalf("err=%v after %v", err, time.Since(began))
	}
	if v, err := withDeadline(time.Second, "the test", func() (int, error) { return 7, nil }); v != 7 || err != nil {
		t.Fatalf("a quick job: %v %v", v, err)
	}
	want := errors.New("plain failure")
	if _, err := withDeadline(time.Second, "the test", func() (int, error) { return 0, want }); !errors.Is(err, want) {
		t.Fatalf("an error is passed on: %v", err)
	}
	// A panic in the library is an error, not a crash of the process.
	if _, err := withDeadline(time.Second, "the test", func() (int, error) { panic("boom") }); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("a panic: %v", err)
	}
}

// One line of a hundred million commas made one row of that many cells.
func TestWideCSVLineIsRefused(t *testing.T) {
	within(t, 5*time.Second, 64<<20, func() {
		wide := "a" + strings.Repeat(",", maxCSVFields+10) + "\n1,2\n"
		if _, err := ReadCSV("wide.csv", []byte(wide)); err == nil || !strings.Contains(err.Error(), "more than") {
			t.Errorf("a wide line: %v", err)
		}
		edge := "a" + strings.Repeat(",", maxCSVFields) + "\n"
		if _, err := ReadCSV("edge.csv", []byte(edge)); err != nil {
			t.Errorf("a line at the limit: %v", err)
		}
		if sheet, err := ReadCSV("ok.csv", []byte("a,b\n1,2\n")); err != nil || sheet.Columns != 2 {
			t.Errorf("an ordinary file: %v", err)
		}
	})
}

// Each row with one cell at the last column is padded to the whole width.
func TestSparseExcelRowsAreBudgeted(t *testing.T) {
	var rows strings.Builder
	const n = 5000
	for r := 1; r <= n; r++ {
		fmt.Fprintf(&rows, `<row r="%d"><c r="AMJ%d"><v>1</v></c></row>`, r, r)
	}
	data := workbook(t, map[string]string{"Sheet1": rows.String()}, nil)
	within(t, 10*time.Second, 160<<20, func() {
		book, err := OpenWorkbook(data)
		if err != nil {
			t.Error(err)
			return
		}
		sheet, err := book.Sheet(0)
		if err != nil {
			t.Error(err)
			return
		}
		slots := 0
		for _, row := range sheet.Rows {
			slots += len(row)
		}
		if slots > maxSheetCells || sheet.MoreColumns == 0 {
			t.Errorf("held %d slots (budget %d), more columns = %d", slots, maxSheetCells, sheet.MoreColumns)
		}
	})
}

// One wide row among many is a square of cells once the rows are padded.
func TestWordTablesAreBounded(t *testing.T) {
	var table strings.Builder
	cell := `<w:tc><w:p><w:r><w:t>x</w:t></w:r></w:p></w:tc>`
	table.WriteString(`<w:tbl><w:tr>` + strings.Repeat(cell, 6000) + `</w:tr>`)
	for i := 0; i < 6000; i++ {
		table.WriteString(`<w:tr>` + cell + `</w:tr>`)
	}
	table.WriteString(`</w:tbl>`)
	data := wordDocument(t, table.String(), nil)
	within(t, 10*time.Second, 160<<20, func() {
		doc, err := OpenWord(data)
		if err != nil {
			t.Error(err)
			return
		}
		if len(doc.Markdown) > MaxMarkdownBytes || !strings.Contains(string(doc.Markdown), "rest of the table was cut") {
			t.Errorf("%d bytes of Markdown, without the note that the table was cut", len(doc.Markdown))
		}
	})
}

func TestHTMLTablesAreBounded(t *testing.T) {
	var big strings.Builder
	big.WriteString("<table><tr>" + strings.Repeat("<td>x</td>", 6000) + "</tr>")
	big.WriteString(strings.Repeat("<tr><td>y</td></tr>", 6000) + "</table>")
	spans := "<table>" + strings.Repeat(`<tr><td colspan="999999999">z</td></tr>`, 5) + "</table>"
	nested := "<table><tr><td><table><tr>" + strings.Repeat("<td>n</td>", 2000) + "</tr>" + strings.Repeat("<tr><td>m</td></tr>", 2000) + "</table></td></tr></table>"
	within(t, 10*time.Second, 160<<20, func() {
		for name, page := range map[string]string{"a wide row and many rows": big.String(), "colspans": spans, "a nested table": nested} {
			if _, err := ReadHTML([]byte(page)); err == nil || !strings.Contains(err.Error(), "table") {
				t.Errorf("%s: %v", name, err)
			}
		}
		ordinary := "<table><tr><th>a</th><th>b</th></tr><tr><td>1</td><td colspan=\"2\">2</td></tr></table>"
		if page, err := ReadHTML([]byte(ordinary)); err != nil || !strings.Contains(string(page.Markdown), "| a | b |") {
			t.Errorf("an ordinary table: %v", err)
		}
	})
}

// A million block quotes, or brackets, cost the parser minutes.
func TestMarkdownNestingIsRefused(t *testing.T) {
	within(t, 10*time.Second, 160<<20, func() {
		for name, source := range map[string]string{
			"quotes":        strings.Repeat("> ", 200000) + "x",
			"packed quotes": strings.Repeat(">", 200000) + "x",
			"open brackets": strings.Repeat("[", 1000000) + "x" + strings.Repeat("]", 1000000),
		} {
			if _, err := loadMarkdown("n.md", []byte(source), t.TempDir()); err == nil {
				t.Errorf("%s were accepted", name)
			}
		}
		deepEnough := strings.Repeat("> ", 20) + "quoted\n\n" + strings.Repeat("[", 50) + "x" + strings.Repeat("]", 50) + "\n\n[a [nested] link](x)\n"
		if _, err := loadMarkdown("n.md", []byte(deepEnough), t.TempDir()); err != nil {
			t.Errorf("ordinary nesting: %v", err)
		}
	})
}

// Fifty components in each of sixteen objects, every one the next: fifty to
// the sixteenth objects, from a few hundred bytes.
func Test3MFComponentFanOutIsBounded(t *testing.T) {
	var objects strings.Builder
	const levels, fan = 15, 50
	for i := 1; i <= levels; i++ {
		fmt.Fprintf(&objects, `<object id="%d"><components>%s</components></object>`, i, strings.Repeat(fmt.Sprintf(`<component objectid="%d"/>`, i+1), fan))
	}
	fmt.Fprintf(&objects, `<object id="%d">%s</object>`, levels+1, tetrahedron)
	data := archive3MF(t, map[string]string{"3D/3dmodel.model": model3MF("", `<resources>`+objects.String()+`</resources><build><item objectid="1"/></build>`)})
	within(t, 20*time.Second, 512<<20, func() {
		if _, err := Parse3MF(data); err == nil || !strings.Contains(err.Error(), "more than") {
			t.Errorf("a 3MF that places fifty to the fifteenth objects: %v", err)
		}
	})
}

// An archive's central directory may name a million entries in a small file;
// the reader makes a record for each before they can be counted.
func TestZipEntryCountIsLookedAtFirst(t *testing.T) {
	archive := func(entries int) []byte {
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		for i := 0; i < entries; i++ {
			if _, err := w.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("f%d", i), Method: zip.Store}); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	few, many := archive(10), archive(maxPackageEntries+50)
	if n := zipEntries(bytes.NewReader(few), int64(len(few))); n != 10 {
		t.Errorf("a small archive says %d entries", n)
	}
	if _, err := openZip(bytes.NewReader(many), int64(len(many))); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Errorf("an archive of %d entries: %v", maxPackageEntries+50, err)
	}
	if _, err := openZip(bytes.NewReader(few), int64(len(few))); err != nil {
		t.Errorf("a small archive: %v", err)
	}
	// 0xFFFF in the count says "see the zip64 record": more than we take.
	sixtyFour := bytes.Clone(few)
	if i := bytes.LastIndex(sixtyFour, []byte("PK\x05\x06")); i >= 0 {
		sixtyFour[i+10], sixtyFour[i+11] = 0xFF, 0xFF
		if n := zipEntries(bytes.NewReader(sixtyFour), int64(len(sixtyFour))); n <= maxPackageEntries {
			t.Errorf("the zip64 marker counted as %d", n)
		}
	}
	// Nothing to read, or a stub: no panic, no count.
	for _, stub := range [][]byte{nil, []byte("PK"), []byte("PK\x05\x06")} {
		if n := zipEntries(bytes.NewReader(stub), int64(len(stub))); n != 0 {
			t.Errorf("%q counted %d", stub, n)
		}
	}
}

func TestFetchDoesNotFollowHTTPSDownToHTTP(t *testing.T) {
	get := func(url string) *http.Request {
		r, err := http.NewRequest("GET", url, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, c := range []struct {
		next string
		via  []string
		ok   bool
	}{
		{"https://b.example/x", []string{"https://a.example/"}, true},
		{"http://b.example/x", []string{"http://a.example/"}, true},
		{"https://b.example/x", []string{"http://a.example/"}, true},
		{"http://b.example/x", []string{"https://a.example/"}, false},
		{"http://c.example/x", []string{"https://a.example/", "http://b.example/"}, false},
		{"ftp://b.example/x", []string{"http://a.example/"}, false},
	} {
		var via []*http.Request
		for _, v := range c.via {
			via = append(via, get(v))
		}
		if err := fetchRedirect(get(c.next), via); (err == nil) != c.ok {
			t.Errorf("%v -> %s: err = %v, want ok = %v", c.via, c.next, err, c.ok)
		}
	}
	tooMany := make([]*http.Request, 10)
	for i := range tooMany {
		tooMany[i] = get("http://a.example/")
	}
	if err := fetchRedirect(get("http://a.example/"), tooMany); err == nil {
		t.Error("ten redirects were followed")
	}
}
