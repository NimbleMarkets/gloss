package app

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/examples"
	"github.com/NimbleMarkets/gloss/internal/document"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/charmbracelet/x/ansi"
)

const samples = "../../examples/"

func writePNG(t *testing.T, path string) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// deliver runs the probe a drop queues, as the Bubble Tea runtime would.
func deliver(m *Model, msg tea.Msg) bool {
	_, cmd := m.Update(msg)
	if cmd == nil {
		return false
	}
	m.Update(cmd())
	return true
}

func TestPastedPathOpensAfterProbe(t *testing.T) {
	picture := writePNG(t, filepath.Join(t.TempDir(), "my photo.png"))
	m := New(Options{Files: []string{samples + "shapes.svg"}, Render: "glyph", Page: 1, DPI: 72})
	defer m.Close()
	m.kind, m.zoom, m.panX = "svg", 2, .25
	_, cmd := m.Update(tea.PasteMsg{Content: "not a file\n" + strings.ReplaceAll(picture, " ", `\ `) + "\n"})
	if cmd == nil {
		t.Fatal("paste of a path was ignored")
	}
	if len(m.opts.Files) != 1 || m.index != 0 || m.zoom != 2 || m.panX != .25 || m.loading {
		t.Fatal("the current view changed before the new file was selected")
	}
	m.Update(cmd())
	if len(m.opts.Files) != 2 || m.opts.Files[1] != picture {
		t.Fatalf("files=%q", m.opts.Files)
	}
	if m.index != 1 || m.zoom != 0 || m.panX != 0 || !m.loading || m.listing() {
		t.Fatalf("new file not opened: index=%d zoom=%d loading=%v menu=%v", m.index, m.zoom, m.loading, m.listing())
	}
	if len(m.Skipped()) != 0 {
		t.Fatalf("junk line reported as a file: %q", m.Skipped())
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if view := m.View().Content; !strings.Contains(view, "added 1") {
		t.Fatalf("status lacks the drop note:\n%s", view)
	}
	m.Update(press("+"))
	if view := m.View().Content; strings.Contains(view, "added 1") {
		t.Fatal("drop note outlived the next key")
	}
}

func TestPasteThatIsNotAPathIsIgnored(t *testing.T) {
	m := New(Options{Files: []string{samples + "shapes.svg"}, Render: "glyph", Page: 1})
	defer m.Close()
	for _, text := range []string{"hello", "please open the picture for me", "", "https://example.com/a.png"} {
		if deliver(m, tea.PasteMsg{Content: text}) {
			t.Fatalf("paste %q was treated as a drop", text)
		}
	}
	if len(m.opts.Files) != 1 || m.listing() || m.loading {
		t.Fatal("ignored paste changed the session")
	}
}

func TestDropWithNothingUsableKeepsTheView(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.bin"), []byte("\x00\x01 binary"), 0600); err != nil {
		t.Fatal(err)
	}
	huge := filepath.Join(dir, "huge.png")
	f, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("\x89PNG\r\n\x1a\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(document.MaxFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	m := New(Options{Files: []string{samples + "shapes.svg"}, Render: "glyph", Page: 1})
	defer m.Close()
	m.kind, m.zoom = "svg", 3
	if !deliver(m, tea.PasteMsg{Content: filepath.Join(dir, "notes.bin") + "\n" + huge + "\n" + filepath.Join(dir, "missing.png")}) {
		t.Fatal("paths were not probed")
	}
	if len(m.opts.Files) != 1 || m.index != 0 || m.zoom != 3 || m.listing() || m.loading || m.opener != nil {
		t.Fatal("unusable drop changed the view")
	}
	skipped := strings.Join(m.Skipped(), "\n")
	if len(m.Skipped()) != 3 || !strings.Contains(skipped, "unsupported format (skipped)") || !strings.Contains(skipped, "128 MiB (skipped)") {
		t.Fatalf("skipped=%q", skipped)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if view := m.View().Content; !strings.Contains(view, "skipped 3") || strings.Contains(view, "added") {
		t.Fatalf("status:\n%s", view)
	}
}

func TestDropSeveralOpensMenuOnFirstNewFile(t *testing.T) {
	m := New(Options{Files: []string{samples + "shapes.svg"}, Render: "glyph", Page: 1, DPI: 72})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.kind, m.page, m.zoom, m.help = "svg", 1, 2, true
	if !deliver(m, DropMsg{Paths: []string{t.TempDir(), samples + "shapes.svg", samples + "readme.md", samples + "landscape.png"}}) {
		t.Fatal("drop was ignored")
	}
	if len(m.opts.Files) != 3 {
		t.Fatalf("files=%q", m.opts.Files)
	}
	if !m.listing() || m.help || m.selection != 1 || m.index != 0 || m.zoom != 2 {
		t.Fatalf("menu=%v help=%v selection=%d index=%d zoom=%d", m.listing(), m.help, m.selection, m.index, m.zoom)
	}
	view := m.View().Content
	for _, want := range []string{"readme.md", "landscape.png", "added 2", "skipped 1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("menu lacks %q:\n%s", want, view)
		}
	}
	m.menuKey("enter")
	if m.listing() || m.index != 1 || m.zoom != 0 || !m.loading {
		t.Fatal("Enter did not open the dropped file")
	}
}

func TestDropDoesNotDuplicateListedFiles(t *testing.T) {
	m := New(Options{Files: []string{samples + "shapes.svg", samples + "readme.md"}, Render: "glyph", Page: 1})
	defer m.Close()
	again, err := filepath.Abs(samples + "readme.md")
	if err != nil {
		t.Fatal(err)
	}
	if !deliver(m, tea.PasteMsg{Content: again + "\n" + again}) {
		t.Fatal("drop was ignored")
	}
	if len(m.opts.Files) != 2 || m.index != 1 || m.listing() {
		t.Fatalf("files=%q index=%d menu=%v", m.opts.Files, m.index, m.listing())
	}
}

func TestDropWhileBrowsingTheMenu(t *testing.T) {
	m := New(Options{Files: []string{samples + "shapes.svg", samples + "readme.md"}, Render: "glyph", Menu: true, Page: 1})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.Init()
	if !deliver(m, DropMsg{Paths: []string{samples + "landscape.png"}}) {
		t.Fatal("drop was ignored")
	}
	if m.listing() || m.index != 2 || !m.loading {
		t.Fatalf("menu=%v index=%d loading=%v", m.listing(), m.index, m.loading)
	}
}

func TestEmbeddedSessionTakesDropsButNotPastedPaths(t *testing.T) {
	files := &document.Overlay{Base: examples.Files}
	m := New(Options{Files: append([]string(nil), examples.Names...), FilesFS: files, Render: "glyph", Menu: true, Page: 1})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if deliver(m, tea.PasteMsg{Content: writePNG(t, filepath.Join(t.TempDir(), "host.png"))}) {
		t.Fatal("a host path was accepted without a host filesystem")
	}
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	first, _ := files.Add("first.png", b.Bytes())
	second, _ := files.Add("second.png", b.Bytes())
	junk, _ := files.Add("notes.bin", []byte{0, 1, 2})
	if !deliver(m, DropMsg{Paths: []string{junk, first, second}}) {
		t.Fatal("drop was ignored")
	}
	if n := len(examples.Names); len(m.opts.Files) != n+2 || m.selection != n || !m.listing() {
		t.Fatalf("files=%q selection=%d", m.opts.Files, m.selection)
	}
	if view := m.View().Content; !strings.Contains(view, "dropped/first.png") || !strings.Contains(view, "dropped/second.png") {
		t.Fatalf("dropped files are not listed:\n%s", view)
	}
	if got := strings.Join(m.Skipped(), "\n"); !strings.Contains(got, "notes.bin: unsupported format (skipped)") {
		t.Fatalf("skipped=%q", got)
	}
}

func TestDropLeavesTheCameraAlone(t *testing.T) {
	m := New(Options{Files: []string{samples + "gloss.stl"}, Render: "glyph", Page: 1})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m.kind = "stl"
	m.chart = charts.New(80, 28, charts.WithRenderMode(charts.Software))
	camera := m.chart.Camera()
	camera.Beta += 30
	m.chart.SetCamera(camera)
	_, cmd := m.Update(tea.PasteMsg{Content: samples + "shapes.svg " + samples + "readme.md"})
	if cmd == nil {
		t.Fatal("drop was ignored")
	}
	if m.chart == nil || m.chart.Camera() != camera {
		t.Fatal("a pending drop moved the camera")
	}
	m.Update(cmd())
	if !m.listing() || m.savedCamera == nil || *m.savedCamera != camera {
		t.Fatal("the menu opened by a drop lost the camera")
	}
}

func TestEmptySessionIsADropTarget(t *testing.T) {
	m := New(Options{Render: "glyph", Page: 1, DPI: 72, Menu: true, Preview: true})
	defer m.Close()
	m.Init()
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 12, Height: 4}, {Width: 1, Height: 1}} {
		m.Update(size)
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
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if view := m.View().Content; !strings.Contains(view, "Drop files here to open") || !strings.Contains(view, "3D:     STL and 3MF meshes") {
		t.Fatalf("no drop target, or no word on what can be opened:\n%s", view)
	}
	// The name is drawn above, and outranks the rest as the screen
	// shortens: the list of formats goes first, then the words on what to
	// do, and the name stays.
	const name = `\_| |_ \_/ __) __)`
	for _, c := range []struct {
		height              int
		banner, text, short bool
	}{{30, true, true, false}, {16, true, true, true}, {12, true, true, false}, {9, true, false, false}, {5, true, false, false}} {
		m.Update(tea.WindowSizeMsg{Width: 100, Height: c.height})
		view := ansi.Strip(m.View().Content)
		if strings.Contains(view, name) != c.banner || strings.Contains(view, "Drop files here to open") != c.text || strings.Contains(view, document.FormatsShort) != c.short {
			t.Fatalf("%d rows: banner=%v text=%v short list=%v:\n%s", c.height, c.banner, c.text, c.short, view)
		}
	}
	// Where even the name does not fit, it is said in letters.
	for _, c := range []struct {
		size tea.WindowSizeMsg
		want string
	}{{tea.WindowSizeMsg{Width: 100, Height: 3}, "GLOSS -- drag here"}, {tea.WindowSizeMsg{Width: 19, Height: 30}, "GLOSS -- drag here"}, {tea.WindowSizeMsg{Width: 12, Height: 30}, "GLOSS"}} {
		m.Update(c.size)
		if view := ansi.Strip(m.View().Content); !strings.Contains(view, c.want) || strings.Contains(view, name) || (c.want == "GLOSS" && strings.Contains(view, "drag")) {
			t.Fatalf("%dx%d:\n%s", c.size.Width, c.size.Height, view)
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	// The labels line up when the block is centred, and a short screen
	// gets the short list.
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	starts := map[int]bool{}
	for _, line := range lines {
		if i := strings.Index(line, "Images: "); i >= 0 {
			starts[i] = true
		}
		if i := strings.Index(line, "3D:     "); i >= 0 {
			starts[i] = true
		}
	}
	if len(starts) != 1 {
		t.Fatalf("labels do not line up:\n%s", ansi.Strip(m.View().Content))
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	if view := ansi.Strip(m.View().Content); strings.Contains(view, "Images: ") || !strings.Contains(view, document.FormatsShort) {
		t.Fatalf("short screen:\n%s", view)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, key := range []string{"m", "]", "[", "n", "p", "R", "r", "+", "g", "f"} {
		m.Update(press(key))
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.listing() || m.loading || m.preview != nil || m.zoom != 0 {
		t.Fatalf("keys acted on an empty session: menu=%v loading=%v zoom=%d", m.listing(), m.loading, m.zoom)
	}
	m.Update(press("?"))
	if view := m.View().Content; !strings.Contains(view, "a visual pager") || !strings.Contains(view, "gloss --help") {
		t.Fatalf("help is unavailable before the first file, or does not say where the options are:\n%s", view)
	}
	m.Update(press("?"))
	if !deliver(m, tea.PasteMsg{Content: filepath.Join(t.TempDir(), "missing.png")}) || len(m.opts.Files) != 0 || m.loading {
		t.Fatal("an unusable drop must leave the target waiting")
	}
	if view := m.View().Content; !strings.Contains(view, "Drop files here to open") || !strings.Contains(view, "skipped 1") {
		t.Fatalf("status:\n%s", view)
	}
	picture := writePNG(t, filepath.Join(t.TempDir(), "first.png"))
	if !deliver(m, tea.PasteMsg{Content: picture}) {
		t.Fatal("drop was ignored")
	}
	if len(m.opts.Files) != 1 || m.index != 0 || !m.loading || m.listing() {
		t.Fatalf("files=%q index=%d loading=%v menu=%v", m.opts.Files, m.index, m.loading, m.listing())
	}
	if view := m.View().Content; !strings.Contains(view, "first.png") || strings.Contains(view, "Drop files here") {
		t.Fatalf("first file not shown:\n%s", view)
	}
}

func TestEmptySessionOpensMenuForSeveralFiles(t *testing.T) {
	m := New(Options{Render: "glyph", Page: 1, DPI: 72})
	defer m.Close()
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !deliver(m, tea.PasteMsg{Content: samples + "shapes.svg " + samples + "readme.md"}) {
		t.Fatal("drop was ignored")
	}
	if !m.listing() || m.selection != 0 || len(m.opts.Files) != 2 {
		t.Fatalf("menu=%v selection=%d files=%q", m.listing(), m.selection, m.opts.Files)
	}
	m.menuKey("enter")
	if m.listing() || m.index != 0 || !m.loading {
		t.Fatal("Enter did not open the first dropped file")
	}
}

func TestDroppedFolderOpensTheBrowserThere(t *testing.T) {
	dir := folder(t)
	m := viewing(t, Options{}, "")
	send(m, tea.PasteMsg{Content: dir + "\n"})
	if m.opener == nil || m.opener.dir != dir {
		t.Fatalf("browser not opened in the folder: opener=%v", m.opener != nil)
	}
	if len(m.Skipped()) != 0 {
		t.Fatalf("the folder was reported skipped: %q", m.Skipped())
	}
	// A folder beside a file is skipped, as it is on the command line.
	m = viewing(t, Options{}, "")
	send(m, tea.PasteMsg{Content: filepath.Join(dir, "alpha.png") + "\n" + dir + "\n"})
	if m.opener != nil || len(m.opts.Files) != 1 || len(m.Skipped()) != 1 || !strings.Contains(m.Skipped()[0], "is a directory") {
		t.Fatalf("files=%q skipped=%q", m.opts.Files, m.Skipped())
	}
}
