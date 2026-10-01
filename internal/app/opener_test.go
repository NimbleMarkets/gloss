//go:build !js

package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/examples"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/charmbracelet/x/ansi"
)

// pumpPatience is how long pump waits on one command before taking it for a
// timer.
var pumpPatience = 100 * time.Millisecond

// pump runs a command and what follows from it, as the runtime would, but
// does not wait on timers such as the cursor's blink. A command that is slow
// rather than a timer is still heard: while a document is loading, pump waits
// for the commands it passed over, so a slow machine loads what a fast one does.
func pump(m *Model, cmd tea.Cmd, depth int) {
	late, waiting := make(chan tea.Msg), 0
	var run func(tea.Cmd, int)
	deliver := func(msg tea.Msg, depth int) {
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				run(c, depth+1)
			}
			return
		}
		// A sequence is a slice of commands under a name of its own.
		if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
			for i := 0; i < v.Len(); i++ {
				if c, ok := v.Index(i).Interface().(tea.Cmd); ok {
					run(c, depth+1)
				}
			}
			return
		}
		if msg == nil {
			return
		}
		_, next := m.Update(msg)
		run(next, depth+1)
	}
	stop := make(chan struct{})
	defer close(stop)
	run = func(cmd tea.Cmd, depth int) {
		if cmd == nil || depth > 12 {
			return
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		select {
		case msg := <-done:
			deliver(msg, depth)
		case <-time.After(pumpPatience):
			waiting++
			go func() {
				select {
				case msg := <-done:
					select {
					case late <- msg:
					case <-stop:
					}
				case <-stop:
				}
			}()
		}
	}
	run(cmd, depth)
	for deadline := time.After(30 * time.Second); m.loading && waiting > 0; {
		select {
		case msg := <-late:
			waiting--
			deliver(msg, 0)
		case <-deadline:
			return
		}
	}
}

func send(m *Model, msgs ...tea.Msg) {
	for _, msg := range msgs {
		_, cmd := m.Update(msg)
		pump(m, cmd, 0)
	}
}

func typed(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

var enter, escape = tea.KeyPressMsg{Code: tea.KeyEnter}, tea.KeyPressMsg{Code: tea.KeyEscape}
var toggleHidden = tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}

// line finds the row of the view that lists name, with its styling.
func line(t *testing.T, m *Model, name string) string {
	t.Helper()
	for _, row := range strings.Split(m.View().Content, "\n") {
		if strings.HasSuffix(strings.TrimRight(ansi.Strip(row), " "), " "+name) {
			return row
		}
	}
	t.Fatalf("%s is not listed:\n%s", name, ansi.Strip(m.View().Content))
	return ""
}

func listed(m *Model, name string) bool {
	for _, row := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if strings.HasSuffix(strings.TrimRight(row, " "), " "+name) {
			return true
		}
	}
	return false
}

