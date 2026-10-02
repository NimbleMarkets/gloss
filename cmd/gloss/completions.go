package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/spf13/pflag"
)

// Completion scripts for bash, zsh, and fish are written from the options
// themselves, like the man page, by the hidden option --docs-completions. The
// options say which take a value and what it is called; what the value may be
// is not in a pflag.Flag, so it is listed here, and a test holds each list to
// what --help says.

// A completer says what an option's value may be.
type completer struct {
	Words []string // The words it may be.
	List  bool     // Several, separated by commas.
	File  bool     // A file.
	Dir   bool     // A folder.
}

// completers are the options whose values can be completed, by long name.
func completers() map[string]completer {
	return map[string]completer{
		"render":         {Words: []string{"auto", "kitty", "glyph"}},
		"3d":             {Words: []string{"auto", "software", "wireframe"}},
		"type":           {Words: []string{"image", "svg", "pdf", "stl", "3mf", "docx", "xlsx", "grist", "csv", "json", "ipynb", "html", "text", "markdown", "md"}},
		"view":           {Words: []string{"front", "back", "left", "right", "top", "bottom", "iso", "all"}, List: true},
		"projection":     {Words: []string{"ortho", "perspective"}},
		"prompt-loc":     {Words: []string{"bottom", "top"}},
		"vision-profile": {Words: document.VisionProfileNames()},
		"output":         {File: true},
		"output-dir":     {Dir: true},
	}
}

func (c completer) empty() bool { return len(c.Words) == 0 && !c.File && !c.Dir }

// completionOption is an option as the scripts need it.
type completionOption struct {
	Short, Long string
	Desc        string
	TakesValue  bool // Not a switch.
	Optional    bool // The value only follows an equals sign: --opt=VALUE.
	Completer   completer
}

// completionOptions lists the options --help shows, by name.
func completionOptions(f *pflag.FlagSet) []completionOption {
	cs := completers()
	var out []completionOption
	f.VisitAll(func(fl *pflag.Flag) {
		if fl.Hidden {
			return
		}
		_, usage := pflag.UnquoteUsage(fl)
		o := completionOption{Short: fl.Shorthand, Long: fl.Name, Desc: shortDesc(usage), Completer: cs[fl.Name]}
		o.TakesValue = fl.Value.Type() != "bool"
		o.Optional = o.TakesValue && fl.NoOptDefVal != ""
		out = append(out, o)
	})
	return out
}

// shortDesc is the first clause of a usage line, short enough for a menu.
func shortDesc(usage string) string {
	if i := strings.IndexAny(usage, ";"); i > 0 {
		usage = usage[:i]
	}
	const limit = 72
	if r := []rune(usage); len(r) > limit {
		cut := string(r[:limit])
		if i := strings.LastIndex(cut, " "); i > 0 {
			cut = cut[:i]
		}
		usage = cut + "…"
	}
	return usage
}

// names is how an option is spelled, for a list of words.
func (o completionOption) names() []string {
	if o.Short != "" {
		return []string{"-" + o.Short, "--" + o.Long}
	}
	return []string{"--" + o.Long}
}

// ---- bash ----

// verbWords are the commands, as a list of words.
var verbWords = func() string { return strings.ReplaceAll(verbNames(), ", ", " ") }()

