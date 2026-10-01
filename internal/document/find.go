package document

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Limits bound a search, so that a folder of any size is safe to ask
// about. A zero value takes the default.
type Limits struct {
	Depth   int           // Folders below the one searched.
	Entries int           // Files and folders looked at.
	Matches int           // Files reported.
	Time    time.Duration // Spent searching.
}

// DefaultLimits are what a search gets when it asks for nothing else.
var DefaultLimits = Limits{Depth: 6, Entries: 10000, Matches: 500, Time: 3 * time.Second}

// Found is what a search turned up, and the limit that stopped it, if one did.
type Found struct {
	Paths []string
	Note  string
}

// The kinds a pattern may name, and the globs each stands for.
var kindGlobs = map[string][]string{
	"images":    {"*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.bmp", "*.tif", "*.tiff", "*.heic", "*.heif", "*.hif"},
	"pictures":  {"*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.bmp", "*.tif", "*.tiff", "*.heic", "*.heif", "*.hif"},
	"svg":       {"*.svg"},
	"pdf":       {"*.pdf"},
	"docs":      {"*.pdf", "*.docx", "*.docm"},
	"word":      {"*.docx", "*.docm"},
	"meshes":    {"*.stl", "*.3mf"},
	"3d":        {"*.stl", "*.3mf"},
	"tables":    {"*.xlsx", "*.xlsm", "*.csv", "*.tsv", "*.grist"},
	"excel":     {"*.xlsx", "*.xlsm"},
	"grist":     {"*.grist"},
	"csv":       {"*.csv", "*.tsv"},
	"markdown":  {"*.md", "*.markdown", "*.mdown"},
	"html":      {"*.html", "*.htm"},
	"text":      {"*.txt", "*.text", "*.log"},
	"json":      {"*.json", "*.jsonl", "*.ndjson"},
	"notebooks": {"*.ipynb"},
}

// globs turns what was asked for into globs: a kind by name, a known
// extension with or without its dot, or a glob as it is.
func globs(patterns []string) ([]string, error) {
	var out []string
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if g, ok := kindGlobs[strings.ToLower(p)]; ok {
			out = append(out, g...)
			continue
		}
		if ext := "." + strings.ToLower(strings.TrimPrefix(p, ".")); !strings.ContainsAny(p, "*?[/") && slicesContainsFold(Extensions, ext) {
			out = append(out, "*"+ext)
			continue
		}
		if _, err := path.Match(p, ""); err != nil {
			return nil, fmt.Errorf("%q is not a pattern: %w", p, err)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nothing to search for")
	}
	return out, nil
}

func slicesContainsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// Find lists the regular files under dir that any of the patterns
// matches, by name for a pattern without a slash and by path within dir
// for one with, without regard to case. Hidden folders and links are not
// entered, and the limits hold throughout.
func Find(dir string, patterns []string, limits Limits) (Found, error) {
	globs, err := globs(patterns)
	if err != nil {
		return Found{}, err
	}
	if limits.Depth <= 0 {
		limits.Depth = DefaultLimits.Depth
	}
	if limits.Entries <= 0 {
		limits.Entries = DefaultLimits.Entries
	}
	if limits.Matches <= 0 {
		limits.Matches = DefaultLimits.Matches
	}
	if limits.Time <= 0 {
		limits.Time = DefaultLimits.Time
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return Found{}, err
	}
	deadline := time.Now().Add(limits.Time)
	var found Found
	entries, deeper, more := 0, false, false
	stop := fmt.Errorf("stopped")
	err = fs.WalkDir(os.DirFS(dir), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || rel == "." {
			return nil
		}
		if time.Now().After(deadline) {
			found.Note = fmt.Sprintf("out of time after %v; search a smaller folder", limits.Time)
			return stop
		}
		if entries++; entries > limits.Entries {
			found.Note = fmt.Sprintf("stopped after %s entries; search a smaller folder", grouped(limits.Entries))
			return stop
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			if strings.Count(rel, "/")+1 > limits.Depth {
				deeper = true
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // Links are not followed, nor devices read.
		}
		for _, g := range globs {
			subject := strings.ToLower(name)
			if strings.Contains(g, "/") {
				subject = strings.ToLower(rel)
			}
			if ok, _ := path.Match(strings.ToLower(g), subject); ok {
				if len(found.Paths) >= limits.Matches {
					more = true
					return stop
				}
				found.Paths = append(found.Paths, filepath.Join(dir, filepath.FromSlash(rel)))
				break
			}
		}
		return nil
	})
	if err != nil && err != stop {
		return Found{}, err
	}
	sort.Strings(found.Paths)
	switch {
	case found.Note != "":
	case more:
		found.Note = fmt.Sprintf("first %d matches shown; ask for fewer", limits.Matches)
	case deeper:
		found.Note = fmt.Sprintf("folders deeper than %d were not searched", limits.Depth)
	}
	return found, nil
}