// folder holds a picture, a drawing, two things gloss cannot show (one of
// them with no extension to judge it by), and a subfolder.
func folder(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(dir, "alpha.png"))
	if err := os.WriteFile(filepath.Join(dir, "beta.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "trips"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"notes.dmg": "\x00\x01junk", "LICENSE": "MIT", "trips/itinerary.txt": "day one"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writePNG(t, filepath.Join(dir, "trips", "coast.png"))
	return dir
}

func browsing(t *testing.T, dir string) *Model {
	t.Helper()
	m := viewing(t, Options{Files: []string{filepath.Join(dir, "alpha.png")}}, "png")
	m.zoom = 2
	send(m, tea.WindowSizeMsg{Width: 240, Height: 30}, press("O"))
	if m.opener == nil {
		t.Fatal("O did not open the file browser")
	}
	return m
}

func TestOpenerListsTheFolderOfTheCurrentFile(t *testing.T) {
	dir := folder(t)
	m := browsing(t, dir)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"alpha.png", "beta.svg", "notes.dmg", "trips", dir} {
		if !strings.Contains(view, want) {
			t.Fatalf("browser lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "coast.png") {
		t.Fatalf("browser lists a subfolder's files:\n%s", view)
	}
	if m.index != 0 || m.zoom != 2 || len(m.opts.Files) != 1 {
		t.Fatal("browsing changed the document")
	}
}

func TestOpenerTypingFiltersRatherThanCommands(t *testing.T) {
	m := browsing(t, folder(t))
	send(m, typed("q?mO")...)
	if m.opener == nil || m.help || m.listing() || m.opener.picker.FilterValue() != "q?mO" {
		t.Fatalf("typed letters acted as commands: opener=%v help=%v menu=%v", m.opener != nil, m.help, m.listing())
	}
	m = browsing(t, folder(t))
	send(m, typed(".svg")...)
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "beta.svg") || strings.Contains(view, "alpha.png") {
		t.Fatalf("filter:\n%s", view)
	}
}

func TestOpenerOpensTheChosenFile(t *testing.T) {
	dir := folder(t)
	m := browsing(t, dir)
	send(m, typed(".svg")...)
	send(m, enter)
	if m.opener != nil {
		t.Fatal("the browser stayed open after a file was chosen")
	}
	if len(m.opts.Files) != 2 || m.opts.Files[1] != filepath.Join(dir, "beta.svg") || m.index != 1 || m.zoom != 0 || m.kind != "svg" {
		t.Fatalf("files=%q index=%d zoom=%d kind=%q", m.opts.Files, m.index, m.zoom, m.kind)
	}
}

func TestOpenerWalksIntoFolders(t *testing.T) {
	dir := folder(t)
	m := browsing(t, dir)
	send(m, typed("trips")...)
	send(m, enter)
	view := ansi.Strip(m.View().Content)
	if m.opener == nil || !strings.Contains(view, "coast.png") || strings.Contains(view, "alpha.png") || !strings.Contains(view, filepath.Join(dir, "trips")) {
		t.Fatalf("inside the folder:\n%s", view)
	}
	send(m, typed("coast")...)
	send(m, enter)
	if m.opener != nil || len(m.opts.Files) != 2 || m.opts.Files[1] != filepath.Join(dir, "trips", "coast.png") {
		t.Fatalf("opener=%v files=%q", m.opener != nil, m.opts.Files)
	}
}

func TestEitherCaseOfOOpensTheBrowser(t *testing.T) {
	for _, key := range []string{"o", "O"} {
		m := viewing(t, Options{Files: []string{filepath.Join(folder(t), "alpha.png")}}, "png")
		send(m, press(key))
		if m.opener == nil {
			t.Fatalf("%s did not open the browser", key)
		}
	}
}

func TestProjectionMovesToFive(t *testing.T) {
	m := viewing(t, Options{Files: []string{filepath.Join(folder(t), "alpha.png")}}, "stl")
	m.chart = charts.New(100, 28, charts.WithRenderMode(charts.Software))
	before := m.chart.Camera().Projection
	send(m, press("5"))
	if m.chart.Camera().Projection == before {
		t.Fatal("5 did not change the projection")
	}
	send(m, press("5"))
	if m.chart.Camera().Projection != before {
		t.Fatal("5 did not change the projection back")
	}
	send(m, press("o"))
	if m.opener == nil || m.chart.Camera().Projection != before {
		t.Fatal("o must open the browser and leave the projection alone")
	}
}

func TestOpenerGreysWhatItCannotShow(t *testing.T) {
	m := browsing(t, folder(t))
	const grey = "38;5;243"
	for name, want := range map[string]bool{"notes.dmg": true, "alpha.png": false, "beta.svg": false, "trips": false, "LICENSE": false} {
		if got := strings.Contains(line(t, m, name), grey); got != want {
			t.Errorf("%s greyed=%v, want %v: %q", name, got, want, line(t, m, name))
		}
	}
	send(m, typed(".dmg")...)
	send(m, enter)
	if m.opener == nil || len(m.opts.Files) != 1 || m.note != "" {
		t.Fatalf("a greyed file was chosen: opener=%v files=%q note=%q", m.opener != nil, m.opts.Files, m.note)
	}
}

func TestOpenerGreysNothingWhenTheTypeIsForced(t *testing.T) {
	dir := folder(t)
	m := viewing(t, Options{Files: []string{filepath.Join(dir, "alpha.png")}, Type: "svg"}, "svg")
	send(m, tea.WindowSizeMsg{Width: 240, Height: 30}, press("o"))
	if strings.Contains(line(t, m, "notes.dmg"), "38;5;243") {
		t.Fatal("--type treats every file as that format")
	}
}

func TestOpenerHidesWhatItCannotShowOnRequest(t *testing.T) {
	dir := folder(t)
	m := browsing(t, dir)
	if view := m.View().Content; !strings.Contains(view, "Ctrl-T hide greyed") {
		t.Fatalf("hint:\n%s", view)
	}
	send(m, toggleHidden)
	for name, want := range map[string]bool{"notes.dmg": false, "alpha.png": true, "beta.svg": true, "trips": true, "LICENSE": true} {
		if got := listed(m, name); got != want {
			t.Errorf("%s listed=%v, want %v:\n%s", name, got, want, ansi.Strip(m.View().Content))
		}
	}
	if view := m.View().Content; !strings.Contains(view, "Ctrl-T show greyed") {
		t.Fatalf("hint:\n%s", view)
	}
	if m.opener.picker.FilterValue() != "" {
		t.Fatalf("the toggle typed %q into the filter", m.opener.picker.FilterValue())
	}
	// A typed path lists another folder; the choice applies there too.
	send(m, typed("trips/")...)
	if !listed(m, "coast.png") || !listed(m, "itinerary.txt") {
		t.Fatalf("typed path:\n%s", ansi.Strip(m.View().Content))
	}
	send(m, toggleHidden)
	if !listed(m, "coast.png") || !listed(m, "itinerary.txt") || m.opener.picker.FilterValue() != "trips/" {
		t.Fatalf("shown again, filter %q:\n%s", m.opener.picker.FilterValue(), ansi.Strip(m.View().Content))
	}
	send(m, toggleHidden, escape, press("o"))
	if m.opener == nil || listed(m, "notes.dmg") || !listed(m, "alpha.png") {
		t.Fatalf("the choice did not last:\n%s", ansi.Strip(m.View().Content))
	}
}

func TestOpenerStaysOpenOnAFileItCannotShow(t *testing.T) {
	m := browsing(t, folder(t))
	send(m, typed("notes.dmg")...)
	send(m, enter)
	// A binary file is greyed: Enter on it does nothing.
	if m.opener == nil || len(m.opts.Files) != 1 || m.index != 0 || m.zoom != 2 {
		t.Fatalf("opener=%v files=%q", m.opener != nil, m.opts.Files)
	}
	if len(m.Skipped()) != 0 {
		t.Fatalf("a browsing misstep is not worth reporting on exit: %q", m.Skipped())
	}
}

func TestOpenerCancels(t *testing.T) {
	m := browsing(t, folder(t))
	send(m, typed(".svg")...)
	send(m, escape)
	if m.opener != nil || len(m.opts.Files) != 1 || m.index != 0 || m.zoom != 2 || m.loading {
		t.Fatal("Esc did not leave the document as it was")
	}
	if view := m.View().Content; strings.Contains(view, "beta.svg") {
		t.Fatalf("the browser is still drawn:\n%s", view)
	}
}

func TestOpenerTakesADropToo(t *testing.T) {
	dir := folder(t)
	m := browsing(t, dir)
	send(m, tea.PasteMsg{Content: filepath.Join(dir, "beta.svg")})
	if m.opener != nil || len(m.opts.Files) != 2 || m.index != 1 {
		t.Fatalf("opener=%v files=%q index=%d", m.opener != nil, m.opts.Files, m.index)
	}
}

func TestOpenerFromAnEmptySession(t *testing.T) {
	dir := folder(t)
	t.Chdir(dir)
	m := viewing(t, Options{}, "")
	if view := m.View().Content; !strings.Contains(view, "press o to browse") {
		t.Fatalf("the empty screen does not mention the browser:\n%s", view)
	}
	send(m, tea.WindowSizeMsg{Width: 240, Height: 30}, press("O"))
	if m.opener == nil {
		t.Fatal("O did nothing")
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "alpha.png") || !strings.Contains(view, dir) {
		t.Fatalf("browser:\n%s", view)
	}
	send(m, typed("alpha")...)
	send(m, enter)
	if m.opener != nil || len(m.opts.Files) != 1 || m.index != 0 || m.kind != "png" {
		t.Fatalf("opener=%v files=%q kind=%q", m.opener != nil, m.opts.Files, m.kind)
	}
}

func TestOpenerNeedsAFilesystem(t *testing.T) {
	m := viewing(t, Options{Files: append([]string(nil), examples.Names...), FilesFS: examples.Files}, "png")
	send(m, press("O"))
	if m.opener != nil {
		t.Fatal("the embedded gallery has no folders to browse")
	}
	if view := m.View().Content; strings.Contains(view, "o browse") {
		t.Fatalf("the hint offers a browser that is not there:\n%s", view)
	}
}

func TestOpenerStatusKeepsTheEndOfALongPath(t *testing.T) {
	dir := folder(t)
	m := browsing(t, dir)
	send(m, tea.WindowSizeMsg{Width: 50, Height: 20})
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	status := lines[len(lines)-2]
	if !strings.Contains(status, "…") || !strings.Contains(status, filepath.Base(dir)) || !strings.Contains(status, "by name") || strings.Contains(status, "/private") {
		t.Fatalf("status: %q", status)
	}
}

func TestOpenerFitsTheTerminal(t *testing.T) {
	dir := folder(t)
	long := filepath.Join(dir, strings.Repeat("a-very-long-name-", 12)+".png")
	writePNG(t, long)
	m := browsing(t, dir)
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 30, Height: 8}, {Width: 9, Height: 4}, {Width: 1, Height: 1}} {
		send(m, size)
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) > size.Height {
			t.Fatalf("view height %d exceeds %d", len(lines), size.Height)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size.Width {
				t.Fatalf("line exceeds width %d: %q", size.Width, line)
			}
		}
	}
}

