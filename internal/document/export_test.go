package document

import (
	"image"
	"image/color"
	"image/draw"
	"os"
	"testing"

	charts "github.com/NimbleMarkets/ntcharts3d"
)

func TestExportSizeAndAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 210, 120))
	src.SetNRGBA(10, 20, color.NRGBA{R: 255, A: 255})
	for _, edge := range []int{100, 400} {
		img, err := ExportImage(Result{Image: src}, edge)
		if err != nil {
			t.Fatal(err)
		}
		wantW := min(edge, 200)
		if img.Bounds().Dx() != wantW || img.Bounds().Dy() != wantW/2 {
			t.Fatal(img.Bounds())
		}
		r, g, b, a := img.At(img.Bounds().Max.X-1, img.Bounds().Max.Y-1).RGBA()
		if r != 65535 || g != 65535 || b != 65535 || a != 65535 {
			t.Fatal("transparency not flattened onto white")
		}
	}
}

func TestExportPDFAtTargetSize(t *testing.T) {
	l := &Loader{}
	defer l.Close()
	r := l.Load(Request{Path: writeFixture(t, "pages.pdf", samplePDF()), Page: 2, DPI: 72, MaxEdge: 256, Generation: 1})
	img, err := ExportImage(r, 256)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatal(img.Bounds())
	}
	red, _, blue, _ := img.At(128, 128).RGBA()
	if blue <= red {
		t.Fatal("wrong PDF page exported")
	}
}

func TestExportMesh(t *testing.T) {
	m, err := ParseSTL([]byte(triangleSTL))
	if err != nil {
		t.Fatal(err)
	}
	img, err := ExportImage(Result{Mesh: m}, 128)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 128, 128) {
		t.Fatal(img.Bounds())
	}
	colored := 0
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r != g || g != b {
				colored++
			}
		}
	}
	if colored < 100 {
		t.Fatalf("mesh not visible: %d colored pixels", colored)
	}
}

// drawing replaces the GPU for a test, and notes what it was asked for.
// A nil picture stands for a machine without a GPU.
func drawing(t *testing.T, img image.Image) *[][]shot {
	t.Helper()
	var asked [][]shot
	real := gpuSnapshot
	gpuSnapshot = func(_ *Mesh, _ int, shots []shot) ([]image.Image, bool) {
		asked = append(asked, shots)
		if img == nil {
			return nil, false
		}
		out := make([]image.Image, len(shots))
		for i := range out {
			out[i] = img
		}
		return out, true
	}
	t.Cleanup(func() { gpuSnapshot = real })
	return &asked
}

func TestMeshExportPrefersTheGPU(t *testing.T) {
	mesh, err := ParseSTL([]byte(triangleSTL))
	if err != nil {
		t.Fatal(err)
	}
	picture := image.NewRGBA(image.Rect(0, 0, 128, 128))
	asked := drawing(t, picture)
	img, err := ExportImage(Result{Mesh: mesh}, 128)
	if err != nil || img != image.Image(picture) || len(*asked) != 1 || len((*asked)[0]) != 1 {
		t.Fatalf("img=%v err=%v asked=%v", img.Bounds(), err, *asked)
	}
	// Software asked for by name is what is used.
	img, err = ExportImage(Result{Mesh: mesh, CPU: true}, 128)
	if err != nil || img == image.Image(picture) || len(*asked) != 1 || img.Bounds() != image.Rect(0, 0, 128, 128) {
		t.Fatalf("img=%v err=%v asked=%v", img.Bounds(), err, *asked)
	}
}

func TestMeshExportFallsBackToTheCPU(t *testing.T) {
	mesh, err := ParseSTL([]byte(triangleSTL))
	if err != nil {
		t.Fatal(err)
	}
	asked := drawing(t, nil)
	img, err := ExportImage(Result{Mesh: mesh}, 128)
	if err != nil || len(*asked) != 1 || img.Bounds() != image.Rect(0, 0, 128, 128) {
		t.Fatalf("err=%v asked=%v", err, *asked)
	}
	colored := 0
	for y := range 128 {
		for x := range 128 {
			if r, g, b, _ := img.At(x, y).RGBA(); r != g || g != b {
				colored++
			}
		}
	}
	if colored < 100 {
		t.Fatalf("mesh not visible: %d colored pixels", colored)
	}
}

