package browsetest

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// An event is something done in a session that a script can say.
type event struct {
	kind string // press, type, click, wheel, resize, snapshot, note, state
	arg  string
}

// recorder keeps what is done in a playground session, to write as a script.
type recorder struct {
	path    string
	fs      []string // The fs part.
	prelude []step   // What the script began with, other than size, start, and options.
	start   string
	options map[string]string
	w, h    int
	events  []event
	shots   int
}

func (r *recorder) add(kind, arg string) { r.events = append(r.events, event{kind, arg}) }

// key records a key press: printable text as typed, the rest by name.
func (r *recorder) key(msg tea.KeyPressMsg) {
	if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 && printable(msg.Text) {
		r.add("type", msg.Text)
		return
	}
	r.add("press", msg.String())
}

func (r *recorder) click(x, y int)  { r.add("click", fmt.Sprintf("%d %d", x, y)) }
func (r *recorder) resize(w, h int) { r.add("resize", fmt.Sprintf("%d %d", w, h)) }
func (r *recorder) wheel(x, y int, up bool) {
	r.add("wheel", fmt.Sprintf("%d %d %s", x, y, map[bool]string{true: "up", false: "down"}[up]))
}

func (r *recorder) snapshot() string {
	r.shots++
	label := fmt.Sprintf("step %d", r.shots)
	r.add("snapshot", label)
	return label
}

// state records what the component says of itself, as checks a script keeps.
func (r *recorder) state(probe map[string]string, keys []string) {
	if len(keys) == 0 {
		for k := range probe {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}
	for _, k := range keys {
		if v, ok := probe[k]; ok {
			r.add("state", k+" "+quoteValue(v))
		}
	}
}

func printable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// quoteValue is a value as a script reads it back: as it is, or quoted when
// it is empty or would be trimmed or taken for quoted.
func quoteValue(v string) string {
	if v == "" || v != strings.TrimSpace(v) || strings.HasPrefix(v, `"`) || strings.HasPrefix(v, "'") || strings.HasPrefix(v, "`") {
		return strconv.Quote(v)
	}
	return v
}

// script is the session as the text of a script: the filesystem it was on,
// the size and options it began with, then what was done, runs of typing and
// of keys joined, a snapshot of the first screen at the start.
func (r *recorder) script() string {
	var b strings.Builder
	b.WriteString("# Recorded with browse:play; edit freely. Add checks (expect, state) where it matters.\n")
	b.WriteString("-- fs --\n")
	for _, l := range r.fs {
		b.WriteString(l + "\n")
	}
	b.WriteString("-- script --\n")
	fmt.Fprintf(&b, "size %d %d\n", r.w, r.h)
	if r.start != "" {
		fmt.Fprintf(&b, "start %s\n", r.start)
	}
	keys := make([]string, 0, len(r.options))
	for k := range r.options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "option %s %s\n", k, r.options[k])
	}
	for _, s := range r.prelude {
		fmt.Fprintf(&b, "%s %s\n", s.cmd, s.arg)
	}
	b.WriteString("snapshot start\n")
	for i := 0; i < len(r.events); {
		e := r.events[i]
		switch e.kind {
		case "type":
			var text strings.Builder
			for ; i < len(r.events) && r.events[i].kind == "type"; i++ {
				text.WriteString(r.events[i].arg)
			}
			fmt.Fprintf(&b, "type %s\n", strconv.Quote(text.String()))
		case "press":
			var run []string
			for ; i < len(r.events) && r.events[i].kind == "press" && len(run) < 8; i++ {
				run = append(run, r.events[i].arg)
			}
			fmt.Fprintf(&b, "press %s\n", strings.Join(run, " "))
		case "note":
			fmt.Fprintf(&b, "note %s\n", e.arg)
			i++
		default:
			fmt.Fprintf(&b, "%s %s\n", e.kind, e.arg)
			i++
		}
	}
	return b.String()
}
