package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/NimbleMarkets/gloss/internal/document"
)

// A pick asked for with no terminal to answer it must not hold its caller.
// gloss then starts the server as a process of its own, prints where it
// is, and leaves. The answer is kept in a small state file, read back by
// `gloss --resume TOKEN`, so nothing needs the first process to stay alive.

// defaultDetachedTimeout bounds a server nobody is waiting on.
const defaultDetachedTimeout = 10 * time.Minute

// startGrace is how long the server has to come up, and to be seen to.
const startGrace = 15 * time.Second

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
	Error    string    `json:"error,omitempty"`
}

// started is the one object a detached start prints on stdout.
type started struct {
	Status         string `json:"status"`
	URL            string `json:"url"`
	Dir            string `json:"dir"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	ResumeToken    string `json:"resume_token"`
	Resume         string `json:"resume"`
	Pick           bool   `json:"pick"`
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
	exe, err := os.Executable()
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
	_ = cmd.Process.Release()
	// Say where it is once it is up, or why it is not.
	for end := time.Now().Add(startGrace); ; time.Sleep(25 * time.Millisecond) {
		cur, err := readState(file)
		if err == nil && cur.Status == statusError {
			return errors.New(cur.Error)
		}
		if err == nil && cur.URL != "" {
			st = cur
			break
		}
		if time.Now().After(end) {
			return errors.New("the server did not start")
		}
	}
	failed = false
	return json.NewEncoder(stdout).Encode(started{
		Status:         statusWaiting,
		URL:            st.URL,
		Dir:            dir,
		TimeoutSeconds: int(timeout.Seconds()),
		ResumeToken:    token,
		Resume:         "gloss --resume " + token,
		Pick:           opts.Pick,
	})
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
func conclude(token string, err error, paths []string, pick bool) {
	dir, file := detachedPaths(token)
	st, _ := readState(file)
	st.Dir, st.URL, st.Paths = dir, "", nil
	switch {
	case err == nil && pick:
		st.Status, st.Paths = statusPicked, paths
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
		st, err := readState(file)
		if err != nil {
			return fmt.Errorf("no such pick, or it was already settled and its state removed")
		}
		if st.Status == statusWaiting {
			switch {
			case !st.Deadline.IsZero() && time.Now().After(st.Deadline):
				// The server is gone or late; the files are not left behind.
				st.Status = statusTimeout
				os.RemoveAll(dir)
				_ = writeState(file, st)
				continue
			case st.PID != 0 && !running(st.PID):
				// It may have written its answer in its last moment.
				if again, err := readState(file); err == nil && again.Status != statusWaiting {
					continue
				}
				st.Status, st.Error = statusError, "the server ended without an answer"
				os.RemoveAll(dir)
				_ = writeState(file, st)
				continue
			}
			select {
			case <-late:
				fmt.Fprintln(stderr, "gloss: still waiting; the pick itself has not timed out")
				return errTimeout
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		return answer(st, file, opts.JSON, stdout)
	}
}

// answer gives a settled state's outcome. A state that holds nothing more
// is removed with it; an answer is kept while its files are.
func answer(st state, file string, asJSON bool, stdout io.Writer) error {
	if st.Status != statusPicked {
		defer os.Remove(file)
	} else if _, err := os.Stat(st.Dir); err != nil {
		defer os.Remove(file) // The caller has cleaned up.
	}
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(map[string]any{"status": st.Status, "paths": st.Paths, "error": st.Error})
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
