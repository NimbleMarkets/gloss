package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/NimbleMarkets/gloss/skills"
)

// gloss has one default verb, view, and a few others. A verb is recognized
// only as the first argument and only by its exact name; any other first
// argument is a file or an option, and the command is an implicit view. A file
// that is named like a verb is opened with the verb spelled out: gloss view skill.
type verb struct {
	Name    string
	Summary string // One line, for help and the reference.
}

var verbs = []verb{
	{"view", "open files in the pager, or export, extract, pick and serve them (the default)"},
	{"skill", "print the skill that teaches agents to use gloss, install it, or print the schema of its JSON"},
	{"help", "say how to use gloss, or one of its commands"},
}

// splitVerb separates the verb from the arguments after it. A first argument
// that names no verb leaves them all to the implicit view.
func splitVerb(args []string) (name string, rest []string) {
	if len(args) > 0 && slices.ContainsFunc(verbs, func(v verb) bool { return v.Name == args[0] }) {
		return args[0], args[1:]
	}
	return "view", args
}

const skillUsage = `Usage: gloss skill [show]
       gloss skill install [FOLDER]
       gloss skill schema

show prints the skill (SKILL.md) that teaches an agent gloss's headless
surface, so it always matches this binary. It is the default.

install writes it as gloss/SKILL.md under the skills folder of each agent found
on this machine, or under FOLDER, and prints the paths it wrote.

schema prints the JSON Schema of the objects gloss writes for a program:
the --json results of an export, --text, --grep, and --info, and the objects
of a session started without a terminal (start, --status, --resume --json).`

// parseSkill is the skill verb: show prints it, and done says nothing more is
// to be done; install leaves the folder in opts for run, as an option does.
func parseSkill(args []string, opts options, out io.Writer) (options, bool, error) {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(out, skillUsage)
		return opts, true, nil
	}
	sub := "show"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch {
	case sub == "show" && len(args) == 0:
		fmt.Fprint(out, skills.Gloss)
		return opts, true, nil
	case sub == "install" && len(args) <= 1:
		opts.SkillInstall = "auto"
		if len(args) == 1 {
			opts.SkillInstall = args[0]
		}
		return opts, false, nil
	case sub == "schema" && len(args) == 0:
		return opts, true, writeSchema(out)
	case sub == "show" || sub == "install" || sub == "schema":
		return opts, false, fmt.Errorf("skill %s: too many arguments (see gloss skill --help)", sub)
	}
	return opts, false, fmt.Errorf("skill: unknown command %q: show, install, or schema (see gloss skill --help)", sub)
}

// parseHelp is the help verb: with no argument, the usage of gloss; with a
// verb, that verb's.
func parseHelp(args []string, usage func(), out io.Writer) (bool, error) {
	if len(args) == 0 {
		usage()
		return true, nil
	}
	if len(args) > 1 {
		return false, fmt.Errorf("help takes one command: %s", verbNames())
	}
	switch args[0] {
	case "view":
		usage()
	case "skill":
		fmt.Fprintln(out, skillUsage)
	case "help":
		fmt.Fprintln(out, "Usage: gloss help [command]\n\nCommands: "+verbNames())
	default:
		return false, fmt.Errorf("no help for %q: the commands are %s", args[0], verbNames())
	}
	return true, nil
}

func verbNames() string {
	var names []string
	for _, v := range verbs {
		names = append(names, v.Name)
	}
	return strings.Join(names, ", ")
}
