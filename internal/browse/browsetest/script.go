package browsetest

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update-screens", false, "rewrite the golden screens of browsetest scripts")

// Config says how scripts build the component they drive.
type Config[M Component[M]] struct {
	// New makes the component over a filesystem, to start in the folder
	// (slash-separated, from the root), under the options a script set.
	New func(fsys *FS, start string, options map[string]string) M

	// Probe reads the component's state by name, for a script's "state"
	// checks. Optional.
	Probe func(M) map[string]string

	// Strict checks every settled screen against the size it was given.
	Strict bool
}

// RunDir runs every script in a folder (its .txt files) as a subtest. Each
// is a file of two parts:
//
//	-- fs --
//	home/evan/notes.md
//	home/evan/projects/
//	-- script --
//	start /home/evan
//	size 60 12
//	press down
//	snapshot after-down
//
// and its snapshots are compared with a .golden file beside it, which
// "go test -update-screens" writes. Commands, one to a line, with "#" for
// comments:
//
//	size W H               the screen size (before anything is sent; default 80 24)
//	start PATH             the folder to start in (default /)
//	option KEY VALUE       an option for Config.New
//	latency DURATION       make each folder read take that long
//	fail DIR MESSAGE       make reading a folder fail
//	add LINE               add to the filesystem (the lines of the fs part)
//	press KEY...           keys by name; see ParseKey
//	type TEXT              text, or a quoted string to keep spaces
//	click X Y | wheel X Y up|down
//	resize W H
//	snapshot [LABEL]       record the screen
//	expect TEXT            fail unless the screen has the text
//	reject TEXT            fail if the screen has the text
//	state KEY VALUE        fail unless Probe says so
//	reads DIR N            fail unless the folder was read N times
//	note TEXT              put a remark in the golden file
//
// Where VHS has a word for it, that word does too: see vhs.go.
func RunDir[M Component[M]](t *testing.T, cfg Config[M], dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no scripts in %s (%v)", dir, err)
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".txt")
		t.Run(name, func(t *testing.T) { RunScript(t, cfg, file) })
	}
}

// RunScript runs one script file.
func RunScript[M Component[M]](t *testing.T, cfg Config[M], file string) {
	t.Helper()
	fsLines, steps, err := readScript(file)
	if err != nil {
		t.Fatal(err)
	}
	r := &runner[M]{t: t, cfg: cfg, file: file, fsys: NewFS(), w: 80, h: 24, start: "/", options: map[string]string{}}
	for _, l := range fsLines {
		r.fsys.Add(l)
	}
	for _, s := range steps {
		r.step(s)
	}
	r.finish()
}

type step struct {
	line int
	cmd  string
	arg  string
}

func readScript(file string) (fsLines []string, steps []step, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	part := ""
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if p, ok := strings.CutPrefix(line, "-- "); ok && strings.HasSuffix(p, " --") {
			part = strings.TrimSuffix(p, " --")
			if part != "fs" && part != "script" {
				return nil, nil, fmt.Errorf("%s:%d: unknown part %q", file, n, part)
			}
			continue
		}
		switch part {
		case "fs":
			if s := strings.TrimSpace(line); s != "" && !strings.HasPrefix(s, "#") {
				fsLines = append(fsLines, line)
			}
		case "script":
			s := strings.TrimSpace(line)
			if s == "" || strings.HasPrefix(s, "#") {
				continue
			}
			cmd, arg, _ := strings.Cut(s, " ")
			steps = append(steps, native(step{n, cmd, strings.TrimSpace(arg)})...)
		default:
			if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") {
				return nil, nil, fmt.Errorf("%s:%d: text before any part", file, n)
			}
		}
	}
	return fsLines, steps, sc.Err()
}

type runner[M Component[M]] struct {
	t       *testing.T
	cfg     Config[M]
	file    string
	fsys    *FS
	d       *Driver[M]
	w, h    int
	start   string
	options map[string]string
	golden  strings.Builder
}

func (r *runner[M]) fail(s step, format string, args ...any) {
	r.t.Helper()
	trail := ""
	if r.d != nil {
		trail = "\nafter: " + r.d.Trail() + "\n" + r.d.Screen().String()
	}
	r.t.Fatalf("%s:%d: %s %s: %s%s", filepath.Base(r.file), s.line, s.cmd, s.arg, fmt.Sprintf(format, args...), trail)
}

// open starts the component, at the first step that needs it.
func (r *runner[M]) open() {
	if r.d != nil {
		return
	}
	r.d = New(r.t, r.cfg.New(r.fsys, r.start, r.options), r.w, r.h)
	r.d.StrictFit = r.cfg.Strict
}

