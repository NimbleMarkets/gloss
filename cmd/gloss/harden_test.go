package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
)

// A range of pages is listed in full, so a range must be bounded: --page
// 1-2000000000 once asked for gigabytes, and one ending at the largest integer
// looped for ever.
func TestPageRangesAreBounded(t *testing.T) {
	for _, bad := range []string{"1-2000000000", "1-9223372036854775807", "5-99999", "1-5000,5001-10001", "1-10001"} {
		done := make(chan error, 1)
		go func() { _, err := parsePages(bad); done <- err }()
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "more than") {
				t.Errorf("--page %s: err = %v, want a refusal", bad, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("--page %s did not return: it is being listed", bad)
		}
	}
	for good, n := range map[string]int{"1-10000": 10000, "1-5000,5001-10000": 10000, "3": 1, "2-5": 4, "99999999999": 1, "1,3-4": 3} {
		got, err := parsePages(good)
		if err != nil || len(got.pages) != n {
			t.Errorf("--page %s: %d pages, %v; want %d", good, len(got.pages), err, n)
		}
	}
}

func FuzzParsePages(f *testing.F) {
	for _, seed := range []string{"3", "1-5", "1,3-5", "all", " 2 - 4 ", "0", "-1", "5-2", "1-", "-", ",", "1-9223372036854775807", "9223372036854775808"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r, err := parsePages(s)
		if err == nil && !r.all && (len(r.pages) == 0 || len(r.pages) > document.MaxPages) {
			t.Fatalf("%q gave %d pages", s, len(r.pages))
		}
	})
}

// What a session keeps from the page is bounded, over every drop, and a drop
// that fails gives back what it took.
func TestServedDropsShareASessionBudget(t *testing.T) {
	old := maxSessionBytes
	maxSessionBytes = 100
	t.Cleanup(func() { maxSessionBytes = old })
	s := serving(t, app.Options{Pick: true})
	sixty := bytes.Repeat([]byte("x"), 60)
	code, first := drop(t, s.URL+"drop", nil, upload{"a.txt", sixty})
	if code != http.StatusOK || len(first) != 1 {
		t.Fatalf("the first drop: %d %q", code, first)
	}
	defer os.RemoveAll(filepath.Dir(first[0]))
	// 60 and 60 is over 100, in two requests: the second is refused whole.
	if code, paths := drop(t, s.URL+"drop", nil, upload{"b.txt", sixty}); code != http.StatusRequestEntityTooLarge || len(paths) != 0 {
		t.Fatalf("over the session's budget: %d %q", code, paths)
	}
	entries, _ := os.ReadDir(filepath.Dir(first[0]))
	if len(entries) != 1 {
		t.Fatalf("the refused drop left files behind: %v", entries)
	}
	// What a refused drop took was given back: 30 fits in the 40 left.
	if code, paths := drop(t, s.URL+"drop", nil, upload{"c.txt", bytes.Repeat([]byte("y"), 30)}); code != http.StatusOK || len(paths) != 1 {
		t.Fatalf("a small drop after a refused one: %d %q", code, paths)
	}
	// Several files in one request that together are too many: none is kept.
	if code, paths := drop(t, s.URL+"drop", nil, upload{"d.txt", bytes.Repeat([]byte("z"), 5)}, upload{"e.txt", sixty}); code != http.StatusRequestEntityTooLarge || len(paths) != 0 {
		t.Fatalf("one drop over the budget: %d %q", code, paths)
	}
	if code, paths := drop(t, s.URL+"drop", nil, upload{"f.txt", bytes.Repeat([]byte("w"), 5)}); code != http.StatusOK || len(paths) != 1 {
		t.Fatalf("the refused drop's files were not given back: %d %q", code, paths)
	}
}

func TestDroppedNamesAreUsable(t *testing.T) {
	for _, name := range []string{"report.pdf", "a b.png", "über.svg", "dots.in.name.csv"} {
		if !usableName(name) {
			t.Errorf("%q was refused", name)
		}
	}
	for _, name := range []string{"a\x00b", "tab\there", "bell\a", "del\x7f"} {
		if usableName(name) {
			t.Errorf("%q was accepted", name)
		}
	}
	// Where a name can mean a device or a stream, it may not.
	windowsOnly := []string{"CON", "nul.txt", "Com1.png", "a:b", "stream:evil.png", "a*b", "trailing.", "trailing "}
	for _, name := range windowsOnly {
		if got := usableName(name); got == (runtime.GOOS == "windows") {
			t.Errorf("%q: usableName = %v on %s", name, got, runtime.GOOS)
		}
	}
}

// The skill is written whole or not at all, and a symbolic link where its file
// goes is replaced, not written through to what it points at.
func TestSkillInstallDoesNotWriteThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "gloss")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	precious := filepath.Join(root, "precious.txt")
	if err := os.WriteFile(precious, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(precious, filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if err := installSkill(filepath.Join(root, "skills"), &out, &errs); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(precious); string(got) != "mine" {
		t.Fatalf("the install wrote through the link: %q", got)
	}
	info, err := os.Lstat(filepath.Join(skillDir, "SKILL.md"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o644 {
		t.Fatalf("SKILL.md afterwards: %v %v", info, err)
	}
	entries, _ := os.ReadDir(skillDir)
	if len(entries) != 1 {
		t.Fatalf("a temporary file was left: %v", entries)
	}
}

// A server that does not come up is stopped, and one that dies at once is
// said so without waiting out the grace period.
func TestDetachedStartStopsAServerThatDoesNotComeUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts stand in for the server")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	script := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	oldExe, oldGrace := executable, startGrace
	t.Cleanup(func() { executable, startGrace = oldExe, oldGrace })
	startGrace = 20 * time.Second

	// Dies at once: reported at once, not after the grace period.
	executable = func() (string, error) { return script("dies", "exit 3"), nil }
	began := time.Now()
	var stdout, stderr bytes.Buffer
	err := detach([]string{"--serve"}, options{}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "stopped") || time.Since(began) > 5*time.Second {
		t.Fatalf("a server that died: err=%v after %v", err, time.Since(began))
	}

	// Alive but never up: stopped when the grace runs out.
	startGrace = 500 * time.Millisecond
	executable = func() (string, error) { return script("hangs", "echo $$ > "+pidFile+"\nexec sleep 60"), nil }
	err = detach([]string{"--serve"}, options{}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "did not start") {
		t.Fatalf("a server that hung: %v", err)
	}
	raw, rerr := os.ReadFile(pidFile)
	pid, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if rerr != nil || perr != nil {
		t.Fatalf("the stand-in server never wrote its pid: %v %v", rerr, perr)
	}
	for end := time.Now().Add(3 * time.Second); running(pid); {
		if time.Now().After(end) {
			t.Fatalf("the server that did not start is still running (pid %d)", pid)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
