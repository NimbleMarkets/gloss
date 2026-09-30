package document

import (
	"fmt"
	"strconv"
	"strings"
)

// ColumnFilter names the columns of a sheet to show, by header or letter,
// or by number. A filter that names nothing shows every column.
type ColumnFilter struct {
	Names   []string // Headers in the first row, or column letters.
	Indexes []int    // Counted from 1.
}

// ParseColumns reads the --cols and --coln arguments: names separated by
// commas, and numbers or ranges such as 1,3-5.
func ParseColumns(names, indexes string) (ColumnFilter, error) {
	var f ColumnFilter
	for _, name := range strings.Split(names, ",") {
		if name = strings.TrimSpace(name); name != "" {
			f.Names = append(f.Names, name)
		}
	}
	if strings.TrimSpace(indexes) == "" {
		return f, nil
	}
	for _, part := range strings.Split(indexes, ",") {
		first, last, found := strings.Cut(strings.TrimSpace(part), "-")
		from, err := strconv.Atoi(first)
		to := from
		if err == nil && found {
			to, err = strconv.Atoi(last)
		}
		if err != nil || from < 1 || to < from {
			return f, fmt.Errorf("--coln: %q is not a column number or range like 3-5", part)
		}
		for i := from; i <= to; i++ {
			f.Indexes = append(f.Indexes, i)
		}
	}
	return f, nil
}

func (f ColumnFilter) Empty() bool { return len(f.Names) == 0 && len(f.Indexes) == 0 }

// Hidden marks the columns of s the filter leaves out. Nothing is hidden
// when the filter is empty or matches no column of s, so a mismatched
// sheet is still shown whole.
func (f ColumnFilter) Hidden(s *Sheet) []bool {
	if f.Empty() || s.Columns == 0 {
		return nil
	}
	var header []string
	if len(s.Rows) > 0 {
		header = s.Rows[0]
	}
	shown := make([]bool, s.Columns)
	any := false
	for _, name := range f.Names {
		matched := false
		for c := range shown {
			if c < len(header) && strings.EqualFold(strings.TrimSpace(header[c]), name) {
				shown[c], matched = true, true
			}
		}
		if !matched {
			for c := range shown {
				if ColumnName(c) == strings.ToUpper(name) {
					shown[c], matched = true, true
				}
			}
		}
		any = any || matched
	}
	for _, i := range f.Indexes {
		if i >= 1 && i <= s.Columns {
			shown[i-1], any = true, true
		}
	}
	if !any {
		return nil
	}
	hidden := make([]bool, s.Columns)
	for c := range hidden {
		hidden[c] = !shown[c]
	}
	return hidden
}
