package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/NimbleMarkets/gloss/internal/document"
)

// A pick asked for with no terminal to answer it must not hold its caller.
// gloss then starts the server as a process of its own, prints where it
// is, and leaves. The answer is kept in a small state file, read back by
// `gloss --resume TOKEN`, so nothing needs the first process to stay alive.
//
// Two secrets come of a start. The page's address carries the server's own
// token, which lets a browser in and nothing more. The resume token names the
// state file, and is what --resume, --status, and --cancel take: the address
// does not lead to it.

// protocolVersion is put on every object a harness reads of a session, so
// that one can tell the shape it is reading. It changes only when a field
// changes meaning or goes; new fields do not change it.
const protocolVersion = 1

// keepSettled is how long, past its deadline, a session's state is kept for
// --status and --resume when nobody cancels it. A later start removes it.
const keepSettled = 24 * time.Hour

// defaultDetachedTimeout bounds a server nobody is waiting on.
const defaultDetachedTimeout = 10 * time.Minute

// startGrace is how long the server has to come up, and to be seen to.
var startGrace = 15 * time.Second // A variable so that tests can shorten it.

// executable finds the program to start as the server; tests stand in another.
var executable = os.Executable

var tokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

const (
	statusWaiting  = "waiting"
	statusPicked   = "picked"
	statusDeclined = "declined"
	statusTimeout  = "timeout"
	statusClosed   = "closed" // A page that only showed something was closed.
	statusError    = "error"
)

