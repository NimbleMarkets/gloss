package main

import (
	"bytes"
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
