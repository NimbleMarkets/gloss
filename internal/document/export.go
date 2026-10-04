package document

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/NimbleMarkets/ntcharts3d/math3d"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// ExportImage produces an opaque image bounded by maxEdge on both axes.
// Raster sources are never enlarged; vector documents render at the target.
func ExportImage(r Result, maxEdge int) (image.Image, error) {
	if maxEdge < 1 || maxEdge > 4096 {
		return nil, fmt.Errorf("max edge must be 1..4096")
	}
	if r.Err != nil {
		return nil, r.Err
	}
	if r.Markdown != nil {
		return nil, fmt.Errorf("Markdown PNG export is not supported; supply its text directly to the model")
	}
	if r.Mesh != nil {
		return exportMesh(r, maxEdge)
	}
	if r.Image == nil || r.Image.Bounds().Empty() {
		return nil, fmt.Errorf("document has no image")
	}
	b := r.Image.Bounds()
	w, h, err := VisionSize(b.Dx(), b.Dy(), maxEdge)
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(out, out.Bounds(), r.Image, b, draw.Over, nil)
	return out, nil
}

// exportMesh draws the mesh from the viewer's camera, or from each view
// asked for. Several views make a sheet: square tiles, as many across as
// down or one more, each named in its corner.
func exportMesh(r Result, maxEdge int) (image.Image, error) {
	shots, names := []shot{{charts.DefaultCamera(), worldLight}}, []string{""}
	named := false
	switch {
	case r.Camera != nil:
		shots[0].camera = *r.Camera
	case len(r.Views) > 0:
		shots, names, named = nil, nil, true
		for _, v := range r.Views {
			shots, names = append(shots, shot{v.Camera(r.Mesh), v.light()}), append(names, v.Name)
		}
	}
	cols := int(math.Ceil(math.Sqrt(float64(len(shots)))))
	rows := (len(shots) + cols - 1) / cols
	// The sheet as a whole keeps to the edge.
	tile := maxEdge / cols
	w, h, err := VisionSize(cols*tile, rows*tile, maxEdge)
	if err != nil {
		return nil, err
	}
	if tile = min(w/cols, h/rows); tile < 1 {
		return nil, fmt.Errorf("%d views do not fit %d pixels", len(shots), maxEdge)
	}
	var tiles []image.Image
	if !r.CPU {
		if drawn, ok := gpuSnapshot(r.Mesh, tile, shots); ok && len(drawn) == len(shots) {
			tiles = drawn
		}
	}
	for i := 0; tiles == nil && i < len(shots) || len(tiles) < len(shots); i++ {
		tiles = append(tiles, renderMesh(r.Mesh, tile, shots[i].camera, shots[i].light))
	}
	// Faces that look the same way are shaded alike, so that from straight
	// ahead a hole, or the floor of a box seen from the top, vanishes into
	// what is behind it. Views asked for by name have their edges drawn.
	if named {
		for i, img := range tiles {
			tiles[i] = outline(img, meshDepth(r.Mesh, tile, shots[i].camera), tile)
		}
	}
	if len(tiles) == 1 {
		return tiles[0], nil
	}
	sheet := image.NewRGBA(image.Rect(0, 0, cols*tile, rows*tile))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for i, img := range tiles {
		at := image.Rect(0, 0, tile, tile).Add(image.Pt(i%cols*tile, i/cols*tile))
		draw.Draw(sheet, at, img, img.Bounds().Min, draw.Src)
		label(sheet, at, names[i])
	}
	return sheet, nil
}

// label names a tile in its top-left corner, in letters large enough to be
// read in a sheet shown small.
func label(sheet *image.RGBA, tile image.Rectangle, name string) {
	face := basicfont.Face7x13
	text := image.NewRGBA(image.Rect(0, 0, len(name)*face.Advance, face.Height))
	(&font.Drawer{Dst: text, Src: image.NewUniform(color.RGBA{R: 40, G: 40, B: 40, A: 255}), Face: face, Dot: fixed.P(0, face.Ascent)}).DrawString(name)
	scale := max(1, tile.Dx()/200)
	margin := max(2, tile.Dx()/40)
	at := image.Rect(0, 0, text.Bounds().Dx()*scale, text.Bounds().Dy()*scale).Add(tile.Min).Add(image.Pt(margin, margin)).Intersect(tile)
	xdraw.NearestNeighbor.Scale(sheet, at, text, text.Bounds(), draw.Over, nil)
}

// shot is one picture of a mesh: where the camera is, and the light.
type shot struct {
	camera charts.Camera
	light  math3d.Vec3 // Toward the light.
}

// worldLight is where the viewer has its light: NTCharts3d's own.
var worldLight = (math3d.Vec3{X: 1, Y: -1, Z: 2}).Normalize()

