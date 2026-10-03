package browsetest

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Component is what the harness drives: a Bubble Tea model by value, as
// bubbles' components are. M is the component's own type.
type Component[M any] interface {
	Init() tea.Cmd
	Update(tea.Msg) (M, tea.Cmd)
	View() string
}

// Driver feeds a component keys and messages, and settles the commands it
// answers with, as the Bubble Tea runtime would, but without waiting on
// timers such as the cursor's blink.
type Driver[M Component[M]] struct {
	T testing.TB
	M M

	// Width and Height are the size last sent to the component.
	Width, Height int

	// Patience is how long one command is waited on before it is taken for
	// a timer and let go.
	Patience time.Duration

	// StrictFit makes every settled screen be checked with Screen.Problems:
	// to fit Width by Height, by both width tables, and hold no control
	// characters.
	StrictFit bool

	trail []string
}

// New starts a component at a size: the window size is sent, Init runs, and
// what it started is settled.
func New[M Component[M]](t testing.TB, m M, w, h int) *Driver[M] {
	t.Helper()
	d := &Driver[M]{T: t, M: m, Patience: 250 * time.Millisecond}
	d.Resize(w, h)
	d.run(m.Init())
	return d
}

// Resize sends a window size.
func (d *Driver[M]) Resize(w, h int) {
	d.T.Helper()
	d.Width, d.Height = w, h
	d.note("resize %dx%d", w, h)
	d.Send(tea.WindowSizeMsg{Width: w, Height: h})
}

// Send delivers a message and settles what follows.
func (d *Driver[M]) Send(msg tea.Msg) {
	d.T.Helper()
	var cmd tea.Cmd
	d.M, cmd = d.M.Update(msg)
	d.run(cmd)
}

// Run executes a command the test got by calling the component directly (its
// Reload, say), and settles what follows.
func (d *Driver[M]) Run(cmd tea.Cmd) {
	d.T.Helper()
	d.run(cmd)
}

// Press sends keys by name: "enter", "esc", "tab", "shift+tab", "ctrl+n",
// "alt+up", "backspace", "pgdown", or a single character such as "a" or
// "G". Each is settled before the next.
func (d *Driver[M]) Press(keys ...string) {
	d.T.Helper()
	for _, name := range keys {
		msg, err := ParseKey(name)
		if err != nil {
			d.T.Fatalf("%v", err)
		}
		d.note("press %s", name)
		d.Send(msg)
	}
}

