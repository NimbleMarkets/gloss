package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef"

// settle puts a state in place as the detached server would leave it.
func settle(t *testing.T, token string, st state) (dir string) {
	t.Helper()
	dir, file := detachedPaths(token)
	t.Cleanup(func() { os.RemoveAll(dir); os.Remove(file) })
	os.MkdirAll(dir, 0700)
	st.Dir = dir
	if err := writeState(file, st); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResumeAnswersAsPickDoes(t *testing.T) {
	dir := settle(t, testToken, state{Status: statusPicked})
	picked := filepath.Join(dir, "invoice.pdf")
	os.WriteFile(picked, []byte("x"), 0600)
	_, file := detachedPaths(testToken)
	st, _ := readState(file)
	st.Paths = []string{picked}
	writeState(file, st)

	var stdout, stderr bytes.Buffer
	if err := resume(options{Resume: testToken}, &stdout, &stderr); err != nil || strings.TrimSpace(stdout.String()) != picked {
		t.Fatalf("picked: %q %v", stdout.String(), err)
	}
	// An answer stays while its files do, so a second ask gives it again.
	stdout.Reset()
	if err := resume(options{Resume: testToken}, &stdout, &stderr); err != nil || strings.TrimSpace(stdout.String()) != picked {
		t.Fatalf("picked again: %q %v", stdout.String(), err)
	}
	os.RemoveAll(dir)
	if err := resume(options{Resume: testToken}, &stdout, &stderr); err != nil {
		t.Fatal(err) // Last answer, then the state goes with the files.
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("state left behind: %v", err)
	}
}

func TestResumeExitCodes(t *testing.T) {
	for status, want := range map[string]int{statusDeclined: 2, statusTimeout: 124, statusError: 1, statusClosed: 0} {
		settle(t, testToken, state{Status: status, Error: "boom"})
		var stdout bytes.Buffer
		err := resume(options{Resume: testToken}, &stdout, &bytes.Buffer{})
		if got := exitCode(err); got != want || stdout.Len() != 0 {
			t.Errorf("%s: exit %d (%v), stdout %q; want %d", status, got, err, stdout.String(), want)
		}
	}
}

func TestResumeTimesOutAWaitingServerThatIsGone(t *testing.T) {
	dir := settle(t, testToken, state{Status: statusWaiting, Deadline: time.Now().Add(-time.Second)})
	os.WriteFile(filepath.Join(dir, "dropped.pdf"), []byte("x"), 0600)
	err := resume(options{Resume: testToken}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, errTimeout) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("dropped files outlived the timeout")
	}
}

func TestResumeOwnTimeoutLeavesThePickAlone(t *testing.T) {
	dir := settle(t, testToken, state{Status: statusWaiting, Deadline: time.Now().Add(time.Hour)})
	err := resume(options{Resume: testToken, Timeout: 300 * time.Millisecond}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, errTimeout) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("waiting gave up the pick")
	}
}

func TestResumeRefusesWhatIsNotAToken(t *testing.T) {
	for _, token := range []string{"../../etc/passwd", "short", strings.Repeat("g", 32)} {
		if err := resume(options{Resume: token}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %q", token)
		}
	}
	if err := resume(options{Resume: strings.Repeat("a", 32)}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || exitCode(err) != 1 {
		t.Fatalf("unknown token: %v", err)
	}
}

func TestConcludeKeepsFilesOnlyForAnAnswer(t *testing.T) {
	for _, tt := range []struct {
		err   error
		pick  bool
		keeps bool
		want  string
	}{
		{nil, true, true, statusPicked}, {nil, false, false, statusClosed},
		{errCancelled, true, false, statusDeclined}, {errTimeout, true, false, statusTimeout},
		{errors.New("bad"), true, false, statusError},
	} {
		dir := settle(t, testToken, state{Status: statusWaiting})
		conclude(testToken, tt.err, []string{filepath.Join(dir, "a")}, tt.pick)
		_, file := detachedPaths(testToken)
		st, err := readState(file)
		_, statErr := os.Stat(dir)
		if err != nil || st.Status != tt.want || (statErr == nil) != tt.keeps {
			t.Errorf("%v: %+v kept=%v", tt.err, st, statErr == nil)
		}
		if tt.want != statusPicked && len(st.Paths) != 0 {
			t.Errorf("%s keeps paths: %v", tt.want, st.Paths)
		}
	}
}

func TestResumeFlags(t *testing.T) {
	opts, _, err := parse([]string{"--resume", testToken, "--timeout", "5s", "--json"}, &bytes.Buffer{})
	if err != nil || opts.Resume != testToken || opts.Timeout != 5*time.Second {
		t.Fatalf("%+v %v", opts, err)
	}
	for _, args := range [][]string{{"--resume", testToken, "a.svg"}, {"--resume", testToken, "--pick"}, {"--resume", testToken, "--text"}, {"--detached", testToken}, {"--serve", "--detached", "nope"}} {
		if _, _, err := parse(args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

// With no terminal, a pick prints one JSON object and leaves; the server
// goes on apart, and a second call gives the answer once it times out.
func TestDetachedPickDoesNotBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detached server is exercised on Unix")
	}
	bin := filepath.Join(t.TempDir(), "gloss")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, args := range [][]string{{"--pick", "--prompt", "A file?"}, {"--serve", "--pick", "--timeout", "2s"}} {
		args = append(args, "--timeout", "2s")
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(bin, args...)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr // Stdin is the null device: no terminal.
		began := time.Now()
		if err := cmd.Run(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, stderr.String())
		}
		if time.Since(began) > 1500*time.Millisecond {
			t.Fatalf("%v held its caller for %v", args, time.Since(began))
		}
		var got started
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || bytes.Count(bytes.TrimSpace(stdout.Bytes()), []byte("\n")) != 0 {
			t.Fatalf("stdout is not one JSON object: %q", stdout.String())
		}
		if !strings.HasPrefix(got.URL, "http://127.0.0.1:") || !strings.HasSuffix(got.URL, "/") || got.TimeoutSeconds != 2 || !got.Pick ||
			got.Resume != "gloss --resume "+got.ResumeToken || !tokenPattern.MatchString(got.ResumeToken) || got.Dir == "" {
			t.Fatalf("started = %+v", got)
		}
		if info, err := os.Stat(got.Dir); err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("drop folder: %v %v", info, err)
		}
		// Nobody comes: the answer is the timeout, and nothing is left.
		var resumed bytes.Buffer
		wait := exec.Command(bin, "--resume", got.ResumeToken)
		wait.Stdout = &resumed
		err := wait.Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 124 || resumed.Len() != 0 {
			t.Fatalf("resume: %v, stdout %q", err, resumed.String())
		}
		if _, err := os.Stat(got.Dir); !os.IsNotExist(err) {
			t.Fatal("the drop folder outlived the timeout")
		}
	}
}

func TestDetachedStartReportsABadInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detached server is exercised on Unix")
	}
	bin := filepath.Join(t.TempDir(), "gloss")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "--serve", "/no/such/file.pdf")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
}
