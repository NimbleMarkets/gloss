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
	f, err := ColumnFilter{Names: parseNames(names)}, error(nil)
	f.Indexes, err = parseIndexes("--coln", "column", indexes)
	return f, err
}

// parseNames splits a list of names on commas.
func parseNames(names string) []string {
	var out []string
	for _, name := range strings.Split(names, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// maxIndexes bounds what one list of numbers and ranges may name, so that a
// range like 1-3000000000 is refused rather than expanded. It is well above the
// columns of a sheet (16,384 in Excel) or the parts of a model.
const maxIndexes = 1 << 16

// parseIndexes reads numbers and ranges counted from 1, such as 1,3-5.
func parseIndexes(flag, what, indexes string) ([]int, error) {
	if strings.TrimSpace(indexes) == "" {
		return nil, nil
	}
	var out []int
	for _, part := range strings.Split(indexes, ",") {
		first, last, found := strings.Cut(strings.TrimSpace(part), "-")
		from, err := strconv.Atoi(first)
		to := from
		if err == nil && found {
			to, err = strconv.Atoi(last)
		}
		if err != nil || from < 1 || to < from {
			return nil, fmt.Errorf("%s: %q is not a %s number or range like 3-5", flag, part, what)
		}
		if to-from >= maxIndexes-len(out) {
			return nil, fmt.Errorf("%s: %q names more than %d numbers", flag, part, maxIndexes)
		}
		for i := from; i <= to; i++ {
			out = append(out, i)
		}
	}
	return out, nil
}

// PartFilter names the parts of a 3MF to show, by name or by number. A
// filter that names nothing shows every part.
type PartFilter struct {
	Names   []string
	Indexes []int // Counted from 1.
}

// ParseParts reads the --parts and --partn arguments.
func ParseParts(names, indexes string) (PartFilter, error) {
	f := PartFilter{Names: parseNames(names)}
	var err error
	f.Indexes, err = parseIndexes("--partn", "part", indexes)
	return f, err
}

func (f PartFilter) Empty() bool { return len(f.Names) == 0 && len(f.Indexes) == 0 }

// Shown marks the parts the filter keeps. Nothing is marked when the filter
// is empty or matches no part, so a model with none of them is shown whole.
func (f PartFilter) Shown(parts []Part) []bool {
	if f.Empty() || len(parts) == 0 {
		return nil
	}
	shown, any := make([]bool, len(parts)), false
	for i, part := range parts {
		for _, name := range f.Names {
			if strings.EqualFold(strings.TrimSpace(part.Name), name) {
				shown[i], any = true, true
			}
		}
	}
	for _, i := range f.Indexes {
		if i >= 1 && i <= len(parts) {
			shown[i-1], any = true, true
		}
	}
	if !any {
		return nil
	}
	return shown
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
