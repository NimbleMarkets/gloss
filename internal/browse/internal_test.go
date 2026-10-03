package browse

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

func TestSplit(t *testing.T) {
	for typed, want := range map[string][2]string{
		"":           {"", ""},
		"abc":        {"", "abc"},
		"a/":         {"a/", ""},
		"a/b":        {"a/", "b"},
		"/":          {"/", ""},
		"~/x/y/z":    {"~/x/y/", "z"},
		"../":        {"../", ""},
		"a//b":       {"a//", "b"},
		"/etc/hosts": {"/etc/", "hosts"},
	} {
		d, r := split(typed)
		if d != want[0] || r != want[1] {
			t.Errorf("split(%q) = %q, %q; want %q, %q", typed, d, r, want[0], want[1])
		}
	}
}

func TestResolve(t *testing.T) {
	m := New(browsetest.NewFS(), "/home/evan/projects", WithHome("/home/evan"))
	for typed, want := range map[string]string{
		"":          "/home/evan/projects",
		"./":        "/home/evan/projects",
		"../":       "/home/evan",
		"../../":    "/home",
		"../../..":  "/",
		"../../../": "/",
		"/etc/":     "/etc",
		"~":         "/home/evan",
		"~/":        "/home/evan",
		"~/docs/":   "/home/evan/docs",
		"sub/dir/":  "/home/evan/projects/sub/dir",
		"/a/../b/":  "/b",
		"~other/":   "/home/evan/projects/~other",
	} {
		if got := m.resolve(typed); got != want {
			t.Errorf("resolve(%q) = %q, want %q", typed, got, want)
		}
	}
	none := New(browsetest.NewFS(), "/x", WithHome(""))
	if got := none.resolve("~/"); got != "/x/~" {
		t.Errorf("with no home, ~/ is an ordinary name: %q", got)
	}
}

// A reading begun before a reload, arriving after it, must not stand in for
// what the reload reads.
func TestAStaleReadingIsLetGo(t *testing.T) {
	m := New(browsetest.NewFS("a/old.txt"), "/a")
	stale := m.load("/a")() // The reading begun before.
	m.Reload()
	if m.lists["/a"] == nil || m.lists["/a"].loaded {
		t.Fatalf("the reload did not begin a new reading: %+v", m.lists["/a"])
	}
	m, _ = m.Update(stale)
	if m.lists["/a"].loaded {
		t.Fatal("a stale reading was taken in after a reload")
	}
	fresh := m.lists["/a"]
	if fresh == nil || !fresh.loading {
		t.Fatalf("listing %+v", fresh)
	}
}

func TestACrumbThatIsAnEllipsisLeadsToTheFirstFolderLeftOut(t *testing.T) {
	m := New(browsetest.NewFS("a/b/c/d/e/f/g.txt"), "/a/b/c/d/e/f", WithHome("/zzz"))
	m.SetSize(14, 6)
	cs := m.crumbs()
	if cs[0].label != "…" {
		t.Fatalf("crumbs %+v", cs)
	}
	if first := cs[1]; cs[0].path != parentOf(first.path) || cs[0].child != first.label {
		t.Errorf("the ellipsis leads to %q (cursor on %q); the first shown is %+v", cs[0].path, cs[0].child, first)
	}
	// What is drawn never exceeds the width, whatever the width.
	for w := 1; w < 60; w++ {
		m.SetSize(w, 6)
		for i, c := range m.crumbs() {
			if c.x+c.w > w {
				t.Errorf("width %d: crumb %d %+v runs past", w, i, c)
			}
		}
	}
}

func TestCommonStart(t *testing.T) {
	entries := func(names ...string) []fs.DirEntry {
		var out []fs.DirEntry
		for _, n := range names {
			out = append(out, fs.FileInfoToDirEntry(infoOf(n)))
		}
		return out
	}
	for _, tc := range []struct {
		names []string
		fold  bool
		want  string
	}{
		{[]string{"readme.md", "readings.txt"}, true, "read"},
		{[]string{"Readme.md", "readings.txt"}, true, "Read"},
		{[]string{"Readme.md", "readings.txt"}, false, ""},
		{[]string{"one"}, true, "one"},
		{[]string{"café", "cafétéria"}, true, "café"},
	} {
		if got := commonStart(entries(tc.names...), tc.fold); got != tc.want {
			t.Errorf("commonStart(%v, fold=%v) = %q, want %q", tc.names, tc.fold, got, tc.want)
		}
	}
}

