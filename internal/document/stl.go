package document

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"image/color"
	"io"
	"math"
	"strings"

	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/NimbleMarkets/ntcharts3d/math3d"
)

// MaxTriangles is the most faces a mesh may have: as many as NTCharts3d
// draws, which is as many as any GPU is sure to hold. Each face has vertices
// of its own, so that its flat normal is preserved.
const MaxTriangles = charts.MaxMeshTriangles

// maxTriangles is the limit in force. Tests lower it.
var maxTriangles = MaxTriangles

type Mesh struct{ geometry charts.Geometry }

func (m *Mesh) Name() string                                     { return "STL" }
func (m *Mesh) Geometry(charts.Palette) (charts.Geometry, error) { return m.geometry, nil }
func (m *Mesh) Triangles() int                                   { return len(m.geometry.Indices) / 3 }

// meshColor is given to faces whose file names none.
var meshColor = color.RGBA{R: 100, G: 180, B: 230, A: 255}

func triangleArea(v [3]math3d.Vec3) float64 {
	n := v[1].Sub(v[0]).Cross(v[2].Sub(v[0]))
	return math.Sqrt(float64(n.X)*float64(n.X)+float64(n.Y)*float64(n.Y)+float64(n.Z)*float64(n.Z)) / 2
}

func (m *Mesh) area() float64 {
	total, g := 0.0, m.geometry
	for i := 0; i+2 < len(g.Indices); i += 3 {
		total += triangleArea([3]math3d.Vec3{g.Vertices[g.Indices[i]].Position, g.Vertices[g.Indices[i+1]].Position, g.Vertices[g.Indices[i+2]].Position})
	}
	return total
}

func ParseSTL(data []byte) (*Mesh, error) {
	m := &Mesh{}
	// Binary headers may begin with "solid". The exact record length is
	// the reliable discriminator, and is checked before ASCII detection.
	if len(data) >= 84 {
		n := uint64(binary.LittleEndian.Uint32(data[80:84]))
		if 84+50*n == uint64(len(data)) {
			if n == 0 || n > uint64(maxTriangles) {
				return nil, fmt.Errorf("STL must have 1 to %s triangles", grouped(maxTriangles))
			}
			for off := 84; off < len(data); off += 50 {
				var v [3]math3d.Vec3
				for j := range v {
					p := data[off+12+j*12:]
					v[j] = math3d.Vec3{X: math.Float32frombits(binary.LittleEndian.Uint32(p)), Y: math.Float32frombits(binary.LittleEndian.Uint32(p[4:])), Z: math.Float32frombits(binary.LittleEndian.Uint32(p[8:]))}
				}
				if err := m.add(v, meshColor); err != nil {
					return nil, err
				}
			}
			return m, nil
		}
	}
	r := bufio.NewReader(bytes.NewReader(data))
	var word string
	if _, err := fmt.Fscan(r, &word); err != nil || word != "solid" {
		return nil, fmt.Errorf("invalid STL header or truncated binary STL")
	}
	if _, err := r.ReadString('\n'); err != nil {
		return nil, fmt.Errorf("incomplete ASCII STL")
	}
	expect := func(want string) error {
		var got string
		if _, err := fmt.Fscan(r, &got); err != nil {
			return fmt.Errorf("STL: expected %s: %w", want, err)
		}
		if got != want {
			return fmt.Errorf("STL: expected %s, got %q", want, got)
		}
		return nil
	}
	for {
		if _, err := fmt.Fscan(r, &word); err != nil {
			return nil, fmt.Errorf("STL: missing endsolid: %w", err)
		}
		if word == "endsolid" {
			_, _ = r.ReadString('\n')
			tail, err := io.ReadAll(r)
			if err != nil || strings.TrimSpace(string(tail)) != "" {
				return nil, fmt.Errorf("STL: unexpected data after endsolid")
			}
			break
		}
		if word != "facet" {
			return nil, fmt.Errorf("STL: expected facet, got %q", word)
		}
		if err := expect("normal"); err != nil {
			return nil, err
		}
		var normal math3d.Vec3
		if _, err := fmt.Fscan(r, &normal.X, &normal.Y, &normal.Z); err != nil {
			return nil, fmt.Errorf("STL normal: %w", err)
		}
		if err := expect("outer"); err != nil {
			return nil, err
		}
		if err := expect("loop"); err != nil {
			return nil, err
		}
		var v [3]math3d.Vec3
		for i := range v {
			if err := expect("vertex"); err != nil {
				return nil, err
			}
			if _, err := fmt.Fscan(r, &v[i].X, &v[i].Y, &v[i].Z); err != nil {
				return nil, fmt.Errorf("STL vertex: %w", err)
			}
		}
		if err := expect("endloop"); err != nil {
			return nil, err
		}
		if err := expect("endfacet"); err != nil {
			return nil, err
		}
		if err := m.add(v, meshColor); err != nil {
			return nil, err
		}
	}
	if m.Triangles() == 0 {
		return nil, fmt.Errorf("STL contains no triangles")
	}
	return m, nil
}

func (m *Mesh) add(v [3]math3d.Vec3, shade color.RGBA) error {
	if m.Triangles() >= maxTriangles {
		return fmt.Errorf("mesh exceeds %s triangles", grouped(maxTriangles))
	}
	for _, p := range v {
		for _, x := range []float32{p.X, p.Y, p.Z} {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) || math.Abs(float64(x)) > 1e15 {
				return fmt.Errorf("STL vertex is non-finite or exceeds coordinate limits")
			}
		}
	}
	// Recompute normals: many exporters leave zero or stale facet normals.
	n := v[1].Sub(v[0]).Cross(v[2].Sub(v[0])).Normalize()
	for _, p := range v {
		m.geometry.Indices = append(m.geometry.Indices, uint32(len(m.geometry.Vertices)))
		m.geometry.Vertices = append(m.geometry.Vertices, charts.Vertex{Position: p, Normal: n, Color: shade})
		m.geometry.Bounds.Include(p)
	}
	return nil
}
