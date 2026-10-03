package browsetest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// PlayOptions say how a component is played with in a terminal.
type PlayOptions struct {
	// Script is a script to take the filesystem, start folder, and options
	// from, and to replay. Without one, FS and Start are used.
	Script string
	// FS are the lines of the filesystem when there is no script (see
	// NewFS, DemoLines, FSFromDir), and Start the folder to begin in.
	FS    []string
	Start string
	// Options are set for the component, as a script's option lines.
	Options map[string]string
	// Record is a file the session is written to, as a script, when it ends.
	Record string
	// Replay steps through the script instead of taking keys.
	Replay bool
	// Width and Height fix the room the component is given, to keep
	// recordings small and alike; zero is as much as the terminal has.
	Width, Height int
	// Delay is the pace of a replay that is playing by itself.
	Delay time.Duration
	// AssertKeys are the Probe values F2 records as checks; none is all.
	AssertKeys []string
}

// Play runs a component in the terminal over an in-memory filesystem: a
// person drives it, optionally recorded as a script; or a script is replayed
// step by step, its checks shown. It is the harness with someone at the keys.
func Play[M Component[M]](cfg Config[M], o PlayOptions) error {
	var fsLines []string
	var prelude, actions []step
	if o.Script != "" {
		lines, steps, err := readScript(o.Script)
		if err != nil {
			return err
		}
		fsLines = lines
		prelude, actions = splitPrelude(steps)
	} else {
		fsLines = o.FS
	}
	start, options := o.Start, map[string]string{}
	for k, v := range o.Options {
		options[k] = v
	}
	var kept []step // The prelude a recording keeps as it was.
	for _, s := range prelude {
		switch s.cmd {
		case "start":
			start = s.arg
		case "option":
			k, v, _ := strings.Cut(s.arg, " ")
			options[k] = strings.TrimSpace(v)
		case "size":
			if f := strings.Fields(s.arg); len(f) == 2 && o.Width == 0 && o.Height == 0 {
				o.Width, _ = strconv.Atoi(f[0])
				o.Height, _ = strconv.Atoi(f[1])
			}
		default:
			kept = append(kept, s)
		}
	}
	if start == "" {
		start = "/"
	}
	build := func() (M, *FS) {
		fsys := NewFS(fsLines...)
		for _, s := range kept {
			applyFSStep(fsys, s)
		}
		return cfg.New(fsys, start, options), fsys
	}
	m, fsys := build()
	p := &player[M]{cfg: cfg, build: build, m: m, fsys: fsys, opts: o, steps: actions, delay: o.Delay}
	if p.delay == 0 {
		p.delay = 700 * time.Millisecond
	}
	if o.Replay {
		if o.Script == "" {
			return fmt.Errorf("replay needs a script")
		}
		p.replay = true
	}
	if o.Record != "" && !o.Replay {
		p.rec = &recorder{path: o.Record, fs: fsLines, prelude: kept, start: start, options: options}
	}
	prog := tea.NewProgram(p)
	if _, err := prog.Run(); err != nil {
		return err
	}
	if p.rec != nil {
		if err := os.WriteFile(p.rec.path, []byte(p.rec.script()), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%d events)\n", p.rec.path, len(p.rec.events))
	}
	if p.replay && p.failed > 0 {
		return fmt.Errorf("%d check(s) failed in the replay", p.failed)
	}
	return nil
}

// splitPrelude divides a script's steps into what sets the stage (size,
// start, options, latency, failures, additions) at its start, and the rest.
func splitPrelude(steps []step) (prelude, actions []step) {
	for i, s := range steps {
		switch s.cmd {
		case "size", "start", "option", "latency", "fail", "add", "note":
			prelude = append(prelude, s)
		default:
			return prelude, steps[i:]
		}
	}
	return prelude, nil
}

func applyFSStep(f *FS, s step) {
	switch s.cmd {
	case "latency":
		if d, err := time.ParseDuration(s.arg); err == nil {
			f.Latency(d)
		}
	case "fail":
		dir, msg, _ := strings.Cut(s.arg, " ")
		f.Fail(dir, fmt.Errorf("%s", strings.TrimSpace(msg)))
	case "add":
		f.Add(s.arg)
	}
}

type advanceMsg struct{ gen int }
type settledMsg struct{ gen int }

// player is the program that holds a component: it passes on what it is
// sent, keeps a log of it for a recording, or sends it a script's steps.
type player[M Component[M]] struct {
	cfg   Config[M]
	build func() (M, *FS)
	m     M
	fsys  *FS
	opts  PlayOptions

	termW, termH int
	w, h         int // The room given to the component.
	sized        bool

	rec *recorder

	replay  bool
	steps   []step
	at      int
	auto    bool
	delay   time.Duration
	gen     int // Tells the timers of one run of a step from another's.
	done    bool
	failed  int
	results []string
	note    string

	flash string // What the last key did, for the status line.
}

func (p *player[M]) Init() tea.Cmd { return p.m.Init() }

