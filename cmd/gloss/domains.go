package main

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// A domain is a cluster of options that belong together. gloss has no
// subcommands: one command, many options, and these are how the options are
// grouped for the reference (the man page and the website). Every option that
// is not hidden belongs to exactly one domain; checkDomains enforces it.
type domain struct {
	ID       string   // The page it has: view -> view.md.
	Title    string   // What it is called.
	Summary  string   // Prose, in Markdown with `code` spans.
	Flags    []string // Long names, in the order they are listed.
	Examples []example
}

type example struct {
	Note string // A comment line above the command.
	Cmd  string
}

// usageLines and usageAbout are the first words of --help, and of the
// reference.
var usageLines = []string{"gloss [options] [file | folder]...", "command | gloss [options] -"}

const usageAbout = "A visual pager for images, SVG, PDF, STL, 3MF, Markdown, Word, Excel and CSV.\nWith no file, gloss opens empty: drop files on it, or press o to browse.\nA folder opens the file browser there.\nOptions may appear before or after filenames. Use -- to end options."

var domains = []domain{
	{
		ID: "view", Title: "Opening and viewing",
		Summary: "How gloss starts and what it draws with: the file menu and preview pane, terminal graphics, the screen it uses, which files it finds, and what it may fetch. gloss never fetches the network on its own.",
		Flags:   []string{"menu", "preview", "render", "no-alt-screen", "type", "glob", "fetch"},
		Examples: []example{
			{"Look at some files, with a menu to choose among them", "gloss --menu photo.png report.pdf model.stl"},
			{"A menu with a preview pane", "gloss --preview *.png"},
			{"Every image under the current folder", "gloss --glob images"},
			{"A file whose name does not say what it is", "gloss --type markdown notes"},
		},
	},
	{
		ID: "documents", Title: "Documents, pages and tables",
		Summary: "Choosing the page of a PDF or the sheet of a workbook, how finely a PDF is drawn, and which columns of a table to show.",
		Flags:   []string{"page", "dpi", "cols", "coln"},
		Examples: []example{
			{"The third page of a PDF", "gloss --page 3 report.pdf"},
			{"Sharper pages", "gloss --dpi 300 report.pdf"},
			{"Only some columns of a sheet, by header or by number", "gloss --cols name,total sales.xlsx\ngloss --coln 2,4-6 sales.csv"},
		},
	},
	{
		ID: "mesh", Title: "Meshes: STL and 3MF",
		Summary: "How a model is drawn and posed: the renderer, the camera or a named view, the projection, its paint, and which parts of a 3MF are shown. These apply to exports as well as to the viewer.",
		Flags:   []string{"3d", "view", "camera", "projection", "color", "parts", "partn"},
		Examples: []example{
			{"Start from the front, in perspective", "gloss --view front --projection perspective model.stl"},
			{"Paint the faces the file left plain", "gloss --color orange model.stl"},
			{"Two parts of a 3MF, by name or by number", "gloss --parts lid,base lantern.3mf\ngloss --partn 1,3 lantern.3mf"},
		},
	},
	{
		ID: "export", Title: "Exporting images",
		Summary: "Writing PNGs without a terminal, sized for the vision models that will read them. Export never overwrites a file; it numbers the name instead. `--max-edge` is the stable way to size an image; `--vision-profile` is only an alias for one.",
		Flags:   []string{"output", "output-dir", "max-edge", "vision-profile"},
		Examples: []example{
			{"One page, as a PNG", "gloss --page 2 --output page2.png report.pdf"},
			{"Every page, into a folder", "gloss --page all --output-dir pages report.pdf"},
			{"A contact sheet of a model's views, to a pipe", "gloss --view front,top,iso --output - model.stl > sheet.png"},
		},
	},
	{
		ID: "extract", Title: "Text and details",
		Summary: "Reading a file without drawing it: its text, or what it says about itself. Both work without a terminal and write only the answer to standard output.",
		Flags:   []string{"text", "info", "json"},
		Examples: []example{
			{"The text of a PDF page", "gloss --text --page 4 report.pdf"},
			{"What files say about themselves, as JSON", "gloss --info --json *.pdf model.stl"},
		},
	},
	{
		ID: "handoff", Title: "Handing files over, and showing them",
		Summary: "Asking the user for a file, or showing them one, when the caller has no terminal of its own: an agent, for instance. `--pick` prints the paths the user sends; `--serve` shows them a page. Without a terminal on standard input both detach, print one line of JSON, and are answered later with `--resume`.",
		Flags:   []string{"pick", "serve", "no-open", "prompt", "prompt-loc", "timeout", "resume"},
		Examples: []example{
			{"Ask in the terminal", "gloss --pick --prompt \"Which report?\""},
			{"Ask in a browser, and print what is sent", "gloss --serve --pick"},
			{"Show the user a file", "gloss --serve report.pdf"},
			{"Collect the answer of a detached pick", "gloss --resume TOKEN"},
		},
	},
	{
		ID: "agents", Title: "Teaching agents",
		Summary: "gloss carries a skill that teaches an agent its headless surface, so it always matches the flags this binary has.",
		Flags:   []string{"skill", "install"},
		Examples: []example{
			{"Install it for the agents found on this machine", "gloss --skill --install"},
			{"Or into the skills folder named", "gloss --skill --install=~/.claude/skills"},
			{"Print it, to place by hand", "gloss --skill"},
		},
	},
	{
		ID: "general", Title: "General",
		Summary: "Help and version.",
		Flags:   []string{"help", "version"},
	},
}

// exitStatus is what gloss exits with, for the reference.
var exitStatus = [][2]string{
	{"0", "Success; with `--pick`, the paths were printed."},
	{"1", "An error."},
	{"2", "`--pick` or `--resume`: nothing was chosen."},
	{"124", "`--pick` or `--resume`: the wait timed out."},
}

// checkDomains reports every visible option that no domain lists, every name
// a domain lists that is not an option, and any option listed twice.
func checkDomains(f *pflag.FlagSet) error {
	seen := map[string]string{}
	var problems []string
	for _, d := range domains {
		for _, name := range d.Flags {
			switch {
			case f.Lookup(name) == nil:
				problems = append(problems, fmt.Sprintf("domain %s lists --%s, which is not an option", d.ID, name))
			case seen[name] != "":
				problems = append(problems, fmt.Sprintf("--%s is in both %s and %s", name, seen[name], d.ID))
			}
			seen[name] = d.ID
		}
	}
	f.VisitAll(func(fl *pflag.Flag) {
		if !fl.Hidden && seen[fl.Name] == "" {
			problems = append(problems, fmt.Sprintf("--%s belongs to no domain: add it to one in domains.go", fl.Name))
		}
	})
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return nil
}
