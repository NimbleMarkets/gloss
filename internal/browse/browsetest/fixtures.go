package browsetest

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// DemoLines are a folder tree to try a chooser on when no script is given:
// a home with the usual folders, a project, hidden files, and a few folders
// that are awkward (a long name, many files).
func DemoLines() []string {
	lines := []string{
		"home/evan/Desktop/",
		"home/evan/Documents/report.pdf 120400 @2026-09-12",
		"home/evan/Documents/taxes/2025.pdf 88000 @2026-04-01",
		"home/evan/Documents/taxes/2024.pdf 91000 @2025-04-02",
		"home/evan/Documents/notes.md 2300 @2026-10-01",
		"home/evan/Downloads/setup.dmg 52000000 @2026-09-30",
		`"home/evan/Downloads/invoice (final) v2.pdf" 31000 @2026-09-29`,
		"home/evan/Pictures/trips/beach.png 2400000 @2026-07-01",
		"home/evan/Pictures/trips/coast.heic 1900000 @2026-07-02",
		"home/evan/Pictures/logo.svg 4200 @2026-05-05",
		"home/evan/projects/gloss/README.md 6100 @2026-10-02",
		"home/evan/projects/gloss/main.go 2048 @2026-10-02",
		"home/evan/projects/gloss/internal/app/model.go 21000 @2026-10-02",
		"home/evan/projects/gloss/internal/browse/browse.go 9000 @2026-10-02",
		"home/evan/projects/glossary/terms.txt 700 @2026-03-03",
		"home/evan/projects/a-very-long-folder-name-that-will-not-fit-in-a-narrow-column/x.txt 1",
		"home/evan/.profile 120 @2025-01-01",
		"home/evan/.config/gloss/config.toml 90 @2025-02-02",
		"usr/local/bin/tool 1000",
		"etc/hosts 200",
	}
	for i := 1; i <= 40; i++ {
		lines = append(lines, fmt.Sprintf("home/evan/Pictures/camera/IMG_%04d.jpg %d @2026-08-%02d", i, 1500000+i*1234, 1+i%28))
	}
	return lines
}

// FSFromDir describes a real folder as lines for NewFS, to try a chooser on
// a tree of your own, or to keep one as a fixture. Names are placed under
// mount, a slash-separated path; sizes and dates are kept; contents are not.
// It stops at maxDepth levels and maxEntries entries, and does not follow
// links. Names and sizes can be private: look before sharing a script.
func FSFromDir(dir, mount string, maxDepth, maxEntries int) ([]string, error) {
	mount = strings.Trim(mount, "/")
	var lines []string
	base := filepath.Clean(dir)
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // What cannot be read is left out, as a chooser would say.
		}
		rel, _ := filepath.Rel(base, p)
		if rel == "." {
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth > maxDepth {
			return fs.SkipDir
		}
		if len(lines) >= maxEntries {
			return fs.SkipAll
		}
		name := path.Join(mount, filepath.ToSlash(rel))
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return nil
		case d.IsDir():
			lines = append(lines, lineName(name+"/"))
		default:
			info, err := d.Info()
			if err != nil {
				return nil
			}
			lines = append(lines, fmt.Sprintf("%s %d @%s", lineName(name), info.Size(), info.ModTime().Format("2006-01-02")))
		}
		return nil
	})
	sort.Strings(lines)
	return lines, err
}

// lineName is a name as the first field of a line: Go-quoted if it needs to be.
func lineName(name string) string {
	if strings.ContainsAny(name, " \t\"\\") || strings.HasPrefix(name, "#") {
		return strconv.Quote(name)
	}
	return name
}
