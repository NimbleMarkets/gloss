package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = "# gloss\n\nintro\n\n## Live demo\n\ndemo\n\n## Install\n\nbrew it\n\n```sh\n# not a heading\n## nor this\n```\n\n## Build and run\n\n### Sub\n\ntext [doc](docs/x.md) ![pic](examples/a.png) [web](https://example.com) [here](#there)\n\n## Controls\n\n### Keys\n\nq quits. See [skill](skills/gloss/SKILL.md).\n\n## License\n\nMIT\n"

func TestSplitKeepsFencedHashesInPlace(t *testing.T) {
	sections, order := split(sample)
	if strings.Join(order, "|") != "Live demo|Install|Build and run|Controls|License" {
		t.Fatalf("order = %v", order)
	}
	if !strings.Contains(strings.Join(sections["Install"], "\n"), "## nor this") {
		t.Fatalf("a heading inside a code fence cut the section:\n%v", sections["Install"])
	}
}

func TestComposeLiftsOneSectionAndKeepsTitlesOfMany(t *testing.T) {
	sections, _ := split(sample)
	one, err := compose(page{"controls.md", "Controls", 1, []string{"Controls"}}, sections)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(one, "## Keys") || strings.Contains(one, "## Controls") {
		t.Fatalf("a single section should lose its title and lift its headings:\n%s", one)
	}
	many, err := compose(page{"x.md", "X", 1, []string{"Install", "Build and run"}}, sections)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(many, "## Install\n") || !strings.Contains(many, "## Build and run\n") || !strings.Contains(many, "### Sub") {
		t.Fatalf("sections of a page keep their titles:\n%s", many)
	}
	if !strings.Contains(many, "```sh\n# not a heading\n## nor this\n```") {
		t.Fatalf("a code fence was changed:\n%s", many)
	}
	if _, err := compose(page{"y.md", "Y", 1, []string{"Gone"}}, sections); err == nil {
		t.Fatal("a missing section was accepted")
	}
}

func TestLinksWorkFromTheSite(t *testing.T) {
	got := links("[doc](docs/x.md) ![pic](examples/a.png) [web](https://example.com) [here](#there) [s](skills/gloss/SKILL.md)")
	for _, want := range []string{
		"[doc](https://github.com/NimbleMarkets/gloss/blob/main/docs/x.md)",
		"![pic](https://raw.githubusercontent.com/NimbleMarkets/gloss/main/examples/a.png)",
		"[web](https://example.com)",
		"[here](#there)",
		"[s](../for-llms/)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestRunWritesPagesAndRefusesAHomelessSection(t *testing.T) {
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	skill := filepath.Join(dir, "SKILL.md")
	// The real README and skill: every section of it has a page.
	realReadme, realSkill := filepath.Join("..", "..", "..", "README.md"), filepath.Join("..", "..", "..", "skills", "gloss", "SKILL.md")
	out := filepath.Join(dir, "guide")
	if err := run(realReadme, realSkill, out); err != nil {
		t.Fatalf("the README does not fit the pages: %v", err)
	}
	for _, p := range pages {
		body, err := os.ReadFile(filepath.Join(out, p.file))
		if err != nil || !strings.HasPrefix(string(body), "---\ntitle: ") {
			t.Fatalf("%s: %v\n%.80s", p.file, err, body)
		}
	}
	if index, err := os.ReadFile(filepath.Join(out, "_index.md")); err != nil || !strings.Contains(string(index), "- [Controls](controls/)") || !strings.Contains(string(index), "- [For LLMs](for-llms/)") {
		t.Fatalf("the guide has no index: %v", err)
	}
	// Short pages turn the table of contents off, so it takes no column; long ones keep it.
	for file, off := range map[string]bool{"controls.md": true, "agents.md": false, "_index.md": true} {
		body, _ := os.ReadFile(filepath.Join(out, file))
		if got := strings.Contains(string(body), "bookToC: false"); got != off {
			t.Errorf("%s: table of contents off = %v, want %v", file, got, off)
		}
	}
	llms, _ := os.ReadFile(filepath.Join(out, "for-llms.md"))
	if strings.Contains(string(llms), "name: gloss") || !strings.Contains(string(llms), "gloss --skill") {
		t.Fatalf("for-llms.md:\n%.400s", llms)
	}
	// A new section in the README must be placed.
	os.WriteFile(readme, []byte(sample+"\n## Brand new\n\nx\n"), 0o644)
	os.WriteFile(skill, []byte("# s\n"), 0o644)
	if err := run(readme, skill, filepath.Join(dir, "g2")); err == nil {
		t.Fatal("a README section with no page was accepted")
	}
}
