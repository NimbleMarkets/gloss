// Package taskcheck holds a test that Taskfile.yml's build task knows what the
// binary is built from. Task skips a build when its sources have not changed, so
// a file the binary depends on that the list lacks would leave a stale binary
// without a word.
package taskcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const module = "github.com/NimbleMarkets/gloss"

// buildSources are the include and exclude patterns of the build task's sources.
func buildSources(t *testing.T, taskfile string) (include, exclude []string) {
	t.Helper()
	lines := strings.Split(taskfile, "\n")
	in, sources := false, false
	for _, l := range lines {
		switch {
		case l == "  build:":
			in = true
		case in && strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") && strings.HasSuffix(l, ":"):
			in = false // The next task.
		case in && strings.HasPrefix(l, "    sources:"):
			sources = true
		case in && sources && strings.HasPrefix(l, "      - "):
			item := strings.TrimSpace(strings.TrimPrefix(l, "      - "))
			if p, ok := strings.CutPrefix(item, "exclude:"); ok {
				exclude = append(exclude, strings.Trim(strings.TrimSpace(p), `'"`))
			} else {
				include = append(include, strings.Trim(item, `'"`))
			}
		case in && sources && strings.HasPrefix(l, "    ") && !strings.HasPrefix(l, "     ") && strings.TrimSpace(l) != "" && !strings.HasPrefix(strings.TrimSpace(l), "#"):
			sources = false // The next key of the task.
		}
	}
	if len(include) == 0 {
		t.Fatal("the build task lists no sources")
	}
	return include, exclude
}

// glob turns a Task glob into a regular expression: ** crosses folders, * does not.
func glob(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; {
		case c == '*' && i+1 < len(pattern) && pattern[i+1] == '*':
			i++
			if i+1 < len(pattern) && pattern[i+1] == '/' {
				i++
				b.WriteString("(.*/)?")
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func matches(patterns []string, file string) bool {
	for _, p := range patterns {
		if glob(p).MatchString(file) {
			return true
		}
	}
	return false
}

func TestGlobs(t *testing.T) {
	for _, tc := range []struct {
		pattern, file string
		want          bool
	}{
		{"cmd/**/*.go", "cmd/gloss/main.go", true},
		{"cmd/**/*.go", "cmd/main.go", true},
		{"internal/**/*.go", "internal/browse/browsetest/play.go", true},
		{"internal/browse/browsetest/**", "internal/browse/browsetest/tape/main.go", true},
		{"skills/*.go", "skills/skills.go", true},
		{"skills/*.go", "skills/gloss/x.go", false},
		{"**/*_test.go", "internal/app/a_test.go", true},
		{"**/*_test.go", "internal/app/a.go", false},
		{"web/serve.mjs", "web/serve.mjs", true},
		{"web/serve.mjs", "web/serveXmjs", false},
	} {
		if got := glob(tc.pattern).MatchString(tc.file); got != tc.want {
			t.Errorf("%q against %q = %v, want %v", tc.pattern, tc.file, got, tc.want)
		}
	}
}

// Every file the gloss binary is built from, Go and embedded, is among the
// build task's sources, and none of them is excluded.
func TestBuildSourcesCoverTheBinary(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	taskfile, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	include, exclude := buildSources(t, string(taskfile))
	format := `{{if .Module}}{{if eq .Module.Path "` + module + `"}}{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}{{range .EmbedFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}{{end}}{{end}}`
	cmd := exec.Command("go", "list", "-deps", "-f", format, "./cmd/gloss")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	files := strings.Fields(string(out))
	if len(files) < 20 {
		t.Fatalf("go list found only %d files: %q", len(files), files)
	}
	for _, abs := range files {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			t.Fatal(err)
		}
		rel = filepath.ToSlash(rel)
		if !matches(include, rel) {
			t.Errorf("%s is part of the binary but not among the build task's sources: task build would not notice it change", rel)
		}
		if matches(exclude, rel) {
			t.Errorf("%s is part of the binary but excluded from the build task's sources", rel)
		}
	}
	for _, f := range []string{"go.mod", "go.sum"} {
		if !matches(include, f) {
			t.Errorf("%s is not among the build task's sources", f)
		}
	}
}
