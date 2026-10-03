package browsetest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

func TestTape(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.txt")
	script := `-- fs --
a/b.txt
-- script --
option layout columns
size 100 20
note go in and filter
press down enter
type "re x"
press ctrl+l alt+up shift+tab pgdown Z home alt+left alt+x alt+enter ctrl+left shift+up
click 3 4
state dir /a
snapshot here
`
	if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := browsetest.Tape(file, browsetest.TapeOptions{Command: "gloss examples", Output: "x.gif", Pause: "200ms", Keys: map[string]string{"alt+left": "Ctrl+O"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `# Made from s.txt by browsetest.Tape; change the script, not this.
# The recording runs against a real folder: the script's fs part is not used.
Output "x.gif"
Set FontSize 16
Set Width 1080
Set Height 2520
Hide
Type "gloss examples"
Enter
Sleep 1s
Ctrl+L
Sleep 300ms
Show

# go in and filter
Down
Sleep 200ms
Enter
Sleep 200ms
Type "re x"
Sleep 200ms
Ctrl+L
Sleep 200ms
# (alt+up: VHS has no such key)
Shift+Tab
Sleep 200ms
PageDown
Sleep 200ms
Type "Z"
Sleep 200ms
# (home: VHS has no such key)
Ctrl+O
Sleep 200ms
Alt+X
Sleep 200ms
Alt+Enter
Sleep 200ms
Ctrl+Left
Sleep 200ms
# (shift+up: VHS has no such key)
# (click 3 4: VHS cannot do this)
Sleep 1500ms
Sleep 1s
`
	// The window is sized from the cells: 100 by 20.
	want = strings.Replace(want, "Set Height 2520", "Set Height 520", 1)
	if got != want {
		t.Errorf("tape:\n%s\nwant:\n%s", got, want)
	}
}

// With VHS installed, every script of the chooser must make a tape it accepts.
func TestTapesAreValidVHS(t *testing.T) {
	vhs, err := exec.LookPath("vhs")
	if err != nil {
		t.Skip("vhs is not installed")
	}
	files, _ := filepath.Glob("../testdata/scripts/*.txt")
	if len(files) == 0 {
		t.Skip("no scripts to make tapes of") // They live with the component being tested.
	}
	for _, file := range files {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".txt"), func(t *testing.T) {
			tape, err := browsetest.Tape(file, browsetest.TapeOptions{Command: "gloss examples", Output: "out.gif"})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "s.tape")
			if err := os.WriteFile(path, []byte(tape), 0o644); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(vhs, "validate", path).CombinedOutput(); err != nil {
				t.Errorf("vhs rejects the tape: %v\n%s\n%s", err, out, tape)
			}
		})
	}
}
