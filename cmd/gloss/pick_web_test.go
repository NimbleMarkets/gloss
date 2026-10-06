package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/NimbleMarkets/gloss/internal/app"
)

func webPicking(t *testing.T, opts app.Options) *server {
	t.Helper()
	s, err := serveWebPick(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); s.Discard() })
	return s
}

func pickRequest(t *testing.T, s *server, method, route, body string, want int) string {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, s.URL+route, strings.NewReader(body))
	s.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, route, w.Code, want, w.Body.String())
	}
	return w.Body.String()
}

func pickUpload(t *testing.T, s *server, want int, files ...upload) []webPickFile {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, f := range files {
		part, err := form.CreateFormFile("file", f.name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(f.data)
	}
	_ = form.Close()
	r := httptest.NewRequest(http.MethodPost, s.URL+"files", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("upload: %d, want %d: %s", w.Code, want, w.Body.String())
	}
	if want != http.StatusOK {
		return nil
	}
	var response struct{ Files []webPickFile }
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), s.dir) || strings.Contains(w.Body.String(), `"path"`) {
		t.Fatal("upload response exposed host paths")
	}
	return response.Files
}

func confirmBody(ids ...string) string {
	data, _ := json.Marshal(map[string][]string{"ids": ids})
	return string(data)
}

func confirmMessage(message string, ids ...string) string {
	data, _ := json.Marshal(struct {
		IDs     []string `json:"ids"`
		Message string   `json:"message"`
	}{ids, message})
	return string(data)
}

func TestWebPickFlags(t *testing.T) {
	opts, _, err := parse([]string{"--pick-web", "--no-open", "--prompt", "Invoice, please", "--accept", "pdf"}, io.Discard)
	if err != nil || !opts.Serve || !opts.Pick || !opts.PickWeb || opts.Accept != "pdf" {
		t.Fatalf("parse: %+v %v", opts, err)
	}
	if opts, _, err := parse([]string{"--pick-web", "--json"}, io.Discard); err != nil || !opts.JSON {
		t.Fatalf("JSON web pick: %+v %v", opts, err)
	}
	for _, args := range [][]string{
		{"invoice.pdf"}, {"-"}, {"--glob", "*.png"}, {"--fetch"}, {"--menu"}, {"--preview"},
		{"--info"}, {"--text"}, {"--output", "a.png"}, {"--resume", testToken}, {"--status", testToken}, {"--cancel", testToken},
	} {
		if _, _, err := parse(append([]string{"--pick-web"}, args...), io.Discard); err == nil {
			t.Errorf("accepted --pick-web %v", args)
		}
	}
}

func TestWebPickPageHasNoViewerOrHostFileAccess(t *testing.T) {
	s := webPicking(t, app.Options{Prompt: "Choose <script>bad()</script>"})
	page := pickRequest(t, s, "GET", "", "", 200)
	if !strings.Contains(page, "Choose &lt;script&gt;") || !strings.Contains(page, "pick.mjs") || strings.Contains(page, "<script>bad()") || strings.Contains(page, "terminal") || s.backend != "" {
		t.Fatalf("page is not an independent, escaped request: %s", page)
	}
	for _, route := range []string{"ws", "drop", "exports", "export/0", "static/booba/booba.js", "serve.mjs"} {
		pickRequest(t, s, "GET", route, "", 404)
	}
	pickRequest(t, s, "DELETE", "files/../../etc/passwd", "", 404)
	for _, route := range []string{"pick.mjs", "pick-api.mjs", "files"} {
		pickRequest(t, s, "GET", route, "", 200)
	}
	for _, edit := range []func(*http.Request){
		func(r *http.Request) { r.URL.Path = "/wrong-token/files" },
		func(r *http.Request) { r.Host = "evil.example:1234" },
		func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		func(r *http.Request) { r.Header.Set("Origin", "https://"+r.Host) },
	} {
		r := httptest.NewRequest("POST", s.URL+"confirm", strings.NewReader(`{"ids":["x"]}`))
		edit(r)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 && w.Code != 404 {
			t.Fatalf("untrusted confirmation: %d", w.Code)
		}
	}
}