func bashCompletion(f *pflag.FlagSet) string {
	opts := completionOptions(f)
	var all []string
	for _, o := range opts {
		all = append(all, o.names()...)
	}
	var b strings.Builder
	b.WriteString("# bash completion for gloss. Written by gloss --docs-completions: do not edit.\n")
	b.WriteString(`_gloss() {
    local cur="${COMP_WORDS[COMP_CWORD]}" prev="" opt=""
    if [[ $cur == = ]]; then
        # --type=<TAB>: bash breaks words at the equals sign.
        opt="${COMP_WORDS[COMP_CWORD-1]}"; cur=""
    elif [[ ${COMP_CWORD} -ge 2 && ${COMP_WORDS[COMP_CWORD-1]} == = ]]; then
        opt="${COMP_WORDS[COMP_CWORD-2]}"
    elif [[ ${COMP_CWORD} -ge 1 ]]; then
        prev="${COMP_WORDS[COMP_CWORD-1]}"; opt="$prev"
    fi
    COMPREPLY=()
    if [[ ${COMP_WORDS[1]} == skill && ${COMP_CWORD} -ge 2 ]]; then
        # gloss skill [show | install [FOLDER]]
        if [[ ${COMP_CWORD} -eq 2 ]]; then
            COMPREPLY=( $(compgen -W "show install" -- "$cur") )
        elif [[ ${COMP_CWORD} -eq 3 && ${COMP_WORDS[2]} == install ]]; then
            compopt -o filenames 2>/dev/null
            COMPREPLY=( $(compgen -d -- "$cur") )
        fi
        return 0
    fi
    if [[ ${COMP_WORDS[1]} == help && ${COMP_CWORD} -ge 2 ]]; then
        [[ ${COMP_CWORD} -eq 2 ]] && COMPREPLY=( $(compgen -W "` + verbWords + `" -- "$cur") )
        return 0
    fi
    case "$opt" in
`)
	for _, o := range opts {
		if !o.TakesValue || o.Completer.empty() {
			continue
		}
		fmt.Fprintf(&b, "        %s)\n", strings.Join(o.names(), "|"))
		if o.Optional {
			// Only --opt=VALUE: a word after a bare --opt is a file.
			b.WriteString("            if [[ $opt == \"$prev\" ]]; then\n                compopt -o filenames 2>/dev/null\n                COMPREPLY=( $(compgen -f -- \"$cur\") )\n                return 0\n            fi\n")
		}
		c := o.Completer
		switch {
		case c.File:
			b.WriteString("            compopt -o filenames 2>/dev/null\n            COMPREPLY=( $(compgen -f -- \"$cur\") )\n")
		case c.Dir:
			b.WriteString("            compopt -o filenames 2>/dev/null\n            COMPREPLY=( $(compgen -d -- \"$cur\") )\n")
		case c.List:
			fmt.Fprintf(&b, "            compopt -o nospace 2>/dev/null\n            COMPREPLY=( $(compgen -P \"${cur%%\"${cur##*,}\"}\" -W %q -- \"${cur##*,}\") )\n", strings.Join(c.Words, " "))
		case len(c.Words) > 0:
			fmt.Fprintf(&b, "            COMPREPLY=( $(compgen -W %q -- \"$cur\") )\n", strings.Join(c.Words, " "))
		}
		b.WriteString("            return 0\n            ;;\n")
	}
	b.WriteString("        *) ;;\n    esac\n")
	// A value the option asked for, with nothing to offer, is the user's own.
	var plain []string
	for _, o := range opts {
		if o.TakesValue && !o.Optional && o.Completer.empty() {
			plain = append(plain, o.names()...)
		}
	}
	slices.Sort(plain)
	fmt.Fprintf(&b, "    case \"$opt\" in %s) return 0 ;; esac\n", strings.Join(plain, "|"))
	fmt.Fprintf(&b, "    if [[ $cur == -* ]]; then\n        COMPREPLY=( $(compgen -W %q -- \"$cur\") )\n        [[ ${COMPREPLY[*]} == --*= ]] && compopt -o nospace 2>/dev/null\n        return 0\n    fi\n", strings.Join(all, " "))
	// The first word may be a command, or else a file.
	fmt.Fprintf(&b, "    compopt -o filenames 2>/dev/null\n    COMPREPLY=( $(compgen -f -- \"$cur\") )\n    if [[ ${COMP_CWORD} -eq 1 ]]; then\n        COMPREPLY+=( $(compgen -W %q -- \"$cur\") )\n    fi\n}\ncomplete -F _gloss gloss\n", verbWords)
	return b.String()
}

// ---- zsh ----

// zshQuote makes s one word in single quotes.
func zshQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// zshDesc escapes a description for the brackets of an _arguments spec.
func zshDesc(s string) string {
	return strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(s)
}