// area is the room the component is given: the terminal less the status
// line, or the size asked for if that is smaller.
func (p *player[M]) area() (int, int) {
	w, h := p.termW, p.termH-1
	if p.opts.Width > 0 {
		w = min(w, p.opts.Width)
	}
	if p.opts.Height > 0 {
		h = min(h, p.opts.Height)
	}
	return max(w, 1), max(h, 1)
}

func (p *player[M]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.termW, p.termH = msg.Width, msg.Height
		return p, p.resized()
	case advanceMsg:
		if msg.gen == p.gen && p.replay {
			return p, p.step()
		}
		return p, nil
	case settledMsg:
		if msg.gen == p.gen && p.replay {
			return p, p.settled()
		}
		return p, nil
	case tea.KeyPressMsg:
		if p.replay {
			return p, p.replayKey(msg)
		}
		return p, p.playKey(msg)
	case tea.MouseMsg:
		if p.replay {
			return p, nil
		}
		return p, p.mouse(msg)
	case tea.PasteMsg:
		if p.rec != nil {
			p.rec.add("type", msg.Content)
		}
	}
	return p, p.forward(msg)
}

func (p *player[M]) forward(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	p.m, cmd = p.m.Update(msg)
	return cmd
}

// resized tells the component the room it has, and the recording of it.
func (p *player[M]) resized() tea.Cmd {
	w, h := p.area()
	first := !p.sized
	changed := w != p.w || h != p.h
	p.w, p.h, p.sized = w, h, true
	if p.rec != nil {
		if first {
			p.rec.w, p.rec.h = w, h
		} else if changed {
			p.rec.resize(w, h)
		}
	}
	return p.forward(tea.WindowSizeMsg{Width: w, Height: h})
}

const hotkeys = "F1 snapshot · F2 check state · F3 note · Ctrl-C quit"

func (p *player[M]) playKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "f1", "f2", "f3":
		if p.rec == nil {
			p.flash = "not recording (give -record FILE)"
			return nil
		}
		switch msg.String() {
		case "f1":
			p.flash = "recorded snapshot: " + p.rec.snapshot()
		case "f2":
			p.rec.state(p.cfg.Probe(p.m), p.opts.AssertKeys)
			p.flash = "recorded checks of the state"
		case "f3":
			p.rec.add("note", "TODO: say what this step shows")
			p.flash = "recorded a note to edit"
		}
		return nil
	}
	if p.rec != nil {
		p.rec.key(msg)
	}
	p.flash = ""
	return p.forward(msg)
}

func (p *player[M]) mouse(msg tea.MouseMsg) tea.Cmd {
	if p.rec != nil {
		switch v := msg.(type) {
		case tea.MouseClickMsg:
			if v.Button == tea.MouseLeft {
				p.rec.click(v.X, v.Y)
			}
		case tea.MouseWheelMsg:
			if v.Button == tea.MouseWheelUp || v.Button == tea.MouseWheelDown {
				p.rec.wheel(v.X, v.Y, v.Button == tea.MouseWheelUp)
			}
		}
	}
	return p.forward(msg)
}

// View draws the component, then a line saying what is going on.
func (p *player[M]) View() tea.View {
	body := p.m.View()
	for n := strings.Count(body, "\n") + 1; n < p.h; n++ {
		body += "\n"
	}
	v := tea.NewView(body + "\n" + p.status())
	v.AltScreen = true
	if !p.replay {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (p *player[M]) status() string {
	var s string
	switch {
	case p.replay:
		s = p.replayStatus()
	case p.rec != nil:
		s = fmt.Sprintf(" ● REC %d · %s · %s", len(p.rec.events), hotkeys, filepath.Base(p.rec.path))
	default:
		s = " PLAY · " + hotkeys
	}
	if p.flash != "" {
		s += " · " + p.flash
	}
	return lipgloss.NewStyle().Reverse(true).Width(max(p.termW, 1)).Render(ansi.Truncate(s, max(p.termW, 1), "…"))
}

// ---- Replay ----

func (p *player[M]) replayStatus() string {
	state := "paused"
	switch {
	case p.done:
		state = "end"
	case p.auto:
		state = "playing"
	}
	last := ""
	if n := len(p.results); n > 0 {
		last = " · " + p.results[n-1]
	}
	note := ""
	if p.note != "" {
		note = " · " + p.note
	}
	next := ""
	if p.at < len(p.steps) {
		next = fmt.Sprintf(" · next: %s %s", p.steps[p.at].cmd, p.steps[p.at].arg)
	}
	keys := "space step · p play · r restart · q quit"
	return fmt.Sprintf(" %s %d/%d%s%s%s · %s", state, p.at, len(p.steps), next, last, note, keys)
}

func (p *player[M]) replayKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return tea.Quit
	case " ", "space", "enter", "n", "right":
		if p.done {
			return nil
		}
		p.auto = false
		p.gen++
		return p.step()
	case "p":
		p.auto = !p.auto
		p.gen++
		if p.auto && !p.done {
			return p.step()
		}
	case "+", "=":
		p.delay = max(p.delay/2, 50*time.Millisecond)
	case "-":
		p.delay *= 2
	case "r":
		p.m, p.fsys = p.build()
		p.at, p.done, p.failed, p.auto, p.results, p.note = 0, false, 0, false, nil, ""
		p.gen++
		return tea.Batch(p.m.Init(), p.resized())
	}
	return nil
}

