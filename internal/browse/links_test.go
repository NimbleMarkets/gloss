package browse_test

import (
	"testing"

	"github.com/NimbleMarkets/gloss/internal/browse"
	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

// quiet leaves out the places, which the chooser would describe too.
var quiet = []browse.Option{browse.WithHome("/h"), browse.WithPlaces(nil)}

func linksFS() *browsetest.FS {
	return browsetest.NewFS("d/real/", "d/a -> /d/real", "d/b -> /d/real", "d/f -> /d/file", "d/file 1", "d/gone -> /nowhere")
}

// A link is looked at once per listing, with Stat, which does not open it; and
// not at all by a chooser whose host has already done so.
func TestLinksAreDescribedOnceAndNeverOpened(t *testing.T) {
	f := linksFS()
	d := browsetest.New(t, browse.New(f, "/d", quiet...), 60, 8)
	if got := f.Stats(); len(got) != 4 { // a, b, f, gone.
		t.Errorf("the chooser described %d links, want 4: %v", len(got), got)
	}
	if got := f.Opens(); len(got) != 0 {
		t.Errorf("the chooser opened %v", got)
	}
	if !d.Screen().Contains("a/") || !d.Screen().Contains("b/") {
		t.Errorf("links to a folder are folders:\n%s", d.Screen())
	}

	f = linksFS()
	d = browsetest.New(t, browse.New(f, "/d", append(quiet, browse.WithLinksFollowed())...), 60, 8)
	if got := f.Stats(); len(got) != 0 {
		t.Errorf("a chooser told links are followed described %v", got)
	}
	if got := f.Opens(); len(got) != 0 {
		t.Errorf("the chooser opened %v", got)
	}
	_ = d
}

// FollowLinks, which a host that filters listings runs first, is the same: it
// describes, and opens nothing.
func TestFollowLinksDoesNotOpen(t *testing.T) {
	f := linksFS()
	entries, _ := f.ReadDir("d")
	out := browse.FollowLinks(f, "/d", entries)
	folders := 0
	for _, e := range out {
		if e.IsDir() {
			folders++
		}
	}
	if folders != 3 { // real, a, b.
		t.Errorf("%d folders, want 3", folders)
	}
	if len(f.Opens()) != 0 || len(f.Stats()) != 4 {
		t.Errorf("opens %v, stats %v", f.Opens(), f.Stats())
	}
}
