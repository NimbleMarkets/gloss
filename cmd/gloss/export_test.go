package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/app"
)

func TestExportCLI(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := app.Options{Files: []string{"../../examples/shapes.svg"}, Output: "-", MaxEdge: 512, Page: 1, DPI: 150}
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
	if err := exportFiles(opts, &stdout, &stderr); err == nil {
		t.Fatal("overwrote existing output")
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