func isCheck(cmd string) bool {
	switch cmd {
	case "state", "expect", "reject", "reads":
		return true
	}
	return false
}

// step does the steps up to and including the next one that acts on the
// component; the checks that follow wait until what it set going has settled.
func (p *player[M]) step() tea.Cmd {
	for p.at < len(p.steps) {
		s := p.steps[p.at]
		if !p.quiet(s) {
			cmds := p.perform(s)
			p.at++
			p.gen++
			gen := p.gen
			cmds = append(cmds, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return settledMsg{gen} }))
			return tea.Batch(cmds...)
		}
		p.at++
	}
	p.done = true
	return nil
}

// quiet is a step that does nothing to the component: a note, a snapshot,
// a change to the filesystem, a check. Checks and the rest are done as met.
func (p *player[M]) quiet(s step) bool {
	switch {
	case isCheck(s.cmd):
		p.check(s)
	case s.cmd == "note":
		p.note = s.arg
	case s.cmd == "snapshot":
		p.note = "snapshot " + s.arg
	case s.cmd == "add" || s.cmd == "fail" || s.cmd == "latency":
		applyFSStep(p.fsys, s)
	case s.cmd == "size" || s.cmd == "start" || s.cmd == "option":
	default:
		return false
	}
	return true
}

// settled runs the checks that follow a step, now what it did has come about,
// and carries on if the replay is playing by itself.
func (p *player[M]) settled() tea.Cmd {
	for p.at < len(p.steps) && p.quiet(p.steps[p.at]) {
		p.at++
	}
	if p.at >= len(p.steps) {
		p.done = true
		return nil
	}
	if p.auto {
		gen := p.gen
		return tea.Tick(p.delay, func(time.Time) tea.Msg { return advanceMsg{gen} })
	}
	return nil
}

// perform sends the component what a step says and returns what it asks for.
func (p *player[M]) perform(s step) []tea.Cmd {
	var cmds []tea.Cmd
	send := func(msg tea.Msg) { cmds = append(cmds, p.forward(msg)) }
	switch s.cmd {
	case "press":
		for _, name := range strings.Fields(s.arg) {
			if k, err := ParseKey(name); err == nil {
				send(k)
			}
		}
	case "type":
		for _, r := range unquote(s.arg) {
			send(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	case "click":
		if f := strings.Fields(s.arg); len(f) == 2 {
			x, _ := strconv.Atoi(f[0])
			y, _ := strconv.Atoi(f[1])
			send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		}
	case "wheel":
		if f := strings.Fields(s.arg); len(f) == 3 {
			x, _ := strconv.Atoi(f[0])
			y, _ := strconv.Atoi(f[1])
			b := tea.MouseWheelDown
			if f[2] == "up" {
				b = tea.MouseWheelUp
			}
			send(tea.MouseWheelMsg{X: x, Y: y, Button: b})
		}
	case "resize":
		if f := strings.Fields(s.arg); len(f) == 2 {
			p.opts.Width, _ = strconv.Atoi(f[0])
			p.opts.Height, _ = strconv.Atoi(f[1])
			cmds = append(cmds, p.resized())
		}
	}
	p.note = s.cmd + " " + s.arg
	return cmds
}

// check judges a check step as the harness would, and keeps the verdict.
func (p *player[M]) check(s step) {
	ok, why := p.judge(s)
	mark := "✓"
	if !ok {
		mark = "✗"
		p.failed++
	}
	line := fmt.Sprintf("%s %s %s", mark, s.cmd, s.arg)
	if why != "" {
		line += " (" + why + ")"
	}
	p.results = append(p.results, line)
}

func (p *player[M]) judge(s step) (bool, string) {
	screen := ansi.Strip(p.m.View())
	switch s.cmd {
	case "expect":
		return strings.Contains(screen, unquote(s.arg)), ""
	case "reject":
		return !strings.Contains(screen, unquote(s.arg)), ""
	case "state":
		if p.cfg.Probe == nil {
			return false, "no Probe"
		}
		k, v, _ := strings.Cut(s.arg, " ")
		probe := p.cfg.Probe(p.m)
		got, ok := probe[k]
		if !ok {
			keys := make([]string, 0, len(probe))
			for key := range probe {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			return false, "no state " + k + "; have " + strings.Join(keys, ", ")
		}
		want := unquote(strings.TrimSpace(v))
		return got == want, fmt.Sprintf("is %q", got)
	case "reads":
		f := strings.Fields(s.arg)
		if len(f) != 2 {
			return false, "want DIR N"
		}
		want, err := strconv.Atoi(f[1])
		got := p.fsys.ReadCount(f[0])
		return err == nil && got == want, fmt.Sprintf("read %d", got)
	}
	return true, ""
}
