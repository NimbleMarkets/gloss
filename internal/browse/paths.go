package browse

import (
	"path"
	"strings"
)

// cleanAbs is a path made absolute and clean: "/" or "/a/b".
func cleanAbs(p string) string { return path.Clean("/" + strings.TrimLeft(p, "/")) }

// fsName is what a filesystem calls a folder: no leading slash, "." the root.
func fsName(dir string) string {
	if n := strings.TrimLeft(dir, "/"); n != "" {
		return n
	}
	return "."
}

// split divides what is typed into the folder it names so far, up to and
// including its last slash, and the rest, which narrows that folder's names.
func split(typed string) (dirPart, rest string) {
	i := strings.LastIndexByte(typed, '/')
	return typed[:i+1], typed[i+1:]
}

// resolve is the folder a typed folder part names: "" is where the browsing
// is, "~" is home, a leading slash is the root, and the rest is relative.
func (m Model) resolve(dirPart string) string {
	switch {
	case dirPart == "":
		return m.dir
	case m.home != "" && (dirPart == "~" || strings.HasPrefix(dirPart, "~/")):
		return path.Join(m.home, strings.TrimPrefix(dirPart, "~"))
	case strings.HasPrefix(dirPart, "/"):
		return cleanAbs(dirPart)
	}
	return path.Join(m.dir, dirPart)
}

// viewDir is the folder whose entries are listed: the one a typed path
// names, else the one browsed.
func (m Model) viewDir() string {
	dirPart, _ := split(m.activeInput())
	return m.resolve(dirPart)
}

// parentOf is the folder above, the root its own.
func parentOf(dir string) string { return path.Dir(dir) }
