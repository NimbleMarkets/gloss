package browse

import (
	"path"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// A crumb is one folder of the path shown above the listing.
type crumb struct {
	label string
	path  string // Where it leads.
	child string // The name beneath it on the way down, for the cursor to land on.
	x, w  int    // Where it is drawn: the column, and the width in cells.
}

const crumbSep = " › "

// crumbs lays out the path of the folder shown as a row of folders: home is
// "~", and when the path is too long the start is left out, a crumb that is
// an ellipsis leading to the nearest folder left out.
func (m Model) crumbs() []crumb {
	dir := m.viewDir()
	var all []crumb
	root, rest := "/", dir
	label := "/"
	if m.home != "" && m.home != "/" && (dir == m.home || strings.HasPrefix(dir, m.home+"/")) {
		root, rest, label = m.home, strings.TrimPrefix(dir, m.home), "~"
	}
	all = append(all, crumb{label: label, path: root})
	at := root
	for _, seg := range strings.Split(strings.Trim(rest, "/"), "/") {
		if seg == "" {
			continue
		}
		at = path.Join(at, seg)
		all = append(all, crumb{label: cleanName(seg), path: at})
	}
	for i := range all[:len(all)-1] {
		all[i].child = path.Base(all[i+1].path)
	}
	avail := max(1, m.width-1)
	width := func(cs []crumb) int {
		w := 0
		for i, c := range cs {
			if i > 0 {
				w += ansi.StringWidth(crumbSep)
			}
			w += ansi.StringWidth(c.label)
		}
		return w
	}
	shown := all
	if width(shown) > avail {
		// Drop from the start until what is left, and an ellipsis, fits.
		for cut := 1; cut < len(all); cut++ {
			shown = append([]crumb{{label: "…", path: parentOf(all[cut].path), child: path.Base(all[cut].path)}}, all[cut:]...)
			if width(shown) <= avail {
				break
			}
		}
		if width(shown) > avail {
			// Not even the last name and an ellipsis fit: the name alone, cut.
			last := shown[len(shown)-1]
			last.label = ansi.TruncateLeft(last.label, max(0, ansi.StringWidth(last.label)-avail+1), "…")
			shown = []crumb{{label: last.label, path: last.path}}
		}
	}
	x := 1
	for i := range shown {
		if i > 0 {
			x += ansi.StringWidth(crumbSep)
		}
		shown[i].x, shown[i].w = x, ansi.StringWidth(shown[i].label)
		x += shown[i].w
	}
	return shown
}

// Crumbs are the labels of the breadcrumbs as drawn, for tests and hosts.
func (m Model) Crumbs() []string {
	var out []string
	for _, c := range m.crumbs() {
		out = append(out, c.label)
	}
	return out
}

// crumbsView draws the breadcrumbs: the folders above dim, the one shown bright.
func (m Model) crumbsView() string {
	cs := m.crumbs()
	var b strings.Builder
	b.WriteString(" ")
	for i, c := range cs {
		if i > 0 {
			b.WriteString(m.styles.CrumbSep.Render(crumbSep))
		}
		if i == len(cs)-1 {
			b.WriteString(m.styles.CrumbCurrent.Render(c.label))
		} else {
			b.WriteString(m.styles.Crumb.Render(c.label))
		}
	}
	return b.String()
}

// crumbAt is the crumb drawn at a column of the top row.
func (m Model) crumbAt(x int) (crumb, bool) {
	for _, c := range m.crumbs() {
		if x >= c.x && x < c.x+c.w {
			return c, true
		}
	}
	return crumb{}, false
}
