// docsite writes the guide pages of the documentation site from the README and
// the agent skill, so that what the site says about the keys, the formats and
// their limits is what the README says, and cannot drift from it. The command
// reference (the options, grouped by domain) is written by gloss itself:
// see `gloss --docs-markdown`.
//
//	go run ./internal/tools/docsite -o docs/hugo/content/guide
//
// Edit the README, not the pages: they are rewritten on every build.
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

// A page of the guide, made of README sections (by their ## titles).
type page struct {
	file, title string
	weight      int
	sections    []string
}

var pages = []page{
	{"install.md", "Install and build", 10, []string{"Install", "Build and run"}},
	{"controls.md", "Controls", 20, []string{"Controls"}},
	{"handoff.md", "Asking for a file", 30, []string{"Asking for a file"}},
	{"markdown.md", "Markdown", 40, []string{"Markdown"}},
	{"agents.md", "For agents", 50, []string{"For agents", "Images for vision models"}},
	{"formats.md", "Formats and limits", 60, []string{"Formats and current limits"}},
	{"development.md", "Development", 70, []string{"Development"}},
}

// leftOut are README sections that no page takes, on purpose. A section in
// neither list is an error: a new part of the README must find a home.
var leftOut = []string{"Live demo", "License"}

func main() {
	out := flag.String("o", "docs/hugo/content/guide", "directory to write the pages into")
	readme := flag.String("readme", "README.md", "the README")
	skill := flag.String("skill", "skills/gloss/SKILL.md", "the agent skill")
	flag.Parse()
	if err := run(*readme, *skill, *out); err != nil {
		fmt.Fprintln(os.Stderr, "docsite:", err)
		os.Exit(1)
	}
}

func run(readmePath, skillPath, out string) error {
	text, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	sections, order := split(string(text))
	taken := map[string]bool{}
	for _, name := range leftOut {
		taken[name] = true
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for _, p := range pages {
		body, err := compose(p, sections)
		if err != nil {
			return err
		}
		for _, name := range p.sections {
			taken[name] = true
		}
		if err := write(filepath.Join(out, p.file), p.title, p.weight, "README.md", body); err != nil {
			return err
		}
	}
	for _, name := range order {
		if !taken[name] {
			return fmt.Errorf("README section %q is on no page: add it to pages in internal/tools/docsite, or to leftOut", name)
		}
	}
	index := "How to install gloss, what its keys do, what it opens, and how an agent uses it. Every page is the README's, kept in step with it.\n\n"
	for _, p := range pages {
		index += fmt.Sprintf("- [%s](%s/)\n", p.title, strings.TrimSuffix(p.file, ".md"))
	}
	index += "- [For LLMs](for-llms/)\n"
	if err := write(filepath.Join(out, "_index.md"), "Guide", 10, "README.md", index); err != nil {
		return err
	}
	skillText, err := os.ReadFile(skillPath)
	if err != nil {
		return err
	}
	return write(filepath.Join(out, "for-llms.md"), "For LLMs", 80, "skills/gloss/SKILL.md", forLLMs(string(skillText)))
}

var fence = regexp.MustCompile("^\\s*(```|~~~)")

// split cuts the README into its ## sections, by title, keeping the order.
// A # inside a code fence is not a heading.
func split(text string) (map[string][]string, []string) {
	sections := map[string][]string{}
	var order []string
	current, inFence := "", false
	for _, line := range strings.Split(text, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "## ") {
			current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			order = append(order, current)
			sections[current] = nil
			continue
		}
		if current != "" {
			sections[current] = append(sections[current], line)
		}
	}
	return sections, order
}

// compose is a page's body. A page of one section takes the section's title
// from its front matter and lifts the headings inside it by a level; a page
// of several keeps each section's title as a heading.
func compose(p page, sections map[string][]string) (string, error) {
	var b strings.Builder
	for _, name := range p.sections {
		lines, ok := sections[name]
		if !ok {
			return "", fmt.Errorf("page %s wants README section %q, which is gone", p.file, name)
		}
		body := strings.Join(lines, "\n")
		if len(p.sections) == 1 {
			body = lift(body)
		} else {
			b.WriteString("## " + name + "\n")
		}
		b.WriteString(strings.TrimSpace(body) + "\n\n")
	}
	return links(strings.TrimSpace(b.String()) + "\n"), nil
}

// lift moves every heading, outside code fences, up one level: ### to ##.
func lift(body string) string {
	lines := strings.Split(body, "\n")
	inFence := false
	for i, line := range lines {
		if fence.MatchString(line) {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "###") {
			lines[i] = line[1:]
		}
	}
	return strings.Join(lines, "\n")
}

var link = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)\s]+)\)`)

// links makes the README's links work from the site: the ones into the
// repository point at GitHub, and the skill points at its page.
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
	skill = strings.TrimLeft(skill, "\n")
	if i := strings.Index(skill, "\n"); strings.HasPrefix(skill, "# ") && i > 0 {
		skill = strings.TrimLeft(skill[i:], "\n")
	}
	var b strings.Builder
	b.WriteString("This is the skill gloss carries for agents. The binary prints it with `gloss --skill`, and `gloss --skill --install` puts it where the agents on a machine look, so it always matches the flags that binary has.\n\n")
	if description != "" {
		b.WriteString("> " + description + "\n\n")
	}
	b.WriteString(links(skill))
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
		if !inFence && strings.HasPrefix(line, "## ") || !inFence && strings.HasPrefix(line, "### ") {
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
	head := fmt.Sprintf("---\ntitle: %q\nweight: %d\n%s---\n\n<!-- Generated from %s by internal/tools/docsite: edit that, not this. -->\n\n", title, weight, toc, source)
	return os.WriteFile(path, []byte(head+body), 0o644)
}