func TestWebPickRecoversUploadsAndConfirmsExactIDs(t *testing.T) {
	s := webPicking(t, app.Options{})
	files := pickUpload(t, s, 200, upload{"a.txt", []byte("one")}, upload{"b.txt", []byte("two")})
	if len(files) != 2 || files[0].ID == files[1].ID || !tokenPattern.MatchString(files[0].ID) {
		t.Fatalf("files: %+v", files)
	}
	for range 2 {
		page := pickRequest(t, s, "GET", "", "", 200)
		list := pickRequest(t, s, "GET", "files", "", 200)
		if page == "" || !strings.Contains(list, files[0].ID) || !strings.Contains(list, `"state":"waiting"`) {
			t.Fatal("refresh lost the pending request")
		}
	}
	for _, body := range []string{`{}`, confirmBody(), confirmBody("/etc/passwd"), confirmBody(files[0].ID, files[0].ID), confirmBody(files[0].ID) + `{}`} {
		pickRequest(t, s, "POST", "confirm", body, 400)
	}
	select {
	case <-s.done:
		t.Fatal("upload or invalid confirmation ended the request")
	default:
	}
	pickRequest(t, s, "POST", "confirm", confirmBody(files[1].ID), 200)
	// Duplicate submissions are safe while the server is draining; a different
	// submission cannot replace the answer, and no more uploads can arrive.
	pickRequest(t, s, "POST", "confirm", confirmBody(files[1].ID), 200)
	pickRequest(t, s, "POST", "confirm", confirmBody(files[0].ID), 409)
	pickRequest(t, s, "POST", "decline", "", 409)
	pickUpload(t, s, 409, upload{"c.txt", []byte("three")})
	paths, _, err := s.waitWebPick(context.Background(), time.Second)
	if err != nil || !slices.Equal(paths, []string{filepath.Join(s.dir, "b.txt")}) {
		t.Fatalf("picked %v, %v", paths, err)
	}
	discardFetched(s.dir, paths)
	if data, err := os.ReadFile(paths[0]); err != nil || string(data) != "two" {
		t.Fatalf("answer was not kept: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("unconfirmed upload was kept")
	}
}

func TestWebPickRemoveAndLimits(t *testing.T) {
	s := webPicking(t, app.Options{Accept: "image/*"})
	data := picture(t)
	before := maxSessionBytes
	maxSessionBytes = int64(len(data) * 2)
	t.Cleanup(func() { maxSessionBytes = before })
	pickUpload(t, s, 415, upload{"a.png", data}, upload{"lie.png", []byte("<html>not a picture</html>")})
	if len(s.webPick.files) != 0 || s.stored.Load() != 0 {
		t.Fatal("rejected batch was not rolled back")
	}
	files := pickUpload(t, s, 200, upload{"a.png", data}, upload{"a.png", data})
	if files[0].Name != "a.png" || files[1].Name != "a-2.png" {
		t.Fatalf("names: %+v", files)
	}
	pickUpload(t, s, 413, upload{"full.png", data})
	pickRequest(t, s, "DELETE", "files/unknown", "", 404)
	pickRequest(t, s, "DELETE", "files/"+files[0].ID, "", 200)
	if s.stored.Load() != int64(len(data)) {
		t.Fatal("removal did not reclaim session bytes")
	}
	pickRequest(t, s, "POST", "confirm", confirmBody(files[0].ID), 400)
	pickUpload(t, s, 200, upload{"a.png", data})
	if _, err := os.Stat(filepath.Join(s.dir, "a-2.png")); err != nil {
		t.Fatal("removal affected another upload")
	}
}

func TestWebPickMessageIsBoundedAndPartOfConfirmation(t *testing.T) {
	s := webPicking(t, app.Options{})
	files := pickUpload(t, s, 200, upload{"a.txt", []byte("one")})
	id := files[0].ID
	pickRequest(t, s, "POST", "confirm", confirmMessage("note without files"), 400)
	for _, message := range []string{strings.Repeat("a", maxWebPickMessage+1), strings.Repeat("☕", maxWebPickMessage+1)} {
		pickRequest(t, s, "POST", "confirm", confirmMessage(message, id), 400)
	}
	pickRequest(t, s, "POST", "confirm", `{"ids":["`+id+`"],"message":123}`, 400)
	pickRequest(t, s, "POST", "confirm", confirmMessage(strings.Repeat("a", 40<<10), id), 400)
	if s.webPick.state != statusWaiting || s.webPick.message != "" {
		t.Fatal("invalid message settled the request")
	}
	message := "  First page is the receipt.\n東京 ☕ <script>alert(1)</script>  "
	body := confirmMessage(message, id)
	pickRequest(t, s, "POST", "confirm", body, 200)
	pickRequest(t, s, "POST", "confirm", body, 200)
	pickRequest(t, s, "POST", "confirm", confirmMessage("changed", id), 409)
	pickRequest(t, s, "POST", "confirm", confirmBody(id), 409)
	var response struct{ Message string }
	if err := json.Unmarshal([]byte(pickRequest(t, s, "GET", "files", "", 200)), &response); err != nil || response.Message != message {
		t.Fatalf("confirmation recovery lost message: %+v %v", response, err)
	}
	_, got, err := s.waitWebPick(context.Background(), time.Second)
	if err != nil || got != message {
		t.Fatalf("message changed: %q %v", got, err)
	}
}

func TestWebPickMessageLimitAndOmission(t *testing.T) {
	for _, message := range []string{"", " \n\t ", strings.Repeat("😀", maxWebPickMessage)} {
		s := webPicking(t, app.Options{})
		files := pickUpload(t, s, 200, upload{"a.txt", []byte("one")})
		body := confirmMessage(message, files[0].ID)
		// Exercise the largest JSON escape representation, not only literal UTF-8.
		body = strings.ReplaceAll(body, "😀", `\ud83d\ude00`)
		reply := pickRequest(t, s, "POST", "confirm", body, 200)
		want := message
		if strings.TrimSpace(want) == "" {
			want = ""
		}
		if s.webPick.message != want || (strings.Contains(reply, `"message"`) != (want != "")) {
			t.Fatal("message boundary or omission failed")
		}
	}
}

func TestWebPickDeclineAndTimeout(t *testing.T) {
	for _, decline := range []bool{true, false} {
		s := webPicking(t, app.Options{})
		pickUpload(t, s, 200, upload{"private.txt", []byte("private")})
		want := errTimeout
		if decline {
			pickRequest(t, s, "POST", "decline", "", 200)
			want = errCancelled
		}
		paths, _, err := s.waitWebPick(context.Background(), 10*time.Millisecond)
		if !errors.Is(err, want) || len(paths) != 0 {
			t.Fatalf("paths %v, error %v; want %v", paths, err, want)
		}
		s.Discard()
		if _, err := os.Stat(s.dir); !os.IsNotExist(err) {
			t.Fatal("abandoned uploads were kept")
		}
	}
}

func TestDetachedWebPickRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detached server is exercised on Unix")
	}
	bin := filepath.Join(t.TempDir(), "gloss")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "--pick-web", "--timeout", "20s", "--accept", "image/*")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("start: %v %s", err, stderr.String())
	}
	var got started
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !got.Pick || got.ResumeToken == "" {
		t.Fatalf("startup: %s %v", stdout.String(), err)
	}
	t.Cleanup(func() { _ = cancel(options{Cancel: got.ResumeToken}) })
	// Upload over HTTP to the detached process, with no WebSocket or viewer.
	code, _ := drop(t, got.URL+"files", nil, upload{"receipt.png", picture(t)})
	if code != 200 {
		t.Fatalf("upload: %d", code)
	}
	_, body := get(t, got.URL+"files", nil)
	var list struct{ Files []webPickFile }
	if err := json.Unmarshal([]byte(body), &list); err != nil || len(list.Files) != 1 {
		t.Fatalf("list: %s %v", body, err)
	}
	if st, err := ask(t, got.ResumeToken); err != nil || st.State != "waiting" {
		t.Fatalf("status before Send: %+v %v", st, err)
	}
	message := "Use this receipt, not the earlier draft.\n東京 ☕ <b>literal text</b>"
	response, err := http.Post(got.URL+"confirm", "application/json", strings.NewReader(confirmMessage(message, list.Files[0].ID)))
	if err != nil {
		t.Fatal(err)
	}
	reply, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || !strings.Contains(string(reply), `"state":"picked"`) {
		t.Fatalf("confirmation response lost at shutdown: %d %s %v", response.StatusCode, reply, err)
	}
	stdout.Reset()
	if err := resume(options{Resume: got.ResumeToken, Timeout: 5 * time.Second}, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(stdout.String())
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, picture(t)) {
		t.Fatalf("resume returned unreadable file: %q %v", path, err)
	}
	if st, err := ask(t, got.ResumeToken); err != nil || st.State != "picked" || !slices.Equal(st.Paths, []string{path}) || st.Message != message {
		t.Fatalf("status after resume: %+v %v", st, err)
	}
	for range 2 {
		stdout.Reset()
		if err := resume(options{Resume: got.ResumeToken, JSON: true}, &stdout, io.Discard); err != nil {
			t.Fatal(err)
		}
		var result resumed
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Message != message || !slices.Equal(result.Paths, []string{path}) {
			t.Fatalf("JSON resume lost message: %+v %v", result, err)
		}
	}
}

