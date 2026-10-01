package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

func table(rows, cols int) *document.Sheet {
	s := &document.Sheet{Name: "Data", Columns: cols}
	for r := range rows {
		var row []string
		for c := range cols {
			row = append(row, fmt.Sprintf("r%dc%d", r+1, c+1))
		}
		s.Rows = append(s.Rows, row)
	}
	return s
}

func tabulated(t *testing.T, sheet *document.Sheet, page, pages int) *Model {
	t.Helper()
	m := viewing(t, Options{Files: []string{"book.xlsx"}}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "xlsx", Page: page, Pages: pages, Sheet: sheet})
	if m.sheet == nil {
		t.Fatal("no sheet is shown")
	}
	return m
}

func plain(m *Model) []string { return strings.Split(ansi.Strip(m.View().Content), "\n") }

func TestSheetIsShownAsAGrid(t *testing.T) {
	sheet := table(3, 3)
	sheet.Rows[0] = []string{"Name", "Count", "Note\x1b]2;owned\a"}
	sheet.Rows[1] = []string{"apples", "3", "a note that runs on and on and on and on and on"}
	sheet.Rows[2] = []string{"pears"}
	m := tabulated(t, sheet, 1, 2)
	lines := plain(m)
	if !strings.Contains(lines[0], "A") || !strings.Contains(lines[0], "B") || !strings.Contains(lines[0], "C") {
		t.Fatalf("no column letters:\n%s", lines[0])
	}
	if !strings.HasPrefix(strings.TrimLeft(lines[1], " "), "1 ") || !strings.Contains(lines[1], "Name") || !strings.Contains(lines[1], "Count") {
		t.Fatalf("row 1:\n%s", lines[1])
	}
	if !strings.Contains(lines[2], "apples") || !strings.Contains(lines[2], "…") || strings.Contains(lines[2], "and on and on and on") {
		t.Fatalf("a long cell is cut short:\n%s", lines[2])
	}
	if strings.Contains(m.View().Content, "\x1b]2;") {
		t.Fatal("a cell injected a terminal escape")
	}
	status := lines[len(lines)-2]
	for _, want := range []string{"xlsx", "sheet 1/2", "Data", "row 1", "/3", "col A"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status lacks %q: %s", want, status)
		}
	}
	if hint := lines[len(lines)-1]; !strings.Contains(hint, "n/p sheets") || !strings.Contains(hint, "move") {
		t.Fatalf("hint: %s", hint)
	}
}

