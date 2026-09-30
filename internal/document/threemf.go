package document

import (
	"bytes"
	"cmp"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"image"
	"image/color"
	"io"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/NimbleMarkets/ntcharts3d/math3d"
)

const (
	max3MFDepth  = 16 // Components within components.
	rootModel3MF = "3D/3dmodel.model"
)

// Model3MF is what a 3MF package holds: a mesh assembled from the objects its
// build places, and often a picture of the result.
type Model3MF struct {
	Mesh      *Mesh // Nil when the model has more triangles than can be shown.
	Thumbnail image.Image
	Triangles int
	Objects   int
	Unit      string
	metadata  map[string]string
	bounds    math3d.AABB
	area      float64
}

func (m *Model3MF) triangles() int {
	if m == nil {
		return 0
	}
	return m.Triangles
}

// matrix3MF is the 4×3 transform of the format, for row vectors: three rows
// turn and scale, and the fourth moves.
type matrix3MF [12]float64

var identity3MF = matrix3MF{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}

func (m matrix3MF) apply(x, y, z float64) (float64, float64, float64) {
	return x*m[0] + y*m[3] + z*m[6] + m[9], x*m[1] + y*m[4] + z*m[7] + m[10], x*m[2] + y*m[5] + z*m[8] + m[11]
}

// then is the transform that applies m and afterwards next.
func (m matrix3MF) then(next matrix3MF) (out matrix3MF) {
	for row := range 4 {
		x, y, z := m[3*row], m[3*row+1], m[3*row+2]
		for col := range 3 {
			out[3*row+col] = x*next[col] + y*next[3+col] + z*next[6+col]
			if row == 3 {
				out[3*row+col] += next[9+col]
			}
		}
	}
	return out
}

// A mirror turns faces inside out.
func (m matrix3MF) mirrors() bool {
	return m[0]*(m[4]*m[8]-m[5]*m[7])-m[1]*(m[3]*m[8]-m[5]*m[6])+m[2]*(m[3]*m[7]-m[4]*m[6]) < 0
}

type placed3MF struct {
	object, part string // The part is empty for an object of the same one.
	transform    matrix3MF
}

type triangle3MF struct {
	v     [3]uint32
	group string // Overrides the object's colors when set.
	index int
}

type object3MF struct {
	group      string
	index      int
	vertices   [][3]float64
	triangles  []triangle3MF
	components []placed3MF
}

type part3MF struct {
	unit     string
	metadata map[string]string
	objects  map[string]*object3MF
	colors   map[string][]color.RGBA
	build    []placed3MF
}

// package3MF is the package with its model parts, read as they are named.
type package3MF struct {
	*opc
	parts map[string]*part3MF
}

func (p *package3MF) read(name string) ([]byte, error) {
	data, err := p.opc.read(name)
	if err != nil {
		return nil, fmt.Errorf("3MF %w", err)
	}
	return data, nil
}

func (p *package3MF) part(name string) (*part3MF, error) {
	key := strings.ToLower(strings.TrimPrefix(name, "/"))
	if part, ok := p.parts[key]; ok {
		return part, nil
	}
	data, err := p.read(name)
	if err != nil {
		return nil, err
	}
	part, err := parsePart3MF(data)
	if err != nil {
		return nil, fmt.Errorf("3MF %s: %w", path.Base(name), err)
	}
	p.parts[key] = part
	return part, nil
}

// slicer3MF holds what Bambu Studio and its relatives keep beside the model:
// the color of each filament, and which filament each object and part takes.
// The standard has its own place for colors, which these slicers leave empty.
type slicer3MF struct {
	filaments []color.RGBA
	extruders map[string]int // By "object" and "object/part"; filaments count from 1.
}

// color is that of the filament a part is printed in, where that is known.
func (s slicer3MF) color(object, part string) color.RGBA {
	extruder, ok := s.extruders[object+"/"+part]
	if !ok {
		extruder = s.extruders[object]
	}
	if extruder < 1 || extruder > len(s.filaments) {
		return meshColor
	}
	return s.filaments[extruder-1]
}

