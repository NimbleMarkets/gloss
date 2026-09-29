package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/gloss/web"
	booba "github.com/NimbleMarkets/go-booba/serve"
)

var (
	errCancelled = errors.New("nothing was chosen")
	errTimeout   = errors.New("timed out waiting for the viewer")
)

const maxDrops = 256 // Files in one drop.

// server shows the viewer on a page of its own, for as long as it is wanted.
//
// Booba runs the viewer and carries its terminal to the page. It listens on
// a port of its own, and answers only to requests bearing a secret. Those
// come from the server in front of it, which serves the page, takes what is
// dropped there, and admits nothing without the token in the page's address.
type server struct {
	URL     string
	backend string

	token, secret string
	cancel        context.CancelFunc
	front         *http.Server
	proxy         *httputil.ReverseProxy
	drops         chan []string
	started, done chan struct{}
	once, ended   sync.Once

	mu    sync.Mutex
	model *app.Model
	dir   string // Where drops are kept; made when the first arrives.
}

func random() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func serve(ctx context.Context, opts app.Options) (*server, error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &server{cancel: cancel, drops: make(chan []string, 16), started: make(chan struct{}), done: make(chan struct{})}
	var err error
	if s.token, err = random(); err == nil {
		s.secret, err = random()
	}
	if err != nil {
		cancel()
		return nil, err
	}
	// Booba opens its own listener, so it is told of a port found free.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		return nil, err
	}
	s.backend = probe.Addr().String()
	probe.Close()
	config := booba.DefaultConfig()
	config.Host, config.Port, config.HTTP3Port, config.MaxConnections = "127.0.0.1", probe.Addr().(*net.TCPAddr).Port, -1, 1
	opts.Drops = s.drops
	viewer := booba.NewServer(config, booba.WithConnectMiddleware(func(next booba.ConnectHandler) booba.ConnectHandler {
		return func(r *http.Request) error {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Gloss-Secret")), []byte(s.secret)) != 1 {
				return errors.New("not from the page's server")
			}
			return next(r)
		}
	}))
	log.SetOutput(io.Discard) // Booba reports its comings and goings.
	failed := make(chan error, 1)
	go func() {
		failed <- viewer.Serve(ctx, func(session booba.Session) (tea.Model, []tea.ProgramOption) {
			m := app.New(opts)
			s.mu.Lock()
			first := s.model == nil
			if first {
				s.model = m
			}
			s.mu.Unlock()
			if first {
				s.once.Do(func() { close(s.started) })
				go func() {
					// The session ends when the viewer quits or the tab closes.
					<-session.Context().Done()
					s.ended.Do(func() { close(s.done) })
				}()
			}
			return m, nil
		})
	}()
	for start := time.Now(); ; time.Sleep(10 * time.Millisecond) {
		conn, err := net.Dial("tcp", s.backend)
		if err == nil {
			conn.Close()
			break
		}
		select {
		case err := <-failed:
			cancel()
			return nil, fmt.Errorf("viewer: %w", err)
		default:
		}
		if time.Since(start) > 5*time.Second {
			cancel()
			return nil, fmt.Errorf("viewer did not start: %w", err)
		}
	}
	target := &url.URL{Scheme: "http", Host: s.backend}
	s.proxy = &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		r.Out.Host = r.In.Host // Booba admits pages of its own host.
		r.Out.Header.Set("X-Gloss-Secret", s.secret)
	}, ErrorLog: log.New(io.Discard, "", 0)}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		return nil, err
	}
	s.URL = "http://" + listener.Addr().String() + "/" + s.token + "/"
	s.front = &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	go s.front.Serve(listener)
	go func() {
		<-ctx.Done()
		s.front.Close()
	}()
	return s, nil
}

// Wait returns the viewer once its session is over. A timeout of zero waits
// as long as it takes.
func (s *server) Wait(timeout time.Duration) (*app.Model, error) {
	var late <-chan time.Time
	if timeout > 0 {
		late = time.After(timeout)
	}
	select {
	case <-s.done:
	case <-late:
		s.Close()
		return nil, errTimeout
	}
	s.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model, nil
}

