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

// sheetView shows a sheet as a grid that scrolls by row and by column, with
// the columns lettered and the rows numbered as in the spreadsheet.
type sheetView struct {
	sheet    *document.Sheet
	row, col int   // The cell in the top-left corner.
	widths   []int // Of each column, in cells.
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

// shown counts the columns that fit across w cells from the first shown.
func (v *sheetView) shown(w int) int {
	room, n := w-v.gutter()-1, 0
	for c := v.col; c < len(v.widths) && (n == 0 || room >= v.widths[c]); c++ {
		room -= v.widths[c] + 1
		n++
	}
	return n
}

// clamp keeps the corner within the sheet, given h rows of cells on show.
func (v *sheetView) clamp(h int) {
	v.row = max(0, min(v.row, v.rows()-max(1, h)))
	v.col = max(0, min(v.col, len(v.widths)-1))
}

func (v *sheetView) scroll(rows, h int) {
	v.row += rows
	v.clamp(h)
}

// key moves the corner, and reports whether the key was one of its own.
// h is the height of the grid's rows, without the header.
func (v *sheetView) key(k string, h int) bool {
	switch k {
	case "j", "down":
		v.row++
	case "k", "up":
		v.row--
	case "l", "right":
		v.col++
	case "h", "left":
		v.col--
	case "space", "pgdown":
		v.row += h
	case "b", "pgup":
		v.row -= h
	case "home", "g":
		v.row, v.col = 0, 0
	case "end", "G":
		v.row = v.rows()
	default:
		return false
	}
	v.clamp(h)
	return true
}

func (v *sheetView) view(w, h int) string {
	if v.rows() == 0 {
		return "(empty sheet)"
	}
	v.clamp(h - 1)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	gutter, n := v.gutter(), v.shown(w)
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
			line += " " + fit(cell, v.widths[c])
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
	n := v.shown(w)
	cols := fmt.Sprintf("col %s–%s/%s", document.ColumnName(v.col), document.ColumnName(v.col+n-1), document.ColumnName(len(v.widths)+v.sheet.MoreColumns-1))
	if n == 1 {
		cols = fmt.Sprintf("col %s/%s", document.ColumnName(v.col), document.ColumnName(len(v.widths)+v.sheet.MoreColumns-1))
	}
	out := rows + " · " + cols
	if v.sheet.MoreRows > 0 {
		out += fmt.Sprintf(" · first %s rows read", grouped(v.rows()))
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
