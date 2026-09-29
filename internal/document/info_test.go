package document

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gen2brain/h265/heic"
)

func field(fields []Field, label string) string {
	for _, f := range fields {
		if f.Label == label && f.Value != "" {
			return f.Value
		}
	}
	return ""
}

func loadInfo(t *testing.T, path string) []Field {
	t.Helper()
	l := &Loader{}
	defer l.Close()
	r := l.Load(Request{Path: path, Page: 1, DPI: 72, Generation: 1})
	if r.Err != nil {
		t.Fatalf("%s: %v", path, r.Err)
	}
	return r.Info
}

func expect(t *testing.T, fields []Field, want map[string]string) {
	t.Helper()
	for label, value := range want {
		if got := field(fields, label); got != value {
			t.Errorf("%s = %q, want %q\nall: %+v", label, got, value, fields)
		}
	}
}

type endian interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

type tiffEntry struct {
	tag, kind uint16
	count     uint32
	value     []byte
}

func ascii(tag uint16, s string) tiffEntry {
	return tiffEntry{tag, 2, uint32(len(s) + 1), append([]byte(s), 0)}
}

func short(order endian, tag, v uint16) tiffEntry {
	return tiffEntry{tag, 3, 1, order.AppendUint16(nil, v)}
}

func rationals(order endian, tag uint16, pairs ...uint32) tiffEntry {
	var b []byte
	for _, v := range pairs {
		b = order.AppendUint32(b, v)
	}
	return tiffEntry{tag, 5, uint32(len(pairs) / 2), b}
}

// tiff lays out IFD0 followed by the Exif and GPS directories it points to.
func tiff(order endian, ifd0, exif, gps []tiffEntry) []byte {
	size := func(entries []tiffEntry) int {
		n := 2 + 12*len(entries) + 4
		for _, e := range entries {
			if len(e.value) > 4 {
				n += len(e.value) + len(e.value)%2
			}
		}
		return n
	}
	ifd0 = append(append([]tiffEntry(nil), ifd0...), tiffEntry{0x8769, 4, 1, nil}, tiffEntry{0x8825, 4, 1, nil})
	exifAt := 8 + size(ifd0)
	gpsAt := exifAt + size(exif)
	ifd0[len(ifd0)-2].value = order.AppendUint32(nil, uint32(exifAt))
	ifd0[len(ifd0)-1].value = order.AppendUint32(nil, uint32(gpsAt))
	out := []byte("II")
	if order.String() == "BigEndian" {
		out = []byte("MM")
	}
	out = order.AppendUint32(order.AppendUint16(out, 42), 8)
	for _, entries := range [][]tiffEntry{ifd0, exif, gps} {
		data := len(out) + 2 + 12*len(entries) + 4
		var tail []byte
		out = order.AppendUint16(out, uint16(len(entries)))
		for _, e := range entries {
			out = order.AppendUint32(order.AppendUint16(order.AppendUint16(out, e.tag), e.kind), e.count)
			if len(e.value) <= 4 {
				out = append(out, append(append([]byte(nil), e.value...), 0, 0, 0, 0)[:4]...)
				continue
			}
			out = order.AppendUint32(out, uint32(data+len(tail)))
			tail = append(tail, e.value...)
			if len(e.value)%2 == 1 {
				tail = append(tail, 0)
			}
		}
		out = append(order.AppendUint32(out, 0), tail...)
	}
	return out
}

func photoEXIF(order endian) []byte {
	return tiff(order,
		[]tiffEntry{ascii(0x010f, "Acme"), ascii(0x0110, "Acme Snap 3"), short(order, 0x0112, 6), ascii(0x0131, "SnapOS 4.2"), ascii(0x013b, "A. Photographer"), ascii(0x8298, "(c) 2026 Example")},
		[]tiffEntry{rationals(order, 0x829a, 1, 250), rationals(order, 0x829d, 28, 10), short(order, 0x8827, 400), ascii(0x9003, "2026:03:04 05:06:07"), rationals(order, 0x920a, 35, 1), ascii(0xa434, "Acme 35mm F2.8")},
		[]tiffEntry{ascii(0x0001, "N"), rationals(order, 0x0002, 48, 1, 51, 1, 2988, 100), ascii(0x0003, "W"), rationals(order, 0x0004, 2, 1, 17, 1, 4020, 100)},
	)
}

func jpegWithEXIF(t *testing.T, payload []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	segment := append([]byte("Exif\x00\x00"), payload...)
	out := append([]byte{0xff, 0xd8, 0xff, 0xe1}, binary.BigEndian.AppendUint16(nil, uint16(len(segment)+2))...)
	return append(append(out, segment...), b.Bytes()[2:]...)
}

