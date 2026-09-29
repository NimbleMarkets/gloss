// Package document reads local files and renders them through NTCharts backends.
package document

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
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
var Extensions = []string{".md", ".markdown", ".mdown", ".pdf", ".svg", ".stl", ".heic", ".heif", ".hif", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff"}

var ErrUnsupported = errors.New("unsupported format; expected an image, SVG, PDF, STL, or Markdown")
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
	MaxEdge    int    // Export raster target; zero keeps interactive defaults.
	BaseDir    string // Relative Markdown assets; empty uses the source directory.
}

type Result struct {
	Generation  uint64
	Kind        string
	Page, Pages int
	Image       image.Image
	Mesh        *Mesh
	Markdown    *Markdown
	Camera      *charts.Camera // Export view of a mesh; nil uses the default.
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
}

func (l *Loader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return l.closePDF()
}

func (l *Loader) closePDF() error {
	l.path, l.pages = "", 0
	l.pdfReader, l.pdfVersion, l.file = nil, "", nil
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
			return section("Document", Field{"Format", strings.TrimSpace("PDF " + pdfVersion(data))})
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
	default:
		out.Image, out.Err = decodeRaster(data)
		details = func() []Field { return imageFields(data) }
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
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown":
		return "markdown", nil
	case ".pdf":
		return "pdf", nil
	case ".svg":
		return "svg", nil
	case ".stl":
		return "stl", nil
	case ".heic", ".heif", ".hif", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff":
		return "image", nil
	}
	return "", ErrUnsupported
}
