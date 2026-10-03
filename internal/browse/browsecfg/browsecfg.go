// Package browsecfg is how the browsetest harness builds and reads the
// chooser: its options by name, and the state a script may check. It is
// shared by the tests and the playground.
package browsecfg

import (
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/NimbleMarkets/gloss/internal/browse"
	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

// Options a script may set, with `option NAME VALUE`:
//
//	layout list|columns|places   how folders are laid out
//	hidden true                  show dot-files
//	home /home/evan              where ~ leads (default /home/evan)
//	marker kinds                 mark folders and pictures with one-cell glyphs
//	marker emoji                 mark files by kind with emoji, as gloss does (two cells wide)
//	only .png                    only that extension may be chosen
//	columns N                    at most N columns
//	dirsonly true                Tab completes folders alone (as in go-to)
//	linksfollowed true           the host has made links to folders folders
//	filters kinds                offer the kinds Pictures (.png .jpg), Documents
//	                             (.md .txt), and Data (.csv); "active NAME,NAME"
//	                             starts with some chosen
const Options = "layout, hidden, home, marker, only, columns, filters, active, dirsonly, linksfollowed"

// StateKeys are the Probe values F2 records in the playground.
var StateKeys = []string{"dir", "filter", "current", "chosen", "layout", "types"}

// Config is the harness's way of making and reading the chooser.
func Config() browsetest.Config[browse.Model] {
	return browsetest.Config[browse.Model]{
		Strict: true,
		New: func(fsys *browsetest.FS, start string, o map[string]string) browse.Model {
			opts := []browse.Option{browse.WithHome("/home/evan")}
			if v, ok := o["layout"]; ok {
				l, ok := browse.ParseLayout(v)
				if !ok {
					panic("unknown layout " + v)
				}
				opts = append(opts, browse.WithLayout(l))
			}
			if o["hidden"] == "true" {
				opts = append(opts, browse.WithShowHidden(true))
			}
			if v, ok := o["home"]; ok {
				opts = append(opts, browse.WithHome(v))
			}
			if o["marker"] == "kinds" {
				opts = append(opts, browse.WithMarker(func(e fs.DirEntry) string {
					switch {
					case e.IsDir():
						return "▪"
					case strings.HasSuffix(e.Name(), ".png"):
						return "▫"
					}
					return " "
				}))
			}
			if o["marker"] == "emoji" {
				opts = append(opts, browse.WithMarker(emojiMark))
			}
			if o["only"] != "" {
				ext := o["only"]
				opts = append(opts, browse.WithSelectable(func(e fs.DirEntry) bool { return path.Ext(e.Name()) == ext }))
			}
			if o["linksfollowed"] == "true" {
				opts = append(opts, browse.WithLinksFollowed())
			}
			if o["dirsonly"] == "true" {
				opts = append(opts, browse.WithDirsOnlyCompletion(true))
			}
			if v, ok := o["columns"]; ok {
				n, _ := strconv.Atoi(v)
				opts = append(opts, browse.WithMaxColumns(n))
			}
			if o["filters"] == "kinds" {
				kind := func(name, mark string, exts ...string) browse.Filter {
					return browse.Filter{Name: name, Mark: mark, Match: func(e fs.DirEntry) bool { return slices.Contains(exts, path.Ext(strings.ToLower(e.Name()))) }}
				}
				opts = append(opts, browse.WithFilters([]browse.Filter{
					kind("Pictures", "▫", ".png", ".jpg"), kind("Documents", "▪", ".md", ".txt"), kind("Data", "▤", ".csv"),
				}))
				if a := o["active"]; a != "" {
					opts = append(opts, browse.WithActiveFilters(strings.Split(a, ",")...))
				}
			}
			return browse.New(fsys, start, opts...)
		},
		Probe: func(m browse.Model) map[string]string {
			chosen, _ := m.Chosen()
			current := ""
			if e := m.Current(); e != nil {
				current = e.Name()
			}
			return map[string]string{
				"dir":     m.Dir(),
				"viewdir": m.ViewDir(),
				"filter":  m.FilterValue(),
				"chosen":  chosen,
				"current": current,
				"rows":    strconv.Itoa(m.Rows()),
				"layout":  m.Layout().String(),
				"types":   strings.Join(m.ActiveFilters(), ","),
				"menu":    strconv.FormatBool(m.InMenu()),
				"sidebar": strconv.FormatBool(m.InSidebar()),
				"crumbs":  strings.Join(m.Crumbs(), " > "),
			}
		},
	}
}

// emojiMarks are gloss's marks of the kinds of file, by extension, as
// internal/app draws them: the browse package has none of its own.
var emojiMarks = map[string]string{
	".png": "📷", ".jpg": "📷", ".jpeg": "📷", ".gif": "📷", ".webp": "📷", ".heic": "📷",
	".svg": "🎨", ".pdf": "📕", ".stl": "🧊", ".3mf": "🧊",
	".md": "📝", ".html": "📝", ".txt": "📝",
	".json": "🧾", ".ipynb": "📓", ".docx": "📄", ".xlsx": "📊", ".csv": "📊", ".grist": "📊",
}

// emojiMark is the mark of an entry: all of one width, two cells.
func emojiMark(e fs.DirEntry) string {
	if e.IsDir() {
		return "📁"
	}
	if m, ok := emojiMarks[strings.ToLower(path.Ext(e.Name()))]; ok {
		return m
	}
	return "  "
}