// slicer reads those settings. They are a courtesy of one family of programs:
// whatever is missing or malformed is passed over.
func (p *package3MF) slicer() slicer3MF {
	s := slicer3MF{extruders: map[string]int{}}
	if data, err := p.read("Metadata/project_settings.config"); err == nil {
		var project struct {
			Colors []any `json:"filament_colour"`
		}
		if json.Unmarshal(data, &project) == nil {
			for _, c := range project.Colors {
				shade, ok := color3MF(fmt.Sprint(c))
				if !ok {
					shade = meshColor
				}
				s.filaments = append(s.filaments, shade)
			}
		}
	}
	data, err := p.read("Metadata/model_settings.config")
	if err != nil {
		return s
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	var object, part string
	for {
		token, err := d.Token()
		if err != nil {
			return s
		}
		switch e := token.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "object":
				object, part = attribute(e, "id"), ""
			case "part":
				part = attribute(e, "id")
			case "metadata":
				extruder, err := strconv.Atoi(attribute(e, "value"))
				if attribute(e, "key") != "extruder" || err != nil || object == "" {
					continue
				}
				if part != "" {
					s.extruders[object+"/"+part] = extruder
				} else {
					s.extruders[object] = extruder
				}
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "object":
				object = ""
			case "part":
				part = ""
			}
		}
	}
}

// relationships names the root model and the thumbnail, where the package
// declares them.
func (p *package3MF) relationships() (model, thumbnail string) {
	data, err := p.read("_rels/.rels")
	if err != nil {
		return "", ""
	}
	var rels struct {
		Relationship []struct {
			Target string `xml:"Target,attr"`
			Type   string `xml:"Type,attr"`
		}
	}
	if xml.Unmarshal(data, &rels) != nil {
		return "", ""
	}
	for _, r := range rels.Relationship {
		switch {
		case strings.HasSuffix(r.Type, "/3dmodel") && model == "":
			model = r.Target
		case strings.HasSuffix(r.Type, "/metadata/thumbnail") && thumbnail == "":
			thumbnail = r.Target
		}
	}
	return model, thumbnail
}

func open3MF(data []byte) (*package3MF, error) {
	p, err := openOPC(data)
	if err != nil {
		return nil, fmt.Errorf("3MF: %w", err)
	}
	return &package3MF{opc: p, parts: map[string]*part3MF{}}, nil
}

