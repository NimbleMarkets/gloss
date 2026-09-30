package app

import (
	"fmt"
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
	if hint := lines[len(lines)-1]; !strings.Contains(hint, "n/p sheets") || !strings.Contains(hint, "scroll") {
		t.Fatalf("hint: %s", hint)
	}
}

func TestSheetScrolls(t *testing.T) {
	m := tabulated(t, table(200, 40), 1, 1)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	at := func() (string, string) {
		lines := plain(m)
		return strings.Fields(lines[1])[1], strings.Fields(lines[0])[0]
	}
	if cell, col := at(); cell != "r1c1" || col != "A" {
		t.Fatalf("at the start: %s %s", cell, col)
	}
	m.Update(press("j"))
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if cell, _ := at(); cell != "r3c1" {
		t.Fatalf("after two rows down: %s", cell)
	}
	m.Update(press(" "))
	if cell, _ := at(); cell != "r12c1" {
		t.Fatalf("after a page: %s", cell)
	}
	m.Update(press("G"))
	if lines := plain(m); !strings.Contains(lines[len(lines)-3], "r200c1") {
		t.Fatalf("the end:\n%s", strings.Join(lines, "\n"))
	}
	m.Update(press("g"))
	m.Update(press("l"))
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if cell, col := at(); cell != "r1c3" || col != "C" {
		t.Fatalf("after two columns right: %s %s", cell, col)
	}
	for range 100 {
		m.Update(press("l"))
	}
	if _, col := at(); col != "AN" {
		t.Fatalf("past the last column: %s", col)
	}
	m.Update(press("h"))
	if _, col := at(); col != "AM" {
		t.Fatalf("one column back: %s", col)
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if cell, _ := at(); cell != "r4c39" {
		t.Fatalf("after the wheel: %s", cell)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if cell, col := at(); cell != "r1c1" || col != "A" {
		t.Fatalf("home: %s %s", cell, col)
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
