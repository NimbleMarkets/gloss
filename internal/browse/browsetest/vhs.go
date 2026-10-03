package browsetest

import (
	"strconv"
	"strings"
)

// Scripts may be written in the vocabulary of Charm's VHS where the two
// overlap, so that people who know tapes can read them, and a script can
// become a tape (see [Tape]). A step is read case-insensitively, and VHS's
// form is turned into ours:
//
//	Type "text"      type TEXT        (also 'text' and `text`; Type@50ms too)
//	Enter, Tab, Escape, Backspace, Delete, Space, Up, Down, Left, Right,
//	Home, End, PageUp, PageDown        press KEY, with a count: Down 3
//	Ctrl+L, Alt+Up, Shift+Tab          press KEY
//	Screenshot [NAME]                  snapshot [NAME]
//	Sleep, Set, Output, Require, Hide, Show, Env, Source
//	                                   nothing: they are about recording
//
// What VHS has no word for is ours alone: fs, state, expect, reads, click.

// vhsKeys are VHS's key commands, and the names ParseKey knows them by.
var vhsKeys = map[string]string{
	"enter": "enter", "tab": "tab", "escape": "esc", "backspace": "backspace", "delete": "delete",
	"space": "space", "up": "up", "down": "down", "left": "left", "right": "right",
	"home": "home", "end": "end", "pageup": "pgup", "pagedown": "pgdown",
}

// vhsIgnored are VHS commands that shape a recording, not a session.
var vhsIgnored = map[string]bool{
	"sleep": true, "set": true, "output": true, "require": true, "hide": true, "show": true,
	"env": true, "source": true, "wait": true,
}

// native turns a step as written, in either vocabulary, into steps in ours.
func native(s step) []step {
	cmd := strings.ToLower(s.cmd)
	if at := strings.IndexByte(cmd, '@'); at > 0 {
		cmd = cmd[:at] // Type@50ms, Down@200ms: the pace is for recordings.
	}
	switch {
	case vhsIgnored[cmd]:
		return nil
	case cmd == "screenshot":
		return []step{{s.line, "snapshot", s.arg}}
	case vhsKeys[cmd] != "" || strings.ContainsRune(cmd, '+') && !strings.HasPrefix(cmd, "+"):
		key := vhsKeys[cmd]
		if key == "" {
			key = cmd // Ctrl+L, Alt+Up.
		}
		n := 1
		if c, err := strconv.Atoi(strings.TrimSpace(s.arg)); err == nil && c > 0 {
			n = c
		}
		var out []step
		for range n {
			out = append(out, step{s.line, "press", key})
		}
		return out
	}
	s.cmd = cmd
	return []step{s}
}
