package document

import (
	"strings"
	"testing"
)

func TestCSVIsASheet(t *testing.T) {
	sheet, err := ReadCSV("sales.csv", []byte("\ufeffName,Count,Note\napples,3,\"a, quoted\"\npears,,\"two\nlines\"\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := rowsOf(sheet); got != "Name|Count|Note\napples|3|a, quoted\npears||two lines" {
		t.Fatalf("rows:\n%s", got)
	}
	if sheet.Name != "sales" || sheet.Columns != 3 || sheet.Delimiter != "comma" {
		t.Fatalf("%+v", sheet)
	}
}

func TestCSVDelimiters(t *testing.T) {
	for name, tt := range map[string]struct {
		text, delimiter, rows string
	}{
		"tabs":                 {"a\tb\tc\n1\t2\t3\n", "tab", "a|b|c\n1|2|3"},
		"semicolons":           {"a;b;c\n1;2;3\n", "semicolon", "a|b|c\n1|2|3"},
		"pipes":                {"a|b|c\n1|2|3\n", "pipe", "a|b|c\n1|2|3"},
		"commas within":        {"a;b;c\n\"1,5\";\"2,5\";3\n", "semicolon", "a|b|c\n1,5|2,5|3"},
		"one column":           {"alone\nby itself\n", "comma", "alone\nby itself"},
		"ragged rows":          {"a,b,c\n1\n2,3,4,5\n", "comma", "a|b|c\n1\n2|3|4|5"},
		"windows line ends":    {"a,b\r\n1,2\r\n", "comma", "a|b\n1|2"},
		"a lone quote":         {"a,b\nit's,fine\n", "comma", "a|b\nit's|fine"},
		"not quite UTF-8":      {"a,b\ncaf\xe9,x\n", "comma", "a|b\ncaf�|x"},
		"a terminal escape":    {"a,b\n\x1b[31mred,x\n", "comma", "a|b\n�[31mred|x"},
		"a tab-separated .tsv": {"a,b\tc\n1,2\t3\n", "tab", "a,b|c\n1,2|3"},
	} {
		sheet, err := ReadCSV(map[bool]string{true: "x.tsv", false: "x.csv"}[name == "a tab-separated .tsv"], []byte(tt.text))
		if err != nil || sheet.Delimiter != tt.delimiter || rowsOf(sheet) != tt.rows {
			t.Errorf("%s: delimiter %q rows %q err %v", name, sheet.Delimiter, rowsOf(sheet), err)
		}
	}
}

func TestCSVLimits(t *testing.T) {
	var b strings.Builder
	for range maxSheetRows + 4 {
		b.WriteString("1,2\n")
	}
	sheet, err := ReadCSV("long.csv", []byte(b.String()))
	if err != nil || len(sheet.Rows) != maxSheetRows || sheet.MoreRows != 4 {
		t.Fatalf("rows=%d more=%d err=%v", len(sheet.Rows), sheet.MoreRows, err)
	}
	wide := strings.Repeat("x,", maxSheetColumns+2) + "x\n"
	sheet, err = ReadCSV("wide.csv", []byte(wide))
	if err != nil || sheet.Columns != maxSheetColumns || sheet.MoreColumns != 3 {
		t.Fatalf("columns=%d more=%d err=%v", sheet.Columns, sheet.MoreColumns, err)
	}
	if _, err := ReadCSV("empty.csv", nil); err == nil {
		t.Fatal("an empty file is not a table")
	}
	if _, err := ReadCSV("blank.csv", []byte("\n\n")); err == nil {
		t.Fatal("blank lines are not a table")
	}
}

func TestCSVIsDetectedByName(t *testing.T) {
	data := []byte("a,b\n1,2\n")
	for name, want := range map[string]string{"t.csv": "csv", "t.tsv": "csv", "t.CSV": "csv"} {
		if kind, err := Detect(name, data, ""); err != nil || kind != want {
			t.Errorf("Detect(%s) = %q, %v", name, kind, err)
		}
	}
	// Text alone does not make a table: a .txt of commas is plain text.
	if kind, err := Detect("t.txt", data, ""); err != nil || kind != "text" {
		t.Errorf("Detect(t.txt) = %q, %v", kind, err)
	}
	l := &Loader{}
	defer l.Close()
	r := l.Load(Request{Path: write(t, "t.tsv", []byte("a\tb\n1\t2\n")), Page: 1, DPI: 72, Generation: 1})
	if r.Err != nil || r.Kind != "csv" || r.Sheet == nil || r.Pages != 1 || r.Sheet.Name != "t" {
		t.Fatalf("err=%v kind=%q sheet=%+v", r.Err, r.Kind, r.Sheet)
	}
	expect(t, r.Info, map[string]string{"Format": "Tab-separated values", "Rows": "2", "Columns": "2"})
}