type fileInfo struct{ name string }

func infoOf(n string) fs.FileInfo { return fileInfo{n} }

func (f fileInfo) Name() string           { return f.name }
func (f fileInfo) Size() int64            { return 0 }
func (f fileInfo) Mode() fs.FileMode      { return 0o644 }
func (f fileInfo) ModTime() (t time.Time) { return }
func (f fileInfo) IsDir() bool            { return false }
func (f fileInfo) Sys() any               { return nil }

func TestLayoutNames(t *testing.T) {
	for l := Layout(0); l < layoutCount; l++ {
		got, ok := ParseLayout(strings.ToUpper(l.String()))
		if !ok || got != l {
			t.Errorf("ParseLayout(%v) = %v, %v", l, got, ok)
		}
	}
	if _, ok := ParseLayout("tree"); ok {
		t.Error("there is no tree layout")
	}
	if !reflect.DeepEqual(DefaultKeyMap().Open.Keys(), []string{"enter"}) {
		t.Error("Enter opens")
	}
}

func TestCleanNamesAreDrawnAsNames(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"plain.txt", "plain.txt"},
		{"世界.png", "世界.png"},
		{"café.pdf", "café.pdf"},
		{"café.pdf", "café.pdf"}, // Combining: the tables agree.
		{"bell\a.txt", "bell␇.txt"},
		{"tab\there", "tab␉here"},
		{"new\nline", "new␊line"},
		{"cr\r\nlf", "cr␍␊lf"},
		{"esc\x1b[31mred", "esc␛[31mred"},
		{"osc\x1b]0;pwned\a", "osc␛]0;pwned␇"},
		{"del\x7f", "del␡"},
		{"c1\u0085x", "c1�x"},
		{"👨‍👩‍👧 family", "👨 family"}, // Two cells to one table, six to the other.
		{"❤️ love", "❤ love"},        // An emoji selector: two cells or one.
		{"👍🏽 ok", "👍 ok"},            // A skin tone: two or four.
		{"🇯🇵 flag", "🇯🇵 flag"},       // Both agree.
	} {
		if got := cleanName(tc.name); got != tc.want {
			t.Errorf("cleanName(%q) = %q, want %q", tc.name, got, tc.want)
		}
		// Whatever it was, what is drawn is one width to both tables, and has no control character.
		got := cleanName(tc.name)
		if a, b := ansi.StringWidth(got), ansi.StringWidthWc(got); a != b {
			t.Errorf("cleanName(%q) = %q is %d cells to one table and %d to the other", tc.name, got, a, b)
		}
		if hasControl([]rune(got)) {
			t.Errorf("cleanName(%q) = %q still holds a control character", tc.name, got)
		}
	}
}

// The letters a filter matched are found again after the name is cleaned: a
// letter dropped shifts those after it.
func TestCleanRunesSaysWhereEachLetterWent(t *testing.T) {
	_, at := cleanRunes("a👨‍👩‍👧b")
	// a, then the cluster (man, ZWJ, woman, ZWJ, girl) of five letters, then b.
	want := []int{0, 1, -1, -1, -1, -1, 2}
	if !reflect.DeepEqual(at, want) {
		t.Errorf("at = %v, want %v", at, want)
	}
	m := New(browsetest.NewFS(), "/")
	r := row{entry: fs.FileInfoToDirEntry(infoOf("x")), name: "a👨‍👩‍👧b", at: []int{6}} // The b.
	shown := ansi.Strip(m.name(r, 20, lipgloss.NewStyle(), lipgloss.NewStyle().Underline(true)))
	if shown != "a👨b" {
		t.Errorf("shown %q", shown)
	}
	underlined := m.name(r, 20, lipgloss.NewStyle(), lipgloss.NewStyle().Underline(true))
	if !strings.HasSuffix(underlined, "b\x1b[m") && !strings.Contains(underlined, "4mb") {
		t.Errorf("the b should be the one marked: %q", underlined)
	}
}
