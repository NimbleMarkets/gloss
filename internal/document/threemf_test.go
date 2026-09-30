package document

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

const coreNS = `xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02"`

const rels3MF = `<?xml version="1.0"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
 <Relationship Target="/3D/3dmodel.model" Id="rel-1" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel"/>
 <Relationship Target="/Metadata/thumbnail.png" Id="rel-2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/thumbnail"/>
</Relationships>`

// A tetrahedron with corners at the origin and one unit along each axis,
// wound so that its faces look outward.
const tetrahedron = `<mesh><vertices>
 <vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/><vertex x="0" y="0" z="1"/>
</vertices><triangles>
 <triangle v1="0" v2="2" v3="1"/><triangle v1="0" v2="1" v3="3"/><triangle v1="0" v2="3" v3="2"/><triangle v1="1" v2="2" v3="3"/>
</triangles></mesh>`

func model3MF(attrs, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><model ` + coreNS + ` ` + attrs + `>` + body + `</model>`
}

// archive3MF packs parts into a 3MF package, adding the relationships that
// name the root model unless the parts supply their own.
func archive3MF(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	if _, ok := parts["_rels/.rels"]; !ok {
		parts["_rels/.rels"] = rels3MF
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range []string{"[Content_Types].xml"} {
		f, _ := w.Create(name)
		fmt.Fprint(f, `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`)
	}
	for name, content := range parts {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(f, content)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func thumbnail(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func simple3MF(t *testing.T) []byte {
	return archive3MF(t, map[string]string{"3D/3dmodel.model": model3MF(`unit="millimeter"`,
		`<resources><object id="1" type="model">`+tetrahedron+`</object></resources><build><item objectid="1"/></build>`)})
}

func TestParse3MF(t *testing.T) {
	m, err := Parse3MF(simple3MF(t))
	if err != nil {
		t.Fatal(err)
	}
	if m.Mesh == nil || m.Mesh.Triangles() != 4 || m.Triangles != 4 || m.Objects != 1 || m.Unit != "millimeter" {
		t.Fatalf("%+v", m)
	}
	g, _ := m.Mesh.Geometry(nil)
	if g.Bounds.Min.X != 0 || g.Bounds.Max.X != 1 || g.Bounds.Max.Y != 1 || g.Bounds.Max.Z != 1 {
		t.Fatalf("bounds %+v", g.Bounds)
	}
	// The face opposite the origin looks away from it.
	if n := g.Vertices[9].Normal; n.X <= 0 || n.Y <= 0 || n.Z <= 0 {
		t.Fatalf("normal %+v", n)
	}
}

func TestDetect3MF(t *testing.T) {
	data := simple3MF(t)
	for _, name := range []string{"part.3mf", "part", "part.zip"} {
		path := write(t, name, data)
		if kind, err := Probe(path, ""); err != nil || kind != "3mf" {
			t.Errorf("Probe(%s) = %q, %v", name, kind, err)
		}
		if kind, err := Detect(path, data, ""); err != nil || kind != "3mf" {
			t.Errorf("Detect(%s) = %q, %v", name, kind, err)
		}
	}
	// Other packages share the container but not the contents.
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("word/document.xml")
	fmt.Fprint(f, "<w:document/>")
	w.Close()
	if kind, err := Probe(write(t, "letter.docx", b.Bytes()), ""); err == nil {
		t.Errorf("a Word document was taken for %q", kind)
	}
	fields := loadInfo(t, write(t, "part.3mf", data))
	expect(t, fields, map[string]string{"Format": "3MF", "Objects": "1", "Triangles": "4", "Extent": "1 × 1 × 1 mm"})
}

func TestPlacement3MF(t *testing.T) {
	// Object 2 holds object 1 twice: as it is, and moved 10 along x and
	// mirrored in z. The build then lifts the pair by 5.
	m, err := Parse3MF(archive3MF(t, map[string]string{"3D/3dmodel.model": model3MF(`unit="inch"`,
		`<resources><object id="1">`+tetrahedron+`</object>
		 <object id="2"><components>
		   <component objectid="1"/>
		   <component objectid="1" transform="1 0 0 0 1 0 0 0 -1 10 0 0"/>
		 </components></object></resources>
		 <build><item objectid="2" transform="1 0 0 0 1 0 0 0 1 0 0 5"/></build>`)}))
	if err != nil {
		t.Fatal(err)
	}
	g, _ := m.Mesh.Geometry(nil)
	if m.Triangles != 8 || m.Objects != 1 || g.Bounds.Min.X != 0 || g.Bounds.Max.X != 11 || g.Bounds.Min.Z != 4 || g.Bounds.Max.Z != 6 {
		t.Fatalf("triangles=%d objects=%d bounds=%+v", m.Triangles, m.Objects, g.Bounds)
	}
	// A mirror turns each face over; its winding is turned back, so the
	// face opposite the origin still looks outward, now downward in z.
	if n := g.Vertices[12+9].Normal; n.X <= 0 || n.Y <= 0 || n.Z >= 0 {
		t.Fatalf("mirrored normal %+v", n)
	}
	expect(t, m.fields(""), map[string]string{"Extent": "11 × 1 × 2 in", "Triangles": "8"})
}

func TestProduction3MF(t *testing.T) {
	// Slicers keep each object in a part of its own.
	const production = `xmlns:p="http://schemas.microsoft.com/3dmanufacturing/production/2015/06" requiredextensions="p"`
	m, err := Parse3MF(archive3MF(t, map[string]string{
		"3D/3dmodel.model": model3MF(production, `<resources><object id="7"><components>
			<component p:path="/3D/Objects/object_1.model" objectid="1" transform="1 0 0 0 1 0 0 0 1 3 0 0"/>
		</components></object></resources><build><item objectid="7"/></build>`),
		"3D/Objects/object_1.model": model3MF(production, `<resources><object id="1">`+tetrahedron+`</object></resources>`),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if g, _ := m.Mesh.Geometry(nil); m.Triangles != 4 || g.Bounds.Min.X != 3 || g.Bounds.Max.X != 4 {
		t.Fatalf("triangles=%d bounds=%+v", m.Triangles, g.Bounds)
	}
}

func TestColors3MF(t *testing.T) {
	colored := strings.Replace(tetrahedron, `<triangle v1="1" v2="2" v3="3"/>`, `<triangle v1="1" v2="2" v3="3" pid="5" p1="1"/>`, 1)
	m, err := Parse3MF(archive3MF(t, map[string]string{"3D/3dmodel.model": model3MF(`xmlns:m="http://schemas.microsoft.com/3dmanufacturing/material/2015/02"`,
		`<resources>
		 <basematerials id="5"><base name="red" displaycolor="#FF0000"/><base name="green" displaycolor="#00FF0080"/></basematerials>
		 <m:colorgroup id="6"><m:color color="#0000FFFF"/></m:colorgroup>
		 <object id="1" pid="6" pindex="0">`+colored+`</object>
		 <object id="2">`+tetrahedron+`</object>
		 </resources><build><item objectid="1"/><item objectid="2"/></build>`)}))
	if err != nil {
		t.Fatal(err)
	}
	g, _ := m.Mesh.Geometry(nil)
	for vertex, want := range map[int]color.RGBA{0: {B: 255, A: 255}, 9: {G: 255, A: 255}, 12: {R: 100, G: 180, B: 230, A: 255}} {
		if got := g.Vertices[vertex].Color; got != want {
			t.Errorf("vertex %d is %v, want %v", vertex, got, want)
		}
	}
}

func TestMetadata3MF(t *testing.T) {
	m, err := Parse3MF(archive3MF(t, map[string]string{
		"Metadata/thumbnail.png": thumbnail(t),
		"3D/3dmodel.model": model3MF(`unit="centimeter"`, `
		 <metadata name="Title">Bracket</metadata><metadata name="Designer">R. Maker</metadata>
		 <metadata name="Application">SliceWorks 2.1</metadata><metadata name="CreationDate">2026-03-04</metadata>
		 <metadata name="Copyright">[]</metadata><metadata name="LicenseTerms">CC BY 4.0</metadata>
		 <metadata name="Description">&amp;lt;p&amp;gt;A &amp;lt;strong&amp;gt;sturdy&amp;lt;/strong&amp;gt; bracket&amp;amp;nbsp;for shelves.&amp;lt;/p&amp;gt;</metadata>
		 <metadata name="Vendor:Secret">hidden</metadata>
		 <resources><object id="1" name="body">`+tetrahedron+`</object></resources><build><item objectid="1"/></build>`)}))
	if err != nil {
		t.Fatal(err)
	}
	fields := m.fields("")
	expect(t, fields, map[string]string{
		"Format": "3MF", "Title": "Bracket", "Designer": "R. Maker", "Application": "SliceWorks 2.1", "Created": "2026-03-04",
		"License": "CC BY 4.0", "Description": "A sturdy bracket for shelves.", "Extent": "1 × 1 × 1 cm", "Thumbnail": "8 × 6",
	})
	for _, label := range []string{"Copyright", "Vendor:Secret", "Shown"} {
		if got := field(fields, label); got != "" {
			t.Errorf("%s = %q", label, got)
		}
	}
	if m.Thumbnail == nil || m.Thumbnail.Bounds().Dx() != 8 {
		t.Fatalf("thumbnail %v", m.Thumbnail)
	}
}

// many3MF has a single object of n triangles, each a copy of the first.
func many3MF(t *testing.T, n int, parts map[string]string) []byte {
	var b strings.Builder
	b.WriteString(`<resources><object id="1"><mesh><vertices><vertex x="0" y="0" z="0"/><vertex x="2" y="0" z="0"/><vertex x="0" y="3" z="0"/></vertices><triangles>`)
	for range n {
		b.WriteString(`<triangle v1="0" v2="1" v3="2"/>`)
	}
	b.WriteString(`</triangles></mesh></object></resources><build><item objectid="1"/></build>`)
	parts["3D/3dmodel.model"] = model3MF("", b.String())
	return archive3MF(t, parts)
}

// limit lowers the most triangles a mesh may have, so that a test of the
// limit need not build a million of them.
func limit(t *testing.T, n int) {
	t.Helper()
	real := maxTriangles
	maxTriangles = n
	t.Cleanup(func() { maxTriangles = real })
}

func TestLarge3MFFallsBackToItsThumbnail(t *testing.T) {
	limit(t, 1000)
	l := &Loader{}
	defer l.Close()
	over := 1001
	r := l.Load(Request{Path: write(t, "plate.3mf", many3MF(t, over, map[string]string{"Metadata/thumbnail.png": thumbnail(t)})), Page: 1, DPI: 72, Generation: 1})
	if r.Err != nil || r.Mesh != nil || r.Image == nil || r.Image.Bounds().Dx() != 8 || r.Kind != "3mf" {
		t.Fatalf("err=%v mesh=%v image=%v kind=%q", r.Err, r.Mesh != nil, r.Image, r.Kind)
	}
	expect(t, r.Info, map[string]string{"Triangles": "1,001", "Extent": "2 × 3 × 0 mm"})
	if got := field(r.Info, "Shown"); !strings.Contains(got, "thumbnail") || !strings.Contains(got, "1,000") {
		t.Errorf("Shown = %q", got)
	}
	r = l.Load(Request{Path: write(t, "plate.3mf", many3MF(t, over, map[string]string{})), Page: 1, DPI: 72, Generation: 2})
	if r.Err == nil || !strings.Contains(r.Err.Error(), "1,001 triangles") || !strings.Contains(r.Err.Error(), "1,000") || r.Image != nil {
		t.Fatalf("without a thumbnail: err=%v", r.Err)
	}
	expect(t, r.Info, map[string]string{"Triangles": "1,001"})
	r = l.Load(Request{Path: write(t, "plate.3mf", many3MF(t, 1000, map[string]string{"Metadata/thumbnail.png": thumbnail(t)})), Page: 1, DPI: 72, Generation: 3})
	if r.Err != nil || r.Mesh == nil || r.Mesh.Triangles() != 1000 || r.Image != nil {
		t.Fatalf("at the limit: err=%v", r.Err)
	}
}

func TestPreviewOf3MFIsItsThumbnail(t *testing.T) {
	l := &Loader{}
	defer l.Close()
	path := write(t, "part.3mf", archive3MF(t, map[string]string{"Metadata/thumbnail.png": thumbnail(t),
		"3D/3dmodel.model": model3MF("", `<resources><object id="1">`+tetrahedron+`</object></resources><build><item objectid="1"/></build>`)}))
	r := l.Load(Request{Path: path, Page: 1, DPI: 72, Generation: 1, Preview: true})
	if r.Err != nil || r.Image == nil || r.Mesh != nil {
		t.Fatalf("err=%v image=%v", r.Err, r.Image)
	}
	if got := field(r.Info, "Shown"); got != "embedded thumbnail" {
		t.Errorf("Shown = %q", got)
	}
	// Without one, the preview is the mesh itself.
	r = l.Load(Request{Path: write(t, "bare.3mf", simple3MF(t)), Page: 1, DPI: 72, Generation: 2, Preview: true})
	if r.Err != nil || r.Mesh == nil || r.Image != nil {
		t.Fatalf("err=%v mesh=%v", r.Err, r.Mesh != nil)
	}
}

func TestDamaged3MF(t *testing.T) {
	object := func(body string) string {
		return model3MF("", `<resources>`+body+`</resources><build><item objectid="1"/></build>`)
	}
	var crowded bytes.Buffer
	w := zip.NewWriter(&crowded)
	for i := range 5000 {
		f, _ := w.Create(fmt.Sprintf("3D/filler-%d.model", i))
		fmt.Fprint(f, "x")
	}
	w.Close()
	for name, tt := range map[string]struct {
		data []byte
		want string
	}{
		"not an archive":      {[]byte("PK\x03\x04 but not really"), "zip"},
		"no model":            {archive3MF(t, map[string]string{"3D/readme.txt": "hello"}), "no model"},
		"empty":               {archive3MF(t, map[string]string{"3D/3dmodel.model": object(`<object id="1"><mesh><vertices/><triangles/></mesh></object>`)}), "no triangles"},
		"malformed":           {archive3MF(t, map[string]string{"3D/3dmodel.model": `<model ` + coreNS + `><resources><object id="1"><mesh>`}), "3MF"},
		"vertex out of range": {archive3MF(t, map[string]string{"3D/3dmodel.model": object(`<object id="1"><mesh><vertices><vertex x="0" y="0" z="0"/></vertices><triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh></object>`)}), "vertex"},
		"not a number":        {archive3MF(t, map[string]string{"3D/3dmodel.model": object(`<object id="1"><mesh><vertices><vertex x="wide" y="0" z="0"/></vertices><triangles/></mesh></object>`)}), "vertex"},
		"infinite":            {archive3MF(t, map[string]string{"3D/3dmodel.model": object(`<object id="1"><mesh><vertices><vertex x="1e999" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices><triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh></object>`)}), "vertex"},
		"missing object":      {archive3MF(t, map[string]string{"3D/3dmodel.model": model3MF("", `<resources/><build><item objectid="9"/></build>`)}), "object 9"},
		"an object in itself": {archive3MF(t, map[string]string{"3D/3dmodel.model": object(`<object id="1"><components><component objectid="1"/></components></object>`)}), "nest"},
		"two in each other":   {archive3MF(t, map[string]string{"3D/3dmodel.model": object(`<object id="1"><components><component objectid="2"/></components></object><object id="2"><components><component objectid="1"/></components></object>`)}), "nest"},
		"bad transform":       {archive3MF(t, map[string]string{"3D/3dmodel.model": model3MF("", `<resources><object id="1">`+tetrahedron+`</object></resources><build><item objectid="1" transform="1 0 0"/></build>`)}), "transform"},
		"missing part":        {archive3MF(t, map[string]string{"3D/3dmodel.model": model3MF(`xmlns:p="http://schemas.microsoft.com/3dmanufacturing/production/2015/06"`, `<resources/><build><item objectid="1" p:path="/3D/gone.model"/></build>`)}), "gone.model"},
		"too many entries":    {crowded.Bytes(), "entries"},
	} {
		m, err := Parse3MF(tt.data)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want an error naming %q (triangles=%d)", name, err, tt.want, m.triangles())
		}
	}
}

func TestExpanding3MFIsRefused(t *testing.T) {
	// A few kilobytes of archive that unpack to more than gloss will read.
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("_rels/.rels")
	fmt.Fprint(f, rels3MF)
	f, _ = w.Create("3D/3dmodel.model")
	fmt.Fprint(f, `<?xml version="1.0"?><model `+coreNS+`><resources><object id="1"><mesh><vertices>`)
	padding := bytes.Repeat([]byte(" "), 1<<20)
	for range MaxFileBytes>>20 + 1 {
		f.Write(padding)
	}
	w.Close()
	if b.Len() > 1<<20 {
		t.Fatalf("the fixture is %d bytes; it should be small", b.Len())
	}
	if _, err := Parse3MF(b.Bytes()); err == nil || !strings.Contains(err.Error(), "128 MiB") {
		t.Fatalf("err = %v", err)
	}
}

const production3MF = `xmlns:p="http://schemas.microsoft.com/3dmanufacturing/production/2015/06" requiredextensions="p"`

func picture(t *testing.T, w, h int) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// slicerProject is a project as Bambu Studio and its relatives save it: one
// object of two parts, each in the filament its settings give it.
func slicerProject(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	all := map[string]string{
		"3D/3dmodel.model": model3MF(production3MF, `<metadata name="License">Standard Digital File License</metadata>
			<resources><object id="2" type="model"><components>
				<component p:path="/3D/Objects/object_1.model" objectid="1"/>
				<component p:path="/3D/Objects/object_1.model" objectid="3" transform="1 0 0 0 1 0 0 0 1 5 0 0"/>
			</components></object></resources><build><item objectid="2"/></build>`),
		"3D/Objects/object_1.model": model3MF(production3MF, `<resources><object id="1">`+tetrahedron+`</object><object id="3">`+tetrahedron+`</object></resources>`),
		"Metadata/model_settings.config": `<?xml version="1.0"?><config><object id="2">
			<metadata key="name" value="toy.stl"/><metadata key="extruder" value="2"/>
			<part id="1" subtype="normal_part"><metadata key="name" value="body"/></part>
			<part id="3" subtype="normal_part"><metadata key="extruder" value="1"/></part>
		</object></config>`,
		"Metadata/project_settings.config": `{"filament_colour": ["#FF0000", "#5E43B7"], "filament_type": ["PLA", "PLA"]}`,
		"Metadata/thumbnail.png":           picture(t, 3, 5),
		"Metadata/plate_1.png":             picture(t, 8, 6),
	}
	for name, content := range parts {
		all[name] = content
	}
	return archive3MF(t, all)
}

func TestSlicerProject3MF(t *testing.T) {
	m, err := Parse3MF(slicerProject(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	g, _ := m.Mesh.Geometry(nil)
	purple, red := color.RGBA{R: 0x5e, G: 0x43, B: 0xb7, A: 255}, color.RGBA{R: 255, A: 255}
	// The first part takes the object's filament; the second has its own.
	if first, second := g.Vertices[0].Color, g.Vertices[12].Color; first != purple || second != red {
		t.Errorf("parts are %v and %v, want %v and %v", first, second, purple, red)
	}
	// The object that only holds the parts is not one of them.
	if m.Objects != 2 || m.Triangles != 8 {
		t.Errorf("objects=%d triangles=%d", m.Objects, m.Triangles)
	}
	// The slicer's rendering of the plate stands in for the model better
	// than the picture the package declares, which may be a photograph.
	if m.Thumbnail == nil || m.Thumbnail.Bounds().Dx() != 8 {
		t.Errorf("thumbnail %v", m.Thumbnail)
	}
	expect(t, m.fields(""), map[string]string{"License": "Standard Digital File License", "Objects": "2", "Thumbnail": "8 × 6"})
}

func TestSlicerSettingsGiveWayToTheModel(t *testing.T) {
	// Colors the model itself declares are the standard, and win.
	m, err := Parse3MF(slicerProject(t, map[string]string{
		"3D/Objects/object_1.model": model3MF(production3MF, `<resources><basematerials id="9"><base name="green" displaycolor="#00FF00"/></basematerials>
			<object id="1" pid="9" pindex="0">`+tetrahedron+`</object><object id="3">`+tetrahedron+`</object></resources>`),
	}))
	if err != nil {
		t.Fatal(err)
	}
	g, _ := m.Mesh.Geometry(nil)
	if first, second := g.Vertices[0].Color, g.Vertices[12].Color; first != (color.RGBA{G: 255, A: 255}) || second != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("parts are %v and %v", first, second)
	}
}

func TestDamagedSlicerSettingsAreIgnored(t *testing.T) {
	for name, parts := range map[string]map[string]string{
		"settings are not XML":     {"Metadata/model_settings.config": "<config><object id="},
		"project is not JSON":      {"Metadata/project_settings.config": "{not json"},
		"colors are not colors":    {"Metadata/project_settings.config": `{"filament_colour": ["mauve", 7]}`},
		"colors are not a list":    {"Metadata/project_settings.config": `{"filament_colour": "#FF0000"}`},
		"no such filament":         {"Metadata/model_settings.config": `<config><object id="2"><metadata key="extruder" value="9"/></object></config>`},
		"filament is not a number": {"Metadata/model_settings.config": `<config><object id="2"><metadata key="extruder" value="-1"/><part id="1"><metadata key="extruder" value="x"/></part></object></config>`},
		"plate is not a picture":   {"Metadata/plate_1.png": "not a png"},
	} {
		m, err := Parse3MF(slicerProject(t, parts))
		if err != nil || m.Mesh == nil || m.Triangles != 8 || m.Thumbnail == nil {
			t.Errorf("%s: %v %+v", name, err, m)
			continue
		}
		if name == "plate is not a picture" && m.Thumbnail.Bounds().Dx() != 3 {
			t.Errorf("%s: the declared thumbnail was not used instead: %v", name, m.Thumbnail.Bounds())
		}
	}
}
