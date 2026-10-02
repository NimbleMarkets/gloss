package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/spf13/pflag"
)

// visibleFlags is every option --help lists, by asking the real parser.
func visibleFlags(t *testing.T) []string {
	t.Helper()
	var out bytes.Buffer
	if _, done, err := parse([]string{"--help"}, &out); err != nil || !done {
		t.Fatalf("--help: done=%v err=%v", done, err)
	}
	var names []string
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-") {
			continue
		}
		for _, word := range strings.Fields(line) {
			if strings.HasPrefix(word, "--") {
				names = append(names, strings.TrimSuffix(strings.SplitN(strings.TrimPrefix(word, "--"), "=", 2)[0], ","))
				break
			}
		}
	}
	if len(names) < 30 {
		t.Fatalf("found only %d options in --help:\n%s", len(names), out.String())
	}
	return names
}

func TestEveryOptionHasADomain(t *testing.T) {
	var out bytes.Buffer
	dir := t.TempDir()
	// Generating the reference checks the domains against the real options,
	// and fails naming any that are in neither.
	if _, _, err := parse([]string{"--docs-markdown", dir}, &out); err != nil {
		t.Fatalf("the reference does not cover the options: %v", err)
	}
	listed := map[string]bool{}
	for _, d := range domains {
		for _, name := range d.Flags {
			listed[name] = true
		}
	}
	for _, name := range visibleFlags(t) {
		if !listed[name] {
			t.Errorf("--%s is in --help but in no domain", name)
		}
	}
}

func TestCheckDomainsSaysWhatIsWrong(t *testing.T) {
	f := pflag.NewFlagSet("t", pflag.ContinueOnError)
	f.Bool("menu", false, "")
	f.Bool("stray", false, "a new option nobody grouped")
	f.Bool("secret", false, "")
	_ = f.MarkHidden("secret")
	err := checkDomains(f)
	if err == nil {
		t.Fatal("an option with no domain was accepted")
	}
	for _, want := range []string{"--stray belongs to no domain", "lists --preview, which is not an option"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("a hidden option was asked for:\n%v", err)
	}
}

func TestHiddenDocsOptionsStayOutOfHelp(t *testing.T) {
	var out bytes.Buffer
	if _, _, err := parse([]string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"docs-man", "docs-markdown", "docs-hugo", "detached"} {
		if strings.Contains(out.String(), "--"+hidden) {
			t.Errorf("--help shows --%s", hidden)
		}
	}
}

func TestManPage(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if _, done, err := parse([]string{"--docs-man", dir}, &out); err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "gloss.1"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(body)
	for _, want := range []string{".TH GLOSS 1", ".SH NAME", ".SH SYNOPSIS", ".SH OPTIONS", ".SS Meshes: STL and 3MF", ".SH EXIT STATUS", `\fB\-\-output\fR`, `\fB\-o\fR, `} {
		if !strings.Contains(page, want) {
			t.Errorf("the man page lacks %q", want)
		}
	}
	for _, name := range visibleFlags(t) {
		if !strings.Contains(page, `\fB\-\-`+strings.ReplaceAll(name, "-", `\-`)+`\fR`) {
			t.Errorf("the man page lacks --%s", name)
		}
	}
	// A line that starts with a dot would be a request, not text.
	for i, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(line, "'") {
			t.Errorf("line %d starts with an apostrophe: %q", i+1, line)
		}
	}
}

func TestRoffEscapes(t *testing.T) {
	if got := roff("a-b \\ c"); got != `a\-b \\ c` {
		t.Errorf("roff = %q", got)
	}
	if got := roff(".dot\n'tick"); got != "\\&.dot\n\\&'tick" {
		t.Errorf("roff = %q", got)
	}
	if got := roffMarkup("see `--pick` now"); got != `see \fB\-\-pick\fR now` {
		t.Errorf("roffMarkup = %q", got)
	}
}

func TestMarkdownReference(t *testing.T) {
	for _, hugo := range []bool{true, false} {
		dir := t.TempDir()
		args := []string{"--docs-markdown", dir}
		if hugo {
			args = append(args, "--docs-hugo")
		}
		var out bytes.Buffer
		if _, done, err := parse(args, &out); err != nil || !done {
			t.Fatalf("hugo=%v done=%v err=%v", hugo, done, err)
		}
		index, err := os.ReadFile(filepath.Join(dir, "_index.md"))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.HasPrefix(string(index), "---\n"); got != hugo {
			t.Errorf("hugo=%v but front matter=%v", hugo, got)
		}
		var all strings.Builder
		for _, d := range domains {
			page, err := os.ReadFile(filepath.Join(dir, d.ID+".md"))
			if err != nil {
				t.Fatalf("no page for domain %s: %v", d.ID, err)
			}
			all.Write(page)
			if !strings.Contains(string(index), "("+d.ID+"/)") {
				t.Errorf("the overview does not link %s", d.ID)
			}
			if hugo && !strings.Contains(string(page), "title: "+`"`+d.Title+`"`) {
				t.Errorf("%s has no title in its front matter", d.ID)
			}
		}
		for _, name := range visibleFlags(t) {
			// A row is "| `--name` |", with the short name before it if it has one.
			row := regexp.MustCompile("(?m)^\\| (`-.`, )?`--" + regexp.QuoteMeta(name) + "` \\|")
			if n := len(row.FindAllString(all.String(), -1)); n != 1 {
				t.Errorf("--%s has %d rows in the domain pages, want 1", name, n)
			}
		}
		// Defaults and argument names are the real ones.
		if !strings.Contains(all.String(), "| `-r`, `--render` | `string` | `\"auto\"` |") {
			t.Errorf("--render row is wrong:\n%s", all.String())
		}
	}
}

func TestExamplesNameRealOptions(t *testing.T) {
	// An example that uses an option gloss does not have teaches a falsehood.
	known := map[string]bool{}
	for _, name := range visibleFlags(t) {
		known[name] = true
	}
	for _, d := range domains {
		for _, e := range d.Examples {
			for _, word := range strings.Fields(e.Cmd) {
				if !strings.HasPrefix(word, "--") || word == "--" {
					continue
				}
				name := strings.SplitN(strings.TrimPrefix(word, "--"), "=", 2)[0]
				if !known[name] {
					t.Errorf("domain %s example %q uses --%s, which is not an option", d.ID, e.Cmd, name)
				}
			}
		}
	}
}

func TestHelpPointsAtTheDocs(t *testing.T) {
	var out bytes.Buffer
	if _, done, err := parse([]string{"--help"}, &out); err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	if !strings.Contains(out.String(), "Documentation: "+app.DocsURL) {
		t.Fatalf("--help does not name the docs:\n%s", out.String())
	}
	dir := t.TempDir()
	if _, _, err := parse([]string{"--docs-man", dir}, &out); err != nil {
		t.Fatal(err)
	}
	if man, err := os.ReadFile(filepath.Join(dir, "gloss.1")); err != nil || !strings.Contains(string(man), app.DocsURL) {
		t.Fatalf("the man page does not name the docs: %v", err)
	}
}
