package document

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"a.png", "b.PNG", "notes.md", "deep/c.png", "deep/deeper/d.pdf", "deep/deeper/still/e.png", ".hidden/f.png", "report-2026.csv", "report-2025.csv"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A link out of the folder is not followed.
	if err := os.Symlink(os.TempDir(), filepath.Join(dir, "elsewhere")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFindMatchesGlobsKindsAndExtensions(t *testing.T) {
	dir := tree(t)
	for _, tt := range []struct {
		patterns []string
		want     []string
	}{
		{[]string{"*.png"}, []string{"a.png", "b.PNG", "deep/c.png", "deep/deeper/still/e.png"}},
		{[]string{"png"}, []string{"a.png", "b.PNG", "deep/c.png", "deep/deeper/still/e.png"}},
		{[]string{".png"}, []string{"a.png", "b.PNG", "deep/c.png", "deep/deeper/still/e.png"}},
		{[]string{"report*"}, []string{"report-2025.csv", "report-2026.csv"}},
		{[]string{"deep/*.png"}, []string{"deep/c.png"}},
		{[]string{"images"}, []string{"a.png", "b.PNG", "deep/c.png", "deep/deeper/still/e.png"}},
		{[]string{"tables", "*.md"}, []string{"notes.md", "report-2025.csv", "report-2026.csv"}},
		{[]string{"docs"}, []string{"deep/deeper/d.pdf"}},
		{[]string{"nothing-here"}, nil},
	} {
		found, err := Find(dir, tt.patterns, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, m := range found.Paths {
			rel, _ := filepath.Rel(dir, m)
			got = append(got, filepath.ToSlash(rel))
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%v: %v, want %v", tt.patterns, got, tt.want)
		}
	}
	if _, err := Find(dir, []string{"["}, Limits{}); err == nil {
		t.Error("a bad glob was accepted")
	}
	if _, err := Find(dir, nil, Limits{}); err == nil {
		t.Error("no pattern was accepted")
	}
}

func TestFindKeepsToItsLimits(t *testing.T) {
	dir := tree(t)
	found, _ := Find(dir, []string{"*.png"}, Limits{Depth: 1})
	if len(found.Paths) != 3 || !strings.Contains(found.Note, "deeper than 1") {
		t.Errorf("depth: %v %q", found.Paths, found.Note)
	}
	found, _ = Find(dir, []string{"*"}, Limits{Matches: 2})
	if len(found.Paths) != 2 || !strings.Contains(found.Note, "first 2") {
		t.Errorf("matches: %v %q", found.Paths, found.Note)
	}
	found, _ = Find(dir, []string{"*"}, Limits{Entries: 3})
	if len(found.Paths) > 3 || !strings.Contains(found.Note, "3 entries") {
		t.Errorf("entries: %v %q", found.Paths, found.Note)
	}
	found, _ = Find(dir, []string{"*"}, Limits{Time: time.Nanosecond})
	if !strings.Contains(found.Note, "time") {
		t.Errorf("time: %q", found.Note)
	}
	// The hidden folder and the link out are never entered.
	found, _ = Find(dir, []string{"f.png", "*"}, Limits{})
	for _, p := range found.Paths {
		if strings.Contains(p, ".hidden") || strings.Contains(p, "elsewhere") {
			t.Errorf("entered %s", p)
		}
	}
}
