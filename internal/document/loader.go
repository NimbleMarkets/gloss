// Package document reads local files and renders them through NTCharts backends.
package document

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/NimbleMarkets/ntcharts-pdf/pdfview"
	"github.com/NimbleMarkets/ntcharts-svg/svg"
	_ "github.com/NimbleMarkets/ntcharts/v2/picture/decoders"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/gen2brain/h265/heic"
	"github.com/ledongthuc/pdf"
)

const MaxFileBytes = 128 << 20
const MaxPixels = 32 << 20

// Extensions are the file extensions Detect accepts on their own. Content is
// examined first, so a supported file need not carry one of them.
var Extensions = []string{".md", ".markdown", ".mdown", ".pdf", ".svg", ".stl", ".3mf", ".xlsx", ".xlsm", ".docx", ".docm", ".csv", ".tsv", ".json", ".jsonl", ".ndjson", ".ipynb", ".html", ".htm", ".txt", ".text", ".log", ".heic", ".heif", ".hif", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff"}

// Formats says, for people, what gloss opens: labelled lines, and a short
// line for when there is no room for them.
var Formats = []string{
	"Images: PNG, JPEG, GIF, WebP, BMP, TIFF, HEIC",
	"Doc:    SVG, PDF, Word, Excel, Jupyter",
	"Text:   Markdown, HTML, JSON, CSV, plain text and source",
	"3D:     STL and 3MF meshes",
}

const FormatsShort = "Images, SVG, PDF, Word, Excel, Jupyter, Markdown, HTML, text, JSON, CSV, STL, 3MF"

var ErrUnsupported = errors.New("unsupported format; expected an image, SVG, PDF, STL, 3MF, Markdown, HTML, JSON, a notebook, Word, Excel, CSV, or text")
var ErrNotRegular = errors.New("not a regular file")
var ErrDirectory = errors.New("is a directory")

// Probe identifies files using a bounded prefix before applying the size limit.
// The total size lets binary STL detection work without reading the whole mesh.
func Probe(path, forced string) (string, error) {
	// Check before opening: a FIFO would otherwise block waiting for a writer.
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", ErrDirectory
	}
	if !info.Mode().IsRegular() {
		return "", ErrNotRegular
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrNotRegular
	}
	header, err := io.ReadAll(io.LimitReader(f, 64<<10))
	if err != nil {
		return "", err
	}
	kind, err := Detect(path, header, forced)
	if err != nil && len(header) >= 84 && uint64(info.Size()) == 84+50*uint64(binary.LittleEndian.Uint32(header[80:84])) {
		kind, err = "stl", nil
	}
	// Image metadata can exceed the probe budget (notably JPEG APP blocks).
	// Recognize signatures here and leave complete validation to the decoder.
	if err != nil && rasterSignature(header) {
		kind, err = "image", nil
	}
	// A ZIP archive lists its contents at its end, beyond the prefix.
	if err != nil && bytes.HasPrefix(header, []byte("PK\x03\x04")) {
		if archive, zipErr := zip.NewReader(f, info.Size()); zipErr == nil && opcKind(archive) != "" {
			kind, err = opcKind(archive), nil
		}
	}
	if err != nil {
		return "", err
	}
	if info.Size() > MaxFileBytes {
		return "", fmt.Errorf("file exceeds 128 MiB")
	}
	return kind, nil
}

func rasterSignature(b []byte) bool {
	if len(b) >= 12 && string(b[4:8]) == "ftyp" {
		switch string(b[8:12]) {
		case "heic", "heix", "heim", "heis", "hevc", "hevx", "hevm", "hevs":
			return true
		}
	}
	for _, magic := range []string{"\x89PNG\r\n\x1a\n", "\xff\xd8\xff", "GIF87a", "GIF89a", "BM", "II\x2a\x00", "MM\x00\x2a"} {
		if bytes.HasPrefix(b, []byte(magic)) {
			return true
		}
	}
	return len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
}

type Request struct {
	Path, Type string
	Page, DPI  int
	Generation uint64
	Reload     bool
	MaxEdge    int         // Export raster target; zero keeps interactive defaults.
	Preview    bool        // A small, quick rendering is wanted: a thumbnail will do.
	Parts      PartFilter  // The parts of a 3MF to show; empty shows them all.
	Shown      []bool      // The parts of a 3MF already chosen, over any filter; nil for all.
	Color      *color.RGBA // Paint for the faces of a mesh that its file left plain.
	PaintAll   bool        // Paint every face, not only the plain ones.
	BaseDir    string      // Relative Markdown assets; empty uses the source directory.
}

