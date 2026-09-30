package document

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/NimbleMarkets/ntcharts3d/math3d"
)

const maxViews = 16 // In one picture.

// View is a way of looking at a mesh: from where, and how projected. X runs
// to the right, Y away from the viewer at the front, and Z up.
type View struct {
	Name        string  // As asked for; it labels the view among others.
	Alpha, Beta float64 // Elevation and azimuth of the camera, in degrees.
	Distance    float64 // From the mesh, in NTCharts3d's units; zero fits the mesh to the picture.
	Projection  charts.Projection
}

// The azimuth counts from the X axis, so the front, on the near side of Y,
// is at -90. NTCharts3d keeps the camera a degree short of straight up and
// down, where which way is up is decided by the azimuth.
var namedViews = map[string][2]float64{
	"front": {0, -90}, "back": {0, 90}, "right": {0, 0}, "left": {0, 180},
	"top": {89, -90}, "bottom": {-89, -90},
	// From the front, the right, and above, the three axes equally foreshortened.
	"iso": {math.Asin(1/math.Sqrt(3)) * 180 / math.Pi, -45},
}

var allViews = []string{"front", "right", "back", "left", "top", "bottom"}

// ParseViews reads a list of named views, as "front,top,iso". The name all
// stands for the six sides.
func ParseViews(s string) ([]View, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var views []View
	for _, name := range strings.Split(s, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		names := []string{name}
		if name == "all" {
			names = allViews
		}
		for _, name := range names {
			at, ok := namedViews[name]
			if !ok {
				return nil, fmt.Errorf("unknown view %q; expected front, back, left, right, top, bottom, iso, all", name)
			}
			views = append(views, View{Name: name, Alpha: at[0], Beta: at[1]})
		}
	}
	if len(views) > maxViews {
		return nil, fmt.Errorf("at most %d views fit one picture", maxViews)
	}
	return views, nil
}

// ParseCamera reads "elevation,azimuth" or "elevation,azimuth,distance".
// Without a distance, the mesh is fitted to the picture.
func ParseCamera(s string) (View, error) {
	fields := strings.Split(s, ",")
	if len(fields) != 2 && len(fields) != 3 {
		return View{}, fmt.Errorf("camera must be elevation,azimuth or elevation,azimuth,distance")
	}
	var v [3]float64
	for i, field := range fields {
		n, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return View{}, fmt.Errorf("camera: %q is not a number", strings.TrimSpace(field))
		}
		v[i] = n
		fields[i] = strings.TrimSpace(field)
	}
	switch {
	case v[0] < -90 || v[0] > 90:
		return View{}, fmt.Errorf("camera: elevation must be -90..90 degrees")
	case len(fields) == 3 && (v[2] < .15 || v[2] > 100):
		return View{}, fmt.Errorf("camera: distance must be 0.15..100")
	}
	return View{Name: strings.Join(fields, ","), Alpha: v[0], Beta: v[1], Distance: v[2]}, nil
}

// basis is the camera's frame: unit vectors toward it, to its right, and up.
func (v View) basis() (toward, right, up [3]float64) {
	a, b := math.Max(-89, math.Min(89, v.Alpha))*math.Pi/180, v.Beta*math.Pi/180
	return [3]float64{math.Cos(a) * math.Cos(b), math.Cos(a) * math.Sin(b), math.Sin(a)},
		[3]float64{-math.Sin(b), math.Cos(b), 0},
		[3]float64{-math.Sin(a) * math.Cos(b), -math.Sin(a) * math.Sin(b), math.Cos(a)}
}

// light comes from over the viewer's left shoulder, so that every view
// asked for is lit alike. A light fixed in the world, as the viewer has,
// leaves the back, the left, and the underside of a mesh in shadow.
func (v View) light() math3d.Vec3 {
	toward, right, up := v.basis()
	var l [3]float64
	for i := range l {
		l[i] = toward[i] + .5*up[i] - .35*right[i]
	}
	return math3d.Vec3{X: float32(l[0]), Y: float32(l[1]), Z: float32(l[2])}.Normalize()
}

// Camera places NTCharts3d's camera for the view of a square picture, as
// an export is. Where no distance was asked for, the mesh fills nine tenths
// of it; without a mesh, the distance is NTCharts3d's own.
func (v View) Camera(mesh *Mesh) charts.Camera { return v.CameraFor(mesh, 1) }

// CameraFor is Camera for a picture of the given aspect, its width over
// its height in pixels: in a wide frame a wide mesh is fitted by its
// height, and the camera comes as close as that allows. An aspect that
// says nothing means a square.
func (v View) CameraFor(mesh *Mesh, aspect float64) charts.Camera {
	if aspect <= 0 || math.IsNaN(aspect) || math.IsInf(aspect, 0) {
		aspect = 1
	}
	camera := charts.DefaultCamera()
	camera.Alpha, camera.Beta, camera.Projection, camera.AutoRotate = math.Max(-89, math.Min(89, v.Alpha)), v.Beta, v.Projection, false
	if v.Distance > 0 {
		camera.Distance = v.Distance
		return camera
	}
	if mesh == nil || !mesh.geometry.Bounds.IsValid() {
		return camera
	}
	// NTCharts3d centers the mesh at the origin and scales its longest side
	// to two units. Its corners, seen from the camera, say how much room
	// the mesh takes across the picture, up it, and toward the camera.
	size := mesh.geometry.Bounds.Max.Sub(mesh.geometry.Bounds.Min)
	longest := float64(max(size.X, size.Y, size.Z))
	if longest <= 0 {
		return camera
	}
	half := [3]float64{float64(size.X) / longest, float64(size.Y) / longest, float64(size.Z) / longest}
	toward, right, up := v.basis()
	reach := func(axis [3]float64) float64 {
		return math.Abs(axis[0])*half[0] + math.Abs(axis[1])*half[1] + math.Abs(axis[2])*half[2]
	}
	// The picture is as tall as the fit says, and aspect times as wide.
	across, near := math.Max(reach(right)/aspect, reach(up)), reach(toward)
	const fill = .9
	if v.Projection == charts.Perspective {
		// The field of view is 45 degrees, and what is nearest looks largest.
		camera.Distance = near + across/fill/math.Tan(math.Pi/8)
	} else {
		// The picture is as tall as the camera is far: in NTCharts3d's
		// orthographic projection, distance is also scale. The camera must
		// still stand clear of the mesh, so one that is long toward the
		// camera, a plank seen from its end, is drawn smaller than it could be.
		camera.Distance = math.Max(2*across/fill, near+.05)
	}
	camera.Distance = math.Max(.15, math.Min(100, camera.Distance))
	return camera
}