func TestGPUDrawsTheMesh(t *testing.T) {
	data, err := os.ReadFile("../../examples/gloss.stl")
	if err != nil {
		t.Fatal(err)
	}
	mesh, err := ParseSTL(data)
	if err != nil {
		t.Fatal(err)
	}
	camera := charts.DefaultCamera()
	imgs, ok := gpuSnapshot(mesh, 256, []shot{{camera, worldLight}})
	if !ok {
		t.Skip("no GPU here; the CPU draws exports")
	}
	img := imgs[0]
	if img.Bounds() != image.Rect(0, 0, 256, 256) {
		t.Fatal(img.Bounds())
	}
	colored, white := 0, 0
	for y := range 256 {
		for x := range 256 {
			r, g, b, a := img.At(x, y).RGBA()
			if a != 0xffff {
				t.Fatalf("the export is not opaque at %d,%d", x, y)
			}
			switch {
			case r == 0xffff && g == 0xffff && b == 0xffff:
				white++
			case r != g || g != b:
				colored++
			}
		}
	}
	// The model, on white, and nothing else: no axes, grid, or legend.
	if colored < 2000 || white < 256*256/3 || white+colored < 256*256*95/100 {
		t.Fatalf("colored=%d white=%d of %d", colored, white, 256*256)
	}
	if img.At(2, 2) != img.At(253, 253) {
		t.Fatal("the corners differ: something is drawn at the edge")
	}
	camera.Beta += 90
	side := camera
	side.Alpha = 0
	both, ok := gpuSnapshot(mesh, 256, []shot{{camera, worldLight}, {side, worldLight}})
	if !ok || len(both) != 2 {
		t.Fatal("the GPU was lost")
	}
	if turned, other := both[0], both[1]; turned.At(128, 128) == nil || other.Bounds() != turned.Bounds() {
		t.Fatal(other.Bounds())
	}
	turned := both[0]
	differing := 0
	for y := range 256 {
		for x := range 256 {
			if img.At(x, y) != turned.At(x, y) {
				differing++
			}
		}
	}
	if differing < 500 {
		t.Fatalf("turning the camera changed %d pixels", differing)
	}
}

// inked counts the pixels of a region that are neither the paper nor the
// model: what is written there.
func inked(img image.Image, region image.Rectangle) (ink, model int) {
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			switch {
			case r != g || g != b:
				model++
			case r < 0x8000:
				ink++
			}
		}
	}
	return ink, model
}

func TestExportFromANamedView(t *testing.T) {
	mesh := box(t, 10, 1, 2)
	views, _ := ParseViews("top")
	plain, err := ExportImage(Result{Mesh: mesh, CPU: true}, 300)
	if err != nil {
		t.Fatal(err)
	}
	top, err := ExportImage(Result{Mesh: mesh, CPU: true, Views: views}, 300)
	if err != nil || top.Bounds() != image.Rect(0, 0, 300, 300) {
		t.Fatalf("%v %v", top.Bounds(), err)
	}
	if wide, high, clipped := spread(top); clipped || wide < .87 || wide > .93 || high > .12 {
		t.Fatalf("from above a plank covers %.2f by %.2f of the picture, clipped=%v", wide, high, clipped)
	}
	if wide, _, _ := spread(plain); wide > .85 {
		t.Fatalf("with no view asked for the camera is as it was: %.2f", wide)
	}
	// One view is a picture, not a sheet: nothing is written on it.
	if ink, _ := inked(top, top.Bounds()); ink != 0 {
		t.Fatalf("%d pixels of writing on a single view", ink)
	}
	// What the viewer's camera sees is what is saved, whatever was asked for at the start.
	camera := charts.DefaultCamera()
	saved, err := ExportImage(Result{Mesh: mesh, CPU: true, Views: views, Camera: &camera}, 300)
	if err != nil || saved.Bounds() != plain.Bounds() {
		t.Fatal(err)
	}
	for y := range 300 {
		for x := range 300 {
			if saved.At(x, y) != plain.At(x, y) {
				t.Fatalf("the viewer's camera was not used at %d,%d", x, y)
			}
		}
	}
}

