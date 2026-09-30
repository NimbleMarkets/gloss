package document

import (
	"image/color"
	"testing"

	"github.com/NimbleMarkets/ntcharts3d/math3d"
)

const facetSTL = "solid t\nfacet normal 0 0 0\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nvertex 0 1 0\nendloop\nendfacet\nendsolid t\n"

func TestMeshRecolor(t *testing.T) {
	mesh, err := ParseSTL([]byte(facetSTL))
	if err != nil {
		t.Fatal(err)
	}
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	if mesh.HasColors() {
		t.Fatal("an STL has colors of its own")
	}
	mesh.Recolor(red, false)
	if g, _ := mesh.Geometry(nil); g.Vertices[0].Color != red {
		t.Fatalf("plain faces not recolored: %v", g.Vertices[0].Color)
	}
	mesh.RestoreColors()
	if g, _ := mesh.Geometry(nil); g.Vertices[0].Color != meshColor {
		t.Fatalf("not restored: %v", g.Vertices[0].Color)
	}
	// A mesh with its own colors keeps them unless all faces are asked for.
	mesh = &Mesh{}
	v := [3]math3d.Vec3{{X: 0, Y: 0, Z: 0}, {X: 1, Y: 0, Z: 0}, {X: 0, Y: 1, Z: 0}}
	mesh.add(v, blue)
	mesh.add(v, meshColor)
	if !mesh.HasColors() {
		t.Fatal("colors not seen")
	}
	mesh.Recolor(red, false)
	g, _ := mesh.Geometry(nil)
	if g.Vertices[0].Color != blue || g.Vertices[3].Color != red {
		t.Fatalf("plain only: %v %v", g.Vertices[0].Color, g.Vertices[3].Color)
	}
	mesh.Recolor(red, true)
	g, _ = mesh.Geometry(nil)
	if g.Vertices[0].Color != red {
		t.Fatalf("all: %v", g.Vertices[0].Color)
	}
	mesh.RestoreColors()
	g, _ = mesh.Geometry(nil)
	if g.Vertices[0].Color != blue || g.Vertices[3].Color != meshColor {
		t.Fatalf("restored: %v %v", g.Vertices[0].Color, g.Vertices[3].Color)
	}
}

func TestParseColor(t *testing.T) {
	for in, want := range map[string]color.RGBA{
		"#ff8000": {R: 255, G: 128, A: 255}, "FF8000": {R: 255, G: 128, A: 255}, "#f80": {R: 255, G: 136, A: 255},
		"orange": {R: 255, G: 140, A: 255},
	} {
		got, err := ParseColor(in)
		if err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "#12", "#ggg", "mauve-ish"} {
		if _, err := ParseColor(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestLoaderPaintsAMeshAsAsked(t *testing.T) {
	l := &Loader{}
	defer l.Close()
	red := color.RGBA{R: 255, A: 255}
	r := l.Load(Request{Path: write(t, "t.stl", []byte(facetSTL)), Page: 1, Generation: 1, Color: &red})
	if r.Err != nil || r.Mesh == nil {
		t.Fatal(r.Err)
	}
	if g, _ := r.Mesh.Geometry(nil); g.Vertices[0].Color != red {
		t.Fatalf("color %v", g.Vertices[0].Color)
	}
}
