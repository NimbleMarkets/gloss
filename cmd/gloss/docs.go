package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/spf13/pflag"
)

// The reference is written from the options themselves, so it cannot say
// anything --help does not: a man page for the Debian package and the archives,
// and Markdown for the website (with Hugo front matter, if asked). The hidden
// options --docs-man and --docs-markdown run it; see `task docs:build`.

// docsRequest is what the hidden options asked for.
type docsRequest struct {
	ManDir, MarkdownDir string
	Hugo                bool
}

// writeDocs writes what was asked of the options f, in the order man pages
// then Markdown, and says what it wrote on out.
func writeDocs(f *pflag.FlagSet, req docsRequest, out *bytes.Buffer) error {
	if err := checkDomains(f); err != nil {
		return err
	}
	if req.ManDir != "" {
		if err := os.MkdirAll(req.ManDir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(req.ManDir, "gloss.1")
		if err := os.WriteFile(path, []byte(manPage(f)), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s\n", path)
	}
	if req.MarkdownDir != "" {
		pages := markdownPages(f, req.Hugo)
		if err := os.MkdirAll(req.MarkdownDir, 0o755); err != nil {
			return err
		}
		for name, body := range pages {
			if err := os.WriteFile(filepath.Join(req.MarkdownDir, name), []byte(body), 0o644); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "wrote %d pages to %s\n", len(pages), req.MarkdownDir)
	}
	return nil
}

// optionLine is an option as the reference spells it: -o, --output string.
type optionLine struct {
	Short, Long, Arg, Default, Usage string
	OptionalArg                      bool // --install[=FOLDER]
}

func optionOf(fl *pflag.Flag) optionLine {
	arg, usage := pflag.UnquoteUsage(fl)
	line := optionLine{Short: fl.Shorthand, Long: fl.Name, Arg: arg, Usage: usage}
	if fl.NoOptDefVal != "" && fl.Value.Type() != "bool" {
		line.OptionalArg = true
	}
	if fl.Value.Type() == "bool" {
		line.Arg = ""
	}
	switch fl.DefValue {
	case "", "false", "0", "0s", "[]":
	default:
		line.Default = fl.DefValue
		if fl.Value.Type() == "string" {
			line.Default = fmt.Sprintf("%q", fl.DefValue)
		}
	}
	return line
}

// flagsOf is the options of a domain, in its order.
func flagsOf(f *pflag.FlagSet, d domain) []optionLine {
	var out []optionLine
	for _, name := range d.Flags {
		out = append(out, optionOf(f.Lookup(name)))
	}
	return out
}

// ---- Markdown ----

// markdownPages is the command reference as Markdown: an overview, and a
// page for each domain. With hugo, each has the front matter the site needs.
func markdownPages(f *pflag.FlagSet, hugo bool) map[string]string {
	front := func(title string, weight int) string {
		if !hugo {
			return ""
		}
		// A reference page is a table and some examples: a table of contents
		// would only take a column from the table.
		return fmt.Sprintf("---\ntitle: %q\nweight: %d\nbookToC: false\n---\n\n", title, weight)
	}
	pages := map[string]string{}

	var b strings.Builder
	b.WriteString(front("Command reference", 20))
	b.WriteString("# gloss\n\n")
	b.WriteString("```\n")
	for i, line := range usageLines {
		if i == 0 {
			b.WriteString("Usage: ")
		} else {
			b.WriteString("       ")
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("```\n\n")
	b.WriteString(strings.ReplaceAll(usageAbout, "\n", " ") + "\n\n")
	b.WriteString("gloss has no subcommands. Its options are grouped by what they are for:\n\n")
	b.WriteString("| Domain | Options |\n| --- | --- |\n")
	for _, d := range domains {
		var names []string
		for _, name := range d.Flags {
			names = append(names, "`--"+name+"`")
		}
		fmt.Fprintf(&b, "| [%s](%s/) | %s |\n", d.Title, d.ID, strings.Join(names, " "))
	}
	b.WriteString("\n## Exit status\n\n| Code | Meaning |\n| --- | --- |\n")
	for _, s := range exitStatus {
		fmt.Fprintf(&b, "| %s | %s |\n", s[0], s[1])
	}
	b.WriteString("\n## See also\n\nIn the viewer, `?` lists the keys; the [controls](../guide/controls/) are also on this site.\n")
	pages["_index.md"] = b.String()

	for i, d := range domains {
		var p strings.Builder
		p.WriteString(front(d.Title, 20+i))
		fmt.Fprintf(&p, "# %s\n\n%s\n\n", d.Title, d.Summary)
		p.WriteString("| Option | Argument | Default | Description |\n| --- | --- | --- | --- |\n")
		for _, o := range flagsOf(f, d) {
			names := "`--" + o.Long + "`"
			if o.Short != "" {
				names = "`-" + o.Short + "`, " + names
			}
			arg := ""
			if o.Arg != "" {
				arg = "`" + o.Arg + "`"
				if o.OptionalArg {
					arg = "`[=" + o.Arg + "]`"
				}
			}
			def := ""
			if o.Default != "" {
				def = "`" + o.Default + "`"
			}
			fmt.Fprintf(&p, "| %s | %s | %s | %s |\n", names, arg, def, strings.ReplaceAll(o.Usage, "|", `\|`))
		}
		if len(d.Examples) > 0 {
			p.WriteString("\n## Examples\n\n```sh\n")
			for j, e := range d.Examples {
				if j > 0 {
					p.WriteString("\n")
				}
				fmt.Fprintf(&p, "# %s\n%s\n", e.Note, e.Cmd)
			}
			p.WriteString("```\n")
		}
		pages[d.ID+".md"] = p.String()
	}
	return pages
}

// ---- Man page ----

// roff makes text safe for a man page: backslashes and hyphens are escaped,
// and a line may not begin with a dot or an apostrophe.
func roff(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "-", `\-`)
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, ".") || strings.HasPrefix(line, "'") {
			line = `\&` + line
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// roffMarkup is roff of prose with `code` spans set in bold.
func roffMarkup(s string) string {
	parts := strings.Split(s, "`")
	for i := range parts {
		parts[i] = roff(parts[i])
		if i%2 == 1 {
			parts[i] = `\fB` + parts[i] + `\fR`
		}
	}
	return strings.Join(parts, "")
}

func manOption(o optionLine) string {
	var b strings.Builder
	b.WriteString(".TP\n")
	if o.Short != "" {
		fmt.Fprintf(&b, `\fB\-%s\fR, `, o.Short)
	}
	fmt.Fprintf(&b, `\fB\-\-%s\fR`, roff(o.Long))
	switch {
	case o.OptionalArg:
		fmt.Fprintf(&b, `[=\fI%s\fR]`, roff(o.Arg))
	case o.Arg != "":
		fmt.Fprintf(&b, ` \fI%s\fR`, roff(o.Arg))
	}
	b.WriteString("\n" + roffMarkup(o.Usage))
	if o.Default != "" {
		b.WriteString(" (default " + roff(o.Default) + ")")
	}
	b.WriteString(".\n")
	return b.String()
}

// manPage is the whole reference as one gloss(1), the options under a
// subsection for each domain.
func manPage(f *pflag.FlagSet) string {
	var b strings.Builder
	fmt.Fprintf(&b, ".TH GLOSS 1 \"\" \"gloss %s\" \"User Commands\"\n", roff(version))
	b.WriteString(".SH NAME\ngloss \\- a visual pager for the terminal\n")
	b.WriteString(".SH SYNOPSIS\n")
	for i, line := range usageLines {
		if i > 0 {
			b.WriteString(".br\n")
		}
		b.WriteString(roffMarkup("`"+line+"`") + "\n")
	}
	b.WriteString(".SH DESCRIPTION\n")
	b.WriteString(roff(strings.ReplaceAll(usageAbout, "\n", " ")) + "\n")
	b.WriteString(".PP\nIt has no subcommands. Its options are grouped by what they are for.\n")
	b.WriteString(".SH OPTIONS\n")
	for _, d := range domains {
		fmt.Fprintf(&b, ".SS %s\n%s\n", roff(d.Title), roffMarkup(d.Summary))
		for _, o := range flagsOf(f, d) {
			b.WriteString(manOption(o))
		}
	}
	b.WriteString(".SH EXAMPLES\n")
	for _, d := range domains {
		for _, e := range d.Examples {
			fmt.Fprintf(&b, ".TP\n%s\n.nf\n%s\n.fi\n", roff(e.Note), roff(e.Cmd))
		}
	}
	b.WriteString(".SH EXIT STATUS\n")
	for _, s := range exitStatus {
		fmt.Fprintf(&b, ".TP\n\\fB%s\\fR\n%s\n", s[0], roffMarkup(s[1]))
	}
	b.WriteString(".SH SEE ALSO\nThe controls of the viewer are listed by \\fB?\\fR in it, and the guide is at " + app.DocsURL + "\n")
	b.WriteString(".SH AUTHOR\nNeomantra Corp. gloss is released under the MIT license.\n")
	return b.String()
}