func TestSheetCursorAndScrolling(t *testing.T) {
	m := tabulated(t, table(200, 40), 1, 1)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	top := func() (string, string) {
		lines := plain(m)
		return strings.Fields(lines[1])[1], strings.Fields(lines[0])[0]
	}
	cursor := func() string {
		status := plain(m)[len(plain(m))-2]
		_, after, _ := strings.Cut(status, "cell ")
		return strings.Fields(after)[0]
	}
	if cell, col := top(); cell != "r1c1" || col != "A" || cursor() != "A1" {
		t.Fatalf("at the start: %s %s %s", cell, col, cursor())
	}
	if !strings.Contains(m.View().Content, "\x1b[7m") {
		t.Fatal("the cell under the cursor is not marked")
	}
	// The cursor moves within the view before the view moves.
	m.Update(press("j"))
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if cell, _ := top(); cell != "r1c1" || cursor() != "A3" {
		t.Fatalf("after two down: top %s, cursor %s", cell, cursor())
	}
	for range 10 {
		m.Update(press("j"))
	}
	// Twelve rows of terminal leave nine of cells below the header.
	if cell, _ := top(); cell != "r5c1" || cursor() != "A13" {
		t.Fatalf("the view follows the cursor: top %s, cursor %s", cell, cursor())
	}
	m.Update(press(" "))
	if cursor() != "A22" {
		t.Fatalf("after a page: %s", cursor())
	}
	m.Update(press("G"))
	if lines := plain(m); cursor() != "A200" || !strings.Contains(lines[len(lines)-3], "r200c1") {
		t.Fatalf("the end: %s", cursor())
	}
	m.Update(press("g"))
	m.Update(press("l"))
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if _, col := top(); col != "A" || cursor() != "C1" {
		t.Fatalf("after two right: first column %s, cursor %s", col, cursor())
	}
	for range 100 {
		m.Update(press("l"))
	}
	if _, col := top(); cursor() != "AN1" || col == "A" {
		t.Fatalf("past the last column: cursor %s, first column %s", cursor(), col)
	}
	m.Update(press("h"))
	if cursor() != "AM1" {
		t.Fatalf("one column back: %s", cursor())
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if cell, col := top(); cell != "r1c1" || col != "A" || cursor() != "A1" {
		t.Fatalf("home: %s %s %s", cell, col, cursor())
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if cell, _ := top(); cell != "r4c1" {
		t.Fatalf("after the wheel: %s", cell)
	}
}

func TestSheetOpensTheAddressUnderTheCursor(t *testing.T) {
	sheet := &document.Sheet{Name: "links", Columns: 2, Rows: [][]string{{"name", "picture"}, {"fern", "https://example.com/fern.png"}, {"moss", "plain"}}}
	dir := t.TempDir()
	var fetched []string
	m := viewing(t, Options{Files: []string{"links.csv"}, Fetch: func(address string) (string, error) {
		fetched = append(fetched, address)
		if strings.HasSuffix(address, "fern.png") {
			return writePNG(t, filepath.Join(dir, "fern.png")), nil
		}
		return "", fmt.Errorf("HTTP 404")
	}}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "csv", Page: 1, Pages: 1, Sheet: sheet})
	send(m, press("j"), press("l"))
	if status := plain(m)[len(plain(m))-2]; !strings.Contains(status, "cell B2") || !strings.Contains(status, "→ https://example.com/fern.png") {
		t.Fatalf("status: %s", status)
	}
	send(m, enter)
	if len(fetched) != 1 || len(m.opts.Files) != 2 || m.index != 1 || m.kind != "png" {
		t.Fatalf("fetched=%q files=%q index=%d kind=%q", fetched, m.opts.Files, m.index, m.kind)
	}
	if got := m.Fetched(); len(got) != 1 || got[0] != m.opts.Files[1] {
		t.Fatalf("fetched files: %q", got)
	}
	// Back on the sheet, a cell with no address does nothing, and a
	// fetch that fails says why.
	send(m, press("["))
	m.Update(document.Result{Generation: m.generation, Kind: "csv", Page: 1, Pages: 1, Sheet: sheet})
	send(m, press("j"), press("j"), press("l"))
	if _, cmd := m.Update(enter); cmd != nil || len(fetched) != 1 {
		t.Fatal("a plain cell was fetched")
	}
	sheet.Rows[2][1] = "https://example.com/gone.png"
	send(m, enter)
	if len(fetched) != 2 || !strings.Contains(m.note, "HTTP 404") || len(m.opts.Files) != 2 {
		t.Fatalf("fetched=%q note=%q", fetched, m.note)
	}
}

func TestSheetAddressesNeedFetchingToBeAllowed(t *testing.T) {
	sheet := &document.Sheet{Name: "links", Columns: 1, Rows: [][]string{{"https://example.com/fern.png"}}}
	m := viewing(t, Options{Files: []string{"links.csv"}}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "csv", Page: 1, Pages: 1, Sheet: sheet})
	if status := plain(m)[len(plain(m))-2]; !strings.Contains(status, "→ https://example.com/fern.png") {
		t.Fatalf("status: %s", status)
	}
	if _, cmd := m.Update(enter); cmd != nil || !strings.Contains(m.note, "--fetch") || len(m.opts.Files) != 1 {
		t.Fatalf("note=%q", m.note)
	}
}

func TestSheetFitsTheTerminal(t *testing.T) {
	m := tabulated(t, table(50, 30), 1, 1)
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 40}, {Width: 30, Height: 6}, {Width: 7, Height: 4}, {Width: 1, Height: 1}} {
		m.Update(size)
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) > size.Height {
			t.Fatalf("%d rows for a terminal of %d", len(lines), size.Height)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size.Width {
				t.Fatalf("line exceeds width %d: %q", size.Width, line)
			}
		}
	}
	empty := tabulated(t, &document.Sheet{Name: "Blank"}, 1, 1)
	if view := empty.View().Content; !strings.Contains(view, "empty") {
		t.Fatalf("an empty sheet:\n%s", view)
	}
}