type Result struct {
	Generation  uint64
	Kind        string
	Page, Pages int
	Image       image.Image
	Mesh        *Mesh
	Parts       []Part                      // What a 3MF build places, for choosing among.
	Shown       []bool                      // Which of them the mesh holds; nil for all.
	Assemble    func([]bool) (*Mesh, error) // The mesh of the parts marked, for a 3MF.
	Markdown    *Markdown
	Sheet       *Sheet         // One sheet of a workbook; Page and Pages count sheets.
	Camera      *charts.Camera // Export view of a mesh, as the viewer has it; nil uses Views.
	Views       []View         // Export views of a mesh: several make a sheet. None uses the default camera.
	CPU         bool           // Export a mesh without trying the GPU.
	Info        []Field        // What the file says about itself, for the info panel.
	Err         error
}

// Loader serializes PDF access and shutdown. Commands that complete out of
// order cannot replace a newer document; Close also covers in-flight loads.
type Loader struct {
	Files      fs.FS // Optional read-only embedded assets; nil uses the host filesystem.
	mu         sync.Mutex
	closed     bool
	generation uint64
	path       string
	pdf        pdfview.Renderer
	pdfReader  *pdf.Reader
	pdfVersion string
	file       []Field
	pages      int
	book       *Workbook // Kept, like the PDF, for turning between sheets.
}

func (l *Loader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return l.closePDF()
}

func (l *Loader) closePDF() error {
	l.path, l.pages = "", 0
	l.pdfReader, l.pdfVersion, l.file, l.book = nil, "", nil, nil
	if l.pdf == nil {
		return nil
	}
	err := l.pdf.Close()
	l.pdf = nil
	return err
}