func TestExportContactSheet(t *testing.T) {
	mesh := box(t, 4, 2, 1)
	for _, tt := range []struct {
		views      string
		cols, rows int
	}{{"front,top", 2, 1}, {"front,top,iso", 2, 2}, {"front,right,top,iso", 2, 2}, {"iso,all", 3, 3}, {"all", 3, 2}, {"all,all", 4, 3}} {
		views, err := ParseViews(tt.views)
		if err != nil {
			t.Fatal(err)
		}
		img, err := ExportImage(Result{Mesh: mesh, CPU: true, Views: views}, 900)
		if err != nil {
			t.Fatal(err)
		}
		tile := 900 / tt.cols
		if img.Bounds() != image.Rect(0, 0, tt.cols*tile, tt.rows*tile) {
			t.Fatalf("%s: %v, want %d by %d tiles of %d", tt.views, img.Bounds(), tt.cols, tt.rows, tile)
		}
		for i := range tt.cols * tt.rows {
			at := image.Rect(0, 0, tile, tile).Add(image.Pt(i%tt.cols*tile, i/tt.cols*tile))
			// Each view is named in its corner, clear of the model.
			ink, _ := inked(img, image.Rect(at.Min.X, at.Min.Y, at.Min.X+tile/2, at.Min.Y+tile/8))
			_, model := inked(img, at)
			switch {
			case i >= len(views) && (ink != 0 || model != 0):
				t.Errorf("%s: tile %d should be blank", tt.views, i)
			case i < len(views) && (ink < 20 || model < tile*tile/50):
				t.Errorf("%s: tile %d (%s) has %d pixels of writing and %d of model", tt.views, i, views[i].Name, ink, model)
			}
		}
		// Tiles are drawn apart: the model in one does not run into the next.
		for i := range views {
			at := image.Rect(0, 0, tile, tile).Add(image.Pt(i%tt.cols*tile, i/tt.cols*tile))
			for _, edge := range []image.Rectangle{
				{at.Min, image.Pt(at.Max.X, at.Min.Y+1)}, {image.Pt(at.Min.X, at.Max.Y-1), at.Max},
				{at.Min, image.Pt(at.Min.X+1, at.Max.Y)}, {image.Pt(at.Max.X-1, at.Min.Y), at.Max},
			} {
				if _, model := inked(img, edge); model != 0 {
					t.Errorf("%s: the model touches the edge of tile %d", tt.views, i)
				}
			}
		}
	}
}

func TestContactSheetFitsAProfileEdge(t *testing.T) {
	views, _ := ParseViews("all")
	for name, profile := range VisionProfiles {
		img, err := ExportImage(Result{Mesh: box(t, 4, 2, 1), CPU: true, Views: views}, profile.MaxEdge)
		if err != nil {
			t.Fatal(err)
		}
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		if w > profile.MaxEdge || h > profile.MaxEdge {
			t.Errorf("%s: a sheet of %d×%d is over the edge", name, w, h)
		}
		if w%3 != 0 || h%2 != 0 || w/3 != h/2 || w < profile.MaxEdge*2/3 {
			t.Errorf("%s: %d×%d is not three by two square tiles, or wastes the edge", name, w, h)
		}
	}
}

func TestContactSheetIsDrawnOnTheGPUAtOnce(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 300, 300))
	asked := drawing(t, picture)
	views, _ := ParseViews("all")
	img, err := ExportImage(Result{Mesh: box(t, 4, 2, 1), Views: views}, 900)
	if err != nil || img.Bounds() != image.Rect(0, 0, 900, 600) {
		t.Fatalf("%v %v", img.Bounds(), err)
	}
	// One chart, and one upload of the mesh, for all six.
	if len(*asked) != 1 || len((*asked)[0]) != 6 {
		t.Fatalf("the GPU was asked %d times: %v", len(*asked), *asked)
	}
	if (*asked)[0][4].camera.Alpha != 89 || (*asked)[0][0].camera.Beta != -90 {
		t.Fatalf("cameras %+v", (*asked)[0])
	}
}