func TestSheetsTurnLikePages(t *testing.T) {
	m := tabulated(t, table(3, 3), 1, 3)
	m.Update(press("n"))
	if !m.loading || m.page != 2 {
		t.Fatalf("n: loading=%v page=%d", m.loading, m.page)
	}
	m.Update(document.Result{Generation: m.generation, Kind: "xlsx", Page: 2, Pages: 3, Sheet: &document.Sheet{Name: "Second", Rows: [][]string{{"x"}}, Columns: 1}})
	if !strings.Contains(plain(m)[1], "x") || !strings.Contains(m.View().Content, "sheet 2/3") {
		t.Fatalf("the second sheet:\n%s", m.View().Content)
	}
	m.Update(press("p"))
	if m.page != 1 {
		t.Fatal("p did not turn back")
	}
	// The tables of a Grist document turn as sheets do, and are called tables.
	m.Update(document.Result{Generation: m.generation, Kind: "grist", Page: 2, Pages: 3, Sheet: &document.Sheet{Name: "Sites", Rows: [][]string{{"Site name"}, {"Reed Marsh"}}, Columns: 1}})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "grist · table 2/3 · Sites") || !strings.Contains(view, "n/p tables") {
		t.Fatalf("a Grist table:\n%s", view)
	}
	m.Update(press("n"))
	if !m.loading || m.page != 3 {
		t.Fatalf("n over a Grist table: loading=%v page=%d", m.loading, m.page)
	}
	m.Update(document.Result{Generation: m.generation, Kind: "xlsx", Page: 1, Pages: 3, Sheet: table(3, 3)})
	// A sheet that says more rows were left unread says so.
	partial := &document.Sheet{Name: "Big", Rows: [][]string{{"a"}}, Columns: 1, MoreRows: 5000}
	m.Update(document.Result{Generation: m.generation, Kind: "xlsx", Page: 1, Pages: 3, Sheet: partial})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "5,001") {
		t.Fatalf("rows unread are not counted:\n%s", view)
	}
	_, cmd := m.Update(press("e"))
	if cmd != nil || !strings.Contains(m.View().Content, "cannot be exported") {
		t.Fatalf("export: %q", m.note)
	}
}

func TestWordDocumentIsNamedAsSuch(t *testing.T) {
	m := viewing(t, Options{Files: []string{"memo.docx"}}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "docx", Page: 1, Pages: 1, Markdown: &document.Markdown{Source: []byte("# Memo\n\nHello.\n")}})
	if view := m.View().Content; !strings.Contains(view, "Word · rendered") || strings.Contains(view, "Markdown · rendered") {
		t.Fatalf("status:\n%s", view)
	}
}

func TestCSVIsShownAsASheetOfItsOwn(t *testing.T) {
	m := viewing(t, Options{Files: []string{"sales.csv"}}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "csv", Page: 1, Pages: 1, Sheet: &document.Sheet{Name: "sales", Rows: [][]string{{"a", "b"}}, Columns: 2, Delimiter: "comma"}})
	view := ansi.Strip(m.View().Content)
	if m.sheet == nil || !strings.Contains(view, "csv · cell A1 · row 1/1") || strings.Contains(view, "sheet 1/1") {
		t.Fatalf("status:\n%s", view)
	}
	if lines := strings.Split(view, "\n"); !strings.Contains(lines[len(lines)-1], "n/p files") {
		t.Fatalf("hint:\n%s", lines[len(lines)-1])
	}
}

