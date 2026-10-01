// docsite writes the two guide pages that are made from files elsewhere in the
// repository, so that they cannot drift from them: "For LLMs", from the agent
// skill, and "Development", from DEVELOP.md. The rest of the guide is written by
// hand in docs/hugo/content/guide, and the command reference (the options,
// grouped by domain) is written by gloss itself: see `gloss --docs-markdown`.
//
//	go run ./internal/tools/docsite -o docs/hugo/content/guide
//
// Edit the sources, not the two pages: they are rewritten on every build.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const repo = "https://github.com/NimbleMarkets/gloss"

func main() {
	out := flag.String("o", "docs/hugo/content/guide", "directory to write the pages into")
	develop := flag.String("develop", "DEVELOP.md", "the development guide")
	skill := flag.String("skill", "skills/gloss/SKILL.md", "the agent skill")
	flag.Parse()
	if err := run(*develop, *skill, *out); err != nil {
		fmt.Fprintln(os.Stderr, "docsite:", err)
		os.Exit(1)
	}
}

func run(developPath, skillPath, out string) error {
	developText, err := os.ReadFile(developPath)
	if err != nil {
		return err
	}
	skillText, err := os.ReadFile(skillPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := write(filepath.Join(out, "development.md"), "Development", 70, "DEVELOP.md", development(string(developText))); err != nil {
		return err
	}
	return write(filepath.Join(out, "for-llms.md"), "For LLMs", 80, "skills/gloss/SKILL.md", forLLMs(string(skillText)))
}

var fence = regexp.MustCompile("^\\s*(```|~~~)")

var link = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)\s]+)\)`)

// links makes a file's links work from the site: the ones into the repository
// point at GitHub, and the skill points at its page.
func links(body string) string {
	return link.ReplaceAllStringFunc(body, func(m string) string {
		parts := link.FindStringSubmatch(m)
		image, text, target := parts[1], parts[2], parts[3]
		switch {
		case strings.Contains(target, "://"), strings.HasPrefix(target, "#"), strings.HasPrefix(target, "mailto:"):
			return m
		case target == "skills/gloss/SKILL.md":
			return image + "[" + text + "](../for-llms/)"
		case image != "":
			return "![" + text + "](https://raw.githubusercontent.com/NimbleMarkets/gloss/main/" + strings.TrimPrefix(target, "./") + ")"
		}
		return "[" + text + "](" + repo + "/blob/main/" + strings.TrimPrefix(target, "./") + ")"
	})
}

// dropTitle removes the first line when it is a # title: the page has one.
func dropTitle(text string) string {
	text = strings.TrimLeft(text, "\n")
	if i := strings.Index(text, "\n"); strings.HasPrefix(text, "# ") && i > 0 {
		return strings.TrimLeft(text[i:], "\n")
	}
	return text
}

// development is DEVELOP.md as a page, under the title the page has.
func development(text string) string {
	return links(dropTitle(text))
}

// forLLMs is the skill as a page: its front matter becomes a lead-in, and its
// own title is dropped, the page having one.
func forLLMs(skill string) string {
	description := ""
	if rest, ok := strings.CutPrefix(skill, "---\n"); ok {
		if front, body, ok := strings.Cut(rest, "\n---\n"); ok {
			for _, line := range strings.Split(front, "\n") {
				if v, ok := strings.CutPrefix(line, "description: "); ok {
					description = v
				}
			}
			skill = body
		}
	}
	var b strings.Builder
	b.WriteString("This is the skill gloss carries for agents. The binary prints it with `gloss --skill`, and `gloss --skill --install` puts it where the agents on a machine look, so it always matches the flags that binary has.\n\n")
	if description != "" {
		b.WriteString("> " + description + "\n\n")
	}
	b.WriteString(links(dropTitle(skill)))
	return b.String()
}

// tocFrom is how many headings a page needs before its table of contents
// earns the column it takes from the page. Below it the page turns it off:
// the theme can switch one off per page but not on.
const tocFrom = 4

// headings counts the headings of a body, outside code fences.
func headings(body string) int {
	n, inFence := 0, false
	for _, line := range strings.Split(body, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
		}
		if !inFence && (strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ")) {
			n++
		}
	}
	return n
}

func write(path, title string, weight int, source, body string) error {
	toc := ""
	if headings(body) < tocFrom {
		toc = "bookToC: false\n"
	}
	head := fmt.Sprintf("---\ntitle: %q\nweight: %d\n%s---\n\n<!-- Generated from %s by internal/tools/docsite: edit that, not this. -->\n\n# %s\n\n", title, weight, toc, source, title)
	return os.WriteFile(path, []byte(head+body), 0o644)
}
