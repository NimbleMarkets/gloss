package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/coder/websocket"
)

func serving(t *testing.T, opts app.Options) *server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	opts.Render, opts.Render3D, opts.Page, opts.DPI = "glyph", "software", 1, 72
	s, err := serve(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Close()
		s.Discard()
	})
	return s
}

func TestServedRequestIsAccessibleAndEscaped(t *testing.T) {
	request := "Choose <script>alert('invoice')</script> & check totals\n" + strings.Repeat("Full request. ", 80)
	s := &server{token: "test-token", prompt: request, pick: true}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1234/test-token/", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	page := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(page, `<h1 id="request">Choose &lt;script&gt;`) ||
		strings.Contains(page, "<script>alert") || strings.Count(page, "Full request.") != 80 ||
		!strings.Contains(page, "aria-labelledby=\"request\"") || !strings.Contains(page, "press Enter in the viewer to send") {
		t.Fatalf("request missing, truncated, or unsafe: %d\n%s", w.Code, page)
	}
	for _, asset := range []string{"pickers.mjs", "drop.mjs"} {
		w = httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1234/test-token/"+asset, nil))
		if w.Code != http.StatusOK {
			t.Errorf("picker dependency %s: %d", asset, w.Code)
		}
	}
}

func get(t *testing.T, target string, edit func(*http.Request)) (int, string) {
	t.Helper()
	r, err := http.NewRequest("GET", target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(r)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

type upload struct {
	name string
	data []byte
}

func drop(t *testing.T, target string, edit func(*http.Request), files ...upload) (int, []string) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, f := range files {
		part, err := form.CreateFormFile("file", f.name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(f.data)
	}
	form.Close()
	r, err := http.NewRequest("POST", target, &body)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", form.FormDataContentType())
	if edit != nil {
		edit(r)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var reply struct{ Paths []string }
	_ = json.NewDecoder(resp.Body).Decode(&reply)
	return resp.StatusCode, reply.Paths
}

func picture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestServedPageNeedsItsToken(t *testing.T) {
	s := serving(t, app.Options{Pick: true})
	u, err := url.Parse(s.URL)
	if err != nil || u.Hostname() != "127.0.0.1" || len(strings.Trim(u.Path, "/")) < 32 {
		t.Fatalf("URL %q must be on this machine and hard to guess", s.URL)
	}
	root := "http://" + u.Host
	for _, path := range []string{"/", "/drop.mjs", "/ws", "/static/booba/booba.js", "/" + strings.Repeat("0", 32) + "/"} {
		if code, _ := get(t, root+path, nil); code != http.StatusNotFound {
			t.Errorf("GET %s without the token: %d", path, code)
		}
	}
	if code, _ := drop(t, root+"/drop", nil, upload{"a.png", picture(t)}); code != http.StatusNotFound {
		t.Errorf("a drop without the token: %d", code)
	}
	code, page := get(t, s.URL, nil)
	if code != 200 || !strings.Contains(page, "Drop files") || !strings.Contains(page, "serve.mjs") {
		t.Fatalf("page: %d\n%s", code, page)
	}
	for _, path := range []string{"serve.mjs", "drop.mjs", "upload.mjs", "static/booba/booba.js", "static/ghostty-web/ghostty-web.js"} {
		if code, body := get(t, s.URL+path, nil); code != 200 || len(body) < 100 {
			t.Errorf("GET %s: %d, %d bytes", path, code, len(body))
		}
	}
	// A page elsewhere that resolves its own name to this machine is refused.
	if code, _ := get(t, s.URL, func(r *http.Request) { r.Host = "evil.example:" + u.Port() }); code != http.StatusForbidden {
		t.Errorf("a foreign Host was served: %d", code)
	}
	// The viewer behind the page answers only to the page's server.
	if _, _, err := websocket.Dial(context.Background(), "ws://"+s.backend+"/ws", nil); err == nil {
		t.Error("the viewer can be reached around the token")
	}
}

func TestServedDropsAreKeptForTheCaller(t *testing.T) {
	s := serving(t, app.Options{Pick: true})
	code, paths := drop(t, s.URL+"drop", nil, upload{"../../etc/pass wd.png", picture(t)}, upload{`C:\Users\me\plan.svg`, []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")}, upload{"pass wd.png", []byte("another")})
	if code != 200 || len(paths) != 3 {
		t.Fatalf("code=%d paths=%q", code, paths)
	}
	dir := filepath.Dir(paths[0])
	for i, want := range []string{"pass wd.png", "plan.svg", "pass wd-2.png"} {
		if filepath.Base(paths[i]) != want || filepath.Dir(paths[i]) != dir || !filepath.IsAbs(paths[i]) {
			t.Errorf("file %d kept as %q, want %q beside the others", i, paths[i], want)
		}
	}
	if got, err := os.ReadFile(paths[0]); err != nil || !bytes.Equal(got, picture(t)) {
		t.Errorf("contents: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 || !strings.HasPrefix(filepath.Base(dir), "gloss-") {
		t.Errorf("the folder of drops must be the user's alone: %v %v", info, err)
	}
	// They outlive the server: the caller has yet to read them.
	s.Close()
	if _, err := os.Stat(paths[0]); err != nil {
		t.Errorf("the drop went with the server: %v", err)
	}
	os.RemoveAll(dir)
}

func TestServedDropsAreBounded(t *testing.T) {
	s := serving(t, app.Options{Pick: true})
	if code, paths := drop(t, s.URL+"drop", nil, upload{"huge.png", make([]byte, document.MaxFileBytes+1)}); code != http.StatusRequestEntityTooLarge || len(paths) != 0 {
		t.Errorf("a drop over 128 MiB: %d %q", code, paths)
	}
	if code, paths := drop(t, s.URL+"drop", nil, upload{"empty.png", nil}, upload{"..", []byte("x")}); code != http.StatusBadRequest || len(paths) != 0 {
		t.Errorf("nothing usable: %d %q", code, paths)
	}
	cross := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
	if code, paths := drop(t, s.URL+"drop", cross, upload{"a.png", picture(t)}); code != http.StatusForbidden || len(paths) != 0 {
		t.Errorf("a drop from another site: %d %q", code, paths)
	}
	if code, _ := get(t, s.URL+"drop", nil); code != http.StatusMethodNotAllowed {
		t.Errorf("GET drop: %d", code)
	}
}

// terminal is the page's side of the conversation, without the page.
type terminal struct {
	t      *testing.T
	conn   *websocket.Conn
	screen strings.Builder
}

func connect(t *testing.T, s *server) *terminal {
	t.Helper()
	conn, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(s.URL, "http")+"ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetReadLimit(64 << 20)
	t.Cleanup(func() { conn.CloseNow() })
	term := &terminal{t: t, conn: conn}
	term.send('2', `{"cols":120,"rows":30}`)
	return term
}

func (term *terminal) send(kind byte, payload string) {
	term.t.Helper()
	if err := term.conn.Write(context.Background(), websocket.MessageBinary, append([]byte{kind}, payload...)); err != nil {
		term.t.Fatal(err)
	}
}

// until reads what the viewer draws until it has drawn text.
func (term *terminal) until(text string) {
	term.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for !strings.Contains(term.screen.String(), text) {
		_, data, err := term.conn.Read(ctx)
		if err != nil {
			term.t.Fatalf("waiting for %q: %v\n%s", text, err, term.screen.String())
		}
		if len(data) > 0 && data[0] == '1' {
			term.screen.Write(data[1:])
		}
	}
	term.screen.Reset()
}

func TestServedPickFromDropToAnswer(t *testing.T) {
	s := serving(t, app.Options{Pick: true})
	term := connect(t, s)
	term.until("Drop a file here to send it")
	code, paths := drop(t, s.URL+"drop", nil, upload{"holiday.png", picture(t)})
	if code != 200 || len(paths) != 1 {
		t.Fatalf("code=%d paths=%q", code, paths)
	}
	defer os.RemoveAll(filepath.Dir(paths[0]))
	term.until("Enter send holiday.png")
	term.send('0', "\r")
	m, err := s.Wait(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Picked(); len(got) != 1 || got[0] != paths[0] {
		t.Fatalf("picked %q, want %q", got, paths)
	}
	if resp, err := http.Get(s.URL); err == nil {
		resp.Body.Close()
		t.Errorf("the server outlived its purpose: %d", resp.StatusCode)
	}
	if conn, err := net.Dial("tcp", s.backend); err == nil {
		conn.Close()
		t.Error("the viewer outlived its purpose")
	}
}

func TestServedPickCancelledAndUnanswered(t *testing.T) {
	s := serving(t, app.Options{Pick: true})
	term := connect(t, s)
	term.until("Drop a file here to send it")
	term.send('0', "q")
	if m, err := s.Wait(5 * time.Second); err != nil || m == nil || m.Picked() != nil {
		t.Fatalf("quitting: %v %v", m, err)
	}
	closed := serving(t, app.Options{Pick: true})
	term = connect(t, closed)
	term.until("Drop a file here to send it")
	term.conn.CloseNow() // The tab is closed.
	if m, err := closed.Wait(5 * time.Second); err != nil || m == nil || m.Picked() != nil {
		t.Fatalf("closing the tab: %v %v", m, err)
	}
	ignored := serving(t, app.Options{Pick: true})
	if _, err := ignored.Wait(200 * time.Millisecond); err != errTimeout {
		t.Fatalf("nobody came: %v", err)
	}
}

func TestServedViewerShowsTheCallersFile(t *testing.T) {
	s := serving(t, app.Options{Files: []string{"../../examples/shapes.svg"}})
	term := connect(t, s)
	term.until("shapes.svg")
	term.send('0', "i")
	term.until("View box")
	term.send('0', "q")
	if m, err := s.Wait(5 * time.Second); err != nil || m.Picked() != nil {
		t.Fatalf("%v %v", m, err)
	}
}

func TestServeFlags(t *testing.T) {
	opts, _, err := parse([]string{"--serve", "--pick", "--no-open", "--timeout", "90s", "a.svg"}, &bytes.Buffer{})
	if err != nil || !opts.Pick || !opts.Serve || !opts.NoOpen || opts.Timeout != 90*time.Second || len(opts.Files) != 1 {
		t.Fatalf("%+v %v", opts, err)
	}
	if opts, _, err = parse([]string{"a.svg"}, &bytes.Buffer{}); err != nil || opts.Pick || opts.Serve || opts.NoOpen || opts.Timeout != 0 {
		t.Fatalf("defaults: %+v %v", opts, err)
	}
	for _, args := range [][]string{
		{"--serve", "-o", "out.png", "a.svg"}, {"--pick", "-O", "out", "a.svg"}, {"--serve", "-X"},
		{"--timeout", "-5s", "--pick"}, {"--timeout", "soon", "--pick"}, {"--no-open"}, {"--timeout", "5s", "a.svg"},
	} {
		if _, _, err := parse(args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestExitCodes(t *testing.T) {
	for err, want := range map[error]int{nil: 0, errCancelled: 2, errTimeout: 124, fmt.Errorf("pick: %w", errCancelled): 2, io.ErrUnexpectedEOF: 1} {
		if got := exitCode(err); got != want {
			t.Errorf("exitCode(%v) = %d, want %d", err, got, want)
		}
	}
}

func TestStandardInputIsReadOnlyWhenItIsTheDocument(t *testing.T) {
	// Whatever starts gloss to ask for a file has no terminal to give it:
	// its standard input is a pipe, or nothing, and holds no document.
	for _, tt := range []struct {
		name     string
		args     []string
		terminal bool
		want     []string
	}{
		{"piped to the viewer", nil, false, []string{"-"}},
		{"at a terminal", nil, true, nil},
		{"asked for by name", []string{"--pick", "-"}, false, []string{"-"}},
		{"a pick started by a program", []string{"--pick"}, false, nil},
		{"a page started by a program", []string{"--serve"}, false, nil},
		{"a page for a file", []string{"--serve", "a.svg"}, false, []string{"a.svg"}},
		{"an export of a pipe", []string{"-o", "out.png"}, false, []string{"-"}},
	} {
		opts, _, err := parse(tt.args, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		if got := arguments(opts, tt.terminal); strings.Join(got, " ") != strings.Join(tt.want, " ") {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

// lines collects what gloss writes, for a reader in another goroutine.
type lines struct {
	mu   sync.Mutex
	text strings.Builder
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.Write(p)
}

func (l *lines) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.String()
}

// asked runs gloss as a program would to ask for a file, and plays the user.
func asked(t *testing.T, user func(term *terminal, page string) []string) (answer string, dropped []string, err error) {
	t.Helper()
	opts, _, parseErr := parse([]string{"--serve", "--pick", "--no-open", "--render", "glyph", "--timeout", "30s"}, &bytes.Buffer{})
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	var stdout, stderr lines
	ended := make(chan error, 1)
	go func() { ended <- served(opts, &stdout, &stderr) }()
	page := ""
	for start := time.Now(); page == ""; time.Sleep(10 * time.Millisecond) {
		if _, after, ok := strings.Cut(stderr.String(), "viewer at "); ok && strings.Contains(after, "\n") {
			page = strings.TrimSpace(after)
		}
		if time.Since(start) > 10*time.Second {
			t.Fatalf("no address: %q", stderr.String())
		}
	}
	term := connect(t, &server{URL: page})
	term.until("Drop a file here to send it")
	dropped = user(term, page)
	select {
	case err = <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("gloss did not finish")
	}
	return stdout.String(), dropped, err
}

func TestAskedForAFile(t *testing.T) {
	answer, dropped, err := asked(t, func(term *terminal, page string) []string {
		_, paths := drop(t, page+"drop", nil, upload{"holiday.png", picture(t)})
		term.until("Enter send holiday.png")
		term.send('0', "\r")
		return paths
	})
	if err != nil || len(dropped) != 1 || answer != dropped[0]+"\n" {
		t.Fatalf("answer %q, dropped %q, err %v", answer, dropped, err)
	}
	if got, err := os.ReadFile(dropped[0]); err != nil || !bytes.Equal(got, picture(t)) {
		t.Fatalf("the answer cannot be read: %v", err)
	}
	os.RemoveAll(filepath.Dir(dropped[0]))
}

func TestAskedAndDeclined(t *testing.T) {
	answer, dropped, err := asked(t, func(term *terminal, page string) []string {
		_, paths := drop(t, page+"drop", nil, upload{"private.png", picture(t)})
		term.until("Enter send private.png")
		term.send('0', "q")
		return paths
	})
	if err != errCancelled || answer != "" || len(dropped) != 1 {
		t.Fatalf("answer %q, dropped %q, err %v", answer, dropped, err)
	}
	// What was dropped and then withheld is not left lying about.
	if _, err := os.Stat(filepath.Dir(dropped[0])); !os.IsNotExist(err) {
		t.Fatalf("the withheld file was kept: %v", err)
	}
}

func TestFetchFlag(t *testing.T) {
	opts, _, err := parse([]string{"--fetch", "a.csv"}, &bytes.Buffer{})
	if err != nil || !opts.FetchAllowed {
		t.Fatalf("%+v %v", opts, err)
	}
	if opts, _, _ := parse([]string{"a.csv"}, &bytes.Buffer{}); opts.FetchAllowed || opts.Fetch != nil {
		t.Fatal("fetching is on without being asked for")
	}
	if _, _, err := parse([]string{"--fetch", "-o", "out.png", "a.csv"}, &bytes.Buffer{}); err == nil {
		t.Fatal("--fetch accepted with an export, which fetches nothing")
	}
}

func TestFetchedFilesAreRemovedUnlessPicked(t *testing.T) {
	dir := t.TempDir()
	keep, drop := filepath.Join(dir, "keep.png"), filepath.Join(dir, "drop.png")
	for _, p := range []string{keep, drop} {
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	discardFetched(dir, []string{keep})
	if _, err := os.Stat(keep); err != nil {
		t.Error("the picked file was removed")
	}
	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Error("the unpicked file was kept")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("the folder went while a picked file was still in it")
	}
	discardFetched(dir, nil)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("an emptied folder was kept")
	}
}

func TestPromptFlags(t *testing.T) {
	opts, _, err := parse([]string{"--pick", "--prompt", "Choose the March invoice"}, &bytes.Buffer{})
	if err != nil || opts.Prompt != "Choose the March invoice" || opts.PromptTop {
		t.Fatalf("%+v %v", opts, err)
	}
	opts, _, err = parse([]string{"--prompt", "Choose", "--prompt-loc", "top", "a.png"}, &bytes.Buffer{})
	if err != nil || !opts.PromptTop {
		t.Fatalf("top: %+v %v", opts, err)
	}
	for _, args := range [][]string{{"--prompt-loc", "left", "--prompt", "x"}, {"--prompt-loc", "top"}, {"--prompt", "x", "-o", "out.png", "a.png"}} {
		if _, _, err := parse(args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestServeFilterRejectsAndRemovesWholeDrop(t *testing.T) {
	s := serving(t, app.Options{Accept: "image/*"})
	code, paths := drop(t, s.URL+"drop", nil, upload{"ok.png", picture(t)}, upload{"lie.png", []byte("<!doctype html><html>Sign in</html>")})
	if code != http.StatusUnsupportedMediaType || len(paths) != 0 {
		t.Fatalf("status=%d paths=%v", code, paths)
	}
	s.mu.Lock()
	dir := s.dir
	s.mu.Unlock()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 || s.stored.Load() != 0 {
		t.Fatalf("rejected files retained: %v %v, bytes=%d", entries, err, s.stored.Load())
	}
}

func TestServedURLDropsAreOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s := &server{token: "test-token", fetch: enabled, accept: "image/*"}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1234/test-token/", nil))
		if !strings.Contains(w.Body.String(), fmt.Sprintf(`data-fetch="%t"`, enabled)) || !strings.Contains(w.Body.String(), "Required formats: image/*") {
			t.Fatal(w.Body.String())
		}
		w = httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1234/test-token/remote.mjs", nil))
		if w.Code != http.StatusOK {
			t.Fatal(w.Code)
		}
	}
}

func TestAcceptFlag(t *testing.T) {
	opts, _, err := parse([]string{"--accept", "image/*,pdf", "--fetch", "--pick"}, &bytes.Buffer{})
	if err != nil || opts.Accept != "image/*,pdf" {
		t.Fatalf("filter=%q err=%v", opts.Accept, err)
	}
	if _, _, err := parse([]string{"--accept", "nonsense"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown filter accepted")
	}
}

// An export made in a served viewer goes to the page, to be downloaded: the
// server's working directory is not the visitor's.
func TestServedExportsAreDownloadedByThePage(t *testing.T) {
	wait := exportWait
	exportWait = 50 * time.Millisecond
	t.Cleanup(func() { exportWait = wait })
	s := &server{token: "test-token", exported: make(chan struct{})}
	get := func(rest string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1234/test-token/"+rest, nil))
		return w
	}
	if w := get("exports?after=0"); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("none yet: %d %q", w.Code, w.Body.String())
	}
	// The page's question is answered as soon as there is one.
	exportWait = time.Minute
	answered := make(chan string, 1)
	go func() { answered <- get("exports?after=0").Body.String() }()
	time.Sleep(10 * time.Millisecond)
	name, err := s.offer("page-2.png", []byte("\x89PNG one"))
	if err != nil || !strings.Contains(name, "page-2.png") {
		t.Fatalf("offer: %q %v", name, err)
	}
	select {
	case list := <-answered:
		if !strings.Contains(list, `{"id":0,"name":"page-2.png"}`) {
			t.Fatalf("list: %s", list)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the page was not told")
	}
	s.offer("page-3.png", []byte("\x89PNG two"))
	if list := get("exports?after=1").Body.String(); strings.Contains(list, "page-2") || !strings.Contains(list, `"id":1`) {
		t.Fatalf("after 1: %s", list)
	}
	w := get("export/1")
	if w.Code != http.StatusOK || w.Body.String() != "\x89PNG two" || w.Header().Get("Content-Type") != "image/png" || !strings.Contains(w.Header().Get("Content-Disposition"), `attachment; filename=page-3.png`) {
		t.Fatalf("export: %d %v %q", w.Code, w.Header(), w.Body.String())
	}
	for _, bad := range []string{"export/2", "export/-1", "export/x"} {
		if w := get(bad); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	// Without the token there is nothing.
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1234/other-token/export/0", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("no token: %d", w.Code)
	}
}
