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

// ask runs --status as the command line would, and decodes its object.
func ask(t *testing.T, token string) (sessionStatus, error) {
	t.Helper()
	var out bytes.Buffer
	err := status(options{Status: token}, &out)
	var got sessionStatus
	if err == nil {
		if e := json.Unmarshal(out.Bytes(), &got); e != nil || bytes.Count(bytes.TrimSpace(out.Bytes()), []byte("\n")) != 0 {
			t.Fatalf("not one JSON object: %q (%v)", out.String(), e)
		}
	}
	return got, err
}

func stateFile(t *testing.T) []byte {
	t.Helper()
	_, file := detachedPaths(testToken)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestStatusNamesEachState(t *testing.T) {
	soon := time.Now().Add(30*time.Second + startGrace)
	for _, c := range []struct {
		name    string
		st      state
		want    string
		settled bool
	}{
		{"waiting", state{Status: statusWaiting, Deadline: soon}, "waiting", false},
		{"picked", state{Status: statusPicked, Paths: []string{"/tmp/a.png", "/tmp/b.png"}}, "picked", true},
		{"declined", state{Status: statusDeclined}, "declined", true},
		{"timed out", state{Status: statusTimeout}, "timeout", true},
		{"closed", state{Status: statusClosed}, "closed", true},
		{"failed", state{Status: statusError, Error: "boom"}, "failed", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			settle(t, testToken, c.st)
			got, err := ask(t, testToken)
			if err != nil || got.State != c.want || got.Settled != c.settled {
				t.Fatalf("got %+v, %v; want state %s settled %v", got, err, c.want, c.settled)
			}
			if (c.want == "picked") != (len(got.Paths) == 2) {
				t.Errorf("paths = %v", got.Paths)
			}
			if c.want == "failed" && got.Error != "boom" {
				t.Errorf("error = %q", got.Error)
			}
			if c.want == "waiting" && (got.SecondsLeft == nil || *got.SecondsLeft < 25 || *got.SecondsLeft > 30) {
				t.Errorf("seconds left = %v, want about 30 (the start-up grace is not the pick's)", got.SecondsLeft)
			}
			if c.want != "waiting" && got.SecondsLeft != nil {
				t.Errorf("a settled pick has no time left: %v", *got.SecondsLeft)
			}
		})
	}
}

