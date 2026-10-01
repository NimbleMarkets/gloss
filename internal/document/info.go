package document

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
	"github.com/yuin/goldmark/ast"
)

// Field is one line of the info panel. A Field without a Value is a heading.
type Field struct{ Label, Value string }

// Section drops the fields a file does not supply, and the heading with them.
func Section(heading string, fields ...Field) []Field {
	out := []Field{{Label: heading}}
	for _, f := range fields {
		// Values come from the file: keep each to one line of sensible length.
		f.Value = strings.Join(strings.Fields(f.Value), " ")
		if r := []rune(f.Value); len(r) > 200 {
			f.Value = string(r[:200]) + "…"
		}
		if f.Value != "" {
			out = append(out, f)
		}
	}
	if len(out) == 1 {
		return nil
	}
	return out
}

// describe keeps a malformed file from costing the view of it: the panel is
// a courtesy, and the document has already loaded.
func describe(read func() []Field) (fields []Field) {
	defer func() {
		if recover() != nil {
			fields = nil
		}
	}()
	return read()
}

// grouped writes a count with its thousands set apart.
func grouped(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func byteSize(n int) string {
	if n == 1 {
		return "1 byte"
	}
	exact := grouped(n) + " bytes"
	for _, unit := range []struct {
		name string
		size float64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}} {
		if float64(n) >= unit.size {
			return fmt.Sprintf("%.1f %s (%s)", float64(n)/unit.size, unit.name, exact)
		}
	}
	return exact
}

func fileFields(files fs.FS, path string, size int) []Field {
	var info fs.FileInfo
	if files == nil {
		info, _ = os.Stat(path)
	} else {
		info, _ = fs.Stat(files, filepath.ToSlash(path))
	}
	modified := ""
	if info != nil && !info.ModTime().IsZero() {
		modified = info.ModTime().Format("2006-01-02 15:04")
	}
	if IsStdin(path) {
		path, modified = "stdin", ""
	}
	return Section("File", Field{"Path", path}, Field{"Size", byteSize(size)}, Field{"Modified", modified})
}

func imageFields(data []byte) []Field {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	name, ok := map[string]string{"png": "PNG", "jpeg": "JPEG", "gif": "GIF", "heic": "HEIC", "webp": "WebP", "bmp": "BMP", "tiff": "TIFF"}[format]
	if !ok {
		name = strings.ToUpper(format)
	}
	dimensions := fmt.Sprintf("%d × %d", cfg.Width, cfg.Height)
	if mp := float64(cfg.Width) * float64(cfg.Height) / 1e6; mp >= .05 {
		dimensions += fmt.Sprintf(" (%.1f MP)", mp)
	}
	shade := ""
	switch model := cfg.ColorModel.(type) {
	case color.Palette:
		shade = fmt.Sprintf("indexed, %d colors", len(model))
	default:
		switch model {
		case color.RGBAModel, color.NRGBAModel:
			shade = "8-bit RGBA"
		case color.RGBA64Model, color.NRGBA64Model:
			shade = "16-bit RGBA"
		case color.GrayModel:
			shade = "8-bit gray"
		case color.Gray16Model:
			shade = "16-bit gray"
		case color.CMYKModel:
			shade = "CMYK"
		case color.YCbCrModel, color.NYCbCrAModel:
			shade = "YCbCr"
		}
	}
	return append(Section("Image", Field{"Format", name}, Field{"Dimensions", dimensions}, Field{"Color", shade}), exifFields(exifPayload(format, data))...)
}

func svgFields(data []byte) []Field {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	var width, height, box, title, desc string
	var text *string
	depth, elements := 0, 0
	const limit = 200000
	for elements < limit {
		token, err := d.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			elements++
			text = nil
			if depth == 1 {
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "width":
						width = a.Value
					case "height":
						height = a.Value
					case "viewBox":
						box = a.Value
					}
				}
			}
			// Only the root's own title and description name the drawing.
			if depth == 2 && t.Name.Local == "title" && title == "" {
				text = &title
			}
			if depth == 2 && t.Name.Local == "desc" && desc == "" {
				text = &desc
			}
		case xml.EndElement:
			depth--
			text = nil
		case xml.CharData:
			if text != nil {
				*text += string(t)
			}
		}
	}
	declared := ""
	if width != "" && height != "" {
		declared = width + " × " + height
	}
	count := strconv.Itoa(elements)
	if elements >= limit {
		count += "+"
	}
	return Section("Drawing", Field{"Format", "SVG"}, Field{"Declared size", declared}, Field{"View box", box}, Field{"Title", title}, Field{"Description", desc}, Field{"Elements", count})
}

