package main

import (
	"testing"

	"github.com/NimbleMarkets/gloss/examples"
)

func TestDemoPrefersTheGPU(t *testing.T) {
	// The browser draws meshes with WebGPU where it can. NTCharts3d falls
	// back to software by itself where it cannot.
	opts, ok := options("")
	if !ok || opts.Render3D != "auto" {
		t.Fatalf("Render3D = %q", opts.Render3D)
	}
	if !opts.Menu || !opts.Preview || len(opts.Files) != len(examples.Names) || opts.Files[0] != examples.Names[0] {
		t.Fatalf("the gallery opens on its menu, in order: %+v", opts)
	}
}

func TestDemoOpensOnASample(t *testing.T) {
	opts, ok := options("gloss.stl")
	if !ok || opts.Menu || opts.Preview || opts.Files[0] != "gloss.stl" || len(opts.Files) != len(examples.Names) || opts.Render3D != "auto" {
		t.Fatalf("%+v", opts)
	}
	if _, ok := options("missing.png"); ok {
		t.Fatal("an unknown sample was accepted")
	}
}