// brightness is the mean lightness of the model in a region, of 255.
func brightness(img image.Image, region image.Rectangle) float64 {
	sum, n := 0.0, 0
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if r, g, b, _ := img.At(x, y).RGBA(); r != g || g != b {
				sum += float64(r+g+b) / 3 / 257
				n++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func TestEveryViewIsLit(t *testing.T) {
	// A light fixed in the world leaves the back, the left, and the
	// underside in shadow. Views asked for are lit from over the viewer's
	// shoulder, so that each shows as much as any other.
	views, _ := ParseViews("all")
	mesh := box(t, 3, 3, 3)
	sheet, err := ExportImage(Result{Mesh: mesh, CPU: true, Views: views}, 900)
	if err != nil {
		t.Fatal(err)
	}
	var lit []float64
	for i := range views {
		lit = append(lit, brightness(sheet, image.Rect(0, 0, 300, 300).Add(image.Pt(i%3*300, i/3*300))))
	}
	for i, v := range views {
		if lit[i] < 100 || lit[i] < .9*lit[0] || lit[i] > 1.1*lit[0] {
			t.Errorf("%s is lit %.0f, the front %.0f", v.Name, lit[i], lit[0])
		}
	}
	// With no view asked for the light is where the viewer has it.
	plain, _ := ExportImage(Result{Mesh: mesh, CPU: true}, 300)
	camera := charts.DefaultCamera()
	saved, _ := ExportImage(Result{Mesh: mesh, CPU: true, Camera: &camera, Views: views}, 300)
	if brightness(plain, plain.Bounds()) != brightness(saved, saved.Bounds()) {
		t.Error("what the viewer saves is lit differently from what it shows")
	}
}

func TestGPULightsEveryView(t *testing.T) {
	views, _ := ParseViews("front,back")
	mesh := box(t, 3, 3, 3)
	var shots []shot
	for _, v := range views {
		shots = append(shots, shot{v.Camera(mesh), v.light()})
	}
	imgs, ok := gpuSnapshot(mesh, 256, shots)
	if !ok {
		t.Skip("no GPU here; the CPU draws exports")
	}
	front, back := brightness(imgs[0], imgs[0].Bounds()), brightness(imgs[1], imgs[1].Bounds())
	if front < 100 || back < .9*front || back > 1.1*front {
		t.Fatalf("the front is lit %.0f and the back %.0f", front, back)
	}
	// The GPU and the CPU draw the same picture.
	cpu := renderMesh(mesh, 256, shots[1].camera, shots[1].light)
	if got := brightness(cpu, cpu.Bounds()); got < .95*back || got > 1.05*back {
		t.Fatalf("the CPU lights the back %.0f, the GPU %.0f", got, back)
	}
}

// An outline marks where depth breaks, as at the rim of a hole, and the edge
// of the mesh; a slope, however steep, is not an edge.
func TestOutlineMarksBreaksNotSlopes(t *testing.T) {
	const size = 40
	depth := make([]float32, size*size)
	for y := range size {
		for x := range size {
			switch {
			case x < 4:
				depth[y*size+x] = 2 // Background.
			case x >= 20 && x < 30 && y >= 10 && y < 20:
				depth[y*size+x] = .9 // Seen through a hole.
			default:
				depth[y*size+x] = float32(x) / size * .5 // A steep slope.
			}
		}
	}
	src := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{R: 200, G: 160, B: 120, A: 255}), image.Point{}, draw.Src)
	img := outline(src, depth, size).(*image.RGBA)
	inked := func(x, y int) bool { return img.RGBAAt(x, y).R < 100 && img.RGBAAt(x, y).R > 0 }
	if !inked(4, 5) || !inked(20, 15) || !inked(25, 10) {
		t.Error("an edge was not drawn")
	}
	if inked(10, 5) || inked(35, 30) || inked(25, 15) || inked(2, 5) {
		t.Error("a slope, the background, or a flat inside was drawn")
	}
}
