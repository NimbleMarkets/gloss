package browsetest_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

func TestFS(t *testing.T) {
	f := browsetest.NewFS("/a/b/c.txt 12", "a/d/", "e.md @2025-12-25", "g.txt = hello")
	entries, err := f.ReadDir("a")
	if err != nil || len(entries) != 2 || entries[0].Name() != "b" || entries[1].Name() != "d" {
		t.Fatalf("ReadDir(a) = %v, %v", entries, err)
	}
	if !entries[0].IsDir() || !entries[1].IsDir() {
		t.Errorf("a's children are folders")
	}
	root, _ := f.ReadDir(".")
	if len(root) != 3 {
		t.Fatalf("root has %d entries, want a, e.md, g.txt", len(root))
	}
	info, _ := root[1].Info()
	if info.ModTime().Format("2006-01-02") != "2025-12-25" {
		t.Errorf("e.md is dated %v", info.ModTime())
	}
	if data, err := fs.ReadFile(f, "g.txt"); err != nil || string(data) != "hello" {
		t.Errorf("ReadFile = %q, %v", data, err)
	}
	if info, _ := fs.Stat(f, "a/b/c.txt"); info == nil || info.Size() != 12 {
		t.Errorf("c.txt is %v", info)
	}
	if _, err := f.ReadDir("nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadDir(nope) = %v", err)
	}
	f.Fail("a", fs.ErrPermission)
	if _, err := f.ReadDir("a"); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("failing folder gave %v", err)
	}
	if n := f.ReadCount("a"); n != 2 {
		t.Errorf("a read %d times, want 2", n)
	}
	f.ResetReads()
	if len(f.Reads()) != 0 {
		t.Errorf("reads not reset")
	}
}

func TestParseKey(t *testing.T) {
	for name, want := range map[string]string{
		"enter": "enter", "ctrl+n": "ctrl+n", "shift+tab": "shift+tab", "alt+up": "alt+up",
		"a": "a", "G": "G", "space": "space", "+": "+", "ctrl++": "ctrl++", "pgdown": "pgdown", "backspace": "backspace",
	} {
		k, err := browsetest.ParseKey(name)
		if err != nil {
			t.Errorf("ParseKey(%q): %v", name, err)
			continue
		}
		if k.String() != want {
			t.Errorf("ParseKey(%q).String() = %q, want %q", name, k.String(), want)
		}
	}
	for _, bad := range []string{"", "hyper+a", "wibble"} {
		if _, err := browsetest.ParseKey(bad); err == nil {
			t.Errorf("ParseKey(%q) should fail", bad)
		}
	}
}

// toy lists a folder, one read at a time, and shows what was typed.
type toy struct {
	fsys  fs.ReadDirFS
	typed string
	names []string
	w, h  int
}

type toyRead struct{ names []string }
type toyTick struct{}

func (m toy) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			entries, _ := m.fsys.ReadDir(".")
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			return toyRead{names}
		},
		func() tea.Msg { time.Sleep(time.Hour); return toyTick{} }, // A timer, never waited for.
	)
}

func (m toy) Update(msg tea.Msg) (toy, tea.Cmd) {
	switch msg := msg.(type) {
	case toyRead:
		m.names = msg.names
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if msg.String() == "enter" {
			return m, func() tea.Msg { return toyRead{[]string{"entered"}} }
		}
		m.typed += msg.String()
	case tea.MouseClickMsg:
		m.typed += "<click>"
	}
	return m, nil
}

func (m toy) View() string { return "typed: " + m.typed + "\n" + strings.Join(m.names, "\n") }

func TestDriver(t *testing.T) {
	d := browsetest.New(t, toy{fsys: browsetest.NewFS("x.txt", "y.txt")}, 30, 5)
	d.Patience = 20 * time.Millisecond
	if !d.Screen().Contains("x.txt") {
		t.Fatalf("Init's read was not delivered:\n%s", d.Screen())
	}
	d.Press("a", "ctrl+b")
	d.Type("hi")
	d.Click(1, 1)
	if got := d.Screen().Row(0); got != "typed: actrl+bhi<click>" {
		t.Errorf("row 0 is %q", got)
	}
	d.Press("enter")
	if !d.Screen().Contains("entered") {
		t.Errorf("a command from an update was not run")
	}
	if r, c, ok := d.Screen().Find("entered"); !ok || r != 1 || c != 0 {
		t.Errorf("Find = %d,%d,%v", r, c, ok)
	}
	if err := d.Screen().Fits(); err != nil {
		t.Errorf("fits: %v", err)
	}
	if err := (browsetest.Screen{Raw: "abc\ndef", Width: 2, Height: 5}).Fits(); err == nil {
		t.Errorf("a wide screen should not fit")
	}
	if err := (browsetest.Screen{Raw: "a\nb\nc", Width: 5, Height: 2}).Fits(); err == nil {
		t.Errorf("a tall screen should not fit")
	}
}

func TestScript(t *testing.T) {
	dir := t.TempDir()
	script := "-- fs --\nfoo.txt\nbar/\n-- script --\nsize 30 4\nnote hello\ntype hi\nsnapshot one\nexpect \"foo.txt\"\nreject nothing-like-this\nstate typed hi\nreads . 1\n"
	file := filepath.Join(dir, "s.txt")
	if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := browsetest.Config[toy]{
		New:   func(f *browsetest.FS, _ string, _ map[string]string) toy { return toy{fsys: f} },
		Probe: func(m toy) map[string]string { return map[string]string{"typed": m.typed} },
	}
	if err := os.WriteFile(filepath.Join(dir, "s.golden"), []byte("# hello\n\n=== one (30x4)\ntyped: hi\nbar\nfoo.txt\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	browsetest.RunScript(t, cfg, file)
}