func TestBrowseOptionStartsInThatFolder(t *testing.T) {
	dir := folder(t)
	m := New(Options{Browse: filepath.Join(dir, "trips"), Render: "glyph", Page: 1, DPI: 72})
	t.Cleanup(func() { m.Close() })
	pump(m, m.Init(), 0)
	send(m, tea.WindowSizeMsg{Width: 240, Height: 30})
	if m.opener == nil {
		t.Fatal("the browser did not open")
	}
	if view := ansi.Strip(m.View().Content); !listed(m, "coast.png") || listed(m, "alpha.png") || !strings.Contains(view, filepath.Join(dir, "trips")) {
		t.Fatalf("browser:\n%s", view)
	}
	send(m, escape)
	if view := m.View().Content; m.opener != nil || !strings.Contains(view, "Drop files here to open") {
		t.Fatalf("Esc must leave the empty viewer:\n%s", view)
	}
}

func TestBrowseOptionOpensOverTheFirstFile(t *testing.T) {
	dir := folder(t)
	m := New(Options{Files: []string{filepath.Join(dir, "beta.svg")}, Browse: dir, Render: "glyph", Page: 1, DPI: 72})
	t.Cleanup(func() { m.Close() })
	pump(m, m.Init(), 0)
	send(m, tea.WindowSizeMsg{Width: 240, Height: 30})
	if m.opener == nil || !listed(m, "alpha.png") {
		t.Fatalf("browser:\n%s", ansi.Strip(m.View().Content))
	}
	send(m, escape)
	if m.opener != nil || m.index != 0 || m.kind != "svg" {
		t.Fatalf("the file given was not loaded beneath the browser: kind=%q", m.kind)
	}
}

