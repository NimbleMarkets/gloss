package browsetest

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// pad is a component that shows a folder's names and what was typed.
type pad struct {
	fsys  fs.ReadDirFS
	typed string
	names []string
	w, h  int
}

type padRead struct{ names []string }

func (m pad) Init() tea.Cmd {
	return func() tea.Msg {
		entries, _ := m.fsys.ReadDir(".")
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return padRead{names}
	}
}

func (m pad) Update(msg tea.Msg) (pad, tea.Cmd) {
	switch msg := msg.(type) {
	case padRead:
		m.names = msg.names
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		m.typed += msg.String()
	}
	return m, nil
}

func (m pad) View() string { return "typed:" + m.typed + "\n" + strings.Join(m.names, ",") }

func padConfig() Config[pad] {
	return Config[pad]{
		New:   func(f *FS, _ string, _ map[string]string) pad { return pad{fsys: f} },
		Probe: func(m pad) map[string]string { return map[string]string{"typed": m.typed} },
	}
}

// drain runs the commands a player returns, and what they answer with, until
// none is left; timers are waited for, being short in these tests.
func drain[M Component[M]](t *testing.T, p *player[M], cmd tea.Cmd) {
	t.Helper()
	for depth := 0; cmd != nil && depth < 50; depth++ {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				drain(t, p, c)
			}
			return
		}
		if msg == nil {
			return
		}
		_, cmd = p.Update(msg)
	}
}

func TestKeyNamesRoundTrip(t *testing.T) {
	for _, name := range []string{"enter", "esc", "tab", "shift+tab", "backspace", "delete", "up", "down", "left", "right",
		"home", "end", "pgup", "pgdown", "ctrl+l", "ctrl+u", "alt+up", "alt+left", "ctrl+shift+a", "f1", "f2", "alt+backspace", "ctrl+left"} {
		k, err := ParseKey(name)
		if err != nil {
			t.Errorf("ParseKey(%q): %v", name, err)
			continue
		}
		if k.String() != name {
			t.Errorf("ParseKey(%q).String() = %q: a recording would not read back", name, k.String())
		}
		r := &recorder{}
		r.key(k)
		if len(r.events) != 1 || r.events[0] != (event{"press", name}) {
			t.Errorf("key %q recorded as %v", name, r.events)
		}
	}
	// Printable keys are typed, space and capitals included.
	r := &recorder{}
	for _, s := range []string{"r", "e", " ", "X", "é"} {
		k, _ := ParseKey(s)
		if s == " " {
			k = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		}
		r.key(k)
	}
	for _, e := range r.events {
		if e.kind != "type" {
			t.Errorf("printable key recorded as %v", e)
		}
	}
}

func TestRecordingReadsBack(t *testing.T) {
	r := &recorder{
		fs: []string{"home/a.txt 12", `"home/two words.txt"`}, start: "/home", w: 80, h: 12,
		options: map[string]string{"layout": "columns", "hidden": "true"},
		prelude: []step{{0, "latency", "20ms"}},
	}
	for _, k := range []string{"d", "o", "w", "n"} {
		key, _ := ParseKey(k)
		r.key(key)
	}
	for _, name := range []string{"down", "enter", "ctrl+l"} {
		key, _ := ParseKey(name)
		r.key(key)
	}
	r.state(map[string]string{"dir": "/home", "filter": "", "current": "a b "}, []string{"dir", "filter", "current"})
	r.click(3, 4)
	r.wheel(1, 2, true)
	r.add("note", "TODO")
	if label := r.snapshot(); label != "step 1" {
		t.Errorf("label %q", label)
	}
	r.resize(60, 10)

	file := filepath.Join(t.TempDir(), "rec.txt")
	if err := os.WriteFile(file, []byte(r.script()), 0o644); err != nil {
		t.Fatal(err)
	}
	fsLines, steps, err := readScript(file)
	if err != nil {
		t.Fatalf("a recording does not read back: %v\n%s", err, r.script())
	}
	if !reflect.DeepEqual(fsLines, r.fs) {
		t.Errorf("fs %q", fsLines)
	}
	var got []string
	for _, s := range steps {
		got = append(got, s.cmd+" "+s.arg)
	}
	want := []string{
		"size 80 12", "start /home", "option hidden true", "option layout columns", "latency 20ms",
		"snapshot start", `type "down"`, "press down enter ctrl+l",
		"state dir /home", `state filter ""`, `state current "a b "`,
		"click 3 4", "wheel 1 2 up", "note TODO", "snapshot step 1", "resize 60 10",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("steps:\n%q\nwant:\n%q", got, want)
	}
}

func replayOf(t *testing.T, script string) *player[pad] {
	t.Helper()
	file := filepath.Join(t.TempDir(), "s.txt")
	if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	fsLines, steps, err := readScript(file)
	if err != nil {
		t.Fatal(err)
	}
	prelude, actions := splitPrelude(steps)
	_ = prelude
	build := func() (pad, *FS) {
		f := NewFS(fsLines...)
		return pad{fsys: f}, f
	}
	m, f := build()
	p := &player[pad]{cfg: padConfig(), build: build, m: m, fsys: f, steps: actions, replay: true, delay: time.Millisecond}
	_, cmd := p.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	drain(t, p, cmd)
	drain(t, p, p.Init())
	return p
}

