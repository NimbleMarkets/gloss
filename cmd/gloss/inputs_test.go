package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/document"
)

func TestSkipUnsupportedBeforeSizeLimit(t *testing.T) {
	dir := t.TempDir()
	dmg := filepath.Join(dir, "Antigravity IDE.dmg")
	f, err := os.Create(dmg)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(document.MaxFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	var stderr bytes.Buffer
	files, err := supportedFiles([]string{dmg, "../../examples/shapes.svg"}, "", &stderr)
	if err != nil || len(files) != 1 || files[0] != "../../examples/shapes.svg" {
		t.Fatalf("files=%v err=%v", files, err)
	}
	if !strings.Contains(stderr.String(), "unsupported format (skipped)") || strings.Contains(stderr.String(), "128 MiB") {
		t.Fatal(stderr.String())
	}
	if _, err := supportedFiles([]string{dmg}, "", &stderr); err == nil || !strings.Contains(err.Error(), "no supported") {
		t.Fatalf("all unsupported: %v", err)
	}
	if _, err := supportedFiles([]string{dmg}, "image", &stderr); err == nil || !strings.Contains(err.Error(), "128 MiB") {
		t.Fatalf("forced type: %v", err)
	}
	png := filepath.Join(dir, "huge.png")
	if err := os.Rename(dmg, png); err != nil {
		t.Fatal(err)
	}
	if _, err := supportedFiles([]string{png}, "", &stderr); err == nil || !strings.Contains(err.Error(), "128 MiB") {
		t.Fatalf("supported oversize: %v", err)
	}
}

func TestProbeContentWithoutKnownExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drawing.unknown")
	if err := os.WriteFile(path, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0600); err != nil {
		t.Fatal(err)
	}
	kind, err := document.Probe(path, "")
	if err != nil || kind != "svg" {
		t.Fatalf("kind=%q err=%v", kind, err)
	}
}

func TestSkipDirectoriesInMixedList(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer
	files, err := supportedFiles([]string{dir, "../../examples/shapes.svg", dir, "../../examples/readme.md"}, "", &stderr)
	if err != nil || len(files) != 2 || files[0] != "../../examples/shapes.svg" || files[1] != "../../examples/readme.md" {
		t.Fatalf("files=%v err=%v", files, err)
	}
	if strings.Count(stderr.String(), "is a directory") != 2 || strings.Count(stderr.String(), "(skipped)") != 2 {
		t.Fatal(stderr.String())
	}
	if _, err := supportedFiles([]string{dir}, "", &stderr); err == nil || !strings.Contains(err.Error(), "no supported input files") {
		t.Fatalf("directories only: %v", err)
	}
}
