package document

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/examples"
)

func gristNotes(t testing.TB) []byte {
	t.Helper()
	data, err := examples.Files.ReadFile("notes.grist")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func plainDatabase(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "plain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGristIsKnownByItsCatalog(t *testing.T) {
	data := gristNotes(t)
	for _, name := range []string{"notes.grist", "NOTES.GRIST", "undecorated", "notes.db"} {
		if kind, err := Detect(name, data, ""); err != nil || kind != "grist" {
			t.Errorf("%s: kind=%q err=%v", name, kind, err)
		}
	}
	// The catalog may lie beyond the start of the file that Probe reads.
	dir := t.TempDir()
	for _, name := range []string{"notes.grist", "undecorated"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if kind, err := Probe(path, ""); err != nil || kind != "grist" {
			t.Errorf("Probe %s: kind=%q err=%v", name, kind, err)
		}
	}
	if isGrist(data[:2048]) {
		t.Fatal("the first pages alone hold the catalog: the fixture no longer tests Probe")
	}
	// Any file may be forced, and says what is wrong with it when read.
	if kind, err := Detect("words.txt", []byte("words"), "grist"); err != nil || kind != "grist" {
		t.Errorf("forced: kind=%q err=%v", kind, err)
	}
}

func TestOtherDatabasesAreNotOpened(t *testing.T) {
	plain := plainDatabase(t)
	_, err := Detect("words.sqlite", plain, "")
	if !errors.Is(err, ErrNotGrist) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Detect: %v", err)
	}
	if reason := SkipReason(err); !strings.Contains(reason, "not a Grist document") {
		t.Fatalf("reason: %q", reason)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "words.sqlite")
	if err := os.WriteFile(path, plain, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Probe(path, ""); !errors.Is(err, ErrNotGrist) {
		t.Fatalf("Probe: %v", err)
	}
	// Named for Grist, or forced, it is read, and refused for what it is.
	for _, q := range []Request{{Path: filepath.Join(dir, "words.grist")}, {Path: path, Type: "grist"}} {
		if err := os.WriteFile(q.Path, plain, 0600); err != nil {
			t.Fatal(err)
		}
		l := &Loader{}
		q.Page, q.Generation = 1, 1
		r := l.Load(q)
		l.Close()
		if !errors.Is(r.Err, ErrNotGrist) || r.Sheet != nil {
			t.Fatalf("%+v: err=%v", q, r.Err)
		}
		if len(r.Info) == 0 {
			t.Fatal("a database that cannot be shown is still described")
		}
	}
}

func TestBrokenGristFails(t *testing.T) {
	data := gristNotes(t)
	dir := t.TempDir()
	for name, content := range map[string][]byte{
		"header.grist":  data[:16],
		"short.grist":   data[:100],
		"cut.grist":     data[:len(data)/2],
		"text.grist":    []byte("not a database at all"),
		"garbage.grist": append(append([]byte{}, data[:100]...), make([]byte, 4000)...),
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
		l := &Loader{}
		r := l.Load(Request{Path: path, Page: 1, Generation: 1})
		// Half a document may still hold the first table whole; what is cut
		// off must fail rather than read as empty.
		for page := 2; r.Err == nil && page <= r.Pages; page++ {
			r = l.Load(Request{Path: path, Page: page, Generation: uint64(page)})
		}
		l.Close()
		if r.Err == nil {
			t.Errorf("%s: read without complaint", name)
		}
	}
}

func TestGristTablesAreSheets(t *testing.T) {
	l := &Loader{Files: examples.Files}
	defer l.Close()
	r := l.Load(Request{Path: "notes.grist", Page: 1, Generation: 1})
	if r.Err != nil || r.Kind != "grist" || r.Page != 1 || r.Pages != 4 {
		t.Fatalf("kind=%q page=%d/%d err=%v", r.Kind, r.Page, r.Pages, r.Err)
	}
	// The labels head the columns, in Grist's order: Site was moved to stand
	// second. Row ids, sort positions, and helper columns are not shown.
	want := [][]string{
		{"Species", "Site", "How many", "Weight (kg)", "Confirmed?", "Seen on", "Logged at", "Tags", "Source", "Count per kg", "Photos"},
		// The otter was dragged above the heron: manualSort, not id, orders rows.
		{"Otter", "Alder Creek", "1", "8", "TRUE", "2026-03-09", "2026-03-09 01:33:20", "mammal", "", "0.125", ""},
		{"Grey heron", "Reed Marsh", "2", "1.8", "TRUE", "2026-03-07", "2026-03-07 11:05", "wading, dawn", "Heron notes", "1.25", "heron.png"},
		// A date Grist could not read stands as written, no site is no
		// text, and an error is named.
		{"Emperor dragonfly", "", "14", "0", "FALSE", "early March", "", "", "", "#ZeroDivisionError", ""},
		{"Kingfisher", "Reed Marsh", "1", "0.04", "TRUE", "2026-03-14", "2026-03-14 09:20", "", "https://example.com/kingfisher", "25", ""},
	}
	if r.Sheet.Name != "Field sightings" || r.Sheet.Columns != 11 || !reflect.DeepEqual(r.Sheet.Rows, want) {
		t.Fatalf("%s:\n%q", r.Sheet.Name, r.Sheet.Rows)
	}
	// A hyperlink shows its words and leads to its address, as a workbook's
	// does; one that is all address stands as it is.
	for cell, want := range map[[2]int]string{{2, 8}: "https://example.com/heron", {4, 8}: "https://example.com/kingfisher", {1, 8}: "", {2, 0}: ""} {
		if got := r.Sheet.URL(cell[0], cell[1]); got != want {
			t.Errorf("address at %v: %q, want %q", cell, got, want)
		}
	}
	for text, want := range map[string][2]string{
		"33Across https://example.com/33across": {"33Across", "https://example.com/33across"},
		"two words https://example.com/":        {"two words", "https://example.com/"},
		"https://example.com/alone":             {},
		"write to mailto:someone@example.com":   {},
		"words that end in https://":            {},
		"no address here":                       {},
		"":                                      {},
	} {
		words, address, ok := gristLink(text)
		if words != want[0] || address != want[1] || ok != (want[1] != "") {
			t.Errorf("%q: %q %q %v", text, words, address, ok)
		}
	}
	// A table without a title goes by its name.
	r = l.Load(Request{Path: "notes.grist", Page: 2, Generation: 2})
	if r.Err != nil || r.Page != 2 || r.Sheet.Name != "Sites" || !reflect.DeepEqual(r.Sheet.Rows, [][]string{{"Site name", "Region", "Elevation (m)"}, {"Reed Marsh", "Lowland", "12"}, {"Alder Creek", "Upland", "340"}}) {
		t.Fatalf("%+v err=%v", r.Sheet, r.Err)
	}
	// An empty table is its headings.
	r = l.Load(Request{Path: "notes.grist", Page: 3, Generation: 3})
	if r.Err != nil || r.Sheet.Name != "Ideas for next season" || !reflect.DeepEqual(r.Sheet.Rows, [][]string{{"Idea", "Who"}}) {
		t.Fatalf("%+v err=%v", r.Sheet, r.Err)
	}
	// Summaries come last, without the rows each sums, titled as Grist
	// titles them. What they group by shows as the summed table shows it:
	// the site's name, not the row it points at.
	r = l.Load(Request{Path: "notes.grist", Page: 9, Generation: 4})
	if r.Err != nil || r.Page != 4 || r.Sheet.Name != "Field sightings [by Site]" || !reflect.DeepEqual(r.Sheet.Rows, [][]string{{"Site", "count"}, {"Reed Marsh", "2"}, {"Alder Creek", "1"}, {"", "1"}}) {
		t.Fatalf("%+v err=%v", r.Sheet, r.Err)
	}
	if csv := string(r.Sheet.CSV()); csv != "Site,count\nReed Marsh,2\nAlder Creek,1\n,1\n" {
		t.Fatalf("csv: %q", csv)
	}
}

func TestGristInfo(t *testing.T) {
	l := &Loader{Files: examples.Files}
	defer l.Close()
	r := l.Load(Request{Path: "notes.grist", Page: 2, Generation: 1})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	got := map[string]string{}
	for _, f := range r.Info {
		got[f.Label] = f.Value
	}
	for label, want := range map[string]string{
		"Format":         "Grist document",
		"Tables":         "Field sightings (4 rows × 11 columns), Sites (2 rows × 3 columns), Ideas for next season (0 rows × 2 columns), Field sightings [by Site] (3 rows × 2 columns)",
		"Time zone":      "UTC",
		"Schema version": "46",
		"Not shown":      "row ids, sort positions, and Grist's helper columns",
	} {
		if got[label] != want {
			t.Errorf("%s: %q", label, got[label])
		}
	}
	if _, ok := got["Size"]; !ok {
		t.Errorf("no file fields: %v", r.Info)
	}
}

func TestGristCells(t *testing.T) {
	g := &GristDoc{zone: "UTC", files: map[int64]string{7: "map.png"}}
	blob := func(s string) []byte {
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, c := range []struct {
		kind string
		v    any
		want string
	}{
		{"Text", nil, ""},
		{"Text", "two\nlines\tof it", "two lines of it"},
		{"Int", int64(42), "42"},
		{"Numeric", 17.25, "17.25"},
		{"Numeric", "Not-a-Number", "Not-a-Number"},
		{"Bool", int64(1), "TRUE"},
		{"Bool", int64(0), "FALSE"},
		{"Bool", int64(7), "7"},
		{"Date", int64(1593561600), "2020-07-01"},
		{"Date", "Not-a-Date", "Not-a-Date"},
		{"DateTime:UTC", 1598030380.705, "2020-08-21 17:19:40"},
		{"DateTime:UTC", int64(1772881500), "2026-03-07 11:05"},
		{"DateTime", int64(1772881500), "2026-03-07 11:05"},
		{"DateTime:Nowhere/Known", int64(1772881500), "2026-03-07 11:05 UTC"},
		{"Ref:Sites", int64(0), ""},
		{"Ref:Sites", int64(3), "Sites[3]"},
		{"Ref:Sites", "No-Ref", "No-Ref"},
		{"RefList:Sites", "[2,24]", "Sites[2], Sites[24]"},
		{"RefList:Sites", "No-RefList", "No-RefList"},
		{"ChoiceList", `["Two","Three"]`, "Two, Three"},
		{"ChoiceList", "[]", ""},
		{"Attachments", "[7,8]", "map.png, attachment 8"},
		{"Text", `["not","a","list","column"]`, `["not","a","list","column"]`},
		// What Grist marshals: an error, a list, a date, a time, a reference, a
		// dictionary, a value still being worked out, and a number too long
		// for SQLite.
		{"Any", blob("5B0200000075010000004575110000005A65726F4469766973696F6E4572726F72"), "#ZeroDivisionError"},
		{"Any", blob("5B0300000075010000004C75040000007265656475050000007365646765"), "reed, sedge"},
		{"Any", blob("5B0200000075010000006469806AAB69"), "2026-03-07"},
		{"Any", blob("5B0300000075010000004469A04EAC697503000000555443"), "2026-03-07 16:13:20"},
		{"Any", blob("5B03000000750100000052750500000053697465736902000000"), "Sites[2]"},
		{"Any", blob("5B0200000075010000004F7B75010000006167000000000000F83F7501000000624E7501000000635430"), "{a: 1.5, b: , c: TRUE}"},
		{"Any", blob("5B01000000750100000050"), "#PENDING"},
		{"Any", blob("6C03000000000000000004"), "1099511627776"},
		{"Any", blob("69FDFFFFFF"), "-3"},
		// What is not marshalled, or is cut short, is not guessed at.
		{"Any", []byte{0xff, 0x00, 0x01}, "[blob 3 bytes]"},
		{"Any", blob("5B02000000750100000045"), "[blob 11 bytes]"},
		{"Any", blob("5BFFFFFF7F"), "[blob 5 bytes]"},
		{"Any", []byte("0"), "[blob 1 bytes]"},
	} {
		if got := g.cell(c.kind, c.v); got != c.want {
			t.Errorf("%s %v: %q, want %q", c.kind, c.v, got, c.want)
		}
	}
}

func FuzzPythonMarshal(f *testing.F) {
	for _, seed := range []string{"5B0200000075010000004575110000005A65726F4469766973696F6E4572726F72", "5B0200000075010000004F7B75010000006167000000000000F83F7501000000624E7501000000635430", "6C03000000000000000004"} {
		b, _ := hex.DecodeString(seed)
		f.Add(b)
	}
	g := &GristDoc{}
	f.Fuzz(func(t *testing.T, data []byte) {
		if v, err := unmarshalPython(data); err == nil {
			g.object(v)
		}
	})
}

func FuzzGrist(f *testing.F) {
	f.Add(gristNotes(f))
	f.Add(plainDatabase(f))
	f.Fuzz(func(t *testing.T, data []byte) {
		g, err := OpenGrist(data)
		if err != nil {
			return
		}
		for i := range g.Names() {
			g.Sheet(i)
		}
	})
}
