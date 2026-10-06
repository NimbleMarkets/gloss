package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/NimbleMarkets/gloss/web"
)

const maxWebPickFiles = 200
const maxWebPickMessage = 2000 // Unicode code points; mirrored by pick-api.mjs.

var webPickPage = template.Must(template.ParseFS(web.Page, "pick.html"))

// webPick owns the request independently of a viewer or browser connection.
// All API operations are serialized, including upload and settlement, so Send
// cannot race an unfinished upload, and cleanup cannot race a disk write.
type webPick struct {
	access  pickAccess // Immutable listener/advertised-host policy.
	mu      sync.Mutex
	files   []webPickFile
	state   string
	paths   []string
	message string
}

// Only an opaque ID and a display name leave the server. Browser requests
// cannot name host paths or ask for file contents.
type webPickFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	path string
}

func serveWebPick(ctx context.Context, opts options) (*server, error) {
	listen := opts.Listen
	if listen == "" {
		listen = defaultPickListen
	}
	network, err := parsePickNetwork(listen, opts.AdvertiseHost)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	token, err := random()
	if err != nil {
		cancel()
		return nil, err
	}
	// Explicit families keep 0.0.0.0 IPv4-only and :: IPv6-only on every OS.
	family := "tcp4"
	if network.listen.Addr().Is6() {
		family = "tcp6"
	}
	listener, err := net.Listen(family, network.listen.String())
	if err != nil {
		cancel()
		return nil, fmt.Errorf("--listen %s: %w", listen, err)
	}
	access := network.access(listener.Addr().(*net.TCPAddr).Port)
	s := &server{
		token: token, prompt: opts.Prompt, accept: opts.Accept, pick: true,
		cancel: cancel, done: make(chan struct{}),
		webPick: &webPick{access: access, state: statusWaiting, files: []webPickFile{}},
	}
	s.URL = (&url.URL{Scheme: "http", Host: access.authority, Path: "/" + token + "/"}).String()
	s.front = &http.Server{
		Handler: s, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 2 * time.Minute, IdleTimeout: 30 * time.Second,
	}
	go s.front.Serve(listener)
	go func() {
		<-ctx.Done()
		s.front.Close()
	}()
	return s, nil
}

func (s *server) waitWebPick(ctx context.Context, timeout time.Duration) ([]string, string, error) {
	var late <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		late = timer.C
	}
	select {
	case <-s.done:
	case <-ctx.Done():
	case <-late:
		s.Close()
		// Close interrupts request bodies; wait for their rollback before cleanup.
		s.webPick.mu.Lock()
		defer s.webPick.mu.Unlock()
		if s.webPick.state != statusPicked {
			s.webPick.state = statusTimeout
			return nil, "", errTimeout
		}
		return slices.Clone(s.webPick.paths), s.webPick.message, nil
	}
	// Let the confirmation response reach the browser before closing the server.
	drain, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.front.Shutdown(drain)
	s.Close()
	s.webPick.mu.Lock()
	defer s.webPick.mu.Unlock()
	if s.webPick.state != statusPicked {
		s.webPick.state = statusDeclined
		return nil, "", errCancelled
	}
	return slices.Clone(s.webPick.paths), s.webPick.message, nil
}

