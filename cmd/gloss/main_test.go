package main

import (
	"bytes"
	"os"
	"path/filepath"
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
	// Source is text, which gloss shows; a binary blob is not.
	blob := filepath.Join(first, "blob.bin")
	if err := os.WriteFile(blob, []byte("\x00\x01\x02"), 0600); err != nil {
		t.Fatal(err)
	}
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
		{name: "source is text, and shown", paths: []string{"main.go"}, files: []string{"main.go"}},
		{name: "a folder beside something unsupported is browsed", paths: []string{blob, first}, dir: first, skipped: blob},
		{name: "something unsupported alone", paths: []string{blob}, skipped: blob, failed: "no supported input files"},
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

func TestColumnFlags(t *testing.T) {
	opts, _, err := parse([]string{"--cols", "Name, city", "--coln", "2,4-5", "a.csv"}, &bytes.Buffer{})
	if err != nil || !slices.Equal(opts.Columns.Names, []string{"Name", "city"}) || !slices.Equal(opts.Columns.Indexes, []int{2, 4, 5}) {
		t.Fatalf("%+v %v", opts.Columns, err)
	}
	if opts, _, err := parse([]string{"a.csv"}, &bytes.Buffer{}); err != nil || !opts.Columns.Empty() {
		t.Fatalf("default: %+v %v", opts.Columns, err)
	}
	if _, _, err := parse([]string{"--coln", "x", "a.csv"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--coln") {
		t.Fatalf("bad --coln: %v", err)
	}
}

func TestPartFlags(t *testing.T) {
	opts, _, err := parse([]string{"--parts", "Lid, hinge", "--partn", "1", "a.3mf"}, &bytes.Buffer{})
	if err != nil || !slices.Equal(opts.Parts.Names, []string{"Lid", "hinge"}) || !slices.Equal(opts.Parts.Indexes, []int{1}) {
		t.Fatalf("%+v %v", opts.Parts, err)
	}
	if _, _, err := parse([]string{"--partn", "0", "a.3mf"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--partn") {
		t.Fatalf("bad --partn: %v", err)
	}
	// Exports and descriptions carry the choice too.
	if q := exportRequest(opts.Options, "a.3mf", 1); len(q.Parts.Names) != 2 {
		t.Fatalf("export request: %+v", q.Parts)
	}
}

func TestColorFlag(t *testing.T) {
	opts, _, err := parse([]string{"--color", "orange", "a.stl"}, &bytes.Buffer{})
	if err != nil || opts.Color == nil || opts.Color.R != 255 || opts.Color.G != 140 {
		t.Fatalf("%+v %v", opts.Color, err)
	}
	if q := exportRequest(opts.Options, "a.stl", 1); q.Color == nil {
		t.Fatal("the export does not carry the color")
	}
	if opts, _, err := parse([]string{"a.stl"}, &bytes.Buffer{}); err != nil || opts.Color != nil {
		t.Fatalf("default: %+v %v", opts.Color, err)
	}
	if _, _, err := parse([]string{"--color", "#zz", "a.stl"}, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted a bad color")
	}
}

func TestGlobSearchesFolders(t *testing.T) {
	opts, _, err := parse([]string{"--glob", "*.png", "--glob", "docs", "photos"}, &bytes.Buffer{})
	if err != nil || !slices.Equal(opts.Globs, []string{"*.png", "docs"}) {
		t.Fatalf("%+v %v", opts.Globs, err)
	}
	dir := t.TempDir()
	for _, name := range []string{"a.png", "sub/b.png", "c.pdf", "d.txt"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0700)
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Folders named are searched; files named are kept as they are.
	var stderr bytes.Buffer
	paths, err := expandGlobs([]string{dir, filepath.Join(dir, "d.txt")}, []string{"png"}, &stderr)
	if err != nil || len(paths) != 3 || filepath.Base(paths[0]) != "a.png" || filepath.Base(paths[1]) != "b.png" || filepath.Base(paths[2]) != "d.txt" {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	// With no folder named, the working folder is searched.
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	paths, err = expandGlobs(nil, []string{"*.pdf"}, &stderr)
	if err != nil || len(paths) != 1 || filepath.Base(paths[0]) != "c.pdf" {
		t.Fatalf("cwd: paths=%v err=%v", paths, err)
	}
	// A bad pattern is an error; no match is an empty list, said on stderr.
	if _, err := expandGlobs([]string{dir}, []string{"["}, &stderr); err == nil {
		t.Fatal("a bad pattern was accepted")
	}
	paths, err = expandGlobs([]string{dir}, []string{"*.stl"}, &stderr)
	if err != nil || len(paths) != 0 || !strings.Contains(stderr.String(), "no match") {
		t.Fatalf("no match: paths=%v err=%v stderr=%q", paths, err, stderr.String())
	}
}
