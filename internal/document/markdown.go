package document

import (
	"fmt"
	"image"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

const MaxMarkdownBytes = 2 << 20
const MaxMarkdownImages = 32
const maxMarkdownImagePixels = 16 << 20

type Markdown struct {
	Source []byte
	Images []MarkdownImage
	// The pictures are inside the file, as a notebook's outputs and a Word
	// document's media are: their links name no file a reader can open.
	Packaged bool
}

// Picture is one picture of packaged Markdown, as a file beside its text
// would hold it.
type Picture struct {
	Destination string // The link as the Markdown has it.
	Name        string // A file name for it, unique among the pictures, ending .png.
	Image       image.Image
}

// Pictures lists the pictures of packaged Markdown that could be read, each
// once; Markdown whose pictures are files beside it has none to list.
func (md *Markdown) Pictures() []Picture {
	if md == nil || !md.Packaged {
		return nil
	}
	var out []Picture
	seen, taken := map[string]bool{}, map[string]bool{}
	for _, img := range md.Images {
		if img.Image == nil || seen[img.Destination] {
			continue
		}
		seen[img.Destination] = true
		stem := strings.TrimSuffix(path.Base(img.Destination), path.Ext(img.Destination))
		name := stem + ".png"
		for n := 2; taken[name]; n++ {
			name = fmt.Sprintf("%s-%d.png", stem, n)
		}
		taken[name] = true
		out = append(out, Picture{Destination: img.Destination, Name: name, Image: img.Image})
	}
	return out
}

// Relink points the links of the Markdown's pictures elsewhere: to, by
// destination, gives where.
func (md *Markdown) Relink(to map[string]string) []byte {
	pairs := make([]string, 0, 2*len(to))
	for from, where := range to {
		pairs = append(pairs, "]("+from, "]("+where)
	}
	return []byte(strings.NewReplacer(pairs...).Replace(string(md.Source)))
}

type MarkdownImage struct {
	Destination, Alt string
	Image            image.Image
	Err              error
}

// MarkdownTree uses the same dialect for asset discovery and Glamour layout.
func MarkdownTree(source []byte) ast.Node {
	return goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList)).Parser().Parse(text.NewReader(source))
}

func loadMarkdown(path string, data []byte, baseDir string) (*Markdown, error) {
	return loadMarkdownFrom(path, data, baseDir, nil)
}

func loadMarkdownFrom(path string, data []byte, baseDir string, files fs.FS) (*Markdown, error) {
	if len(data) > MaxMarkdownBytes {
		return nil, fmt.Errorf("Markdown exceeds 2 MiB")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("Markdown must be UTF-8")
	}
	// Preserve text whitespace while excluding terminal controls and bidi marks.
	source := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r == '\r' {
			return -1
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return '\ufffd'
		}
		return r
	}, string(data))
	if err := checkMarkdownNesting(source); err != nil {
		return nil, err
	}
	md := &Markdown{Source: []byte(source)}
	if baseDir == "" {
		baseDir = filepath.Dir(path)
	}
	cache := make(map[string]MarkdownImage)
	pixels := 0
	root, err := withDeadline(parseDeadline, "reading the Markdown", func() (ast.Node, error) { return MarkdownTree(md.Source), nil })
	if err != nil {
		return nil, err
	}
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		img, ok := n.(*ast.Image)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		if len(md.Images) >= MaxMarkdownImages {
			return ast.WalkSkipChildren, nil
		}
		dest := string(img.Destination)
		asset, cached := cache[dest]
		if !cached {
			asset.Destination = dest
			resolved, err := localMarkdownPathFrom(baseDir, dest, files)
			asset.Err = err
			if err == nil && pixels >= maxMarkdownImagePixels {
				asset.Err = fmt.Errorf("document image budget reached")
			}
			if asset.Err == nil {
				// Reuse the same bounded image/SVG decoders as standalone files.
				data, err := readFileFrom(files, resolved)
				asset.Err = err
				if err == nil {
					kind, err := Detect(resolved, data, "")
					asset.Err = err
					if err == nil {
						switch kind {
						case "svg":
							asset.Image, asset.Err = renderSVG(resolved, data, 1600)
						case "heic", "image", "png", "jpeg", "gif", "webp", "bmp", "tiff":
							asset.Image, asset.Err = decodeRaster(data)
						default:
							asset.Err = fmt.Errorf("embedded format %s is not an image or SVG", kind)
						}
					}
				}
				if asset.Err == nil {
					asset.Image, asset.Err = ExportImage(Result{Image: asset.Image}, 1600)
					if asset.Err == nil {
						area := asset.Image.Bounds().Dx() * asset.Image.Bounds().Dy()
						if pixels+area > maxMarkdownImagePixels {
							asset.Image = nil
							asset.Err = fmt.Errorf("document image budget reached")
						} else {
							pixels += area
						}
					}
				}
			}
			cache[dest] = asset
		}
		asset.Alt = inlineText(img, md.Source)
		md.Images = append(md.Images, asset)
		return ast.WalkSkipChildren, nil
	})
	return md, nil
}

// inlineText is the text of a node's inline children, what the deprecated
// ast.BaseNode.Text gathered.
func inlineText(n ast.Node, source []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Value(source))
			if t.SoftLineBreak() {
				b.WriteByte('\n')
			}
		case *ast.String:
			b.Write(t.Value)
		default:
			b.WriteString(inlineText(c, source))
		}
	}
	return b.String()
}

func localMarkdownPathFrom(base, destination string, files fs.FS) (string, error) {
	u, err := url.Parse(destination)
	if err != nil {
		return "", fmt.Errorf("invalid image path: %w", err)
	}
	if (u.Scheme != "" && u.Scheme != "file") || (u.Host != "" && !(u.Scheme == "file" && u.Host == "localhost")) {
		return "", fmt.Errorf("remote and data images are not loaded")
	}
	if u.Path == "" {
		return "", fmt.Errorf("empty image path")
	}
	path := filepath.FromSlash(u.Path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	var info fs.FileInfo
	if files == nil {
		info, err = os.Stat(path)
	} else {
		info, err = fs.Stat(files, filepath.ToSlash(path))
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("image is not a regular file")
	}
	if files != nil {
		path = filepath.ToSlash(path)
	}
	return path, nil
}
