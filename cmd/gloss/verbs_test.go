package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestSplitVerb(t *testing.T) {
	for _, tt := range []struct {
		args []string
		verb string
		rest []string
	}{
		{nil, "view", nil},
		{[]string{"a.png"}, "view", []string{"a.png"}},
		{[]string{"view", "a.png"}, "view", []string{"a.png"}},
		{[]string{"skill"}, "skill", []string{}},
		{[]string{"skill", "install", "x"}, "skill", []string{"install", "x"}},
		{[]string{"help", "skill"}, "help", []string{"skill"}},
		// Only the first argument is a verb, and only exactly.
		{[]string{"-X", "skill"}, "view", []string{"-X", "skill"}},
		{[]string{"a.png", "skill"}, "view", []string{"a.png", "skill"}},
		{[]string{"Skill"}, "view", []string{"Skill"}},
		{[]string{"skills"}, "view", []string{"skills"}},
		{[]string{"--", "skill"}, "view", []string{"--", "skill"}},
	} {
		verb, rest := splitVerb(tt.args)
		if verb != tt.verb || !slices.Equal(rest, tt.rest) {
			t.Errorf("splitVerb(%q) = %q, %q; want %q, %q", tt.args, verb, rest, tt.verb, tt.rest)
		}
	}
}

// A file named like a verb is opened by spelling out view.
func TestViewOpensAFileNamedLikeAVerb(t *testing.T) {
	for _, name := range []string{"view", "skill", "help"} {
		opts, done, err := parse([]string{"view", name}, &bytes.Buffer{})
		if err != nil || done || !slices.Equal(opts.Files, []string{name}) {
			t.Errorf("gloss view %s = %+v, done=%v, %v", name, opts.Files, done, err)
		}
		// View takes every option the implicit one does.
		opts, _, err = parse([]string{"view", "--page", "2", name}, &bytes.Buffer{})
		if err != nil || opts.Page != 2 || !slices.Equal(opts.Files, []string{name}) {
			t.Errorf("gloss view --page 2 %s = %+v, %v", name, opts, err)
		}
	}
	// After an option, or after a file, a verb's name is a file.
	opts, _, err := parse([]string{"-X", "skill"}, &bytes.Buffer{})
	if err != nil || !slices.Equal(opts.Files, []string{"skill"}) || !opts.KeepScreen {
		t.Errorf("gloss -X skill = %+v, %v", opts, err)
	}
}

func TestHelpVerb(t *testing.T) {
	var all bytes.Buffer
	if _, done, err := parse([]string{"help"}, &all); err != nil || !done {
		t.Fatalf("help: done=%v err=%v", done, err)
	}
	var viaFlag bytes.Buffer
	parse([]string{"--help"}, &viaFlag)
	if all.String() != viaFlag.String() {
		t.Errorf("gloss help and gloss --help differ")
	}
	for _, v := range verbs {
		if !strings.Contains(all.String(), v.Name+" ") || !strings.Contains(all.String(), v.Summary) {
			t.Errorf("help does not list %s", v.Name)
		}
		var out bytes.Buffer
		if _, done, err := parse([]string{"help", v.Name}, &out); err != nil || !done || out.Len() == 0 {
			t.Errorf("help %s: done=%v err=%v", v.Name, done, err)
		}
	}
	for _, args := range [][]string{{"help", "nonsense"}, {"help", "skill", "view"}} {
		if _, _, err := parse(args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