func zshCompletion(f *pflag.FlagSet) string {
	var b strings.Builder
	b.WriteString("#compdef gloss\n# zsh completion for gloss. Written by gloss --docs-completions: do not edit.\n\n")
	b.WriteString("_gloss_first() {\n    local -a commands=(")
	for _, v := range verbs {
		b.WriteString(zshQuote(v.Name+":"+strings.ReplaceAll(v.Summary, ":", `\:`)) + " ")
	}
	b.WriteString(")\n    _alternative 'commands:command:(($commands))' 'files:file or folder:_files'\n}\n\n")
	b.WriteString("_arguments -s -S \\\n")
	for _, o := range completionOptions(f) {
		desc := zshDesc(o.Desc)
		var action string
		if o.TakesValue {
			c := o.Completer
			switch {
			case c.File:
				action = ":" + o.Long + ":_files"
			case c.Dir:
				action = ":folder:_files -/"
			case c.List:
				action = ":" + o.Long + ":_values -s , " + o.Long + " " + strings.Join(c.Words, " ")
			case len(c.Words) > 0:
				action = ":" + o.Long + ":(" + strings.Join(c.Words, " ") + ")"
			default:
				action = ":" + o.Long + ": "
			}
		}
		var spec string
		switch {
		case o.Optional:
			spec = "--" + o.Long + "=-[" + desc + "]" + action
			b.WriteString("  " + zshQuote(spec) + " \\\n")
			continue
		case o.TakesValue && o.Short != "":
			excl := "(-" + o.Short + " --" + o.Long + ")"
			fmt.Fprintf(&b, "  %s{-%s,--%s=}%s \\\n", zshQuote(excl), o.Short, o.Long, zshQuote("["+desc+"]"+action))
			continue
		case o.TakesValue:
			spec = "--" + o.Long + "=[" + desc + "]" + action
		case o.Short != "":
			excl := "(-" + o.Short + " --" + o.Long + ")"
			fmt.Fprintf(&b, "  %s{-%s,--%s}%s \\\n", zshQuote(excl), o.Short, o.Long, zshQuote("["+desc+"]"))
			continue
		default:
			spec = "--" + o.Long + "[" + desc + "]"
		}
		b.WriteString("  " + zshQuote(spec) + " \\\n")
	}
	b.WriteString("  '1:command or file:_gloss_first' \\\n  '*:file or folder:_files'\n")
	return b.String()
}

// ---- fish ----

// fishQuote makes s one word in single quotes.
func fishQuote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(s) + "'"
}

func fishCompletion(f *pflag.FlagSet) string {
	var b strings.Builder
	b.WriteString("# fish completion for gloss. Written by gloss --docs-completions: do not edit.\n\n")
	b.WriteString(`# The words of a comma-separated list, each after what is already typed.
function __gloss_list
    set -l prefix (string replace -r '[^,]*$' '' -- (string replace -r '^--[a-z0-9-]+=' '' -- (commandline -ct)))
    for word in $argv
        echo $prefix$word
    end
end

`)
	for _, v := range verbs {
		b.WriteString("complete -c gloss -n __fish_use_subcommand -a " + v.Name + " -d " + fishQuote(v.Summary) + "\n")
	}
	b.WriteString("complete -c gloss -n '__fish_seen_subcommand_from skill; and not __fish_seen_subcommand_from show install' -f -a 'show install'\n")
	b.WriteString("complete -c gloss -n '__fish_seen_subcommand_from skill; and __fish_seen_subcommand_from install' -x -a '(__fish_complete_directories)'\n")
	b.WriteString("complete -c gloss -n '__fish_seen_subcommand_from help' -f -a '" + verbWords + "'\n\n")
	for _, o := range completionOptions(f) {
		line := "complete -c gloss"
		if o.Short != "" {
			line += " -s " + o.Short
		}
		line += " -l " + o.Long
		if o.TakesValue && !o.Optional {
			c := o.Completer
			switch {
			case c.File:
				line += " -r -F"
			case c.Dir:
				line += ` -x -a "(__fish_complete_directories)"`
			case c.List:
				line += ` -x -a "(__gloss_list ` + strings.Join(c.Words, " ") + `)"`
			case len(c.Words) > 0:
				line += ` -x -a "` + strings.Join(c.Words, " ") + `"`
			default:
				line += " -x"
			}
		}
		line += " -d " + fishQuote(o.Desc)
		b.WriteString(line + "\n")
	}
	return b.String()
}