type webPickLog chan string

func (c webPickLog) Write(p []byte) (int, error) {
	c <- string(p)
	return len(p), nil
}

func TestForegroundWebPickJSONMessage(t *testing.T) {
	var stdout bytes.Buffer
	log, done := make(webPickLog, 1), make(chan error, 1)
	go func() {
		done <- served(options{Options: app.Options{Pick: true}, PickWeb: true, JSON: true, NoOpen: true, Timeout: 5 * time.Second}, &stdout, log)
	}()
	var address string
	select {
	case line := <-log:
		address = strings.TrimSpace(strings.TrimPrefix(line, "gloss: viewer at "))
	case err := <-done:
		t.Fatalf("server stopped before listening: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not announce itself")
	}
	code, _ := drop(t, address+"files", nil, upload{"receipt.txt", []byte("receipt")})
	if code != 200 {
		t.Fatalf("upload: %d", code)
	}
	_, body := get(t, address+"files", nil)
	var list struct{ Files []webPickFile }
	if err := json.Unmarshal([]byte(body), &list); err != nil || len(list.Files) != 1 {
		t.Fatalf("files: %s %v", body, err)
	}
	message := "The invoice is attached.\nPlease check the date."
	response, err := http.Post(address+"confirm", "application/json", strings.NewReader(confirmMessage(message, list.Files[0].ID)))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var result resumed
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Status != statusPicked || result.Message != message || len(result.Paths) != 1 {
		t.Fatalf("foreground answer: %s %v", stdout.String(), err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Dir(result.Paths[0])) })
	if data, err := os.ReadFile(result.Paths[0]); err != nil || string(data) != "receipt" {
		t.Fatalf("file not retained: %q %v", data, err)
	}
}