func TestBrowseOptionNeedsAFilesystem(t *testing.T) {
	m := New(Options{Files: append([]string(nil), examples.Names...), FilesFS: examples.Files, Browse: "/", Render: "glyph", Page: 1})
	t.Cleanup(func() { m.Close() })
	pump(m, m.Init(), 0)
	if m.opener != nil {
		t.Fatal("the embedded gallery has no folders to browse")
	}
}

func TestOpenerMarksKindsInsteadOfModes(t *testing.T) {
	dir := folder(t)
	m := browsing(t, dir)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"📁", "📷", "🎨"} {
		if !strings.Contains(view, want) {
			t.Errorf("no %s in:\n%s", want, view)
		}
	}
	if strings.Contains(view, "-rw-") || strings.Contains(view, "drwx") {
		t.Fatalf("modes are shown:\n%s", view)
	}
}

func TestOpenerSortsByNameDateAndKind(t *testing.T) {
	dir := folder(t)
	// beta.svg is the newest, alpha.png the oldest.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "alpha.png"), old, old); err != nil {
		t.Fatal(err)
	}
	m := browsing(t, dir)
	order := func() string {
		var names []string
		for _, line := range plain(m) {
			for _, name := range []string{"alpha.png", "beta.svg", "trips"} {
				if strings.Contains(line, name) {
					names = append(names, name)
				}
			}
		}
		return strings.Join(names, " ")
	}
	status := func() string { return plain(m)[len(plain(m))-2] }
	if order() != "trips alpha.png beta.svg" || !strings.Contains(status(), "by name") {
		t.Fatalf("by name: %s · %s", order(), status())
	}
	send(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if order() != "trips beta.svg alpha.png" || !strings.Contains(status(), "by date") {
		t.Fatalf("by date: %s · %s", order(), status())
	}
	send(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	// Kinds are grouped: pictures before drawings.
	if order() != "trips alpha.png beta.svg" || !strings.Contains(status(), "by kind") {
		t.Fatalf("by kind: %s · %s", order(), status())
	}
	if hints := plain(m)[len(plain(m))-1]; !strings.Contains(hints, "Ctrl-S sort") {
		t.Fatalf("hints: %s", hints)
	}
	// The order is kept for the next folder, and the next browse.
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape}, press("O"))
	if !strings.Contains(status(), "by kind") {
		t.Fatalf("after reopening: %s", status())
	}
}