func (s *server) serveWebPick(w http.ResponseWriter, r *http.Request, rest string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; img-src blob:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	if rest == "" || rest == "pick.mjs" || rest == "pick-api.mjs" || rest == "drop.mjs" || rest == "pickers.mjs" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "GET the page", http.StatusMethodNotAllowed)
			return
		}
		if rest == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = webPickPage.Execute(w, struct{ Prompt, Accept string }{s.prompt, string(s.accept)})
		} else {
			data, err := fs.ReadFile(web.Page, rest)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = w.Write(data)
		}
		return
	}
	if rest != "files" && rest != "confirm" && rest != "decline" && !strings.HasPrefix(rest, "files/") {
		http.NotFound(w, r)
		return
	}
	// No CORS: mutations belong to this page, at its exact origin. Missing
	// Origin is permitted for local clients, which still need the URL token.
	if !samePickOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	p := s.webPick
	p.mu.Lock()
	defer p.mu.Unlock()
	if rest == "files" && r.Method == http.MethodGet {
		p.reply(w)
		return
	}
	if (rest == "files" || rest == "confirm" || rest == "decline") && r.Method != http.MethodPost ||
		strings.HasPrefix(rest, "files/") && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if p.state != statusWaiting && rest != "confirm" && rest != "decline" {
		http.Error(w, "this request has finished", http.StatusConflict)
		return
	}
	switch {
	case rest == "files":
		// Includes a small allowance for multipart headers; files have their own
		// 128 MiB limit, and the shared receiver enforces the session byte cap.
		r.Body = http.MaxBytesReader(w, r.Body, maxSessionBytes+(1<<20))
		s.receive(w, r)
	case strings.HasPrefix(rest, "files/"):
		id := strings.TrimPrefix(rest, "files/")
		i := slices.IndexFunc(p.files, func(f webPickFile) bool { return f.ID == id })
		if i < 0 {
			http.NotFound(w, r)
			return
		}
		if err := os.Remove(p.files[i].path); err != nil {
			http.Error(w, "could not remove upload", http.StatusInternalServerError)
			return
		}
		s.stored.Add(-p.files[i].Size)
		p.files = slices.Delete(p.files, i, i+1)
		p.reply(w)
	case rest == "decline":
		if p.state == statusPicked {
			http.Error(w, "files were already sent", http.StatusConflict)
			return
		}
		p.state = statusDeclined
		p.reply(w)
		s.ended.Do(func() { close(s.done) })
	case rest == "confirm":
		var request struct {
			IDs     []string `json:"ids"`
			Message string   `json:"message"`
		}
		// Includes 200 IDs and 2,000 characters even when JSON escapes each
		// character as a surrogate pair (12 bytes per character).
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil || len(request.IDs) == 0 || len(request.IDs) > maxWebPickFiles {
			http.Error(w, "choose completed uploads to send", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "expected one confirmation", http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(request.Message) > maxWebPickMessage {
			http.Error(w, "message must be at most 2,000 characters", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(request.Message) == "" {
			request.Message = ""
		}
		paths := make([]string, 0, len(request.IDs))
		seen := make(map[string]bool, len(request.IDs))
		for _, id := range request.IDs {
			i := slices.IndexFunc(p.files, func(f webPickFile) bool { return f.ID == id })
			if i < 0 || seen[id] {
				http.Error(w, "unknown or repeated upload", http.StatusBadRequest)
				return
			}
			seen[id] = true
			paths = append(paths, p.files[i].path)
		}
		if p.state != statusWaiting && (p.state != statusPicked || !slices.Equal(paths, p.paths) || request.Message != p.message) {
			http.Error(w, "this request has finished", http.StatusConflict)
			return
		}
		p.paths, p.message, p.state = paths, request.Message, statusPicked
		p.reply(w)
		s.ended.Do(func() { close(s.done) })
	}
}

// add registers an entire successful upload atomically. receive rolls back
// files and byte accounting if an ID or metadata could not be obtained.
func (p *webPick) add(w http.ResponseWriter, paths []string) error {
	files := make([]webPickFile, 0, len(paths))
	for _, path := range paths {
		id, err := random()
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("upload: %w", err)
		}
		files = append(files, webPickFile{ID: id, Name: filepath.Base(path), Size: info.Size(), path: path})
	}
	p.files = append(p.files, files...)
	w.Header().Set("Content-Type", "application/json")
	// A lost response does not undo a completed upload: a refresh recovers it.
	_ = json.NewEncoder(w).Encode(struct {
		Files []webPickFile `json:"files"`
	}{files})
	return nil
}

func (p *webPick) reply(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		State   string        `json:"state"`
		Files   []webPickFile `json:"files"`
		Message string        `json:"message,omitempty"`
	}{p.state, p.files, p.message})
}
