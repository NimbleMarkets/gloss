package document

import (
	"image"
	"math"
	"strings"
	"testing"

	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/NimbleMarkets/ntcharts3d/math3d"
)

// box is a mesh filling a block of the given size, with a corner at the origin.
func box(t *testing.T, x, y, z float32) *Mesh {
	t.Helper()
	m := &Mesh{}
	c := func(i int) math3d.Vec3 {
		return math3d.Vec3{X: x * float32(i&1), Y: y * float32(i>>1&1), Z: z * float32(i>>2&1)}
	}
	// Each face as two triangles, wound to look outward.
	for _, f := range [][4]int{{0, 2, 3, 1}, {4, 5, 7, 6}, {0, 1, 5, 4}, {2, 6, 7, 3}, {0, 4, 6, 2}, {1, 3, 7, 5}} {
		for _, tri := range [][3]int{{f[0], f[1], f[2]}, {f[0], f[2], f[3]}} {
			if err := m.add([3]math3d.Vec3{c(tri[0]), c(tri[1]), c(tri[2])}, meshColor); err != nil {
				t.Fatal(err)
			}
		}
	}
	return m
}

func TestNamedViews(t *testing.T) {
	// Where a point one unit along each axis lands in the picture, as seen
	// from each side: right of center (+), left (-), or at it (0), and
	// likewise above and below.
	for name, want := range map[string][3][2]int{
		"front":  {{+1, 0}, {0, 0}, {0, +1}}, // X to the right, Z up, looking along Y
		"back":   {{-1, 0}, {0, 0}, {0, +1}},
		"right":  {{0, 0}, {+1, 0}, {0, +1}}, // Y to the right
		"left":   {{0, 0}, {-1, 0}, {0, +1}},
		"top":    {{+1, 0}, {0, +1}, {0, 0}}, // X to the right, Y up, looking down
		"bottom": {{+1, 0}, {0, -1}, {0, 0}},
		"iso":    {{+1, -1}, {+1, +1}, {0, +1}}, // From the front, the right, and above
	} {
		views, err := ParseViews(name)
		if err != nil || len(views) != 1 || views[0].Name != name {
			t.Fatalf("%s: %+v %v", name, views, err)
		}
		camera := views[0].Camera(nil)
		if camera.Projection != charts.Orthographic || camera.AutoRotate {
			t.Errorf("%s: %+v", name, camera)
		}
		matrix := camera.Matrix(1)
		ox, oy, _, _ := matrix.Project(math3d.Vec3{}, 1000, 1000)
		for axis, point := range []math3d.Vec3{{X: 1}, {Y: 1}, {Z: 1}} {
			x, y, _, _ := matrix.Project(point, 1000, 1000)
			side := func(d float32) int {
				switch {
				case d > 8:
					return +1
				case d < -8:
					return -1
				}
				return 0
			}
			if got := [2]int{side(x - ox), side(oy - y)}; got != want[axis] {
				t.Errorf("%s: axis %d lands %v of center, want %v", name, axis, got, want[axis])
			}
		}
	}
}

func TestParseViews(t *testing.T) {
	names := func(views []View) string {
		var out []string
		for _, v := range views {
			out = append(out, v.Name)
		}
		return strings.Join(out, " ")
	}
	for in, want := range map[string]string{
		"": "", "front": "front", "Front, TOP ,iso": "front top iso", "all": "front right back left top bottom",
		"iso,all": "iso front right back left top bottom", "top,top": "top top",
	} {
		views, err := ParseViews(in)
		if err != nil || names(views) != want {
			t.Errorf("ParseViews(%q) = %q, %v; want %q", in, names(views), err, want)
		}
	}
	for _, in := range []string{"sideways", "front,,top", ",", "front;top", strings.Repeat("front,", 16) + "front"} {
		if views, err := ParseViews(in); err == nil {
			t.Errorf("ParseViews(%q) = %q", in, names(views))
		} else if in == "sideways" && !strings.Contains(err.Error(), "front, back, left, right, top, bottom, iso, all") {
			t.Errorf("the error does not say what may be asked for: %v", err)
		}
	}
}

