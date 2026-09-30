package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/app"
)

func TestExportCLI(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{Options: app.Options{Files: []string{"../../examples/shapes.svg"}, Output: "-", MaxEdge: 512, Page: 1, DPI: 150}}
	if err := exportFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(&stdout)
	if err != nil || cfg.Width != 512 || cfg.Height != 320 {
		t.Fatalf("PNG: %+v %v", cfg, err)
	}
	opts.Output = ""
	opts.OutputDir = t.TempDir()
	opts.Files = append(opts.Files, "../../examples/tetrahedron.stl")
	if err := exportFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001-shapes.png", "002-tetrahedron.png"} {
		if _, err := os.Stat(filepath.Join(opts.OutputDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	// Exported again, the names gain a number; nothing is overwritten.
	stdout.Reset()
	if err := exportFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001-shapes-2.png", "002-tetrahedron-2.png"} {
		if _, err := os.Stat(filepath.Join(opts.OutputDir, name)); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), name) {
			t.Fatalf("the path written is not on stdout: %q", stdout.String())
		}
	}
}

func TestExportFlagValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--output", "one.png", "a.svg", "b.svg"},
		{"--output", "one.png", "--menu", "a.svg"},
		{"--output", "one.png", "--output-dir", "out", "a.svg"},
		{"--max-edge", "0"}, {"--max-edge", "4097"},
	} {
		if _, _, err := parse(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestExportHonoursTheRendererAskedFor(t *testing.T) {
	for mode, cpu := range map[string]bool{"auto": false, "software": true, "wireframe": true} {
		opts, _, err := parse([]string{"--3d", mode, "-o", "out.png", "model.stl"}, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		if got := exportRequest(opts.Options, "model.stl", 1); got.MaxEdge != 1536 || got.Path != "model.stl" {
			t.Fatalf("%+v", got)
		}
		if got := onCPU(opts.Options); got != cpu {
			t.Errorf("--3d %s: on the CPU = %v, want %v", mode, got, cpu)
		}
	}
}
