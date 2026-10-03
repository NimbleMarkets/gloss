package browse_test

import (
	"fmt"
	"github.com/NimbleMarkets/gloss/internal/browse/browsecfg"
	"strings"
	"testing"
	"time"

	"github.com/NimbleMarkets/gloss/internal/browse"
	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

// awkward are names that break layouts: wide, joined, selected, combined,
// long, and carrying bytes that are not text.
var awkward = []string{
	"plain.txt", "世界の写真.png", "📷 holiday 🏖️.png", "👨‍👩‍👧 family.md", "❤️ love ✔️ notes.txt",
	"café résumé.pdf", "مرحبا.txt", strings.Repeat("long-name-", 12) + ".csv",
	"bell\a.txt", "tab\there.md", "carriage\rreturn.txt", "new\nline.txt", "esc\x1b[31mred.txt", "osc\x1b]0;pwned\a.txt",
}

func awkwardFS() *browsetest.FS {
	f := browsetest.NewFS("home/evan/projects/gloss/internal/browse/deep/")
	for _, n := range awkward {
		f.Add(fmt.Sprintf("%q 100", "home/evan/files/"+n))
		f.Add(fmt.Sprintf("%q", "home/evan/files/dir of "+n+"/"))
	}
	return f
}

// Whatever the size, the layout, the names, or the popups, the screen is
// drawn soundly: no row wider than the terminal by either width table, no
// more rows than there are, no control character. These are the faults that
// mess up a terminal.
func TestNoSizeOrNameMakesAnUnsoundScreen(t *testing.T) {
	cfg := browsecfg.Config()
	widths := []int{1, 2, 3, 4, 5, 6, 8, 10, 14, 19, 20, 21, 22, 23, 24, 30, 41, 48, 59, 60, 61, 70, 80, 95, 120, 200}
	heights := []int{1, 2, 3, 4, 5, 6, 9, 14, 30}
	for _, layout := range []string{"list", "columns", "places"} {
		for _, marker := range []string{"none", "emoji"} {
			t.Run(layout+"/"+marker, func(t *testing.T) {
				fsys := awkwardFS()
				for _, w := range widths {
					for _, h := range heights {
						d := browsetest.New(t, cfg.New(fsys, "/home/evan/files", map[string]string{"layout": layout, "marker": marker, "filters": "kinds", "hidden": "true"}), w, h)
						d.StrictFit = true
						for step, keys := range [][]string{nil, {"down", "down", "down"}, {"ctrl+f"}, {"down", "space", "esc"}, {"ctrl+g"}, {"esc", "tab", "tab"}, {"ctrl+u", "e", "tab"}, {"enter"}, {"right", "left"}} {
							d.Press(keys...)
							if problems := d.Screen().Problems(); len(problems) > 0 {
								t.Fatalf("%dx%d after %v (step %d): %s\n%s", w, h, d.Trail(), step, strings.Join(problems, "; "), d.Screen())
							}
						}
					}
				}
			})
		}
	}
	_ = browse.LayoutList
}

// Reading the folders again (as toggling what is shown does) leaves the cursor
// on the same name once they are read, not on the first row.
func TestReloadKeepsTheCursorOnItsName(t *testing.T) {
	f := browsetest.NewFS("home/evan/a", "home/evan/b", "home/evan/c", "home/evan/d")
	f.Latency(20 * time.Millisecond)
	d := browsetest.New(t, browsecfg.Config().New(f, "/home/evan", nil), 60, 9)
	d.Press("down", "down")
	if cur := browsecfg.Config().Probe(d.M)["current"]; cur != "c" {
		t.Fatalf("setup: the cursor is on %q", cur)
	}
	d.Run(d.M.Reload())
	if cur := browsecfg.Config().Probe(d.M)["current"]; cur != "c" {
		t.Errorf("after Reload the cursor is on %q, want c\n%s", cur, d.Screen())
	}
}

// A popup open is a popup in sight: whatever the size it was opened at and the
// size the terminal becomes, a menu or a sidebar that has the cursor can be
// seen. Otherwise keys act on what nobody can see.
func TestAPopupHasTheCursorOnlyWhileItIsDrawn(t *testing.T) {
	cfg := browsecfg.Config()
	sizes := [][2]int{{200, 40}, {90, 12}, {70, 9}, {60, 8}, {59, 7}, {40, 6}, {30, 5}, {20, 4}, {10, 3}}
	opens := map[string][]string{"menu": {"ctrl+f"}, "sidebar": {"ctrl+g"}}
	for name, keys := range opens {
		for _, from := range sizes {
			for _, to := range sizes {
				fsys := awkwardFS()
				d := browsetest.New(t, cfg.New(fsys, "/home/evan/files", map[string]string{"layout": "places", "filters": "kinds"}), from[0], from[1])
				d.StrictFit = true
				check := func(when string) {
					t.Helper()
					p, scr := cfg.Probe(d.M), d.Screen()
					if p["menu"] == "true" && !scr.Contains("File types") {
						t.Fatalf("%s %v→%v %s: the menu has the keys and is not drawn\n%s", name, from, to, when, scr)
					}
					if p["sidebar"] == "true" && !scr.Contains("Places") {
						t.Fatalf("%s %v→%v %s: the sidebar has the keys and is not drawn\n%s", name, from, to, when, scr)
					}
				}
				d.Press(keys...)
				check("opened")
				d.Resize(to[0], to[1])
				check("resized")
				d.Press("down", "down")
				check("after keys")
			}
		}
	}
}