// state is what the server says of itself, and finally of the answer.
type state struct {
	Status   string    `json:"status"`
	URL      string    `json:"url,omitempty"`
	Dir      string    `json:"dir"`
	PID      int       `json:"pid,omitempty"`
	Deadline time.Time `json:"deadline,omitempty"` // After this a waiting server is taken as timed out.
	Paths    []string  `json:"paths,omitempty"`
	Message  string    `json:"message,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// started is the one object a detached start prints on stdout.
type started struct {
	Protocol       int    `json:"protocol" const:"1" doc:"Version of this object's shape."`
	Status         string `json:"status" enum:"waiting" doc:"Always waiting: the session started, nobody has answered yet."`
	URL            string `json:"url" doc:"The page for the human, on 127.0.0.1. Its token lets a browser in, and nothing else; treat it as a secret."`
	Dir            string `json:"dir" doc:"The private folder (mode 0700) where dropped files land."`
	TimeoutSeconds int    `json:"timeout_seconds" doc:"How long the server lives."`
	ResumeToken    string `json:"resume_token" doc:"The secret that --resume, --status, and --cancel take; not the token in url."`
	Resume         string `json:"resume" doc:"The command that collects the answer."`
	Pick           bool   `json:"pick" doc:"Whether the session asks for files (--pick) or only shows them."`
}

// The folder for drops, and the file for state, are named by a hash of the
// token, so that listing the temporary directory does not give it away.
func detachedPaths(token string) (dir, file string) {
	sum := sha256.Sum256([]byte(token))
	base := filepath.Join(os.TempDir(), "gloss-pick-"+hex.EncodeToString(sum[:8]))
	return base, base + ".json"
}

func writeState(file string, st state) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".gloss-state-*")
	if err != nil {
		return err
	}
	_, writeErr := tmp.Write(data)
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0600); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), file) // Atomic: a reader sees all of it or the last.
}

func readState(file string) (state, error) {
	var st state
	data, err := os.ReadFile(file)
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(data, &st)
}

// detach starts the server in a process of its own and prints, as one JSON
// object, where to find it and how to ask what came of it. args are those
// gloss was given; "-" in them is replaced by standard input, kept in the
// private folder, since the server will have none.
func detach(args []string, opts options, stdout, stderr io.Writer) error {
	pruneSessions(time.Now())
	token, err := random()
	if err != nil {
		return err
	}
	dir, file := detachedPaths(token)
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	failed := true
	defer func() {
		if failed {
			os.RemoveAll(dir)
			os.Remove(file)
		}
	}()
	child := slices.Clone(args)
	for i, arg := range child {
		if arg != "-" {
			continue
		}
		data, err := document.ReadLimited(os.Stdin)
		if err != nil {
			return fmt.Errorf("stdin: %w", err)
		}
		kept, err := document.WriteNew(filepath.Join(dir, "stdin"), data)
		if err != nil {
			return err
		}
		for j := i; j < len(child); j++ {
			if child[j] == "-" {
				child[j] = kept
			}
		}
		break
	}
	timeout := opts.Timeout
	extra := []string{"--serve", "--no-open", "--detached=" + token}
	if timeout == 0 {
		timeout = defaultDetachedTimeout
		extra = append(extra, "--timeout="+timeout.String())
	}
	// Ahead of the rest, so that nothing after a -- is taken for a flag.
	child = append(extra, child...)
	exe, err := executable()
	if err != nil {
		return err
	}
	st := state{Status: statusWaiting, Dir: dir, Deadline: time.Now().Add(timeout + startGrace)}
	if err := writeState(file, st); err != nil {
		return err
	}
	cmd := exec.Command(exe, child...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil // The null device: the server answers through its state.
	detached(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Watch the child for the moment, so that one that dies at once (a flag it
	// refuses, say) is said so, and not waited on for the whole grace period.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	// Say where it is once it is up, or why it is not. A child that is not up
	// by then is stopped: it would otherwise run on, to its timeout, with a
	// state file nobody holds the token to.
	for end := time.Now().Add(startGrace); ; {
		cur, err := readState(file)
		if err == nil && cur.Status == statusError {
			return errors.New(cur.Error)
		}
		if err == nil && cur.URL != "" {
			st = cur
			break
		}
		select {
		case <-exited:
			return errors.New("the server stopped before it was up")
		case <-time.After(25 * time.Millisecond):
		}
		if time.Now().After(end) {
			_ = cmd.Process.Kill()
			return errors.New("the server did not start")
		}
	}
	failed = false
	return json.NewEncoder(stdout).Encode(started{
		Protocol:       protocolVersion,
		Status:         statusWaiting,
		URL:            st.URL,
		Dir:            dir,
		TimeoutSeconds: int(timeout.Seconds()),
		ResumeToken:    token,
		Resume:         "gloss --resume " + token,
		Pick:           opts.Pick,
	})
}

// pruneSessions removes the state of sessions long over, which nobody
// cancelled: their deadline passed more than keepSettled ago. A session that
// still says waiting was abandoned, and its dropped files go too; an answer's
// files are the caller's, and stay.
func pruneSessions(now time.Time) {
	files, _ := filepath.Glob(filepath.Join(os.TempDir(), "gloss-pick-*.json"))
	for _, file := range files {
		st, err := readState(file)
		if err != nil || st.Deadline.IsZero() || !now.After(st.Deadline.Add(keepSettled)) {
			continue
		}
		if st.Status == statusWaiting {
			os.RemoveAll(strings.TrimSuffix(file, ".json"))
		}
		os.Remove(file)
	}
}

// announce records where the detached server is.
func announce(token, address string) {
	dir, file := detachedPaths(token)
	st, err := readState(file)
	if err != nil {
		st = state{Dir: dir}
	}
	st.Status, st.URL, st.PID = statusWaiting, address, os.Getpid()
	_ = writeState(file, st)
}

// conclude records how the detached server ended. What was dropped is
// kept only for an answer; for anything else the folder goes first, so that
// whoever reads the state can count on it being gone.
func conclude(token string, err error, paths []string, pick bool, message string) {
	dir, file := detachedPaths(token)
	st, readErr := readState(file)
	if errors.Is(readErr, fs.ErrNotExist) {
		os.RemoveAll(dir) // Cancelled: nothing is to be kept, or said.
		return
	}
	st.Dir, st.URL, st.Paths, st.Message = dir, "", nil, ""
	switch {
	case err == nil && pick:
		st.Status, st.Paths, st.Message = statusPicked, paths, message
	case err == nil:
		st.Status = statusClosed
	case errors.Is(err, errCancelled):
		st.Status = statusDeclined
	case errors.Is(err, errTimeout):
		st.Status = statusTimeout
	default:
		st.Status, st.Error = statusError, err.Error()
	}
	if st.Status != statusPicked {
		os.RemoveAll(dir)
	}
	_ = writeState(file, st)
}

// resume waits for the answer of a detached pick and gives it as pick does:
// the paths, one to a line, and exit status 0, 2, or 124. With --timeout it
// gives up on waiting, not on the pick, which a later call can still ask.
func resume(opts options, stdout, stderr io.Writer) error {
	if !tokenPattern.MatchString(opts.Resume) {
		return fmt.Errorf("--resume takes the token the start printed")
	}
	dir, file := detachedPaths(opts.Resume)
	var late <-chan time.Time
	if opts.Timeout > 0 {
		late = time.After(opts.Timeout)
	}
	for {
		st, derived, err := observe(file, time.Now())
		if err != nil {
			return errNoSession
		}
		if derived {
			// The server is gone or late; the files are not left behind. This
			// is the part of asking that changes anything: --status leaves it.
			os.RemoveAll(dir)
			_ = writeState(file, st)
			continue
		}
		if st.Status == statusWaiting {
			select {
			case <-late:
				fmt.Fprintln(stderr, "gloss: still waiting; the pick itself has not timed out")
				return errTimeout
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		return answer(st, opts.JSON, stdout)
	}
}

var errNoSession = errors.New("no such session: the token is not a resume token gloss gave, or the session was cancelled")

// observe reads a pick's state as it now stands, without changing it. A state
// that says waiting may be out of date: the deadline has passed, or the server
// died without writing its answer. Then the state is what it has become, and
// derived says it was worked out here and is not on disk. Asking the question
// is the same for --resume, which then settles the matter, and for --status,
// which does not.
func observe(file string, now time.Time) (st state, derived bool, err error) {
	st, err = readState(file)
	if err != nil || st.Status != statusWaiting {
		return st, false, err
	}
	switch {
	case !st.Deadline.IsZero() && now.After(st.Deadline):
		st.Status = statusTimeout
		return st, true, nil
	case st.PID != 0 && !running(st.PID):
		// It may have written its answer in its last moment.
		if again, err := readState(file); err == nil && again.Status != statusWaiting {
			return again, false, nil
		}
		st.Status, st.Error = statusError, "the server ended without an answer"
		return st, true, nil
	}
	return st, false, nil
}

// sessionStatus is what --status prints: how a pick stands, at once.
type sessionStatus struct {
	Protocol    int      `json:"protocol" const:"1" doc:"Version of this object's shape."`
	State       string   `json:"state" enum:"waiting,picked,declined,timeout,closed,failed" doc:"How the session stands."`
	Settled     bool     `json:"settled" doc:"False only while waiting."`
	Paths       []string `json:"paths,omitempty" doc:"The answer, once picked."`
	Message     string   `json:"message,omitempty" doc:"Optional user-authored message sent with the files by --pick-web (up to 2,000 Unicode characters); present only when picked and nonempty."`
	Error       string   `json:"error,omitempty" doc:"Why, when failed."`
	SecondsLeft *int     `json:"seconds_left,omitempty" doc:"About how long the session has, while waiting."`
}

// status says at once how a pick stands, as one JSON object, and exit status
// 0: the state is in the object. It waits for nothing and changes nothing,
// neither the answer, nor the files, nor the session, so it can be asked as
// often as wanted, and --resume then answers as it would have.
func status(opts options, stdout io.Writer) error {
	if !tokenPattern.MatchString(opts.Status) {
		return fmt.Errorf("--status takes the token the start printed")
	}
	_, file := detachedPaths(opts.Status)
	now := time.Now()
	st, _, err := observe(file, now)
	if err != nil {
		return errNoSession
	}
	out := sessionStatus{Protocol: protocolVersion, State: st.Status, Settled: st.Status != statusWaiting, Error: st.Error}
	switch st.Status {
	case statusError:
		out.State = "failed"
	case statusPicked:
		out.Paths = st.Paths
		out.Message = st.Message
	case statusWaiting:
		if !st.Deadline.IsZero() {
			// The deadline allows the server its start-up as well as its run.
			left := max(0, int(math.Ceil(st.Deadline.Add(-startGrace).Sub(now).Seconds())))
			out.SecondsLeft = &left
		}
	}
	return json.NewEncoder(stdout).Encode(out)
}

// resumed is what --resume --json prints, also used for a confirmed
// foreground --pick-web --json answer.
type resumed struct {
	Protocol int      `json:"protocol" const:"1" doc:"Version of this object's shape."`
	Status   string   `json:"status" enum:"picked,declined,timeout,closed,error" doc:"How the session ended."`
	Paths    []string `json:"paths" doc:"The answer when picked; null otherwise."`
	Message  string   `json:"message,omitempty" doc:"Optional user-authored message sent with the files by --pick-web (up to 2,000 Unicode characters); omitted when empty."`
	Error    string   `json:"error" doc:"Why, when status is error; empty otherwise."`
}

// answer gives a settled state's outcome. The state stays, so that --status
// still says what came of the session, and --resume says it again, until
// --cancel or pruneSessions removes it.
func answer(st state, asJSON bool, stdout io.Writer) error {
	if st.Status != statusPicked {
		st.Message = ""
	}
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(resumed{Protocol: protocolVersion, Status: st.Status, Paths: st.Paths, Message: st.Message, Error: st.Error})
	}
	switch st.Status {
	case statusPicked:
		if !asJSON {
			for _, path := range st.Paths {
				fmt.Fprintln(stdout, path)
			}
		}
		return nil
	case statusClosed:
		return nil
	case statusDeclined:
		return errCancelled
	case statusTimeout:
		return errTimeout
	}
	return errors.New(st.Error)
}

// cancelWait is how long --cancel gives a server to notice and go, before it
// is stopped. A variable so that tests can shorten it.
var cancelWait = 5 * time.Second

// cancel ends a session at once, whatever it stands at: its state goes, which
// the server watches for and stops at, and so does its folder, dropped files
// and answer alike. --status and --resume then know of no such session. It
// prints nothing.
func cancel(opts options) error {
	if !tokenPattern.MatchString(opts.Cancel) {
		return fmt.Errorf("--cancel takes the resume token the start printed")
	}
	dir, file := detachedPaths(opts.Cancel)
	st, err := readState(file)
	if err != nil {
		return errNoSession
	}
	clear := func() {
		os.Remove(file)
		os.RemoveAll(dir)
	}
	clear()
	if st.Status == statusWaiting && st.PID != 0 {
		for end := time.Now().Add(cancelWait); running(st.PID) && time.Now().Before(end); {
			time.Sleep(50 * time.Millisecond)
		}
		if running(st.PID) {
			if p, err := os.FindProcess(st.PID); err == nil {
				_ = p.Kill()
			}
		}
		clear() // What a server wrote, or dropped, as it went.
	}
	return nil
}

// watchCancel stops a detached server once its state is gone, which is how
// --cancel tells it to.
func watchCancel(token string, stop func(), done <-chan struct{}) {
	_, file := detachedPaths(token)
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			if _, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) {
				stop()
				return
			}
		}
	}
}
