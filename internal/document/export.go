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
)

// ExportImage produces an opaque image bounded by maxEdge on both axes.
// Raster sources are never enlarged; vector documents render at the target.
func ExportImage(r Result, maxEdge int) (image.Image, error) {
	return ExportForVision(r, maxEdge, "")
}

func ExportForVision(r Result, maxEdge int, profile string) (image.Image, error) {
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
		w, _, err := VisionSize(maxEdge, maxEdge, maxEdge, profile)
		if err != nil {
			return nil, err
		}
		camera := charts.DefaultCamera()
		if r.Camera != nil {
			camera = *r.Camera
		}
		return renderMesh(r.Mesh, w, camera), nil
	}
	if r.Image == nil || r.Image.Bounds().Empty() {
		return nil, fmt.Errorf("document has no image")
	}
	b := r.Image.Bounds()
	w, h, err := VisionSize(b.Dx(), b.Dy(), maxEdge, profile)
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(out, out.Bounds(), r.Image, b, draw.Over, nil)
	return out, nil
}

// Headless STL export uses NTCharts3d geometry, normalization, and camera.
// Its interactive software backend caps resolution and samples large meshes;
// here we rasterize every face at the requested size with a depth buffer.
func renderMesh(mesh *Mesh, size int, camera charts.Camera) image.Image {
	g, _ := mesh.Geometry(nil)
	normalize, _, _ := g.Bounds.Normalization()
	matrix := camera.Matrix(1).Mul(normalize)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	depth := make([]float32, size*size)
	for i := range depth {
		depth[i] = 2
	}
	light := (math3d.Vec3{X: 1, Y: -1, Z: 2}).Normalize()
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
		brightness := .3 + .7*max(float32(0), a.Normal.Dot(light))
		shade := color.RGBA{R: uint8(float32(a.Color.R) * brightness), G: uint8(float32(a.Color.G) * brightness), B: uint8(float32(a.Color.B) * brightness), A: 255}
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
				img.SetRGBA(x, y, shade)
			}
		}
	}
	return img
}
