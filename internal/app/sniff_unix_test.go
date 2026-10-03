//go:build unix

package app

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO blocks whoever opens it until something writes: sniffing a file's
// start must not open what is not a regular file, or the folder never lists.
func TestAFolderWithAPipeInItStillLists(t *testing.T) {
	dir := folder(t)
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600); err != nil {
		t.Skip("cannot make a FIFO here:", err)
	}
	if err := os.Symlink(filepath.Join(dir, "pipe"), filepath.Join(dir, "pipelink")); err != nil {
		t.Skip("cannot make a symlink here:", err)
	}
	done := make(chan *Model, 1)
	go func() { done <- browsing(t, dir) }()
	var m *Model
	select {
	case m = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the browser hangs on a folder with a FIFO: something opened it")
	}
	for _, name := range []string{"alpha.png", "pipe", "pipelink"} {
		if !listed(m, name) {
			t.Errorf("%s is not listed:\n%s", name, plain(m))
		}
	}
	// Neither is text, nor can be chosen.
	for _, name := range []string{"pipe", "pipelink"} {
		if got := line(t, m, name); !contains(got, "38;5;243") {
			t.Errorf("%s should be greyed: %q", name, got)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
