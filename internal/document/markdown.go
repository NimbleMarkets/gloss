package document

import (
	"fmt"
	"image"
	"io/fs"
	"net/url"
	"os"
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
		asset.Alt = string(img.Text(md.Source))
		md.Images = append(md.Images, asset)
		return ast.WalkSkipChildren, nil
	})
	return md, nil
}

func localMarkdownPath(base, destination string) (string, error) {
	return localMarkdownPathFrom(base, destination, nil)
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