func (s *server) Close() {
	s.cancel()
	s.front.Close()
}

// Discard removes what was dropped on the page. Drops are kept only when
// they are the answer to a pick; otherwise they were for looking at.
func (s *server) Discard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir != "" {
		os.RemoveAll(s.dir)
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// A page elsewhere can point a name of its own at this machine.
	if host, _, err := net.SplitHostPort(r.Host); err != nil || (host != "127.0.0.1" && host != "localhost") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	token, rest, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case rest == "":
		rest = "serve.html"
		fallthrough
	case rest == "serve.mjs" || rest == "drop.mjs" || rest == "upload.mjs":
		data, err := fs.ReadFile(web.Page, rest)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		kind := "text/javascript; charset=utf-8"
		if rest == "serve.html" {
			kind = "text/html; charset=utf-8"
		}
		w.Header().Set("Content-Type", kind)
		w.Write(data)
	case rest == "drop":
		s.receive(w, r)
	case rest == "ws" || strings.HasPrefix(rest, "static/"):
		r.URL.Path, r.URL.RawPath = "/"+rest, ""
		s.proxy.ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

// receive keeps the files of a drop and tells the viewer where. They are
// kept past the server's end: whoever started gloss has yet to read them.
func (s *server) receive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST a drop", http.StatusMethodNotAllowed)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		if from, err := url.Parse(origin); err != nil || from.Host != r.Host {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	form, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "not a drop", http.StatusBadRequest)
		return
	}
	var paths []string
	fail := func(code int, reason string) {
		for _, path := range paths {
			os.Remove(path)
		}
		http.Error(w, reason, code)
	}
	for {
		part, err := form.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			fail(http.StatusBadRequest, "not a drop")
			return
		}
		// Only the last element of the name is the page's to choose.
		name := path.Base(strings.ReplaceAll(part.FileName(), `\`, "/"))
		if part.FileName() == "" || name == "." || name == ".." || name == "/" || strings.ContainsFunc(name, func(r rune) bool { return r < ' ' || r == 0x7f }) {
			continue
		}
		if len(paths) == maxDrops {
			fail(http.StatusRequestEntityTooLarge, "too many files")
			return
		}
		kept, size, err := s.keep(name, io.LimitReader(part, document.MaxFileBytes+1))
		switch {
		case err != nil:
			fail(http.StatusInternalServerError, "the drop could not be kept")
			return
		case size > document.MaxFileBytes:
			os.Remove(kept)
			fail(http.StatusRequestEntityTooLarge, "file exceeds 128 MiB")
			return
		case size == 0:
			os.Remove(kept)
			continue
		}
		paths = append(paths, kept)
	}
	if len(paths) == 0 {
		fail(http.StatusBadRequest, "nothing in the drop could be used")
		return
	}
	select {
	case s.drops <- paths:
	default:
		fail(http.StatusServiceUnavailable, "the viewer is busy")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"paths": paths})
}

// keep writes a dropped file under its own name, or the nearest one free.
func (s *server) keep(name string, from io.Reader) (string, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		dir, err := os.MkdirTemp("", "gloss-")
		if err != nil {
			return "", 0, err
		}
		s.dir = dir
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; n < 1000; n++ {
		if n > 1 {
			name = fmt.Sprintf("%s-%d%s", stem, n, ext)
		}
		f, err := os.OpenFile(filepath.Join(s.dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", 0, err
		}
		size, copyErr := io.Copy(f, from)
		if err := errors.Join(copyErr, f.Close()); err != nil {
			os.Remove(f.Name())
			return "", 0, err
		}
		return f.Name(), size, nil
	}
	return "", 0, fmt.Errorf("%s: too many drops with this name", name)
}

// browse opens the page in the user's browser.
func browse(address string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", address).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", address).Start()
	}
	return exec.Command("xdg-open", address).Start()
}

func exitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errCancelled):
		return 2
	case errors.Is(err, errTimeout):
		return 124
	}
	return 1
}
