package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	for _, args := range [][]string{{"--page", "0"}, {"--dpi", "601"}, {"--render", "bad"}, {"--type", "bad"}, {"--3d", "bad"}, {"--no-such-flag"}} {
		if _, _, err := parse(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, flag := range []string{"--help", "-h", "--version", "-V"} {
		var out bytes.Buffer
		_, done, _ := parse([]string{flag}, &out)
		if !done || out.Len() == 0 {
			t.Fatalf("failed %s", flag)
		}
	}
	opts, done, err := parse([]string{"--render", "glyph", "--page", "2", "--", "-odd.pdf", "other.svg"}, &bytes.Buffer{})
	if err != nil || done || opts.Page != 2 || len(opts.Files) != 2 || opts.Files[0] != "-odd.pdf" {
		t.Fatalf("parse = %+v, %v", opts, err)
	}
}

func TestGNUOptions(t *testing.T) {
	opts, _, err := parse([]string{"report.pdf", "-p3", "-o", "page.png", "--vision-profile=claude-standard", "-s1024"}, &bytes.Buffer{})
	if err != nil || opts.Page != 3 || opts.Output != "page.png" || opts.MaxEdge != 1024 || len(opts.Files) != 1 {
		t.Fatalf("interspersed options: %+v %v", opts, err)
	}
	opts, _, err = parse([]string{"a.svg", "-mP", "b.stl", "--render=glyph"}, &bytes.Buffer{})
	if err != nil || !opts.Menu || !opts.Preview || opts.Render != "glyph" || len(opts.Files) != 2 {
		t.Fatalf("bundled flags: %+v %v", opts, err)
	}
	opts, _, err = parse([]string{"-oout.png", "--vision-profile", "claude-standard", "a.svg"}, &bytes.Buffer{})
	if err != nil || opts.MaxEdge != 1568 {
		t.Fatalf("profile default: %+v %v", opts, err)
	}
}

func TestBareGlossIsNotAnError(t *testing.T) {
	opts, done, err := parse(nil, &bytes.Buffer{})
	if err != nil || done || len(opts.Files) != 0 {
		t.Fatalf("parse = %+v, done=%v, %v", opts, done, err)
	}
	// The viewer needs no flag to start, so none is offered.
	for _, flag := range []string{"--tui", "-f"} {
		if _, _, err := parse([]string{flag}, &bytes.Buffer{}); err == nil {
			t.Fatalf("%s is still accepted", flag)
		}
	}
	var help bytes.Buffer
	if _, done, _ := parse([]string{"--help"}, &help); !done || !strings.Contains(help.String(), "gloss [options] [file | folder]...") {
		t.Fatalf("help:\n%s", help.String())
	}
}

func TestInputs(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	svg, md := "../../examples/shapes.svg", "../../examples/readme.md"
	for _, tt := range []struct {
		name            string
		paths           []string
		exporting       bool
		dir             string
		files           []string
		skipped, failed string
	}{
		{name: "nothing opens the empty viewer"},
		{name: "a file", paths: []string{svg}, files: []string{svg}},
		{name: "a folder is browsed", paths: []string{first}, dir: first},
		{name: "the first of several folders is browsed", paths: []string{first, second}, dir: first, skipped: second},
		{name: "folders among files are skipped, as a glob needs", paths: []string{svg, first, md}, files: []string{svg, md}, skipped: first},
		{name: "a folder beside something unsupported is browsed", paths: []string{"main.go", first}, dir: first, skipped: "main.go"},
		{name: "something unsupported alone", paths: []string{"main.go"}, skipped: "main.go", failed: "no supported input files"},
		{name: "a missing file", paths: []string{"missing.png"}, failed: "missing.png"},
		{name: "a missing file beside a folder", paths: []string{first, "missing.png"}, skipped: first, failed: "missing.png"},
		{name: "an export of nothing", exporting: true, failed: "no input"},
		{name: "an export of a folder", paths: []string{first}, exporting: true, skipped: first, failed: "no supported input files"},
		{name: "an export skips folders", paths: []string{first, svg}, exporting: true, files: []string{svg}, skipped: first},
	} {
		var stderr bytes.Buffer
		dir, files, err := inputs(tt.paths, "", tt.exporting, &stderr)
		if dir != tt.dir || !slices.Equal(files, tt.files) {
			t.Errorf("%s: dir=%q files=%q", tt.name, dir, files)
		}
		if (err == nil) != (tt.failed == "") || (err != nil && !strings.Contains(err.Error(), tt.failed)) {
			t.Errorf("%s: err=%v, want %q", tt.name, err, tt.failed)
		}
		if got := strings.Count(stderr.String(), "(skipped)"); (tt.skipped == "" && got != 0) || (tt.skipped != "" && (got != 1 || !strings.Contains(stderr.String(), tt.skipped+":"))) {
			t.Errorf("%s: stderr=%q, want %q skipped", tt.name, stderr.String(), tt.skipped)
		}
	}
}

func TestTypeAccepts3MF(t *testing.T) {
	opts, _, err := parse([]string{"--type", "3mf", "part"}, &bytes.Buffer{})
	if err != nil || opts.Type != "3mf" {
		t.Fatalf("%+v %v", opts, err)
	}
	var help bytes.Buffer
	if _, _, _ = parse([]string{"--help"}, &help); strings.Count(help.String(), "3MF")+strings.Count(help.String(), "3mf") < 2 {
		t.Fatalf("help does not mention 3MF:\n%s", help.String())
	}
}

func TestMainScreenFlag(t *testing.T) {
	for _, flag := range []string{"-X", "--no-alt-screen"} {
		opts, _, err := parse([]string{flag, "a.svg"}, &bytes.Buffer{})
		if err != nil || !opts.KeepScreen || len(opts.Files) != 1 {
			t.Fatalf("%s: %+v %v", flag, opts, err)
		}
	}
	if opts, _, err := parse([]string{"a.svg"}, &bytes.Buffer{}); err != nil || opts.KeepScreen {
		t.Fatalf("default: %+v %v", opts, err)
	}
	if _, _, err := parse([]string{"-X", "-o", "out.png", "a.svg"}, &bytes.Buffer{}); err == nil {
		t.Fatal("-X accepted with an export, which draws nothing")
	}
}