// Type sends text, one character at a time.
func (d *Driver[M]) Type(s string) {
	d.T.Helper()
	d.note("type %q", s)
	for _, r := range s {
		d.Send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// Click sends a left click at a screen cell, column x and row y from 0.
func (d *Driver[M]) Click(x, y int) {
	d.T.Helper()
	d.note("click %d,%d", x, y)
	d.Send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// Wheel sends a scroll of the wheel at a cell, up or down.
func (d *Driver[M]) Wheel(x, y int, up bool) {
	d.T.Helper()
	b := tea.MouseWheelDown
	if up {
		b = tea.MouseWheelUp
	}
	d.note("wheel %d,%d up=%v", x, y, up)
	d.Send(tea.MouseWheelMsg{X: x, Y: y, Button: b})
}

// Screen is the component's view as it stands.
func (d *Driver[M]) Screen() Screen {
	d.T.Helper()
	s := Screen{Raw: d.M.View(), Width: d.Width, Height: d.Height}
	if d.StrictFit {
		if err := s.Fits(); err != nil {
			d.T.Fatalf("%v\n%s\nafter: %s", err, s, d.Trail())
		}
	}
	return s
}

// Trail is what was done to reach the screen, for failure messages.
func (d *Driver[M]) Trail() string { return strings.Join(d.trail, ", ") }

func (d *Driver[M]) note(format string, args ...any) {
	d.trail = append(d.trail, fmt.Sprintf(format, args...))
}

// run executes a command and the commands that follow from its messages.
// A command that does not answer within the patience is taken for a timer.
func (d *Driver[M]) run(cmd tea.Cmd) {
	d.T.Helper()
	d.exec(cmd, 0)
}

func (d *Driver[M]) exec(cmd tea.Cmd, depth int) {
	d.T.Helper()
	if cmd == nil || depth > 24 {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(d.Patience):
		return
	}
	if msg == nil {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			d.exec(c, depth+1)
		}
		return
	}
	// A sequence is a slice of commands under a name of its own.
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
		for i := 0; i < v.Len(); i++ {
			if c, ok := v.Index(i).Interface().(tea.Cmd); ok {
				d.exec(c, depth+1)
			}
		}
		return
	}
	var next tea.Cmd
	d.M, next = d.M.Update(msg)
	d.exec(next, depth+1)
}

// Screen is a rendered view.
type Screen struct {
	Raw           string // As drawn, with its styling.
	Width, Height int    // The size it was drawn for, to check it against.
}

// Lines are the rows with styling removed and trailing spaces trimmed.
func (s Screen) Lines() []string {
	rows := strings.Split(ansi.Strip(s.Raw), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return rows
}

// Styled is the rows as drawn, styling kept.
func (s Screen) Styled() []string { return strings.Split(s.Raw, "\n") }

// String is the plain text, trailing blank rows dropped.
func (s Screen) String() string {
	rows := s.Lines()
	for len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	return strings.Join(rows, "\n")
}

// Contains says whether the text is anywhere on the screen.
func (s Screen) Contains(text string) bool { return strings.Contains(ansi.Strip(s.Raw), text) }

// Find is the row and column (in cells) where the text first appears.
func (s Screen) Find(text string) (row, col int, ok bool) {
	for r, line := range s.Lines() {
		if i := strings.Index(line, text); i >= 0 {
			return r, ansi.StringWidth(line[:i]), true
		}
	}
	return 0, 0, false
}

// Row is the plain text of one row, or "" beyond the screen.
func (s Screen) Row(i int) string {
	rows := s.Lines()
	if i < 0 || i >= len(rows) {
		return ""
	}
	return rows[i]
}

// Fits says whether the view is drawn soundly for its size: see Problems.
func (s Screen) Fits() error {
	if p := s.Problems(); len(p) > 0 {
		return fmt.Errorf("%s", strings.Join(p, "; "))
	}
	return nil
}

// Problems lists what is wrong with how the view is drawn for the size it was
// given, the faults that mess up a terminal:
//
//   - more rows than there are;
//   - a row wider than the terminal, which wraps. Width is measured by two
//     tables, grapheme clusters and wcwidth, which disagree about some emoji
//     (a family of people joined, a heart with its emoji selector): a row that
//     fits by one and overflows by the other wraps on some terminals, so it
//     counts;
//   - a control character in the text (a tab, a carriage return, a bell), as a
//     file name can carry, which moves the cursor and breaks the columns.
//
// Escape sequences that style the text are not text and are not counted.
func (s Screen) Problems() []string {
	var out []string
	rows := strings.Split(s.Raw, "\n")
	if s.Height > 0 && len(rows) > s.Height {
		out = append(out, fmt.Sprintf("view is %d rows, over the %d it was given", len(rows), s.Height))
	}
	for i, r := range rows {
		if s.Width > 0 {
			grapheme, wc := ansi.StringWidth(r), ansi.StringWidthWc(r)
			switch {
			case grapheme > s.Width:
				out = append(out, fmt.Sprintf("row %d is %d cells wide, over the %d it was given", i, grapheme, s.Width))
			case wc > s.Width:
				out = append(out, fmt.Sprintf("row %d is %d cells wide by wcwidth (%d by grapheme clusters), over the %d it was given: it wraps on a terminal that counts so", i, wc, grapheme, s.Width))
			}
		}
		for _, c := range ansi.Strip(r) {
			if c < 0x20 || c >= 0x7f && c < 0xa0 {
				out = append(out, fmt.Sprintf("row %d holds the control character %U", i, c))
				break
			}
		}
	}
	return out
}

// RawContains says whether the text is in the view as drawn, escape sequences
// and all: for checking that nothing a file's name says gets to the terminal.
func (s Screen) RawContains(text string) bool { return strings.Contains(s.Raw, text) }

// ParseKey makes the key press a name stands for.
func ParseKey(name string) (tea.KeyPressMsg, error) {
	if name == "" {
		return tea.KeyPressMsg{}, fmt.Errorf("empty key name")
	}
	if name == "+" {
		return tea.KeyPressMsg{Code: '+', Text: "+"}, nil
	}
	// A plus as the key itself ends the name: "ctrl++".
	mods, base := name, ""
	if m, ok := strings.CutSuffix(name, "++"); ok {
		mods, base = m, "+"
	} else if i := strings.LastIndexByte(name, '+'); i >= 0 {
		mods, base = name[:i], name[i+1:]
	} else {
		mods, base = "", name
	}
	parts := append(strings.Split(mods, "+"), base)
	if mods == "" {
		parts = []string{base}
	}
	var mod tea.KeyMod
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(p) {
		case "ctrl":
			mod |= tea.ModCtrl
		case "alt":
			mod |= tea.ModAlt
		case "shift":
			mod |= tea.ModShift
		default:
			return tea.KeyPressMsg{}, fmt.Errorf("unknown modifier %q in key %q", p, name)
		}
	}
	if code, ok := keyCodes[strings.ToLower(base)]; ok {
		if code == tea.KeySpace && mod == 0 {
			return tea.KeyPressMsg{Code: code, Text: " "}, nil
		}
		return tea.KeyPressMsg{Code: code, Mod: mod}, nil
	}
	runes := []rune(base)
	if len(runes) != 1 {
		return tea.KeyPressMsg{}, fmt.Errorf("unknown key %q", name)
	}
	if mod&(tea.ModCtrl|tea.ModAlt) != 0 {
		return tea.KeyPressMsg{Code: unicode.ToLower(runes[0]), Mod: mod}, nil // As VHS writes Ctrl+L.
	}
	return tea.KeyPressMsg{Code: runes[0], Mod: mod, Text: string(runes[0])}, nil
}

var keyCodes = map[string]rune{
	"enter": tea.KeyEnter, "return": tea.KeyEnter, "esc": tea.KeyEscape, "escape": tea.KeyEscape,
	"tab": tea.KeyTab, "backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
	"space": tea.KeySpace,
	"f1":    tea.KeyF1, "f2": tea.KeyF2, "f3": tea.KeyF3, "f4": tea.KeyF4, "f5": tea.KeyF5, "f6": tea.KeyF6,
	"f7": tea.KeyF7, "f8": tea.KeyF8, "f9": tea.KeyF9, "f10": tea.KeyF10, "f11": tea.KeyF11, "f12": tea.KeyF12,
}