func write(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

var camera = map[string]string{
	"Camera":      "Acme Snap 3",
	"Lens":        "Acme 35mm F2.8",
	"Taken":       "2026-03-04 05:06:07",
	"Exposure":    "1/250 s · f/2.8 · ISO 400 · 35 mm",
	"Orientation": "rotated 90° clockwise",
	"Location":    "48.85830° N, 2.29450° W",
	"Software":    "SnapOS 4.2",
	"Artist":      "A. Photographer",
	"Copyright":   "(c) 2026 Example",
}

func TestInfoForImages(t *testing.T) {
	fields := loadInfo(t, "../../examples/landscape.png")
	expect(t, fields, map[string]string{"Path": "../../examples/landscape.png", "Format": "PNG", "Dimensions": "640 × 400 (0.3 MP)"})
	for _, label := range []string{"Size", "Modified", "Color"} {
		if field(fields, label) == "" {
			t.Errorf("no %s in %+v", label, fields)
		}
	}
	if field(fields, "Camera") != "" {
		t.Error("a drawing has no camera")
	}
	expect(t, loadInfo(t, "../../examples/landscape.heic"), map[string]string{"Format": "HEIC", "Dimensions": "640 × 400 (0.3 MP)"})
}

func TestInfoReadsEXIF(t *testing.T) {
	for _, order := range []endian{binary.LittleEndian, binary.BigEndian} {
		fields := loadInfo(t, write(t, "photo.jpg", jpegWithEXIF(t, photoEXIF(order))))
		expect(t, fields, camera)
		expect(t, fields, map[string]string{"Format": "JPEG", "Dimensions": "40 × 30"})
	}
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	if err := heic.Encode(&b, img, heic.EncodeOptions{Quality: 50, Exif: photoEXIF(binary.LittleEndian)}); err != nil {
		t.Fatal(err)
	}
	expect(t, loadInfo(t, write(t, "photo.heic", b.Bytes())), camera)
}

func TestDamagedEXIFDoesNotStopTheImage(t *testing.T) {
	good := photoEXIF(binary.LittleEndian)
	loop := append([]byte(nil), good[:8]...)
	loop = binary.LittleEndian.AppendUint16(loop, 1)
	loop = append(loop, 0x69, 0x87, 4, 0, 1, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0) // Exif IFD points at IFD0.
	huge := append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(huge[8+2+4:], 1<<31) // An absurd count for the first tag.
	for name, payload := range map[string][]byte{"truncated": good[:len(good)/2], "short": good[:6], "loop": loop, "huge count": huge, "garbage": bytes.Repeat([]byte{0xff}, 64)} {
		fields := loadInfo(t, write(t, "photo.jpg", jpegWithEXIF(t, payload)))
		if field(fields, "Dimensions") == "" {
			t.Errorf("%s: image details lost: %+v", name, fields)
		}
	}
}

func TestInfoExplainsAFileTooLargeToShow(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	header := data[12:29] // "IHDR", then width and height.
	binary.BigEndian.PutUint32(header[4:], 9000)
	binary.BigEndian.PutUint32(header[8:], 8000)
	binary.BigEndian.PutUint32(data[29:], crc32.ChecksumIEEE(header))
	l := &Loader{}
	defer l.Close()
	r := l.Load(Request{Path: write(t, "poster.png", data), Page: 1, DPI: 72, Generation: 1})
	if r.Err == nil || !strings.Contains(r.Err.Error(), "exceeds") {
		t.Fatalf("err = %v", r.Err)
	}
	expect(t, r.Info, map[string]string{"Format": "PNG", "Dimensions": "9000 × 8000 (72.0 MP)"})
	if field(r.Info, "Size") == "" {
		t.Errorf("%+v", r.Info)
	}
	broken := l.Load(Request{Path: write(t, "part.stl", []byte("solid x\nfacet nonsense")), Page: 1, DPI: 72, Generation: 2})
	if broken.Err == nil || field(broken.Info, "Size") == "" || field(broken.Info, "Triangles") != "" {
		t.Fatalf("err=%v info=%+v", broken.Err, broken.Info)
	}
	torn := l.Load(Request{Path: write(t, "torn.pdf", []byte("%PDF-1.7\nnot much of a document")), Page: 1, DPI: 72, Generation: 3})
	if torn.Err == nil || field(torn.Info, "Size") == "" {
		t.Fatalf("err=%v info=%+v", torn.Err, torn.Info)
	}
}

func TestInfoForSVG(t *testing.T) {
	svg := `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" width="12cm" height="80" viewBox="0 0 120 80">
  <title>Floor plan</title><desc>Ground floor,
  to scale</desc>
  <g><rect width="10" height="10"/><circle r="4"><title>not the document title</title></circle></g>
</svg>`
	expect(t, loadInfo(t, write(t, "plan.svg", []byte(svg))), map[string]string{
		"Format": "SVG", "Declared size": "12cm × 80", "View box": "0 0 120 80", "Title": "Floor plan", "Description": "Ground floor, to scale", "Elements": "7",
	})
	if fields := loadInfo(t, "../../examples/shapes.svg"); field(fields, "Elements") == "" || field(fields, "Size") == "" {
		t.Errorf("%+v", fields)
	}
}

// pdfFile writes objects 1..n and a cross-reference table for them.
func pdfFile(version string, trailer string, objects ...string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%%PDF-%s\n", version)
	offsets := make([]int, len(objects))
	for i, body := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R %s >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, trailer, xref)
	return b.Bytes()
}

