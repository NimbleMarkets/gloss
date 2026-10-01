package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var guideDir = filepath.Join("..", "..", "..", "docs", "hugo", "content", "guide")

func TestLinksWorkFromTheSite(t *testing.T) {
	got := links("[doc](docs/x.md) ![pic](examples/a.png) [web](https://example.com) [here](#there) [s](skills/gloss/SKILL.md) [a](AGENTS.md)")
	for _, want := range []string{
		"[doc](https://github.com/NimbleMarkets/gloss/blob/main/docs/x.md)",
		"![pic](https://raw.githubusercontent.com/NimbleMarkets/gloss/main/examples/a.png)",
		"[web](https://example.com)",
		"[here](#there)",
		"[s](../for-llms/)",
		"[a](https://github.com/NimbleMarkets/gloss/blob/main/AGENTS.md)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestDropTitleAndHeadingsIgnoreFences(t *testing.T) {
	if got := dropTitle("\n# Title\n\nbody\n"); got != "body\n" {
		t.Errorf("dropTitle = %q", got)
	}
	if got := dropTitle("no title\n"); got != "no title\n" {
		t.Errorf("dropTitle of untitled text = %q", got)
	}
	if n := headings("## one\n```sh\n## not\n### nor\n```\n### two\n"); n != 2 {
		t.Errorf("headings = %d, want 2 (a # in a fence is not one)", n)
	}
}

func TestRunWritesDevelopmentAndForLLMs(t *testing.T) {
	out := t.TempDir()
	if err := run(filepath.Join("..", "..", "..", "DEVELOP.md"), filepath.Join("..", "..", "..", "skills", "gloss", "SKILL.md"), out); err != nil {
		t.Fatal(err)
	}
	dev, err := os.ReadFile(filepath.Join(out, "development.md"))
	if err != nil || !strings.HasPrefix(string(dev), "---\ntitle: \"Development\"") {
		t.Fatalf("development.md: %v\n%.120s", err, dev)
	}
	if strings.Contains(string(dev), "\n# Developing gloss") || !strings.Contains(string(dev), "## Build and run") {
		t.Fatalf("development.md should drop the title and keep the sections:\n%.400s", dev)
	}
	if strings.Contains(string(dev), "](AGENTS.md)") {
		t.Fatalf("a relative link was left in development.md:\n%.600s", dev)
	}
	llms, _ := os.ReadFile(filepath.Join(out, "for-llms.md"))
	if strings.Contains(string(llms), "name: gloss") || !strings.Contains(string(llms), "gloss --skill") {
		t.Fatalf("for-llms.md:\n%.400s", llms)
	}
	// Long pages keep the table of contents; the theme can only turn it off.
	for file, wantOff := range map[string]bool{"development.md": false, "for-llms.md": false} {
		body, _ := os.ReadFile(filepath.Join(out, file))
		if got := strings.Contains(string(body), "bookToC: false"); got != wantOff {
			t.Errorf("%s: table of contents off = %v, want %v", file, got, wantOff)
		}
	}
}

// The guide is written by hand, except two pages: every page in it must have a
// title and weight, and the guide's index must link each, so none is lost.
func TestGuideIsComplete(t *testing.T) {
	index, err := os.ReadFile(filepath.Join(guideDir, "_index.md"))
	if err != nil {
		t.Fatal(err)
	}
	generated := map[string]bool{"development.md": true, "for-llms.md": true}
	entries, err := os.ReadDir(guideDir)
	if err != nil {
		t.Fatal(err)
	}
	front := regexp.MustCompile(`(?s)\A---\ntitle: ".+"\nweight: \d+\n(bookToC: false\n)?---\n`)
	for _, e := range entries {
		name := e.Name()
		if name == "_index.md" || !strings.HasSuffix(name, ".md") {
			continue
		}
		page := strings.TrimSuffix(name, ".md")
		if !strings.Contains(string(index), "]("+page+"/)") {
			t.Errorf("the guide's index does not link %s", name)
		}
		if generated[name] {
			continue // Written by `task docs`; it may not be there yet.
		}
		body, err := os.ReadFile(filepath.Join(guideDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !front.Match(body) {
			t.Errorf("%s lacks a title and weight in its front matter", name)
		}
		title := regexp.MustCompile(`title: "(.+)"`).FindStringSubmatch(string(body))[1]
		if !strings.Contains(string(body), "---\n\n# "+title+"\n") {
			t.Errorf("%s does not open with its title as a heading (the theme does not draw one)", name)
		}
		if strings.Contains(string(body), "Generated from") {
			t.Errorf("%s says it is generated, but it is written by hand", name)
		}
	}
	// And every page the index promises is there, or is one of the two made.
	for _, m := range regexp.MustCompile(`\]\(([a-z-]+)/\)`).FindAllStringSubmatch(string(index), -1) {
		file := m[1] + ".md"
		if _, err := os.Stat(filepath.Join(guideDir, file)); err != nil && !generated[file] {
			t.Errorf("the guide's index links %s, which is not there", file)
		}
	}
}
