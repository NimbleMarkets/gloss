package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeCompletions runs the hidden option, as the release does.
func writeCompletions(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	var out bytes.Buffer
	if _, _, err := parse([]string{"--docs-completions", dir}, &out); err != nil {
		t.Fatal(err)
	}
	return dir
}

// helpUsage is what --help says of each option, by long name.
func helpUsage(t *testing.T) map[string]string {
	t.Helper()
	var out bytes.Buffer
	if _, _, err := parse([]string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	usage := map[string]string{}
	var name string
	for _, line := range strings.Split(out.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-") {
			for _, word := range strings.Fields(trimmed) {
				if strings.HasPrefix(word, "--") {
					name = strings.TrimSuffix(strings.SplitN(strings.TrimPrefix(word, "--"), "=", 2)[0], ",")
					break
				}
			}
		}
		if name != "" && trimmed != "" && !strings.HasPrefix(trimmed, "Keys:") {
			usage[name] += " " + trimmed
		}
	}
	return usage
}

// The words an option completes are listed apart from the option, so they are
// held to what its usage line says.
func TestCompletionWordsAreInTheUsage(t *testing.T) {
	usage := helpUsage(t)
	for name, c := range completers() {
		text, ok := usage[name]
		if !ok {
			t.Errorf("--%s completes words but is not an option", name)
			continue
		}
		for _, w := range c.Words {
			if !strings.Contains(text, w) {
				t.Errorf("--%s completes %q, which its usage does not mention: %s", name, w, text)
			}
		}
	}
}

func TestCompletionsNameEveryOption(t *testing.T) {
	dir := writeCompletions(t)
	for _, file := range []string{"gloss.bash", "_gloss", "gloss.fish"} {
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range visibleFlags(t) {
			if !strings.Contains(string(data), name) {
				t.Errorf("%s does not mention --%s", file, name)
			}
		}
		for _, hidden := range []string{"docs-man", "docs-markdown", "docs-hugo", "detached"} {
			if strings.Contains(string(data), hidden) {
				t.Errorf("%s offers the hidden --%s", file, hidden)
			}
		}
	}
}

func TestCompletionScriptsParse(t *testing.T) {
	dir := writeCompletions(t)
	for shell, file := range map[string]string{"bash": "gloss.bash", "zsh": "_gloss", "fish": "gloss.fish"} {
		path, err := exec.LookPath(shell)
		if err != nil {
			t.Logf("no %s here: its script is not checked", shell)
			continue
		}
		args := []string{"-n", filepath.Join(dir, file)}
		if shell == "fish" {
			args = []string{"--no-execute", filepath.Join(dir, file)}
		}
		if out, err := exec.Command(path, args...).CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", shell, err, out)
		}
	}
}

// bash is asked for completions as readline would.
func TestBashCompletes(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	dir := writeCompletions(t)
	script := `source "$1"; shift
COMP_WORDS=("$@"); COMP_CWORD=$(( $# - 1 )); COMPREPLY=(); _gloss; echo "${COMPREPLY[*]}"`
	for _, tt := range []struct {
		words []string
		want  string
	}{
		{[]string{"gloss", "--ty"}, "--type"},
		{[]string{"gloss", "--type", "sv"}, "svg"},
		{[]string{"gloss", "--type", "=", "pd"}, "pdf"},
		{[]string{"gloss", "-r", ""}, "auto kitty glyph"},
		{[]string{"gloss", "--view", "front,to"}, "front,top"},
		{[]string{"gloss", "--vision-profile", "cl"}, "claude-high claude-standard"},
		{[]string{"gloss", "--dpi", ""}, ""}, // a number is the user's own
		{[]string{"gloss", "skill", ""}, "show install"},
		{[]string{"gloss", "skill", "in"}, "install"},
		{[]string{"gloss", "help", ""}, "view skill help"},
	} {
		out, err := exec.Command(bash, append([]string{"-c", script, "bash", filepath.Join(dir, "gloss.bash")}, tt.words...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", tt.words, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.words, got, tt.want)
		}
	}
}