func (l *Loader) Load(q Request) (out Result) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out.Generation = q.Generation
	defer func() {
		if p := recover(); p != nil {
			out.Err = fmt.Errorf("cannot decode document: %v", p)
			_ = l.closePDF()
		}
	}()
	if l.closed || q.Generation < l.generation {
		return out
	}
	l.generation = q.Generation
	if l.path != q.Path || q.Reload {
		_ = l.closePDF()
	}
	if l.pdf != nil {
		return l.renderPDF(q)
	}
	if l.book != nil {
		return l.turnSheet(q)
	}
	if l.Files == nil {
		if _, err := Probe(q.Path, q.Type); err != nil {
			out.Err = err
			return out
		}
	}
	data, err := readFileFrom(l.Files, q.Path)
	if err != nil {
		out.Err = err
		return out
	}
	kind, err := Detect(q.Path, data, q.Type)
	if err != nil {
		out.Err = err
		return out
	}
	out.Kind, out.Page, out.Pages = kind, 1, 1
	file := fileFields(l.Files, q.Path, len(data))
	var details func() []Field
	// A file that cannot be shown is still described: its size or dimensions
	// are often the reason.
	defer func() {
		if out.Info == nil && details != nil {
			out.Info = append(file, describe(details)...)
		}
	}()
	switch kind {
	case "markdown":
		out.Markdown, out.Err = loadMarkdownFrom(q.Path, data, q.BaseDir, l.Files)
		details = func() []Field { return markdownFields(out.Markdown) }
	case "pdf":
		details = func() []Field {
			return Section("Document", Field{"Format", strings.TrimSpace("PDF " + pdfVersion(data))})
		}
		reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			out.Err = err
			return out
		}
		pages := reader.NumPage()
		if pages < 1 || pages > 10000 {
			out.Err = fmt.Errorf("PDF page count must be 1..10000")
			return out
		}
		renderer, err := pdfview.DefaultRendererFactoryWithLimits(pdfview.Limits{MaxRenderPixels: MaxPixels})(q.Path, data)
		if err != nil {
			out.Err = err
			return out
		}
		l.pdf, l.path, l.pages = renderer, q.Path, pages
		l.pdfReader, l.pdfVersion, l.file = reader, pdfVersion(data), file
		return l.renderPDF(q)
	case "svg":
		edge := 2400
		if q.MaxEdge > 0 {
			edge = q.MaxEdge
		}
		out.Image, out.Err = renderSVG(q.Path, data, edge)
		details = func() []Field { return svgFields(data) }
	case "stl":
		out.Mesh, out.Err = ParseSTL(data)
		details = func() []Field { return meshFields(data, out.Mesh) }
	case "csv":
		out.Sheet, out.Err = ReadCSV(q.Path, data)
		if out.Sheet != nil {
			details = out.Sheet.csvFields
		}
	case "json":
		details = func() []Field { return Section("JSON", Field{"Format", "JSON"}) }
		doc, err := ReadJSON(q.Path, data)
		if err != nil {
			out.Err = err
			return out
		}
		out.Markdown, out.Err = loadMarkdownFrom(q.Path, doc.Markdown, q.BaseDir, l.Files)
		details = doc.fields
	case "ipynb":
		details = func() []Field { return Section("Notebook", Field{"Format", "Jupyter notebook"}) }
		nb, err := ReadNotebook(data)
		if err != nil {
			out.Err = err
			return out
		}
		// The outputs' pictures are kept beside the Markdown, in memory.
		out.Markdown, out.Err = loadMarkdownFrom("notebook.md", nb.Markdown, ".", nb.Files)
		details = nb.fields
	case "text":
		details = func() []Field { return Section("Text", Field{"Format", "Plain text"}) }
		doc, err := ReadText(q.Path, data)
		if err != nil {
			out.Err = err
			return out
		}
		out.Markdown, out.Err = loadMarkdownFrom(q.Path, doc.Markdown, q.BaseDir, l.Files)
		details = doc.fields
	case "html":
		details = func() []Field { return Section("Page", Field{"Format", "HTML"}) }
		page, err := ReadHTML(data)
		if err != nil {
			out.Err = err
			return out
		}
		// Pictures are looked for beside the page, as a browser would.
		out.Markdown, out.Err = loadMarkdownFrom(q.Path, page.Markdown, q.BaseDir, l.Files)
		details = func() []Field { return page.fields(out.Markdown) }
	case "docx":
		details = func() []Field { return Section("Document", Field{"Format", "Word document"}) }
		doc, err := OpenWord(data)
		if err != nil {
			out.Err = err
			return out
		}
		// Pictures are in the package, beside the document.
		out.Markdown, out.Err = loadMarkdownFrom("word/document.md", doc.Markdown, "word", doc.pkg.archive)
		details = doc.fields
	case "xlsx":
		details = func() []Field { return Section("Workbook", Field{"Format", "Excel workbook"}) }
		book, err := OpenWorkbook(data)
		if err != nil {
			out.Err = err
			return out
		}
		l.book, l.path, l.file = book, q.Path, file
		return l.turnSheet(q)
	case "3mf":
		model, err := Parse3MF(data)
		if err != nil {
			out.Err = err
			details = func() []Field { return Section("Model", Field{"Format", "3MF"}) }
			return out
		}
		shown := ""
		out.Parts, out.Assemble = model.Parts, model.Assemble
		chosen := q.Parts.Shown(model.Parts)
		if q.Shown != nil {
			chosen = q.Shown
		}
		out.Shown = chosen
		switch {
		case q.Preview && model.Thumbnail != nil:
			out.Image, shown = model.Thumbnail, "embedded thumbnail"
		case chosen != nil:
			out.Mesh, out.Err = model.Assemble(chosen)
			shown = partsShown(model.Parts, chosen)
		case model.Mesh != nil:
			out.Mesh = model.Mesh
		case model.Thumbnail != nil:
			out.Image, shown = model.Thumbnail, fmt.Sprintf("embedded thumbnail; the limit for a mesh is %s triangles", grouped(maxTriangles))
		default:
			out.Err = fmt.Errorf("3MF has %s triangles; the limit is %s", grouped(model.Triangles), grouped(maxTriangles))
		}
		details = func() []Field { return model.fields(shown) }
	default:
		out.Image, out.Err = decodeRaster(data)
		details = func() []Field { return imageFields(data) }
	}
	if out.Mesh != nil && q.Color != nil {
		out.Mesh.Recolor(*q.Color, q.PaintAll)
	}
	return out
}

