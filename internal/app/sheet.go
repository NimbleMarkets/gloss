package app

import (
	"fmt"
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
type sheetView struct {
	sheet    *document.Sheet
	row, col int    // The cell in the top-left corner.
	at       [2]int // The cell under the cursor.
	widths   []int  // Of each column, in cells.
}

func newSheetView(s *document.Sheet) *sheetView {
	v := &sheetView{sheet: s, widths: make([]int, s.Columns)}
	// Sized to the first rows, so that a long sheet need not be read twice.
	for c := range v.widths {
		v.widths[c] = max(minColumnWidth, len(document.ColumnName(c)))
		for r := 0; r < min(len(s.Rows), 200); r++ {
			if c < len(s.Rows[r]) {
				v.widths[c] = max(v.widths[c], min(maxColumnWidth, ansi.StringWidth(s.Rows[r][c])))
			}
		}
	}
	return v
}

func (v *sheetView) rows() int { return len(v.sheet.Rows) }

// gutter is the width of the row numbers.
func (v *sheetView) gutter() int {
	return max(2, len(strconv.Itoa(v.rows()+v.sheet.MoreRows)))
}

// shown counts the columns that fit across w cells from column first.
func (v *sheetView) shown(first, w int) int {
	room, n := w-v.gutter()-1, 0
	for c := first; c < len(v.widths) && (n == 0 || room >= v.widths[c]); c++ {
		room -= v.widths[c] + 1
		n++
	}
	return n
}

// settle keeps the cursor on the sheet and the corner where the cursor can
// be seen, in a grid w cells wide with h rows of cells.
func (v *sheetView) settle(w, h int) {
	h = max(1, h)
	v.at[0] = max(0, min(v.at[0], v.rows()-1))
	v.at[1] = max(0, min(v.at[1], len(v.widths)-1))
	v.row = max(0, min(v.row, v.rows()-h))
	v.col = max(0, min(v.col, len(v.widths)-1))
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

// key moves the cursor, and reports whether the key was one of its own.
func (v *sheetView) key(k string, w, h int) bool {
	switch k {
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
func (v *sheetView) url() string { return v.sheet.URL(v.at[0], v.at[1]) }

func (v *sheetView) view(w, h int) string {
	if v.rows() == 0 {
		return "(empty sheet)"
	}
	v.settle(w, h-1)
	dim, mark := lipgloss.NewStyle().Foreground(lipgloss.Color("245")), lipgloss.NewStyle().Reverse(true)
	gutter, n := v.gutter(), v.shown(v.col, w)
	var lines []string
	head := strings.Repeat(" ", gutter)
	for c := v.col; c < v.col+n; c++ {
		head += " " + fit(document.ColumnName(c), v.widths[c])
	}
	lines = append(lines, dim.Render(ansi.Truncate(head, w, "")))
	for r := v.row; r < v.rows() && len(lines) < h; r++ {
		line := dim.Render(fmt.Sprintf("%*d", gutter, r+1))
		row := v.sheet.Rows[r]
		for c := v.col; c < v.col+n; c++ {
			cell := ""
			if c < len(row) {
				cell = safe(row[c])
			}
			cell = fit(cell, v.widths[c])
			if r == v.at[0] && c == v.at[1] {
				cell = mark.Render(cell)
			}
			line += " " + cell
		}
		lines = append(lines, ansi.Truncate(line, w, ""))
	}
	return strings.Join(lines, "\n")
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
	cols := fmt.Sprintf("col %s–%s/%s", document.ColumnName(v.col), document.ColumnName(v.col+n-1), document.ColumnName(len(v.widths)+v.sheet.MoreColumns-1))
	if n == 1 {
		cols = fmt.Sprintf("col %s/%s", document.ColumnName(v.col), document.ColumnName(len(v.widths)+v.sheet.MoreColumns-1))
	}
	out := fmt.Sprintf("cell %s%d · %s · %s", document.ColumnName(v.at[1]), v.at[0]+1, rows, cols)
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