func TestInfoForPDFProperties(t *testing.T) {
	data := pdfFile("1.6", "/Info 5 0 R",
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>",
		"<< /Type /Page /Parent 2 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595.276 841.89] >>",
		"<< /Title (Field Notes) /Author (R. Naturalist) /Subject (Birds) /Keywords (owls, wrens) /Creator (Quill 2) /Producer (libquill) /CreationDate (D:20260304050607+02'00') /ModDate (D:20260405) >>",
	)
	// Quartz writes a zone after the Z that the specification says stands alone.
	for date, want := range map[string]string{"D:20190429190000Z00'00'": "2019-04-29 19:00:00 UTC", "D:20190429190000Z": "2019-04-29 19:00:00 UTC", "D:2019": "2019", "D:20190429190000-08'00": "2019-04-29 19:00:00 -08:00", "last Tuesday": "last Tuesday", "": ""} {
		if got := pdfDate(date); got != want {
			t.Errorf("pdfDate(%q) = %q, want %q", date, got, want)
		}
	}
	fields, err := pdfInfo(data, 1)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, fields, map[string]string{
		"Format": "PDF 1.6", "Pages": "2", "Page size": "612 × 792 pt (8.50 × 11.00 in)",
		"Title": "Field Notes", "Author": "R. Naturalist", "Subject": "Birds", "Keywords": "owls, wrens",
		"Creator": "Quill 2", "Producer": "libquill", "Created": "2026-03-04 05:06:07 +02:00", "Changed": "2026-04-05",
	})
	fields, err = pdfInfo(data, 2)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, fields, map[string]string{"Page size": "595 × 842 pt (8.27 × 11.69 in)"})
}

func TestInfoForPDFPages(t *testing.T) {
	l := &Loader{}
	defer l.Close()
	for page := 1; page <= 2; page++ {
		r := l.Load(Request{Path: "../../examples/field-guide.pdf", Page: page, DPI: 72, Generation: uint64(page)})
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		expect(t, r.Info, map[string]string{"Path": "../../examples/field-guide.pdf", "Pages": "2", "Page size": "640 × 400 pt (8.89 × 5.56 in)"})
		if !strings.HasPrefix(field(r.Info, "Format"), "PDF 1.") || field(r.Info, "Size") == "" {
			t.Errorf("page %d: %+v", page, r.Info)
		}
	}
}

func TestInfoForSTL(t *testing.T) {
	expect(t, loadInfo(t, write(t, "part.stl", []byte(triangleSTL))), map[string]string{
		"Format": "STL (ASCII)", "Name": "triangle", "Triangles": "1", "Extent": "1 × 1 × 0", "Surface area": "0.5",
	})
	binary := binarySTL()
	copy(binary, make([]byte, 80))
	copy(binary, "bracket v2")
	expect(t, loadInfo(t, write(t, "part.stl", binary)), map[string]string{"Format": "STL (binary)", "Name": "bracket v2", "Triangles": "1"})
	if got := field(loadInfo(t, "../../examples/gloss.stl"), "Triangles"); got != "888" {
		t.Errorf("Triangles = %q", got)
	}
}

func TestInfoForMarkdown(t *testing.T) {
	source := "# Trip report\n\nTwo days in the [hills](https://example.com) and a [map](map.pdf).\n\n## Day one\n\n![ridge](missing.png)\n\n### Notes\n\n- wet\n- cold\n"
	expect(t, loadInfo(t, write(t, "trip.md", []byte(source))), map[string]string{
		"Format": "Markdown", "Title": "Trip report", "Lines": "12", "Words": "16", "Headings": "3", "Links": "2", "Images": "1",
	})
}

func TestInfoForFilesWithoutADisk(t *testing.T) {
	files := fstest.MapFS{"shapes.svg": {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"/>`)}}
	l := &Loader{Files: files}
	defer l.Close()
	r := l.Load(Request{Path: "shapes.svg", Page: 1, DPI: 72, Generation: 1})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	expect(t, r.Info, map[string]string{"Path": "shapes.svg", "Size": fmt.Sprintf("%d bytes", len(files["shapes.svg"].Data))})
	if got := field(r.Info, "Modified"); got != "" {
		t.Errorf("Modified = %q for a file with no timestamp", got)
	}
	stdin := loadInfo(t, write(t, "gloss-stdin-123", files["shapes.svg"].Data))
	expect(t, stdin, map[string]string{"Path": "stdin"})
	if got := field(stdin, "Modified"); got != "" {
		t.Errorf("Modified = %q for piped input", got)
	}
}

func TestByteSizes(t *testing.T) {
	for size, want := range map[int]string{59: "59 bytes", 1: "1 byte", 2048: "2.0 KiB (2,048 bytes)", 5 << 20: "5.0 MiB (5,242,880 bytes)", 3 << 30: "3.0 GiB (3,221,225,472 bytes)"} {
		if got := byteSize(size); got != want {
			t.Errorf("byteSize(%d) = %q, want %q", size, got, want)
		}
	}
}
