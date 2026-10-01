package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

const (
	minColumnWidth = 3
	maxColumnWidth = 24
)

// sheetView shows a sheet as a grid with a cursor on one cell. The grid
// scrolls to keep the cursor in view; its columns are lettered and its rows
// numbered as in the spreadsheet.
//
// Columns can be hidden: cols lists those shown, and the corner and cursor
// hold places in cols, not columns of the sheet.
type sheetView struct {
	sheet    *document.Sheet
	row, col int    // The cell in the top-left corner: a row and a place in cols.
	at       [2]int // The cell under the cursor: a row and a place in cols.
	widths   []int  // Of each column, in cells.
	hidden   []bool // Of each column.
	cols     []int  // The columns shown, in order.
	picker   *columnPicker
}

func newSheetView(s *document.Sheet) *sheetView {
	v := &sheetView{sheet: s, widths: make([]int, s.Columns), hidden: make([]bool, s.Columns)}
	// Sized to the first rows, so that a long sheet need not be read twice.
	for c := range v.widths {
		v.widths[c] = max(minColumnWidth, len(document.ColumnName(c)))
		for r := 0; r < min(len(s.Rows), 200); r++ {
			if c < len(s.Rows[r]) {
				v.widths[c] = max(v.widths[c], min(maxColumnWidth, ansi.StringWidth(linked(s, r, c, s.Rows[r][c]))))
			}
		}
	}
	v.rebuild()
	return v
}

func (v *sheetView) rows() int { return len(v.sheet.Rows) }

// column is the sheet column under the cursor.
func (v *sheetView) column() int {
	if len(v.cols) == 0 {
		return 0
	}
	return v.cols[min(v.at[1], len(v.cols)-1)]
}

// rebuild lists the shown columns anew, keeping the cursor and the corner on
// their columns, or the nearest shown after them.
func (v *sheetView) rebuild() {
	at, corner := 0, 0
	if len(v.cols) > 0 {
		at, corner = v.cols[min(v.at[1], len(v.cols)-1)], v.cols[min(v.col, len(v.cols)-1)]
	}
	v.cols = v.cols[:0]
	for c, hidden := range v.hidden {
		if !hidden {
			v.cols = append(v.cols, c)
		}
	}
	v.at[1], v.col = v.place(at), v.place(corner)
}

// place is where column c, or the first shown after it, stands in cols.
func (v *sheetView) place(c int) int {
	for p, col := range v.cols {
		if col >= c {
			return p
		}
	}
	return max(0, len(v.cols)-1)
}

// setHidden hides the columns marked in hidden, all of them shown when it
// is nil. One column is always left, so the grid shows something.
func (v *sheetView) setHidden(hidden []bool) {
	for c := range v.hidden {
		v.hidden[c] = hidden != nil && c < len(hidden) && hidden[c]
	}
	if !slices.Contains(v.hidden, false) && len(v.hidden) > 0 {
		v.hidden[len(v.hidden)-1] = false
	}
	v.rebuild()
}

// hide takes column c out of the grid, unless it is the last one shown.
func (v *sheetView) hide(c int) {
	if len(v.cols) > 1 && c < len(v.hidden) && !v.hidden[c] {
		v.hidden[c] = true
		v.rebuild()
	}
}

// showAll brings every column back, the grid starting again from the left.
func (v *sheetView) showAll() {
	v.setHidden(nil)
	v.col = 0
}

// hiding counts the hidden columns.
func (v *sheetView) hiding() int { return len(v.hidden) - len(v.cols) }

// gutter is the width of the row numbers.
func (v *sheetView) gutter() int {
	return max(2, len(strconv.Itoa(v.rows()+v.sheet.MoreRows)))
}

// shown counts the columns that fit across w cells from column first.
func (v *sheetView) shown(first, w int) int {
	room, n := w-v.gutter()-1, 0
	for p := first; p < len(v.cols) && (n == 0 || room >= v.widths[v.cols[p]]); p++ {
		room -= v.widths[v.cols[p]] + 1
		n++
	}
	return n
}

// settle keeps the cursor on the sheet and the corner where the cursor can
// be seen, in a grid w cells wide with h rows of cells.
func (v *sheetView) settle(w, h int) {
	h = max(1, h)
	v.at[0] = max(0, min(v.at[0], v.rows()-1))
	v.at[1] = max(0, min(v.at[1], len(v.cols)-1))
	v.row = max(0, min(v.row, v.rows()-h))
	v.col = max(0, min(v.col, len(v.cols)-1))
	if v.at[0] < v.row {
		v.row = v.at[0]
	}
	if v.at[0] >= v.row+h {
		v.row = v.at[0] - h + 1
	}
	if v.at[1] < v.col {
		v.col = v.at[1]
	}
	for v.col < v.at[1] && v.at[1] >= v.col+v.shown(v.col, w) {
		v.col++
	}
}