func TestParseCamera(t *testing.T) {
	for in, want := range map[string]View{
		"25,40":         {Name: "25,40", Alpha: 25, Beta: 40},
		" -30 , 400 ":   {Name: "-30,400", Alpha: -30, Beta: 400},
		"90,0,2.5":      {Name: "90,0,2.5", Alpha: 90, Beta: 0, Distance: 2.5},
		"12.5,-45,0.15": {Name: "12.5,-45,0.15", Alpha: 12.5, Beta: -45, Distance: .15},
	} {
		if got, err := ParseCamera(in); err != nil || got != want {
			t.Errorf("ParseCamera(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "25", "25,40,3,4", "a,b", "25,", "91,0", "-91,0", "0,NaN", "0,0,0", "0,0,-1", "0,0,Inf", "0,0,101", "0,1e999"} {
		if got, err := ParseCamera(in); err == nil {
			t.Errorf("ParseCamera(%q) = %+v", in, got)
		}
	}
}

// spread measures how much of a picture's width and height the mesh covers,
// and whether it touches the edge.
func spread(img image.Image) (wide, high float64, clipped bool) {
	b := img.Bounds()
	x0, y0, x1, y1 := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, g, bl, _ := img.At(x, y).RGBA(); r != 0xffff || g != 0xffff || bl != 0xffff {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
			}
		}
	}
	if x1 < x0 {
		return 0, 0, false
	}
	return float64(x1-x0+1) / float64(b.Dx()), float64(y1-y0+1) / float64(b.Dy()), x0 == b.Min.X || y0 == b.Min.Y || x1 == b.Max.X-1 || y1 == b.Max.Y-1
}

func TestViewsFitTheMeshToThePicture(t *testing.T) {
	for _, tt := range []struct {
		name       string
		mesh       *Mesh
		view       string
		wide, high float64 // Of the picture, as the mesh is seen.
	}{
		{"a plank from the front", box(t, 10, 1, 2), "front", .9, .18},
		// The camera stands clear of the plank's near end, a unit from its
		// middle, and the picture is as tall as the camera is far.
		{"a plank from its end", box(t, 10, 1, 2), "right", .2 / 1.05, .4 / 1.05},
		{"a plank from above", box(t, 10, 1, 2), "top", .9, .09},
		{"a tower from the front", box(t, 1, 1, 8), "front", .1125, .9},
		{"a cube from the front", box(t, 3, 3, 3), "front", .9, .9},
	} {
		views, _ := ParseViews(tt.view)
		for _, projection := range []charts.Projection{charts.Orthographic, charts.Perspective} {
			view := views[0]
			view.Projection = projection
			img := renderMesh(tt.mesh, 400, view.Camera(tt.mesh), worldLight)
			wide, high, clipped := spread(img)
			if clipped {
				t.Errorf("%s, %v: the mesh runs off the picture", tt.name, projection)
			}
			if projection == charts.Orthographic && (math.Abs(wide-tt.wide) > .03 || math.Abs(high-tt.high) > .03) {
				t.Errorf("%s: covers %.2f by %.2f of the picture, want %.2f by %.2f", tt.name, wide, high, tt.wide, tt.high)
			}
			// Perspective enlarges what is near; the fit allows for it.
			if projection == charts.Perspective && (max(wide, high) < .6 || max(wide, high) > .97) {
				t.Errorf("%s in perspective: covers %.2f by %.2f", tt.name, wide, high)
			}
		}
	}
	// Seen from a corner, a cube is wider than any of its faces.
	for _, mesh := range []*Mesh{box(t, 3, 3, 3), box(t, 10, 1, 2), box(t, 1, 1, 8)} {
		views, _ := ParseViews("iso")
		wide, high, clipped := spread(renderMesh(mesh, 400, views[0].Camera(mesh), worldLight))
		if clipped || max(wide, high) < .8 || max(wide, high) > .95 {
			t.Errorf("iso: covers %.2f by %.2f, clipped=%v", wide, high, clipped)
		}
	}
	// A distance that was asked for is the one used.
	view, _ := ParseCamera("0,-90,6")
	if got := view.Camera(box(t, 3, 3, 3)).Distance; got != 6 {
		t.Errorf("distance %v, want the 6 asked for", got)
	}
	if wide, _, _ := spread(renderMesh(box(t, 3, 3, 3), 400, view.Camera(box(t, 3, 3, 3)), worldLight)); math.Abs(wide-2.0/6) > .03 {
		t.Errorf("at distance 6 a cube covers %.2f of the picture", wide)
	}
}
