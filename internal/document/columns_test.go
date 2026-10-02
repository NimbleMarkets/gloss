package document

import (
	"slices"
	"testing"
)

func TestColumnFilter(t *testing.T) {
	sheet := &Sheet{Columns: 4, Rows: [][]string{{"Name", " Age", "City", "Note"}, {"a", "1", "x", "y"}}}
	for _, tt := range []struct {
		names, indexes string
		hidden         []bool
	}{
		{names: "name, CITY", hidden: []bool{false, true, false, true}},
		{indexes: "1,3-4", hidden: []bool{false, true, false, false}},
		{names: "D", hidden: []bool{true, true, true, false}},
		{names: "age", indexes: "4", hidden: []bool{true, false, true, false}},
		{names: "nope", hidden: nil},
		{indexes: "9", hidden: nil},
		{hidden: nil},
	} {
		f, err := ParseColumns(tt.names, tt.indexes)
		if err != nil {
			t.Fatalf("%q %q: %v", tt.names, tt.indexes, err)
		}
		if got := f.Hidden(sheet); !slices.Equal(got, tt.hidden) {
			t.Errorf("%q %q: hidden %v, want %v", tt.names, tt.indexes, got, tt.hidden)
		}
		if f.Empty() != (tt.names == "" && tt.indexes == "") {
			t.Errorf("%q %q: empty %v", tt.names, tt.indexes, f.Empty())
		}
	}
	// A range is not expanded past what a sheet or a model could have.
	for _, bad := range []string{"0", "a", "3-1", "1,", "-2", "17-3333333370", "1-65537", "1-40000,1-40000"} {
		if _, err := ParseColumns("", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseIndexesLimit(t *testing.T) {
	if got, err := ParseColumns("", "1-65536"); err != nil || len(got.Indexes) != maxIndexes {
		t.Errorf("a range of exactly %d: %d, %v", maxIndexes, len(got.Indexes), err)
	}
}