// scroll moves the view, taking the cursor along if it would be left behind.
func (v *sheetView) scroll(rows, w, h int) {
	h = max(1, h)
	v.row = max(0, min(v.row+rows, v.rows()-h))
	v.at[0] = max(v.row, min(v.at[0], v.row+h-1))
	v.settle(w, h)
}

// key moves the cursor, hides and shows columns, and reports whether the
// key was one of its own. The column list, when open, has the keys instead.
func (v *sheetView) key(k string, w, h int) bool {
	switch k {
	case "x":
		v.hide(v.column())
	case "X":
		v.showAll()
	case "j", "down":
		v.at[0]++
	case "k", "up":
		v.at[0]--
	case "l", "right":
		v.at[1]++
	case "h", "left":
		v.at[1]--
	case "space", "pgdown":
		v.at[0] += h
		v.row += h
	case "b", "pgup":
		v.at[0] -= h
		v.row -= h
	case "home", "g":
		v.at = [2]int{}
	case "end", "G":
		v.at[0] = v.rows() - 1
	default:
		return false
	}
	v.settle(w, h)
	return true
}

// url is the web address under the cursor, if any.
func (v *sheetView) url() string { return v.sheet.URL(v.at[0], v.column()) }

func (v *sheetView) view(w, h int) string {
	if v.rows() == 0 {
		return "(empty sheet)"
	}
	v.settle(w, h-1)
	dim, mark := lipgloss.NewStyle().Foreground(lipgloss.Color("245")), lipgloss.NewStyle().Reverse(true)
	gutter, n := v.gutter(), v.shown(v.col, w)
	var lines []string
	head := strings.Repeat(" ", gutter)
	for _, c := range v.cols[v.col : v.col+n] {
		head += " " + fit(document.ColumnName(c), v.widths[c])
	}
	lines = append(lines, dim.Render(ansi.Truncate(head, w, "")))
	for r := v.row; r < v.rows() && len(lines) < h; r++ {
		line := dim.Render(fmt.Sprintf("%*d", gutter, r+1))
		row := v.sheet.Rows[r]
		for p, c := range v.cols[v.col : v.col+n] {
			cell := ""
			if c < len(row) {
				cell = linked(v.sheet, r, c, safe(row[c]))
			}
			cell = fit(cell, v.widths[c])
			if r == v.at[0] && v.col+p == v.at[1] {
				cell = mark.Render(cell)
			}
			// The terminal opens the address when the cell is clicked as
			// its links are; gloss itself opens nothing.
			if url := safe(v.sheet.URL(r, c)); url != "" {
				cell = ansi.SetHyperlink(url) + cell + ansi.ResetHyperlink()
			}
			line += " " + cell
		}
		lines = append(lines, ansi.Truncate(line, w, ""))
	}
	return strings.Join(lines, "\n")
}

// linkMark stands before a cell that holds or leads to a web address.
const linkMark = "🔗 "

// linked marks the text of a cell that has an address to open.
func linked(s *document.Sheet, row, col int, text string) string {
	if s.URL(row, col) != "" {
		return linkMark + text
	}
	return text
}

// fit pads or cuts text to width cells; a number is set to the right.
func fit(text string, width int) string {
	if ansi.StringWidth(text) > width {
		return ansi.Truncate(text, width, "…")
	}
	pad := strings.Repeat(" ", width-ansi.StringWidth(text))
	if _, err := strconv.ParseFloat(text, 64); err == nil && text != "" {
		return pad + text
	}
	return text + pad
}

// status describes what is on show, for the status bar.
func (v *sheetView) status(w, h int) string {
	if v.rows() == 0 {
		return "empty"
	}
	last := min(v.rows(), v.row+h-1)
	rows := fmt.Sprintf("row %d–%d/%s", v.row+1, last, grouped(v.rows()+v.sheet.MoreRows))
	if last == v.row+1 {
		rows = fmt.Sprintf("row %d/%s", v.row+1, grouped(v.rows()+v.sheet.MoreRows))
	}
	n := v.shown(v.col, w)
	end := document.ColumnName(len(v.widths) + v.sheet.MoreColumns - 1)
	cols := fmt.Sprintf("col %s–%s/%s", document.ColumnName(v.cols[v.col]), document.ColumnName(v.cols[v.col+n-1]), end)
	if n == 1 {
		cols = fmt.Sprintf("col %s/%s", document.ColumnName(v.cols[v.col]), end)
	}
	out := fmt.Sprintf("cell %s%d · %s · %s", document.ColumnName(v.column()), v.at[0]+1, rows, cols)
	if v.hiding() > 0 {
		out += fmt.Sprintf(" · cols %d/%d", len(v.cols), len(v.hidden))
	}
	if v.sheet.MoreRows > 0 {
		out += fmt.Sprintf(" · first %s rows read", grouped(v.rows()))
	}
	if url := v.url(); url != "" {
		out += " → " + safe(url)
	}
	return out
}

func grouped(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
