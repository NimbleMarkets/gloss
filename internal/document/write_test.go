package document

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
)

func TestWriteNewFromNumbersAndNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.png")
	want := []string{"page.png", "page-2.png", "page-3.png"}
	for i, name := range want {
		got, size, err := WriteNewFrom(path, strings.NewReader("data"+string(rune('a'+i))), 0o600)
		if err != nil || filepath.Base(got) != name || size != 5 {
			t.Fatalf("write %d: %q %d %v, want %s", i, got, size, err, name)
		}
	}
	for i, name := range want {
		if data, _ := os.ReadFile(filepath.Join(dir, name)); string(data) != "data"+string(rune('a'+i)) {
			t.Errorf("%s holds %q: an earlier write was overwritten or crossed", name, data)
		}
	}
	// A name with no extension numbers at its end, and a dot in the folder is not one.
	dotted := filepath.Join(dir, "v1.2", "download")
	os.MkdirAll(filepath.Dir(dotted), 0o700)
	a, _, _ := WriteNewFrom(dotted, strings.NewReader("1"), 0o600)
	b, _, _ := WriteNewFrom(dotted, strings.NewReader("2"), 0o600)
	if filepath.Base(a) != "download" || filepath.Base(b) != "download-2" || filepath.Dir(b) != filepath.Dir(dotted) {
		t.Errorf("no extension: %q then %q", a, b)
	}
}

func TestWriteNewFromKeepsPermissionsAndStreams(t *testing.T) {
	dir := t.TempDir()
	got, size, err := WriteNewFrom(filepath.Join(dir, "private.bin"), strings.NewReader(strings.Repeat("x", 1<<20)), 0o600)
	if err != nil || size != 1<<20 {
		t.Fatalf("%q %d %v", got, size, err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(got); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
	// A bounded reader is the caller's to bound, and the size says what came.
	_, size, _ = WriteNewFrom(filepath.Join(dir, "capped.bin"), io.LimitReader(strings.NewReader(strings.Repeat("y", 100)), 10+1), 0o600)
	if size != 11 {
		t.Errorf("a reader limited to 11 gave %d", size)
	}
	// WriteNew is this with bytes and the permissions of a file the user asked for.
	p, err := WriteNew(filepath.Join(dir, "out.txt"), []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(p); info.Mode().Perm() != 0o644 {
			t.Errorf("WriteNew mode = %v, want 0644", info.Mode().Perm())
		}
	}
}

// A copy that fails halfway leaves nothing, so a half-sent drop is never taken
// for a file, and the name is free again.
func TestWriteNewFromLeavesNothingWhenTheCopyFails(t *testing.T) {
	dir := t.TempDir()
	boom := errors.New("connection lost")
	_, _, err := WriteNewFrom(filepath.Join(dir, "half.bin"), io.MultiReader(strings.NewReader("part"), iotest.ErrReader(boom)), 0o600)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
	if got, _, err := WriteNewFrom(filepath.Join(dir, "half.bin"), strings.NewReader("whole"), 0o600); err != nil || filepath.Base(got) != "half.bin" {
		t.Fatalf("the name was not free again: %q %v", got, err)
	}
}

func TestWriteNewFromGivesUpOnTooManyOfAName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	os.WriteFile(path, nil, 0o600)
	for n := 2; n < 1000; n++ {
		os.WriteFile(filepath.Join(dir, "a-"+itoa(n)+".txt"), nil, 0o600)
	}
	if _, _, err := WriteNewFrom(path, strings.NewReader("x"), 0o600); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("err = %v", err)
	}
}

func itoa(n int) string { return fmt.Sprint(n) }