func TestReplayStepsThroughAScriptAndJudgesItsChecks(t *testing.T) {
	p := replayOf(t, "-- fs --\nx.txt\ny.txt\n-- script --\nsize 40 8\nnote hello\ntype \"ab\"\nstate typed ab\nexpect x.txt\npress enter\nreject nothing\nstate typed abenter\nstate typed zz\nreads . 1\n")
	if p.at != 0 || p.done {
		t.Fatalf("at %d done %v: a replay waits to be told", p.at, p.done)
	}
	// Space steps: the first step acts (typing), and its checks follow it.
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	drain(t, p, cmd)
	if p.at != 3 || len(p.results) != 2 || p.failed != 0 {
		t.Fatalf("after the first step: at=%d results=%q failed=%d", p.at, p.results, p.failed)
	}
	if !strings.HasPrefix(p.results[0], "✓ state typed ab") || !strings.HasPrefix(p.results[1], "✓ expect x.txt") {
		t.Errorf("results %q", p.results)
	}
	if !strings.Contains(p.m.View(), "typed:ab") || !strings.Contains(p.View().Content, "x.txt,y.txt") {
		t.Errorf("the component was not driven:\n%s", p.View().Content)
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	drain(t, p, cmd)
	if !p.done || p.failed != 1 {
		t.Fatalf("done=%v failed=%d results=%q", p.done, p.failed, p.results)
	}
	last := p.results[len(p.results)-2]
	if !strings.HasPrefix(last, "✗ state typed zz") || !strings.Contains(last, `is "abenter"`) {
		t.Errorf("the failed check was not reported: %q", p.results)
	}
	if got := p.results[len(p.results)-1]; !strings.HasPrefix(got, "✓ reads . 1") {
		t.Errorf("reads: %q", got)
	}
	if status := p.status(); !strings.Contains(status, "end") {
		t.Errorf("status %q", status)
	}
	// r starts again, with the filesystem and component as they were.
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	drain(t, p, cmd)
	if p.at != 0 || p.failed != 0 || p.done || strings.Contains(p.m.View(), "typed:ab") {
		t.Errorf("restart: at=%d failed=%d done=%v\n%s", p.at, p.failed, p.done, p.m.View())
	}
}

func TestReplayPlaysByItselfToTheEnd(t *testing.T) {
	p := replayOf(t, "-- fs --\nx\n-- script --\npress a b c\nstate typed abc\npress d\nstate typed abcd\n")
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	drain(t, p, cmd)
	if !p.done || p.failed != 0 || len(p.results) != 2 {
		t.Fatalf("done=%v failed=%d results=%q", p.done, p.failed, p.results)
	}
}

func TestPlayRecordsWhatIsTypedAndHotkeys(t *testing.T) {
	f := NewFS("a.txt")
	m := pad{fsys: f}
	r := &recorder{path: "x", fs: []string{"a.txt"}, start: "/", options: map[string]string{}}
	p := &player[pad]{cfg: padConfig(), m: m, fsys: f, rec: r}
	_, cmd := p.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	drain(t, p, cmd)
	if r.w != 50 || r.h != 11 {
		t.Errorf("recorded size %dx%d: the status line takes a row", r.w, r.h)
	}
	for _, k := range []tea.KeyPressMsg{{Code: 'h', Text: "h"}, {Code: 'i', Text: "i"}, {Code: tea.KeyEnter}, {Code: tea.KeyF1}, {Code: tea.KeyF2}, {Code: tea.KeyF3}} {
		_, cmd := p.Update(k)
		drain(t, p, cmd)
	}
	_, _ = p.Update(tea.PasteMsg{Content: "pasted"})
	_, _ = p.Update(tea.MouseClickMsg{X: 2, Y: 3, Button: tea.MouseLeft})
	script := r.script()
	for _, want := range []string{"snapshot start", `type "hi"`, "press enter", "snapshot step 1", "state typed hienter", "note TODO", `type "pasted"`, "click 2 3"} {
		if !strings.Contains(script, want) {
			t.Errorf("recording lacks %q:\n%s", want, script)
		}
	}
	// A hotkey is the player's: it is not sent to the component.
	if strings.Contains(p.m.View(), "f1") || strings.Contains(p.m.View(), "f3") {
		t.Errorf("hotkeys reached the component: %s", p.m.View())
	}
	// Without a recording, the hotkeys only say so.
	q := &player[pad]{cfg: padConfig(), m: m, fsys: f}
	_, _ = q.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	_, _ = q.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	if !strings.Contains(q.status(), "not recording") {
		t.Errorf("status %q", q.status())
	}
}

func TestPlayRefusesToReplayWithoutAScript(t *testing.T) {
	if err := Play(padConfig(), PlayOptions{Replay: true, FS: []string{"a"}}); err == nil {
		t.Error("a replay needs a script")
	}
}