// What a state has become without being told is worked out, not written: the
// answer to the next --resume is still its own.
func TestStatusWorksOutWhatItIsWithoutChangingIt(t *testing.T) {
	// A deadline that has passed.
	dir := settle(t, testToken, state{Status: statusWaiting, Deadline: time.Now().Add(-time.Minute)})
	before := stateFile(t)
	for i := 0; i < 3; i++ {
		if got, err := ask(t, testToken); err != nil || got.State != "timeout" || !got.Settled {
			t.Fatalf("overdue: %+v %v", got, err)
		}
	}
	if !bytes.Equal(stateFile(t), before) {
		t.Fatal("--status rewrote the state")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("--status removed the folder: %v", err)
	}
	// --resume is the one that settles it, as it always did.
	var out, errs bytes.Buffer
	if err := resume(options{Resume: testToken}, &out, &errs); !errors.Is(err, errTimeout) {
		t.Fatalf("resume of an overdue pick: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("--resume no longer removes the folder of a pick that ran out")
	}

	// A server that is gone.
	if runtime.GOOS == "windows" {
		return
	}
	gone := exec.Command("true")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	dir = settle(t, testToken, state{Status: statusWaiting, PID: gone.Process.Pid, Deadline: time.Now().Add(time.Hour)})
	before = stateFile(t)
	got, err := ask(t, testToken)
	if err != nil || got.State != "failed" || !strings.Contains(got.Error, "without an answer") {
		t.Fatalf("dead server: %+v %v", got, err)
	}
	if !bytes.Equal(stateFile(t), before) {
		t.Fatal("--status rewrote the state of a dead server")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("--status removed the folder of a dead server: %v", err)
	}
}

// Asking does not use the answer up: ask as often as you like, then resume.
func TestStatusDoesNotConsumeTheAnswer(t *testing.T) {
	dir := settle(t, testToken, state{Status: statusPicked})
	picked := filepath.Join(dir, "invoice.pdf")
	os.WriteFile(picked, []byte("x"), 0o600)
	_, file := detachedPaths(testToken)
	st, _ := readState(file)
	st.Paths = []string{picked}
	writeState(file, st)
	before := stateFile(t)

	var first sessionStatus
	for i := 0; i < 4; i++ {
		got, err := ask(t, testToken)
		if err != nil || got.State != "picked" || len(got.Paths) != 1 || got.Paths[0] != picked {
			t.Fatalf("ask %d: %+v %v", i, got, err)
		}
		if i == 0 {
			first = got
		} else if got.State != first.State || got.Paths[0] != first.Paths[0] {
			t.Fatalf("ask %d differs: %+v vs %+v", i, got, first)
		}
	}
	if !bytes.Equal(stateFile(t), before) {
		t.Fatal("asking changed the state")
	}
	var out, errs bytes.Buffer
	if err := resume(options{Resume: testToken}, &out, &errs); err != nil || strings.TrimSpace(out.String()) != picked {
		t.Fatalf("resume after status: %q %v", out.String(), err)
	}
	if got, err := ask(t, testToken); err != nil || got.State != "picked" {
		t.Fatalf("status after resume, files still there: %+v %v", got, err)
	}

	// A decline is used up by resume, and not before: status never uses it.
	settle(t, testToken, state{Status: statusDeclined})
	for i := 0; i < 3; i++ {
		if got, err := ask(t, testToken); err != nil || got.State != "declined" {
			t.Fatalf("declined, ask %d: %+v %v", i, got, err)
		}
	}
	if err := resume(options{Resume: testToken}, &out, &errs); !errors.Is(err, errCancelled) {
		t.Fatalf("resume of a decline: %v", err)
	}
	if _, err := ask(t, testToken); !errors.Is(err, errNoSession) {
		t.Fatalf("status after the decline was collected: %v", err)
	}
}

func TestStatusRefusesWhatIsNotAPick(t *testing.T) {
	for _, bad := range []string{"", "nope", "0123456789ABCDEF0123456789ABCDEF", testToken + "0", "../" + testToken[3:]} {
		if _, err := ask(t, bad); err == nil || exitCode(err) != 1 {
			t.Errorf("token %q: err = %v", bad, err)
		}
	}
	// Well formed, and no such pick.
	if _, err := ask(t, strings.Repeat("a", 32)); !errors.Is(err, errNoSession) || exitCode(err) != 1 {
		t.Errorf("an unknown pick: %v", err)
	}
	for _, args := range [][]string{
		{"--status", testToken, "a.svg"}, {"--status", testToken, "--pick"}, {"--status", testToken, "--serve"},
		{"--status", testToken, "--json"}, {"--status", testToken, "--timeout", "1s"}, {"--status", testToken, "--resume", testToken},
		{"--status", testToken, "--info"}, {"--status", testToken, "-o", "x.png"}, {"--status", testToken, "--prompt", "hi"},
	} {
		if _, _, err := parse(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
	if opts, done, err := parse([]string{"--status", testToken}, &bytes.Buffer{}); err != nil || done || opts.Status != testToken {
		t.Errorf("--status alone: %+v %v %v", opts, done, err)
	}
}

// The whole thing, with the real server: asking at once finds it waiting, and
// asking again, and asking after it has run out, change nothing about what
// --resume says.
func TestStatusOfARealDetachedPick(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detached server is exercised on Unix")
	}
	bin := filepath.Join(t.TempDir(), "gloss")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	var started bytes.Buffer
	cmd := exec.Command(bin, "--pick", "--timeout", "2s")
	cmd.Stdout = &started // Stdin is the null device: no terminal, so it detaches.
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	var s struct {
		ResumeToken string `json:"resume_token"`
		Dir         string
	}
	if err := json.Unmarshal(started.Bytes(), &s); err != nil || !tokenPattern.MatchString(s.ResumeToken) {
		t.Fatalf("start: %q %v", started.String(), err)
	}
	t.Cleanup(func() { os.RemoveAll(s.Dir) })
	query := func() (sessionStatus, int) {
		var out bytes.Buffer
		c := exec.Command(bin, "--status", s.ResumeToken)
		c.Stdout = &out
		err := c.Run()
		code := 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		}
		var got sessionStatus
		_ = json.Unmarshal(out.Bytes(), &got)
		return got, code
	}
	began := time.Now()
	for i := 0; i < 3; i++ {
		got, code := query()
		if code != 0 || got.State != "waiting" || got.Settled || got.SecondsLeft == nil {
			t.Fatalf("while waiting, ask %d: %+v exit %d", i, got, code)
		}
	}
	if time.Since(began) > 3*time.Second {
		t.Fatalf("three questions took %v: --status must not wait", time.Since(began))
	}
	// The pick runs out; the question is answered the same, and unchanged.
	time.Sleep(3 * time.Second)
	for i := 0; i < 2; i++ {
		if got, code := query(); code != 0 || got.State != "timeout" || !got.Settled {
			t.Fatalf("after the timeout, ask %d: %+v exit %d", i, got, code)
		}
	}
	wait := exec.Command(bin, "--resume", s.ResumeToken)
	err := wait.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 124 {
		t.Fatalf("resume after status: %v", err)
	}
	if _, code := query(); code != 1 {
		t.Fatalf("status once the timeout was collected: exit %d, want 1", code)
	}
}
