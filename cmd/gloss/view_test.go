package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	charts "github.com/NimbleMarkets/ntcharts3d"
)

func TestViewFlags(t *testing.T) {
	opts, _, err := parse([]string{"--view", "front,top", "-o", "out.png", "model.stl"}, &bytes.Buffer{})
	if err != nil || len(opts.Views) != 2 || opts.Views[0].Name != "front" || opts.Views[1].Alpha != 89 || opts.Views[0].Projection != charts.Orthographic {
		t.Fatalf("%+v %v", opts.Views, err)
	}
	opts, _, err = parse([]string{"--camera", "30,-60,2.5", "--projection", "perspective", "model.stl"}, &bytes.Buffer{})
	if err != nil || len(opts.Views) != 1 || opts.Views[0].Alpha != 30 || opts.Views[0].Beta != -60 || opts.Views[0].Distance != 2.5 || opts.Views[0].Projection != charts.Perspective {
		t.Fatalf("%+v %v", opts.Views, err)
	}
	opts, _, err = parse([]string{"--view", "all", "--projection", "perspective", "model.stl"}, &bytes.Buffer{})
	if err != nil || len(opts.Views) != 6 || opts.Views[5].Projection != charts.Perspective {
		t.Fatalf("%+v %v", opts.Views, err)
	}
	// The projection alone changes it for the camera there would have been.
	opts, _, err = parse([]string{"--projection", "perspective", "model.stl"}, &bytes.Buffer{})
	want := charts.DefaultCamera()
	if err != nil || len(opts.Views) != 1 || opts.Views[0].Alpha != want.Alpha || opts.Views[0].Beta != want.Beta || opts.Views[0].Distance != want.Distance || opts.Views[0].Projection != charts.Perspective {
		t.Fatalf("%+v %v", opts.Views, err)
	}
	if opts, _, err = parse([]string{"model.stl"}, &bytes.Buffer{}); err != nil || opts.Views != nil {
		t.Fatalf("with nothing asked for: %+v %v", opts.Views, err)
	}
	for _, args := range [][]string{
		{"--view", "sideways"}, {"--camera", "1"}, {"--camera", "0,0,0"}, {"--projection", "fisheye"},
		{"--view", "front", "--camera", "0,0"}, {"--view", ""},
	} {
		if _, _, err := parse(append(args, "model.stl"), &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestExportSheetOfViews(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sheet.png")
	opts, _, err := parse([]string{"--view", "all", "--3d", "software", "--max-edge", "600", "-o", out, "../../examples/gloss.stl"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := exportFiles(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "600×400 PNG") {
		t.Fatalf("six views make three by two tiles: %q", stderr.String())
	}
	if info, err := os.Stat(out); err != nil || info.Size() < 1000 {
		t.Fatalf("%v %v", info, err)
	}
}

func TestInfoFlags(t *testing.T) {
	opts, _, err := parse([]string{"--info", "--json", "a.stl", "b.pdf"}, &bytes.Buffer{})
	if err != nil || !opts.Info || !opts.JSON || len(opts.Files) != 2 {
		t.Fatalf("%+v %v", opts, err)
	}
	for _, args := range [][]string{
		{"--json", "a.stl"}, {"--info", "-o", "out.png", "a.stl"}, {"--info", "--serve", "a.stl"}, {"--info", "--pick", "a.stl"},
		{"--info", "-m", "a.stl"}, {"--info", "-X", "a.stl"},
	} {
		if _, _, err := parse(args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestInfoAsText(t *testing.T) {
	opts, _, _ := parse([]string{"--info", "../../examples/gloss.stl", "../../examples/field-guide.pdf", "-p", "2"}, &bytes.Buffer{})
	var stdout, stderr bytes.Buffer
	if err := describe(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{
		"../../examples/gloss.stl\n", "  Mesh\n", "    Triangles     888\n", "    Format        STL (ASCII)\n",
		"\n../../examples/field-guide.pdf\n", "    Pages      2\n", "    Page size  640 × 400 pt",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
	if stderr.Len() != 0 || strings.Contains(out, "\x1b") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestInfoDescribesTheModelNotItsPicture(t *testing.T) {
	// A 3MF with a thumbnail is previewed by it. Asked about, it is the
	// model that is described, and nothing is on show.
	var picture, archive bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(&archive)
	for name, content := range map[string]string{
		"Metadata/thumbnail.png": picture.String(),
		"3D/3dmodel.model": `<?xml version="1.0"?><model xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02" unit="inch"><resources><object id="1"><mesh>` +
			`<vertices><vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices>` +
			`<triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh></object></resources><build><item objectid="1"/></build></model>`,
	} {
		f, _ := w.Create(name)
		f.Write([]byte(content))
	}
	w.Close()
	path := filepath.Join(t.TempDir(), "part.3mf")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	opts, _, _ := parse([]string{"--info", "--json", path}, &bytes.Buffer{})
	var stdout, stderr bytes.Buffer
	if err := describe(opts, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var files []struct{ Details map[string]map[string]string }
	if err := json.Unmarshal(stdout.Bytes(), &files); err != nil || len(files) != 1 {
		t.Fatalf("%v\n%s", err, stdout.String())
	}
	model := files[0].Details["Model"]
	if _, shown := model["Shown"]; shown || model["Triangles"] != "1" || model["Extent"] != "1 × 1 × 0 in" || model["Thumbnail"] != "8 × 6" {
		t.Fatalf("%+v", model)
	}
}

func TestInfoAsJSON(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.pdf")
	if err := os.WriteFile(broken, []byte("%PDF-1.7\nnot much of a document"), 0600); err != nil {
		t.Fatal(err)
	}
	opts, _, _ := parse([]string{"--info", "--json", "../../examples/gloss.stl", dir, broken, filepath.Join(dir, "missing.png"), "../../examples/landscape.png"}, &bytes.Buffer{})
	var stdout, stderr bytes.Buffer
	err := describe(opts, &stdout, &stderr)
	// What can be described is; what cannot is said so, and the exit says
	// that not everything could be.
	if err == nil || !strings.Contains(err.Error(), "3 of 5") {
		t.Fatalf("err = %v", err)
	}
	var files []struct {
		Path    string
		Kind    string
		Page    int
		Pages   int
		Error   string
		Details map[string]map[string]string
	}
	if err := json.Unmarshal(stdout.Bytes(), &files); err != nil {
		t.Fatalf("%v\n%s", err, stdout.String())
	}
	if len(files) != 5 {
		t.Fatalf("%d files described:\n%s", len(files), stdout.String())
	}
	mesh, folder, torn, missing, picture := files[0], files[1], files[2], files[3], files[4]
	if mesh.Kind != "stl" || mesh.Error != "" || mesh.Details["Mesh"]["Triangles"] != "888" || mesh.Details["File"]["Path"] != "../../examples/gloss.stl" || mesh.Pages != 0 {
		t.Errorf("%+v", mesh)
	}
	if picture.Kind != "png" || picture.Details["Image"]["Dimensions"] != "640 × 400 (0.3 MP)" {
		t.Errorf("%+v", picture)
	}
	if folder.Path != dir || folder.Error != "is a directory" || folder.Details != nil {
		t.Errorf("%+v", folder)
	}
	if torn.Kind != "pdf" || torn.Error == "" || torn.Details["File"]["Size"] == "" || torn.Details["Document"]["Format"] != "PDF 1.7" {
		t.Errorf("%+v", torn)
	}
	if missing.Error != "no such file or directory" || missing.Kind != "" {
		t.Errorf("%+v", missing)
	}
	if stderr.Len() != 0 {
		t.Errorf("JSON on standard output is the whole report; stderr %q", stderr.String())
	}
	var pdf []struct{ Page, Pages int }
	opts, _, _ = parse([]string{"--info", "--json", "-p", "2", "../../examples/field-guide.pdf"}, &bytes.Buffer{})
	stdout.Reset()
	if err := describe(opts, &stdout, &stderr); err != nil || json.Unmarshal(stdout.Bytes(), &pdf) != nil || len(pdf) != 1 || pdf[0].Page != 2 || pdf[0].Pages != 2 {
		t.Fatalf("%v %+v\n%s", err, pdf, stdout.String())
	}
}