// gpuSnapshot draws the mesh alone, on white, with NTCharts3d's GPU renderer:
// once for each shot, from one upload of the mesh. It reports false where
// there is no GPU to draw with. It is a variable so that tests can stand in
// for the hardware.
var gpuSnapshot = func(mesh *Mesh, size int, shots []shot) ([]image.Image, bool) {
	chart := charts.New(1, 3, charts.WithAutoRotate(false), charts.WithBackground(color.White))
	defer chart.Close()
	chart.SetAxes(charts.Axes{X: charts.Axis{Hidden: true}, Y: charts.Axis{Hidden: true}, Z: charts.Axis{Hidden: true}})
	chart.SetColorLegendVisible(false)
	chart.SetSeries(mesh)
	var out []image.Image
	for _, s := range shots {
		chart.SetCamera(s.camera)
		chart.SetLight(charts.Light{Direction: s.light, Ambient: .3})
		if chart.Err() != nil {
			return nil, false
		}
		// NTCharts3d falls back to its software renderer. The one below
		// is preferred to that: it was written for exports.
		img, mode, err := chart.Snapshot(size, size)
		if err != nil || mode != charts.WebGPU {
			return nil, false
		}
		out = append(out, img)
	}
	return out, true
}

// The CPU's export uses NTCharts3d geometry, normalization, and camera.
// It rasterizes every face at the requested size with a depth buffer.
func renderMesh(mesh *Mesh, size int, camera charts.Camera, light math3d.Vec3) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	rasterize(mesh, size, camera, light, img)
	return img
}

// meshDepth is the depth of the nearest face at each pixel, as renderMesh
// finds it: from 0, near, to 1, far; 2 where there is none.
func meshDepth(mesh *Mesh, size int, camera charts.Camera) []float32 {
	return rasterize(mesh, size, camera, math3d.Vec3{}, nil)
}

// rasterize draws the faces of the mesh into img, nil for none, and gives the
// depth buffer.
func rasterize(mesh *Mesh, size int, camera charts.Camera, light math3d.Vec3, img *image.RGBA) []float32 {
	g, _ := mesh.Geometry(nil)
	normalize, _, _ := g.Bounds.Normalization()
	matrix := camera.Matrix(1).Mul(normalize)
	depth := make([]float32, size*size)
	for i := range depth {
		depth[i] = 2
	}
	for i := 0; i < len(g.Indices); i += 3 {
		a, b, c := g.Vertices[g.Indices[i]], g.Vertices[g.Indices[i+1]], g.Vertices[g.Indices[i+2]]
		ax, ay, az, _ := matrix.Project(a.Position, size, size)
		bx, by, bz, _ := matrix.Project(b.Position, size, size)
		cx, cy, cz, _ := matrix.Project(c.Position, size, size)
		area := (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
		if area >= -1e-6 {
			continue
		}
		x0, x1 := max(0, int(math.Floor(float64(min(ax, bx, cx))))), min(size-1, int(math.Ceil(float64(max(ax, bx, cx)))))
		y0, y1 := max(0, int(math.Floor(float64(min(ay, by, cy))))), min(size-1, int(math.Ceil(float64(max(ay, by, cy)))))
		var shade color.RGBA
		if img != nil {
			brightness := .3 + .7*max(float32(0), a.Normal.Dot(light))
			shade = color.RGBA{R: uint8(float32(a.Color.R) * brightness), G: uint8(float32(a.Color.G) * brightness), B: uint8(float32(a.Color.B) * brightness), A: 255}
		}
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				px, py := float32(x)+.5, float32(y)+.5
				u := ((bx-px)*(cy-py) - (by-py)*(cx-px)) / area
				v := ((cx-px)*(ay-py) - (cy-py)*(ax-px)) / area
				w := 1 - u - v
				if min(u, v, w) < 0 {
					continue
				}
				z := u*az + v*bz + w*cz
				if z < 0 || z > 1 || z > depth[y*size+x] {
					continue
				}
				depth[y*size+x] = z
				if img != nil {
					img.SetRGBA(x, y, shade)
				}
			}
		}
	}
	return depth
}

// outline darkens the edges of a mesh's picture, in the colors of its faces,
// found in its depth: where
// the mesh meets the background, and where the depth breaks or folds, as at
// the rim of a hole or of a box. A slope, however steep, changes depth
// evenly, and is not an edge.
func outline(src image.Image, depth []float32, size int) image.Image {
	shaded := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(shaded, shaded.Bounds(), src, src.Bounds().Min, draw.Src)
	img := image.NewRGBA(shaded.Bounds())
	copy(img.Pix, shaded.Pix)
	near, far := float32(2), float32(-1)
	for _, z := range depth {
		if z <= 1 {
			near, far = min(near, z), max(far, z)
		}
	}
	if near > far {
		return img
	}
	step := max((far-near)*.02, 1e-5)
	at := func(x, y int) float32 {
		if x < 0 || y < 0 || x >= size || y >= size {
			return 2
		}
		return depth[y*size+x]
	}
	edge := func(x, y int) bool {
		z := at(x, y)
		if z > 1 {
			return false
		}
		l, r, u, d := at(x-1, y), at(x+1, y), at(x, y-1), at(x, y+1)
		if max(l, r, u, d) > 1 {
			return true
		}
		return abs32(l+r-2*z) > step || abs32(u+d-2*z) > step
	}
	width := max(1, size/500)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !edge(x, y) {
				continue
			}
			for dy := 0; dy < width; dy++ {
				for dx := 0; dx < width; dx++ {
					if x+dx < size && y+dy < size && at(x+dx, y+dy) <= 1 {
						c := shaded.RGBAAt(x+dx, y+dy)
						img.SetRGBA(x+dx, y+dy, color.RGBA{R: c.R / 4, G: c.G / 4, B: c.B / 4, A: 255})
					}
				}
			}
		}
	}
	return img
}

func abs32(v float32) float32 { return max(v, -v) }