func TestEscapeClosesAFetchedFileAndReturnsToItsCell(t *testing.T) {
	sheet := &document.Sheet{Name: "links", Columns: 2, Rows: [][]string{{"name", "picture"}, {"fern", "https://example.com/fern.png"}, {"moss", "plain"}}}
	dir := t.TempDir()
	m := viewing(t, Options{Files: []string{"links.csv"}, Fetch: func(address string) (string, error) {
		return writePNG(t, filepath.Join(dir, "fern.png")), nil
	}}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "csv", Page: 1, Pages: 1, Sheet: sheet})
	send(m, press("j"), press("l"), enter)
	if len(m.opts.Files) != 2 || m.index != 1 {
		t.Fatalf("files=%q index=%d", m.opts.Files, m.index)
	}
	fetched := m.opts.Files[1]
	if hints := plain(m)[len(plain(m))-1]; !strings.HasPrefix(hints, " Esc close") {
		t.Fatalf("hints: %s", hints)
	}
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(m.opts.Files) != 1 || m.index != 0 || len(m.Fetched()) != 0 {
		t.Fatalf("files=%q index=%d fetched=%q", m.opts.Files, m.index, m.Fetched())
	}
	if _, err := os.Stat(fetched); !os.IsNotExist(err) {
		t.Fatal("the closed file was kept")
	}
	m.Update(document.Result{Generation: m.generation, Kind: "csv", Page: 1, Pages: 1, Sheet: sheet})
	if status := plain(m)[len(plain(m))-2]; !strings.Contains(status, "cell B2") {
		t.Fatalf("status: %s", status)
	}
	// Escape on a file that was not fetched closes nothing: it goes to the list.
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(m.opts.Files) != 1 || !m.listing() {
		t.Fatalf("files=%q menu=%v", m.opts.Files, m.listing())
	}
}

func TestColumnsCanBeHiddenFromTheGrid(t *testing.T) {
	m := tabulated(t, table(3, 3), 1, 1)
	header := func() string { return strings.Join(strings.Fields(plain(m)[0]), " ") }
	status := func() string { return plain(m)[len(plain(m))-2] }
	send(m, press("l"), press("x"))
	if header() != "A C" || !strings.Contains(status(), "cell C1") || !strings.Contains(status(), "cols 2/3") {
		t.Fatalf("after x: header %q status %q", header(), status())
	}
	if row := strings.Fields(plain(m)[1]); len(row) != 3 || row[1] != "r1c1" || row[2] != "r1c3" {
		t.Fatalf("row 1: %q", row)
	}
	// The cursor moves across visible columns only.
	send(m, press("h"))
	if !strings.Contains(status(), "cell A1") {
		t.Fatalf("after h: %s", status())
	}
	// The last column stays: a grid must show something.
	send(m, press("x"), press("x"))
	if header() != "C" || strings.Contains(status(), "cols 0/3") {
		t.Fatalf("after hiding all: header %q status %q", header(), status())
	}
	send(m, press("X"))
	if header() != "A B C" || strings.Contains(status(), "cols ") {
		t.Fatalf("after X: header %q status %q", header(), status())
	}
}

func TestColumnPickerListsAndTogglesColumns(t *testing.T) {
	sheet := table(3, 3)
	sheet.Rows[0] = []string{"Name", "Age", "City"}
	m := tabulated(t, sheet, 1, 1)
	view := func() string { return ansi.Strip(m.View().Content) }
	send(m, press("c"))
	if v := view(); !strings.Contains(v, "[x] A  Name") || !strings.Contains(v, "[x] B  Age") || !strings.Contains(v, "[x] C  City") {
		t.Fatalf("picker:\n%s", v)
	}
	send(m, press("j"), press(" "))
	if v := view(); !strings.Contains(v, "[ ] B  Age") {
		t.Fatalf("after toggling B:\n%s", v)
	}
	send(m, press("n"))
	if v := view(); !strings.Contains(v, "[ ] A  Name") || !strings.Contains(v, "[x] C  City") {
		t.Fatalf("none keeps one column:\n%s", v)
	}
	send(m, press("a"), press(" "), tea.KeyPressMsg{Code: tea.KeyEscape})
	if v := view(); strings.Contains(v, "[x]") || strings.Contains(v, "Age") || !strings.Contains(v, "City") {
		t.Fatalf("after closing:\n%s", v)
	}
	if hints := plain(m)[len(plain(m))-1]; !strings.Contains(hints, "c columns") {
		t.Fatalf("hints: %s", hints)
	}
}

func TestColumnsOptionHidesColumnsOnLoad(t *testing.T) {
	sheet := table(3, 3)
	sheet.Rows[0] = []string{"Name", "Age", "City"}
	filter, _ := document.ParseColumns("city", "1")
	m := viewing(t, Options{Files: []string{"book.xlsx"}, Columns: filter}, "")
	m.Update(document.Result{Generation: m.generation, Kind: "xlsx", Page: 1, Pages: 1, Sheet: sheet})
	if header := strings.Join(strings.Fields(plain(m)[0]), " "); header != "A C" {
		t.Fatalf("header %q", header)
	}
}