func (r *runner[M]) step(s step) {
	r.t.Helper()
	ints := func(n int) []int {
		f := strings.Fields(s.arg)
		if len(f) != n {
			r.fail(s, "want %d numbers", n)
		}
		out := make([]int, n)
		for i, v := range f {
			x, err := strconv.Atoi(v)
			if err != nil {
				r.fail(s, "%v", err)
			}
			out[i] = x
		}
		return out
	}
	configure := func() {
		if r.d != nil {
			r.fail(s, "must come before the first action")
		}
	}
	switch s.cmd {
	case "size":
		configure()
		v := ints(2)
		r.w, r.h = v[0], v[1]
	case "start":
		configure()
		r.start = s.arg
	case "option":
		configure()
		k, v, _ := strings.Cut(s.arg, " ")
		r.options[k] = strings.TrimSpace(v)
	case "latency":
		configure()
		d, err := time.ParseDuration(s.arg)
		if err != nil {
			r.fail(s, "%v", err)
		}
		r.fsys.Latency(d)
	case "fail":
		dir, msg, _ := strings.Cut(s.arg, " ")
		r.fsys.Fail(dir, errors.New(strings.TrimSpace(msg)))
	case "add":
		r.fsys.Add(s.arg)
	case "press":
		r.open()
		r.d.Press(strings.Fields(s.arg)...)
		r.d.Screen() // Checks the fit, if asked.
	case "type":
		r.open()
		r.d.Type(r.text(s))
		r.d.Screen()
	case "click":
		r.open()
		v := ints(2)
		r.d.Click(v[0], v[1])
	case "wheel":
		r.open()
		f := strings.Fields(s.arg)
		if len(f) != 3 {
			r.fail(s, "want X Y up|down")
		}
		x, errX := strconv.Atoi(f[0])
		y, errY := strconv.Atoi(f[1])
		if errX != nil || errY != nil {
			r.fail(s, "bad position")
		}
		r.d.Wheel(x, y, f[2] == "up")
	case "resize":
		r.open()
		v := ints(2)
		r.d.Resize(v[0], v[1])
	case "snapshot":
		r.open()
		label := s.arg
		if label == "" {
			label = "screen"
		}
		scr := r.d.Screen()
		fmt.Fprintf(&r.golden, "=== %s (%dx%d)\n%s\n\n", label, r.d.Width, r.d.Height, scr)
	case "note":
		fmt.Fprintf(&r.golden, "# %s\n\n", s.arg)
	case "expect":
		r.open()
		if want := r.text(s); !r.d.Screen().Contains(want) {
			r.fail(s, "not on the screen")
		}
	case "reject":
		r.open()
		if want := r.text(s); r.d.Screen().Contains(want) {
			r.fail(s, "is on the screen")
		}
	case "state":
		r.open()
		if r.cfg.Probe == nil {
			r.fail(s, "no Probe configured")
		}
		k, v, _ := strings.Cut(s.arg, " ")
		got, ok := r.cfg.Probe(r.d.M)[k]
		if !ok {
			keys := make([]string, 0)
			for key := range r.cfg.Probe(r.d.M) {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			r.fail(s, "no state %q (have %s)", k, strings.Join(keys, ", "))
		}
		if want := unquote(strings.TrimSpace(v)); got != want {
			r.fail(s, "state %s is %q, want %q", k, got, want)
		}
	case "reads":
		r.open()
		f := strings.Fields(s.arg)
		if len(f) != 2 {
			r.fail(s, "want DIR N")
		}
		want, err := strconv.Atoi(f[1])
		if err != nil {
			r.fail(s, "%v", err)
		}
		if got := r.fsys.ReadCount(f[0]); got != want {
			r.fail(s, "read %d times (%v), want %d", got, slices.Clone(r.fsys.Reads()), want)
		}
	default:
		r.fail(s, "unknown command")
	}
}

// text is a step's argument, unquoted if it is quoted.
func (r *runner[M]) text(s step) string { return unquote(s.arg) }

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '`') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1] // As VHS quotes with ' and ` too.
	}
	if u, err := strconv.Unquote(s); err == nil && strings.HasPrefix(s, `"`) {
		return u
	}
	return s
}

// finish compares the snapshots taken with the golden file.
func (r *runner[M]) finish() {
	r.t.Helper()
	golden := strings.TrimSuffix(r.file, ".txt") + ".golden"
	got := r.golden.String()
	if *update {
		if got == "" {
			_ = os.Remove(golden)
			return
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			r.t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if os.IsNotExist(err) {
		if got != "" {
			r.t.Fatalf("%s has no golden file; run go test -update-screens", filepath.Base(r.file))
		}
		return
	}
	if err != nil {
		r.t.Fatal(err)
	}
	if string(want) != got {
		r.t.Errorf("screens differ from %s (go test -update-screens writes them):\n%s", filepath.Base(golden), diff(string(want), got))
	}
}

// diff shows the first snapshot that differs, side by side in order.
func diff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < max(len(w), len(g)); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			lo := max(0, i-8)
			var out strings.Builder
			fmt.Fprintf(&out, "first difference at line %d\n--- want\n", i+1)
			out.WriteString(strings.Join(w[lo:min(len(w), i+4)], "\n"))
			out.WriteString("\n--- got\n")
			out.WriteString(strings.Join(g[lo:min(len(g), i+4)], "\n"))
			return out.String()
		}
	}
	return "(no difference found)"
}