func Parse3MF(data []byte) (*Model3MF, error) {
	p, err := open3MF(data)
	if err != nil {
		return nil, err
	}
	root, picture := p.relationships()
	if root == "" || !p.has(root) {
		root = rootModel3MF
	}
	if !p.has(root) {
		return nil, fmt.Errorf("3MF has no model")
	}
	model, err := p.part(root)
	if err != nil {
		return nil, err
	}
	build := model.build
	if len(build) == 0 {
		// Without a build, show what there is.
		for id := range model.objects {
			build = append(build, placed3MF{object: id, transform: identity3MF})
		}
	}
	out := &Model3MF{Unit: model.unit, metadata: model.metadata}
	objects := map[*object3MF]bool{}
	settings := p.slicer()
	// emit is given each object the build places, where it is placed, and
	// the color for faces that name none.
	type emit func(*part3MF, *object3MF, matrix3MF, color.RGBA) error
	var walk func(at placed3MF, from, top string, transform matrix3MF, depth int, emit emit) error
	walk = func(at placed3MF, from, top string, transform matrix3MF, depth int, emit emit) error {
		if depth > max3MFDepth {
			return fmt.Errorf("3MF objects nest too deeply, or within themselves")
		}
		if at.part != "" {
			from = at.part
		}
		part, err := p.part(from)
		if err != nil {
			return err
		}
		object := part.objects[at.object]
		if object == nil {
			return fmt.Errorf("3MF places object %s, which it does not define", at.object)
		}
		transform = at.transform.then(transform)
		if err := emit(part, object, transform, settings.color(top, at.object)); err != nil {
			return err
		}
		for _, c := range object.components {
			if err := walk(c, from, top, transform, depth+1, emit); err != nil {
				return err
			}
		}
		return nil
	}
	each := func(emit emit) error {
		for _, item := range build {
			if err := walk(item, root, item.object, identity3MF, 0, emit); err != nil {
				return err
			}
		}
		return nil
	}
	// Count before building: a model over the limit is measured, not stored.
	err = each(func(_ *part3MF, o *object3MF, _ matrix3MF, _ color.RGBA) error {
		// An object that only holds others is not counted among them.
		if len(o.triangles) > 0 {
			objects[o] = true
		}
		if out.Triangles += len(o.triangles); out.Triangles > 64<<20 {
			return fmt.Errorf("3MF has too many triangles to count")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out.Objects = len(objects); out.Triangles == 0 {
		return nil, fmt.Errorf("3MF contains no triangles")
	}
	mesh := &Mesh{}
	if out.Triangles > maxTriangles {
		mesh = nil
	}
	err = each(func(part *part3MF, o *object3MF, transform matrix3MF, plain color.RGBA) error {
		mirrored := transform.mirrors()
		for _, t := range o.triangles {
			var v [3]math3d.Vec3
			for i, index := range t.v {
				x, y, z := transform.apply(o.vertices[index][0], o.vertices[index][1], o.vertices[index][2])
				v[i] = math3d.Vec3{X: float32(x), Y: float32(y), Z: float32(z)}
			}
			if mirrored {
				v[1], v[2] = v[2], v[1]
			}
			if mesh != nil {
				shade := plain
				group, index := o.group, o.index
				if t.group != "" {
					group, index = t.group, t.index
				}
				if colors := part.colors[group]; index >= 0 && index < len(colors) {
					shade = colors[index]
				}
				if err := mesh.add(v, shade); err != nil {
					return fmt.Errorf("3MF vertex: %w", err)
				}
				continue
			}
			for _, corner := range v {
				out.bounds.Include(corner)
			}
			out.area += triangleArea(v)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out.Mesh = mesh; mesh != nil {
		out.bounds, out.area = mesh.geometry.Bounds, mesh.area()
	}
	// A slicer's rendering of the plate stands in for the model better than
	// the picture the package declares, which may be a photograph of a print.
	for _, name := range []string{"Metadata/plate_1.png", picture, "Metadata/thumbnail.png", "Auxiliaries/.thumbnails/thumbnail_3mf.png"} {
		if name == "" || !p.has(name) {
			continue
		}
		if data, err := p.read(name); err == nil {
			if out.Thumbnail, err = decodeRaster(data); err == nil {
				break
			}
			out.Thumbnail = nil
		}
	}
	return out, nil
}

func attribute(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func transform3MF(s string) (matrix3MF, error) {
	if s == "" {
		return identity3MF, nil
	}
	fields := strings.Fields(s)
	if len(fields) != 12 {
		return matrix3MF{}, fmt.Errorf("transform must have 12 numbers")
	}
	var m matrix3MF
	for i, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return matrix3MF{}, fmt.Errorf("transform is not a number: %q", f)
		}
		m[i] = v
	}
	return m, nil
}

func color3MF(s string) (color.RGBA, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 && len(s) != 8 {
		return color.RGBA{}, false
	}
	v, err := strconv.ParseUint(s[:6], 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	// Transparency is not drawn.
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, true
}

func place3MF(e xml.StartElement) (placed3MF, error) {
	transform, err := transform3MF(attribute(e, "transform"))
	return placed3MF{object: attribute(e, "objectid"), part: attribute(e, "path"), transform: transform}, err
}

// parsePart3MF reads a model part. It walks the XML as a stream: a mesh is
// millions of small elements.
func parsePart3MF(data []byte) (*part3MF, error) {
	part := &part3MF{unit: "millimeter", metadata: map[string]string{}, objects: map[string]*object3MF{}, colors: map[string][]color.RGBA{}}
	d := xml.NewDecoder(bytes.NewReader(data))
	var object *object3MF
	var group, name string
	var text *strings.Builder
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch e := token.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "model":
				if unit := attribute(e, "unit"); unit != "" {
					part.unit = unit
				}
			case "metadata":
				name, text = attribute(e, "name"), &strings.Builder{}
			case "basematerials", "colorgroup":
				group = attribute(e, "id")
			case "base", "color":
				shade, ok := color3MF(attribute(e, "displaycolor") + attribute(e, "color"))
				if !ok {
					shade = meshColor
				}
				part.colors[group] = append(part.colors[group], shade)
			case "object":
				object = &object3MF{group: attribute(e, "pid"), index: -1}
				if index, err := strconv.Atoi(attribute(e, "pindex")); err == nil {
					object.index = index
				}
				part.objects[attribute(e, "id")] = object
			case "vertex":
				if object == nil {
					continue
				}
				var v [3]float64
				for i, axis := range []string{"x", "y", "z"} {
					// Coordinates are kept to the precision they are drawn with.
					if v[i], err = strconv.ParseFloat(attribute(e, axis), 32); err != nil {
						return nil, fmt.Errorf("vertex %s=%q is not a usable number", axis, attribute(e, axis))
					}
				}
				object.vertices = append(object.vertices, v)
			case "triangle":
				if object == nil {
					continue
				}
				t := triangle3MF{group: attribute(e, "pid"), index: -1}
				for i, corner := range []string{"v1", "v2", "v3"} {
					index, err := strconv.ParseUint(attribute(e, corner), 10, 32)
					if err != nil || index >= uint64(len(object.vertices)) {
						return nil, fmt.Errorf("triangle names vertex %q, which does not exist", attribute(e, corner))
					}
					t.v[i] = uint32(index)
				}
				if index, err := strconv.Atoi(attribute(e, "p1")); err == nil {
					t.index = index
				}
				object.triangles = append(object.triangles, t)
			case "component":
				if object == nil {
					continue
				}
				c, err := place3MF(e)
				if err != nil {
					return nil, err
				}
				object.components = append(object.components, c)
			case "item":
				item, err := place3MF(e)
				if err != nil {
					return nil, err
				}
				part.build = append(part.build, item)
			}
		case xml.CharData:
			if text != nil {
				text.Write(e)
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "metadata":
				if text != nil {
					part.metadata[name] = text.String()
				}
				text = nil
			case "object":
				object = nil
			}
		}
	}
	return part, nil
}

var markup = regexp.MustCompile(`<[^>]*>`)

// fields describes the model. shown says what is on screen when it is not
// the mesh.
func (m *Model3MF) fields(shown string) []Field {
	number := func(v float64) string { return strconv.FormatFloat(v, 'g', 6, 64) }
	unit := map[string]string{"micron": "µm", "millimeter": "mm", "centimeter": "cm", "inch": "in", "foot": "ft", "meter": "m"}[m.Unit]
	meta := func(name string) string {
		value := m.metadata[name]
		// Sites that host models put HTML, escaped twice over, in descriptions.
		value = markup.ReplaceAllString(html.UnescapeString(html.UnescapeString(value)), " ")
		value = strings.Join(strings.Fields(strings.ReplaceAll(value, " ", " ")), " ")
		if value == "[]" {
			return ""
		}
		return strings.NewReplacer(" .", ".", " ,", ",").Replace(value)
	}
	extent := m.bounds.Max.Sub(m.bounds.Min)
	thumbnail := ""
	if m.Thumbnail != nil {
		thumbnail = fmt.Sprintf("%d × %d", m.Thumbnail.Bounds().Dx(), m.Thumbnail.Bounds().Dy())
	}
	area := number(m.area)
	if unit != "" {
		area += " " + unit + "²"
	}
	return Section("Model", Field{"Format", "3MF"}, Field{"Title", meta("Title")}, Field{"Designer", meta("Designer")}, Field{"Description", meta("Description")},
		Field{"Application", meta("Application")}, Field{"Created", meta("CreationDate")}, Field{"Changed", meta("ModificationDate")},
		Field{"License", cmp.Or(meta("LicenseTerms"), meta("License"))}, Field{"Copyright", meta("Copyright")},
		Field{"Objects", grouped(m.Objects)}, Field{"Triangles", grouped(m.Triangles)},
		Field{"Extent", strings.TrimSpace(number(float64(extent.X)) + " × " + number(float64(extent.Y)) + " × " + number(float64(extent.Z)) + " " + unit)},
		Field{"Surface area", area}, Field{"Thumbnail", thumbnail}, Field{"Shown", shown})
}
