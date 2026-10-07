package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/app"
)

func TestWebPickSecurityHeaders(t *testing.T) {
	s := webPicking(t, app.Options{})
	for _, route := range []string{"", "pick.mjs", "pick-api.mjs", "files"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", s.URL+route, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", route, w.Code)
		}
		directives := strings.Split(w.Header().Get("Content-Security-Policy"), "; ")
		want := []string{"default-src 'none'", "script-src 'self'", "style-src 'unsafe-inline'", "img-src blob:", "connect-src 'self'", "base-uri 'none'", "frame-ancestors 'none'", "form-action 'none'"}
		if strings.Join(directives, "; ") != strings.Join(want, "; ") {
			t.Errorf("%s CSP: %v", route, directives)
		}
		for key, want := range map[string]string{"Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "X-Content-Type-Options": "nosniff", "Access-Control-Allow-Origin": ""} {
			if got := w.Header().Get(key); got != want {
				t.Errorf("%s %s: %q, want %q", route, key, got, want)
			}
		}
	}
	if s.front.ErrorLog == nil || s.front.ErrorLog.Writer() != io.Discard {
		t.Fatal("HTTP errors could write over the QR screen")
	}
}

func TestWebPickFileCountBoundary(t *testing.T) {
	batch := make([]upload, maxWebPickFiles+1)
	for i := range batch {
		batch[i] = upload{fmt.Sprintf("file-%03d.txt", i), []byte("x")}
	}
	t.Run("one batch rolls back", func(t *testing.T) {
		s := webPicking(t, app.Options{})
		pickUpload(t, s, 413, batch...)
		entries, err := os.ReadDir(s.dir)
		if err != nil || len(entries) != 0 || len(s.webPick.files) != 0 || s.stored.Load() != 0 {
			t.Fatalf("201-file batch left uploads: %v %v", entries, err)
		}
		if got := pickUpload(t, s, 200, batch[:maxWebPickFiles]...); len(got) != maxWebPickFiles {
			t.Fatalf("200 files: %d", len(got))
		}
	})
	t.Run("session count and removal", func(t *testing.T) {
		s := webPicking(t, app.Options{})
		pickUpload(t, s, 200, batch[:maxWebPickFiles-1]...)
		pickUpload(t, s, 413, batch[maxWebPickFiles-1:]...)
		if len(s.webPick.files) != maxWebPickFiles-1 || s.stored.Load() != maxWebPickFiles-1 {
			t.Fatal("failed batch changed session")
		}
		files := pickUpload(t, s, 200, batch[maxWebPickFiles-1])
		pickUpload(t, s, 413, batch[maxWebPickFiles])
		pickRequest(t, s, "DELETE", "files/"+files[0].ID, "", 200)
		pickUpload(t, s, 200, batch[maxWebPickFiles])
		if len(s.webPick.files) != maxWebPickFiles || s.stored.Load() != maxWebPickFiles {
			t.Fatal("removal did not reclaim a file slot")
		}
	})
}

func TestWebPickFilenameBytesAndRollback(t *testing.T) {
	for _, name := range []string{strings.Repeat("a", 256), strings.Repeat("é", 128)} {
		s := webPicking(t, app.Options{})
		pickUpload(t, s, 400, upload{"valid.txt", []byte("one")}, upload{name, []byte("two")})
		entries, err := os.ReadDir(s.dir)
		if err != nil || len(entries) != 0 || len(s.webPick.files) != 0 || s.stored.Load() != 0 {
			t.Fatal("overlong filename did not roll back its batch")
		}
	}
	s := webPicking(t, app.Options{})
	name := strings.Repeat("a", 251) + ".txt"
	files := pickUpload(t, s, 200, upload{name, []byte("kept")})
	pickUpload(t, s, 400, upload{name, []byte("duplicate")})
	data, err := os.ReadFile(s.webPick.files[0].path)
	if err != nil || string(data) != "kept" || len(s.webPick.files) != 1 || s.stored.Load() != 4 || files[0].Name != name {
		t.Fatal("duplicate-name suffix failure damaged original")
	}
}

func TestWebPickUploadCanonicalOrigin(t *testing.T) {
	s := webPicking(t, app.Options{})
	u, _ := url.Parse(s.URL)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("file", "test.txt")
	_, _ = part.Write([]byte("hello"))
	_ = form.Close()
	r := httptest.NewRequest("POST", s.URL+"files", &body)
	r.Host = "LOCALHOST:" + u.Port()
	r.Header.Set("Origin", "http://localhost:"+u.Port())
	r.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("canonical same-origin upload: %d %s", w.Code, w.Body.String())
	}
}

func TestWebPickConcurrentRequests(t *testing.T) {
	s := webPicking(t, app.Options{})
	const count = 24
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, count)
	var wg sync.WaitGroup
	for range count {
		// Each request owns its multipart body and recorder; only the server is shared.
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, err := form.CreateFormFile("file", "same.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("data"))
		_ = form.Close()
		r := httptest.NewRequest("POST", s.URL+"files", &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		wg.Go(func() { <-start; w := httptest.NewRecorder(); s.ServeHTTP(w, r); results <- w })
	}
	close(start)
	wg.Wait()
	close(results)
	ids, names := map[string]bool{}, map[string]bool{}
	for w := range results {
		var reply struct{ Files []webPickFile }
		if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || w.Code != 200 || len(reply.Files) != 1 {
			t.Fatalf("concurrent upload: %d %s %v", w.Code, w.Body.String(), err)
		}
		f := reply.Files[0]
		if ids[f.ID] || names[f.Name] {
			t.Fatal("concurrent uploads collided")
		}
		ids[f.ID], names[f.Name] = true, true
	}
	if len(s.webPick.files) != count || s.stored.Load() != 4*count {
		t.Fatal("concurrent accounting lost uploads")
	}
	// A removal and confirmation of the same ID may settle in either order.
	// A confirmed path must never be removed by the losing request.
	file := s.webPick.files[0]
	start = make(chan struct{})
	codes := make(chan int, 2)
	for _, r := range []*http.Request{
		httptest.NewRequest("DELETE", s.URL+"files/"+file.ID, nil),
		httptest.NewRequest("POST", s.URL+"confirm", strings.NewReader(confirmBody(file.ID))),
	} {
		wg.Go(func() { <-start; w := httptest.NewRecorder(); s.ServeHTTP(w, r); codes <- w.Code })
	}
	close(start)
	wg.Wait()
	close(codes)
	got := map[int]int{}
	for code := range codes {
		got[code]++
	}
	if s.webPick.state == statusPicked {
		if got[200] != 1 || got[409] != 1 {
			t.Fatalf("confirmation won: %v", got)
		}
		if data, err := os.ReadFile(file.path); err != nil || string(data) != "data" {
			t.Fatal("confirmed file was removed")
		}
	} else {
		if s.webPick.state != statusWaiting || got[200] != 1 || got[400] != 1 {
			t.Fatalf("removal won: state %s, %v", s.webPick.state, got)
		}
		if _, err := os.Stat(file.path); !os.IsNotExist(err) {
			t.Fatal("removed file remains")
		}
	}
}
