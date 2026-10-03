package browse

import (
	"reflect"
	"slices"
	"testing"
)

func TestMatchRanking(t *testing.T) {
	for _, tc := range []struct {
		query string
		names []string // best first
	}{
		{"re", []string{"readme.md", "report.pdf", "projects"}},
		{"png", []string{"png", "png-notes.md", "photo.png", "p-n-g.txt"}},
		{".png", []string{".png", "photo.png", "my.pdf.png"}},
		{"gl", []string{"gloss", "global.go", "a-gl"}},
		{"fb", []string{"foo-bar.txt", "fabulous"}},
		{"fb", []string{"fooBar.txt", "fabulous"}},
	} {
		q := parseQuery(tc.query)
		prev := 1 << 30
		for _, n := range tc.names {
			score, _, ok := q.match(n)
			if !ok {
				t.Errorf("%q should match %q", tc.query, n)
				continue
			}
			if score >= prev {
				t.Errorf("query %q: %q (%d) should rank below the one before it (%d); order %v", tc.query, n, score, prev, tc.names)
			}
			prev = score
		}
	}
}

func TestMatchRejects(t *testing.T) {
	for _, tc := range []struct{ query, name string }{
		{"xyz", "readme.md"},
		{"mr", "readme.md"}, // Out of order: m comes after r... but r precedes m, so "mr" does not fit.
		{"readmee", "readme"},
		{"Re", "readme.md"}, // A capital means case counts.
		{"*.png", "photo.jpg"},
	} {
		if _, _, ok := parseQuery(tc.query).match(tc.name); ok {
			t.Errorf("%q should not match %q", tc.query, tc.name)
		}
	}
}

func TestMatchPositions(t *testing.T) {
	for _, tc := range []struct {
		query, name string
		at          []int
	}{
		{"re", "readme.md", []int{0, 1}},
		{"md", "readme.md", []int{7, 8}},
		{"rmd", "readme.md", []int{0, 7, 8}},
		{"bar", "foo-bar.txt", []int{4, 5, 6}},
		{"é", "café", []int{3}},
		{"Ab", "xAbab", []int{1, 2}},
	} {
		_, at, ok := parseQuery(tc.query).match(tc.name)
		if !ok || !reflect.DeepEqual(at, tc.at) {
			t.Errorf("%q in %q matched at %v (%v), want %v", tc.query, tc.name, at, ok, tc.at)
		}
	}
}

func TestMatchSmartCase(t *testing.T) {
	if _, _, ok := parseQuery("readme").match("README.md"); !ok {
		t.Error("a lowercase query ignores case")
	}
	if _, _, ok := parseQuery("README").match("readme.md"); ok {
		t.Error("a capital makes the match exact")
	}
	if _, _, ok := parseQuery("README").match("README.md"); !ok {
		t.Error("exact case should match")
	}
}

func TestMatchGlob(t *testing.T) {
	for _, tc := range []struct {
		query, name string
		want        bool
	}{
		{"*.png", "photo.PNG", true},
		{"*.png", "photo.png.bak", false},
		{"img_??.jpg", "img_01.jpg", true},
		{"img_??.jpg", "img_1.jpg", false},
		{"[ab]*", "beta", true},
		{"[ab", "beta", false}, // Not a glob: a bad pattern is typed letters.
		{"*", "anything", true},
	} {
		_, _, ok := parseQuery(tc.query).match(tc.name)
		if ok != tc.want {
			t.Errorf("%q against %q = %v, want %v", tc.query, tc.name, ok, tc.want)
		}
	}
}

func TestMatchEmpty(t *testing.T) {
	if s, at, ok := parseQuery("").match("anything"); !ok || s != 0 || at != nil {
		t.Errorf("empty query = %d %v %v", s, at, ok)
	}
}

func TestMatchSorts(t *testing.T) {
	names := []string{"projects", "readme.md", "report.pdf", "notes.md", "tree"}
	q := parseQuery("re")
	type hit struct {
		name  string
		score int
	}
	var hits []hit
	for _, n := range names {
		if s, _, ok := q.match(n); ok {
			hits = append(hits, hit{n, s})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return b.score - a.score })
	var got []string
	for _, h := range hits {
		got = append(got, h.name)
	}
	if want := []string{"readme.md", "report.pdf", "tree", "projects"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ranked %v, want %v", got, want)
	}
}