func TestShiftGGoesToATypedFolder(t *testing.T) {
	dir := folder(t)
	m := browsing(t, dir)
	status := func() string { return plain(m)[len(plain(m))-2] }
	// G with an empty filter asks for a path, starting from the folder shown.
	send(m, press("G"))
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Go to: "+dir+string(filepath.Separator)) || !strings.Contains(view, "trips") {
		t.Fatalf("no go-to prompt, or the folder is not listed:\n%s", view)
	}
	if hints := plain(m)[len(plain(m))-1]; !strings.Contains(hints, "Tab complete") || !strings.Contains(hints, "Enter go") {
		t.Fatalf("hints: %s", hints)
	}
	// Tab completes the folder, and Enter goes there.
	send(m, typed("tr")...)
	send(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Go to: "+filepath.Join(dir, "trips")+string(filepath.Separator)) {
		t.Fatalf("Tab did not complete:\n%s", view)
	}
	send(m, enter)
	if !strings.Contains(status(), filepath.Join(dir, "trips")) || strings.Contains(ansi.Strip(m.View().Content), "Go to: ") {
		t.Fatalf("Enter did not go: %s\n%s", status(), ansi.Strip(m.View().Content))
	}
	if m.opener == nil {
		t.Fatal("the browser closed")
	}
	// Esc in the go-to leaves the browser where it was.
	send(m, press("G"), tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.opener == nil || strings.Contains(ansi.Strip(m.View().Content), "Go to: ") || !strings.Contains(status(), filepath.Join(dir, "trips")) {
		t.Fatalf("Esc: opener=%v status=%s", m.opener != nil, status())
	}
	// With text in the filter, G is a letter.
	send(m, typed("aG")...)
	if !strings.Contains(ansi.Strip(m.View().Content), "Filter: aG") {
		t.Fatalf("G was not typed:\n%s", ansi.Strip(m.View().Content))
	}
}

func TestGoToExpandsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	m := browsing(t, folder(t))
	send(m, press("G"), tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	send(m, typed("~/")...)
	send(m, enter)
	if status := plain(m)[len(plain(m))-2]; !strings.Contains(status, "Open · "+home) {
		t.Fatalf("status: %s", status)
	}
}

func TestOpenerOffersTextFilesUnlessToldNotTo(t *testing.T) {
	dir := folder(t)
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("all:\n\tgo build\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := browsing(t, dir)
	if hints := plain(m)[len(plain(m))-1]; !strings.Contains(hints, "Ctrl-X no text") {
		t.Fatalf("hints: %s", hints)
	}
	// Only the binary file is greyed: text of any name can be chosen.
	send(m, toggleHidden)
	for name, want := range map[string]bool{"notes.dmg": false, "LICENSE": true, "Makefile": true, "alpha.png": true} {
		if got := listed(m, name); got != want {
			t.Errorf("%s listed=%v, want %v:\n%s", name, got, want, ansi.Strip(m.View().Content))
		}
	}
	// Ctrl-X sets text files aside, and again takes them back.
	send(m, tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	for name, want := range map[string]bool{"LICENSE": false, "Makefile": false, "alpha.png": true} {
		if got := listed(m, name); got != want {
			t.Errorf("without text, %s listed=%v, want %v:\n%s", name, got, want, ansi.Strip(m.View().Content))
		}
	}
	if hints := plain(m)[len(plain(m))-1]; !strings.Contains(hints, "Ctrl-X text") {
		t.Fatalf("hints: %s", hints)
	}
	send(m, tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	if !listed(m, "Makefile") {
		t.Fatal("text files did not come back")
	}
	// A text file chosen opens as text.
	send(m, typed("Makef")...)
	send(m, enter)
	if m.opener != nil || len(m.opts.Files) != 2 || m.kind != "text" {
		t.Fatalf("opener=%v files=%q kind=%q", m.opener != nil, m.opts.Files, m.kind)
	}
}

func TestSlashFindsFilesUnderTheFolder(t *testing.T) {
	dir := folder(t)
	m := browsing(t, dir)
	send(m, press("/"))
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Find: ") || !strings.Contains(plain(m)[len(plain(m))-1], "Enter search") {
		t.Fatalf("no find prompt:\n%s", view)
	}
	send(m, typed("png")...)
	send(m, enter)
	view = ansi.Strip(m.View().Content)
	for _, want := range []string{"alpha.png", "trips/coast.png", "2 found"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "beta.svg") {
		t.Errorf("beta.svg found for png:\n%s", view)
	}
	// Enter opens the one under the cursor; the browser closes.
	send(m, tea.KeyPressMsg{Code: tea.KeyDown}, enter)
	if m.opener != nil || len(m.opts.Files) != 2 || filepath.Base(m.opts.Files[1]) != "coast.png" {
		t.Fatalf("opener=%v files=%q", m.opener != nil, m.opts.Files)
	}
	// The browser reopens at the file's folder: a search there with
	// nothing found says so.
	send(m, press("O"), press("/"))
	send(m, typed("*.stl")...)
	send(m, enter)
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "nothing found") {
		t.Fatalf("no match:\n%s", view)
	}
	// Ctrl-A adds every match.
	m = browsing(t, dir)
	send(m, press("/"))
	send(m, typed("images svg")...)
	send(m, enter, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if m.opener != nil || len(m.opts.Files) != 3 {
		t.Fatalf("after Ctrl-A: opener=%v files=%q", m.opener != nil, m.opts.Files)
	}
	m = browsing(t, dir)
	send(m, press("/"), enter)
	// Esc leaves the finder for the listing, then the listing for the viewer.
	send(m, escape)
	if m.opener == nil || strings.Contains(ansi.Strip(m.View().Content), "Find: ") {
		t.Fatal("Esc did not return to the listing")
	}
	send(m, escape)
	if m.opener != nil {
		t.Fatal("Esc did not close the browser")
	}
}