func pdfVersion(data []byte) string {
	head := bytes.TrimSpace(data[:min(len(data), 1024)])
	if !bytes.HasPrefix(head, []byte("%PDF-")) {
		return ""
	}
	version := string(head[5:min(len(head), 16)])
	if i := strings.IndexFunc(version, func(r rune) bool { return r != '.' && !unicode.IsDigit(r) }); i >= 0 {
		version = version[:i]
	}
	return version
}

func pdfInfo(data []byte, page int) ([]Field, error) {
	if err := checkPDFStreams(data); err != nil {
		return nil, err
	}
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	return pdfFields(reader, pdfVersion(data), reader.NumPage(), page), nil
}

// pdfDate reads "D:20260304050607+02'00'", of which only the year is required.
func pdfDate(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "D:")
	digits := strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	if digits < 0 {
		digits = len(s)
	}
	if digits < 4 {
		return s
	}
	zone := strings.NewReplacer("'", ":").Replace(strings.TrimSuffix(s[digits:], "'"))
	s = s[:min(digits, 14)]
	out := s[:4]
	for i, sep := range []string{"-", "-", " ", ":", ":"} {
		if len(s) < 6+2*i {
			return out
		}
		out += sep + s[4+2*i:6+2*i]
	}
	switch {
	case strings.HasPrefix(zone, "Z"):
		out += " UTC"
	case len(zone) == 6:
		out += " " + zone
	}
	return out
}

func pdfFields(reader *pdf.Reader, version string, pages, page int) []Field {
	size := ""
	if w, h, ok, err := mediaBox(reader, page); err == nil && ok {
		size = fmt.Sprintf("%.0f × %.0f pt (%.2f × %.2f in)", w, h, w/72, h/72)
	}
	info := reader.Trailer().Key("Info")
	text := func(key string) string { return info.Key(key).Text() }
	return Section("Document", Field{"Format", strings.TrimSpace("PDF " + version)}, Field{"Pages", strconv.Itoa(pages)}, Field{"Page size", size},
		Field{"Title", text("Title")}, Field{"Author", text("Author")}, Field{"Subject", text("Subject")}, Field{"Keywords", text("Keywords")},
		Field{"Creator", text("Creator")}, Field{"Producer", text("Producer")}, Field{"Created", pdfDate(text("CreationDate"))}, Field{"Changed", pdfDate(text("ModDate"))})
}

func meshFields(data []byte, mesh *Mesh) []Field {
	if mesh == nil {
		return nil
	}
	number := func(v float64) string { return strconv.FormatFloat(v, 'g', 6, 64) }
	format, name := "STL (ASCII)", ""
	if len(data) >= 84 && 84+50*uint64(binary.LittleEndian.Uint32(data[80:84])) == uint64(len(data)) {
		format = "STL (binary)"
		header, _, _ := bytes.Cut(data[:80], []byte{0})
		name = string(header)
	} else if line, _, _ := strings.Cut(string(data[:min(len(data), 1024)]), "\n"); strings.HasPrefix(strings.TrimSpace(line), "solid") {
		name = strings.TrimPrefix(strings.TrimSpace(line), "solid")
	}
	name = strings.Map(func(r rune) rune {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, name)
	extent := mesh.geometry.Bounds.Max.Sub(mesh.geometry.Bounds.Min)
	// STL records no unit of length.
	return Section("Mesh", Field{"Format", format}, Field{"Name", name}, Field{"Triangles", grouped(mesh.Triangles())},
		Field{"Extent", number(float64(extent.X)) + " × " + number(float64(extent.Y)) + " × " + number(float64(extent.Z))}, Field{"Surface area", number(mesh.area())})
}

func markdownFields(markdown *Markdown) []Field {
	if markdown == nil {
		return nil
	}
	source, title := markdown.Source, ""
	headings, links, images := 0, 0, 0
	_ = ast.Walk(MarkdownTree(source), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			headings++
			if n.Level == 1 && title == "" {
				_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
					if text, ok := child.(*ast.Text); ok && entering {
						title += string(text.Segment.Value(source))
					}
					return ast.WalkContinue, nil
				})
			}
		case *ast.Link, *ast.AutoLink:
			links++
		case *ast.Image:
			images++
		}
		return ast.WalkContinue, nil
	})
	words := 0
	for _, word := range strings.Fields(string(source)) {
		if strings.ContainsFunc(word, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			words++
		}
	}
	lines := bytes.Count(source, []byte("\n"))
	if len(source) > 0 && source[len(source)-1] != '\n' {
		lines++
	}
	count := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	return Section("Text", Field{"Format", "Markdown"}, Field{"Title", title}, Field{"Lines", strconv.Itoa(lines)}, Field{"Words", strconv.Itoa(words)},
		Field{"Headings", count(headings)}, Field{"Links", count(links)}, Field{"Images", count(images)})
}
