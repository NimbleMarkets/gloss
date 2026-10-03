package browse

import (
	"io/fs"
	"path"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// expandedMsg is the answer to expand: what the typed path came to.
type expandedMsg struct {
	id       uint64
	from, to string
}

// expand completes a path whose folders are only begun, as zsh does: with
// ~/pr/gl, each segment that is not a folder as typed is the one folder it
// begins, if there is just one, and the last is completed as Tab would.
// It reads folders, so it is a command.
func (m Model) expand(typed string) tea.Cmd {
	fsys, home, dir, id := m.fsys, m.home, m.dir, m.id
	hidden, dirsOnly := m.showHidden, m.dirsOnly
	return func() tea.Msg {
		return expandedMsg{id: id, from: typed, to: expandPath(fsys, home, dir, typed, hidden, dirsOnly)}
	}
}

// expandPath is what the typed path comes to, or itself if some segment
// begins no folder, or more than one.
func expandPath(fsys fs.ReadDirFS, home, dir, typed string, hidden, dirsOnly bool) string {
	dirPart, rest := split(typed)
	cur, out := dir, ""
	segs := strings.Split(strings.TrimSuffix(dirPart, "/"), "/")
	switch {
	case strings.HasPrefix(dirPart, "/"):
		cur, out, segs = "/", "/", segs[1:]
	case home != "" && (segs[0] == "~"):
		cur, out, segs = home, "~/", segs[1:]
	}
	for _, seg := range segs {
		switch seg {
		case "":
			continue
		case ".":
			out += "./"
			continue
		case "..":
			cur, out = parentOf(cur), out+"../"
			continue
		}
		if isDir(fsys, path.Join(cur, seg)) {
			cur, out = path.Join(cur, seg), out+seg+"/"
			continue
		}
		names := prefixed(fsys, cur, seg, hidden, true)
		if len(names) != 1 {
			return typed
		}
		cur, out = path.Join(cur, names[0]), out+names[0]+"/"
	}
	names := prefixed(fsys, cur, rest, hidden, dirsOnly)
	switch len(names) {
	case 0:
		return out + rest
	case 1:
		if isDir(fsys, path.Join(cur, names[0])) {
			return out + names[0] + "/"
		}
		return out + names[0]
	}
	if lcp := commonPrefix(names, !strings.ContainsFunc(rest, unicode.IsUpper)); len([]rune(lcp)) > len([]rune(rest)) {
		return out + lcp
	}
	return out + rest
}

func isDir(fsys fs.ReadDirFS, dir string) bool {
	info, err := fs.Stat(fsys, fsName(dir))
	return err == nil && info.IsDir()
}

// prefixed are the names in a folder that begin with the text, case folded
// unless it has a capital; only folders when dirs is set. Hidden ones only
// if asked for, or the text starts with a dot.
func prefixed(fsys fs.ReadDirFS, dir, text string, hidden, dirs bool) []string {
	entries, err := fsys.ReadDir(fsName(dir))
	if err != nil && len(entries) == 0 {
		return nil
	}
	entries = FollowLinks(fsys, dir, entries)
	exact := strings.ContainsFunc(text, unicode.IsUpper)
	if !exact {
		text = strings.ToLower(text)
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if dirs && !e.IsDir() || !hidden && !strings.HasPrefix(text, ".") && strings.HasPrefix(name, ".") {
			continue
		}
		if !exact {
			name = strings.ToLower(name)
		}
		if strings.HasPrefix(name, text) {
			names = append(names, e.Name())
		}
	}
	return names
}

// commonPrefix is the longest start the names share; the first one's spelling.
func commonPrefix(names []string, fold bool) string {
	first := []rune(names[0])
	n := len(first)
	for _, name := range names[1:] {
		other := []rune(name)
		i := 0
		for i < n && i < len(other) && same(first[i], other[i], fold) {
			i++
		}
		n = i
	}
	return string(first[:n])
}