func decodeRaster(data []byte) (image.Image, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxPixels/cfg.Height {
		return nil, fmt.Errorf("image exceeds %d pixels", MaxPixels)
	}
	if format == "heic" {
		return heic.Decode(bytes.NewReader(data), heic.Options{AutoRotate: true, FrameSizeLimit: MaxPixels, Threads: 2})
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

func renderSVG(name string, data []byte, edge int) (image.Image, error) {
	r, err := svg.DefaultRendererFactory()(name, data)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return r.Render(edge, edge)
}

func (l *Loader) renderPDF(q Request) Result {
	page := max(1, min(q.Page, l.pages))
	dpi := q.DPI
	if q.MaxEdge > 0 && l.pdfReader != nil {
		// MediaBox is inherited from the nearest page-tree ancestor.
		for node, depth := l.pdfReader.Page(page).V, 0; !node.IsNull() && depth < 64; node, depth = node.Key("Parent"), depth+1 {
			box := node.Key("MediaBox")
			if box.Len() != 4 {
				continue
			}
			edge := math.Max(math.Abs(box.Index(2).Float64()-box.Index(0).Float64()), math.Abs(box.Index(3).Float64()-box.Index(1).Float64()))
			if edge > 0 && !math.IsNaN(edge) && !math.IsInf(edge, 0) {
				dpi = int(math.Ceil(float64(q.MaxEdge) * 72 / edge))
			}
			break
		}
	}
	img, err := l.pdf.RenderPage(page, max(36, min(dpi, 600)))
	out := Result{Generation: q.Generation, Kind: "pdf", Page: page, Pages: l.pages, Image: img, Err: err}
	if reader, version, pages := l.pdfReader, l.pdfVersion, l.pages; err == nil && reader != nil {
		out.Info = append(append([]Field(nil), l.file...), describe(func() []Field { return pdfFields(reader, version, pages, page) })...)
	}
	return out
}

// turnSheet shows one sheet of the open workbook, as renderPDF shows a page.
func (l *Loader) turnSheet(q Request) Result {
	page := max(1, min(q.Page, len(l.book.Names())))
	sheet, err := l.book.Sheet(page - 1)
	out := Result{Generation: q.Generation, Kind: "xlsx", Page: page, Pages: len(l.book.Names()), Sheet: sheet, Err: err}
	out.Info = append(append([]Field(nil), l.file...), describe(l.book.fields)...)
	return out
}

func ReadFile(path string) ([]byte, error) { return readFileFrom(nil, path) }

func readFileFrom(files fs.FS, path string) ([]byte, error) {
	var f fs.File
	var err error
	if files == nil {
		f, err = os.Open(path)
	} else {
		f, err = files.Open(path)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	if info.Size() > MaxFileBytes {
		return nil, fmt.Errorf("file exceeds 128 MiB")
	}
	return ReadLimited(f)
}

func ReadLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxFileBytes {
		return nil, fmt.Errorf("input exceeds 128 MiB")
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	return b, nil
}

// Detect favors content signatures so extensionless files and stdin work.
func Detect(path string, data []byte, forced string) (string, error) {
	if forced != "" {
		return forced, nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown":
		return "markdown", nil
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("%PDF-")) {
		return "pdf", nil
	}
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		if archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil && opcKind(archive) != "" {
			return opcKind(archive), nil
		}
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		return format, nil
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for i := 0; i < 100; i++ {
		t, err := dec.Token()
		if err != nil {
			break
		}
		if root, ok := t.(xml.StartElement); ok {
			if root.Name.Local == "svg" {
				return "svg", nil
			}
			break
		}
	}
	if len(data) >= 84 && uint64(len(data)) == 84+50*uint64(binary.LittleEndian.Uint32(data[80:84])) {
		return "stl", nil
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("solid")) {
		return "stl", nil
	}
	if kind := textKind(path, data); kind != "" {
		return kind, nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown":
		return "markdown", nil
	case ".pdf":
		return "pdf", nil
	case ".svg":
		return "svg", nil
	case ".stl":
		return "stl", nil
	case ".3mf":
		return "3mf", nil
	case ".xlsx", ".xlsm":
		return "xlsx", nil
	case ".csv", ".tsv":
		return "csv", nil
	case ".json", ".jsonl", ".ndjson":
		return "json", nil
	case ".ipynb":
		return "ipynb", nil
	case ".html", ".htm":
		return "html", nil
	case ".txt", ".text", ".log":
		return "text", nil
	case ".docx", ".docm":
		return "docx", nil
	case ".heic", ".heif", ".hif", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff":
		return "image", nil
	}
	// Whatever is not binary can be read as text, as less would.
	if IsText(data) {
		return "text", nil
	}
	return "", ErrUnsupported
}

// stdinPrefix names the file stdin is read into, so that it is known for
// what it is wherever it is named.
const stdinPrefix = "gloss-stdin-"

// StdinPattern is the name pattern to create the file for stdin with.
const StdinPattern = stdinPrefix + "*"

// IsStdin says whether path is the file stdin was read into.
func IsStdin(path string) bool { return strings.HasPrefix(filepath.Base(path), stdinPrefix) }
