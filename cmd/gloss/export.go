package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/ntcharts-svg/svg"
)

func exportRequest(opts app.Options, path string, generation int) document.Request {
	return document.Request{Path: path, Type: opts.Type, Page: opts.Page, DPI: opts.DPI, MaxEdge: opts.MaxEdge, Generation: uint64(generation), Parts: opts.Parts, Color: opts.Color, Stdin: opts.IsStdin(path), Strict: true}
}

// onCPU reports whether meshes are to be drawn without the GPU: --3d names
// a renderer, and any but auto is one of the CPU's.
func onCPU(opts app.Options) bool { return opts.Render3D != "auto" && opts.Render3D != "" }

// A made file, or a text, as the manifest lists it: one line of JSON per
// page of each input, with what went wrong where something did.
type made struct {
	Path   string `json:"path"`
	Kind   string `json:"kind,omitempty"`
	Page   int    `json:"page,omitempty"`
	Pages  int    `json:"pages,omitempty"`
	Output string `json:"output,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	// What the size was resolved to; the profile and its reason only when one was named.
	MaxEdge       int    `json:"max_edge,omitempty"`
	VisionProfile string `json:"vision_profile,omitempty"`
	VisionReason  string `json:"vision_reason,omitempty"`
	Text          string `json:"text,omitempty"`
	// What was made instead of what was asked, as a 3MF's thumbnail for a
	// mesh too large to draw.
	Note  string `json:"note,omitempty"`
	Error string `json:"error,omitempty"`
}

// reported is an error already written to stderr, beside the file it
// concerns: main exits with it, and says it no more.
type reported struct{ error }

func (r reported) Unwrap() error { return r.error }

// once marks the first failure of a batch as reported.
func once(err error) error {
	if err == nil {
		return nil
	}
	return reported{err}
}

// exportFiles draws each input, and each page asked for, as a PNG. The
// answer goes to stdout: the bytes with --output -, else the paths
// written, one to a line, or the manifest with --json. A file that fails
// is reported and the rest go on; the error is the first one's.
func exportFiles(opts options, stdout, stderr io.Writer) error {
	loader := &document.Loader{}
	defer loader.Close()
	if opts.OutputDir != "" {
		if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
			return err
		}
	}
	var manifest []made
	var failed error
	generation := 0
	for i, path := range opts.Files {
		for _, page := range pagesOf(loader, opts, path, &generation) {
			generation++
			q := exportRequest(opts.Options, path, generation)
			q.Page = page
			r := loader.Load(q)
			entry := made{Path: path, Kind: r.Kind, Page: r.Page, Pages: r.Pages, MaxEdge: opts.MaxEdge, VisionProfile: opts.VisionProfile, VisionReason: opts.EdgeReason}
			r.CPU, r.Views = onCPU(opts.Options), opts.Views
			img, err := document.ExportImage(r, opts.MaxEdge)
			if err == nil && r.Standin != "" {
				entry.Note = r.Standin + standinLoses(opts)
				fmt.Fprintf(stderr, "gloss: %s: %s\n", svg.SanitizeForTerminal(path), svg.SanitizeForTerminal(entry.Note))
			}
			if err == nil {
				var data bytes.Buffer
				if err = png.Encode(&data, img); err == nil {
					entry.Width, entry.Height = img.Bounds().Dx(), img.Bounds().Dy()
					entry.Output, err = deliver(opts, i, path, r, ".png", data.Bytes(), stdout)
				}
			}
			if err != nil {
				entry.Error = err.Error()
				fmt.Fprintf(stderr, "gloss: %s: %s\n", svg.SanitizeForTerminal(path), svg.SanitizeForTerminal(err.Error()))
				if failed == nil {
					failed = fmt.Errorf("%s: %w", path, err)
				}
			} else if entry.Output != "" {
				fmt.Fprintf(stderr, "%s: %d×%d PNG\n", svg.SanitizeForTerminal(entry.Output), entry.Width, entry.Height)
				if !opts.JSON {
					fmt.Fprintln(stdout, entry.Output)
				}
			}
			manifest = append(manifest, entry)
		}
	}
	if opts.JSON {
		if err := json.NewEncoder(stdout).Encode(manifest); err != nil {
			return err
		}
	}
	return once(failed)
}

// standinLoses says what of the request a stand-in picture leaves out.
func standinLoses(opts options) string {
	var lost []string
	if len(opts.Views) > 0 {
		lost = append(lost, "the views asked for")
	}
	if opts.Color != nil {
		lost = append(lost, "--color")
	}
	if len(lost) == 0 {
		return "; exported it instead of the mesh"
	}
	return "; exported it instead of the mesh, without " + strings.Join(lost, " or ")
}

// textFiles takes the text out of each input, and each page asked for:
// to stdout as it is for one input, to files with --output or
// --output-dir, whose paths are then the answer, or to a manifest with
// --json.
func textFiles(opts options, stdout, stderr io.Writer) error {
	loader := &document.Loader{}
	defer loader.Close()
	if opts.OutputDir != "" {
		if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
			return err
		}
	}
	var manifest []made
	var failed error
	generation, streamed := 0, 0
	for i, path := range opts.Files {
		for _, page := range pagesOf(loader, opts, path, &generation) {
			generation++
			q := exportRequest(opts.Options, path, generation)
			q.Page, q.TextOnly = page, true
			r := loader.Load(q)
			entry := made{Path: path, Kind: r.Kind, Page: r.Page, Pages: r.Pages}
			text, ext, err := document.Text(r)
			pictures := r.Markdown.Pictures()
			toFile := (opts.Output != "" || opts.OutputDir != "") && !opts.JSON && opts.Output != "-"
			if err == nil && len(pictures) > 0 && !toFile {
				entry.Note = fmt.Sprintf("%d %s not written: the links name files inside the %s; --text --output-dir writes them beside the text", len(pictures), plural(len(pictures), "picture"), map[string]string{"ipynb": "notebook", "docx": "Word document"}[r.Kind])
				fmt.Fprintf(stderr, "gloss: %s: %s\n", svg.SanitizeForTerminal(path), entry.Note)
			}
			switch {
			case err != nil:
			case opts.JSON:
				entry.Text = string(text)
			case toFile && len(pictures) > 0:
				entry.Output, err = deliverWithPictures(opts, i, path, r, ext, pictures)
				if err == nil {
					fmt.Fprintln(stdout, entry.Output)
				}
			case opts.Output != "" || opts.OutputDir != "":
				entry.Output, err = deliver(opts, i, path, r, ext, text, stdout)
				if err == nil {
					fmt.Fprintln(stdout, entry.Output)
				}
			default:
				// One stream holds one text: several would run together.
				if streamed++; streamed > 1 {
					err = fmt.Errorf("several texts: write them with --output-dir, or ask for --json")
				} else {
					_, err = stdout.Write(text)
				}
			}
			if err != nil {
				entry.Error = err.Error()
				fmt.Fprintf(stderr, "gloss: %s: %s\n", svg.SanitizeForTerminal(path), svg.SanitizeForTerminal(err.Error()))
				if failed == nil {
					failed = fmt.Errorf("%s: %w", path, err)
				}
			}
			manifest = append(manifest, entry)
		}
	}
	if opts.JSON {
		if err := json.NewEncoder(stdout).Encode(manifest); err != nil {
			return err
		}
	}
	return once(failed)
}

// pagesOf lists the pages of path to be made: those asked for, of as many
// as the document has, which a range means finding out first.
func pagesOf(loader *document.Loader, opts options, path string, generation *int) []int {
	if page, ok := opts.Pages.single(); ok {
		return []int{page}
	}
	if !opts.Pages.all && len(opts.Pages.pages) == 0 {
		return []int{max(1, opts.Page)} // Nothing asked: the page as set.
	}
	*generation++
	q := exportRequest(opts.Options, path, *generation)
	q.Page, q.TextOnly = 1, opts.Text
	r := loader.Load(q)
	if r.Pages < 1 { // A page without text still counts its siblings.
		return []int{1} // The failure is reported by the page's own load.
	}
	return opts.Pages.of(r.Pages)
}

// deliver puts data where it was asked for: to --output as named, into
// --output-dir under a name made from the input's, or, for --output -, to
// stdout. It gives the path written, or nothing for stdout.
func deliver(opts options, i int, path string, r document.Result, ext string, data []byte, stdout io.Writer) (string, error) {
	target := targetFor(opts, i, path, r, ext)
	if target == "-" {
		_, err := stdout.Write(data)
		return "", err
	}
	return document.WriteNew(target, data)
}

// deliverWithPictures writes packaged Markdown with its pictures: they go,
// as PNGs, into a folder named after the text, beside it, and the text's
// links are made to name them there. It gives the text's path.
func deliverWithPictures(opts options, i int, path string, r document.Result, ext string, pictures []document.Picture) (string, error) {
	target := targetFor(opts, i, path, r, ext)
	dir, err := document.MkdirNew(strings.TrimSuffix(target, ext) + "-pictures")
	if err != nil {
		return "", err
	}
	links := map[string]string{}
	for _, picture := range pictures {
		var data bytes.Buffer
		if err := png.Encode(&data, picture.Image); err != nil {
			return "", err
		}
		written, err := document.WriteNew(filepath.Join(dir, picture.Name), data.Bytes())
		if err != nil {
			return "", err
		}
		links[picture.Destination] = filepath.Base(dir) + "/" + filepath.Base(written)
	}
	return document.WriteNew(target, r.Markdown.Relink(links))
}

// targetFor names what is written for an input: --output as given, or a
// name in --output-dir made from the input's, its page or sheet, and ext.
func targetFor(opts options, i int, path string, r document.Result, ext string) string {
	if opts.OutputDir == "" {
		return opts.Output
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if opts.IsStdin(path) {
		base = "stdin"
	}
	suffix := ""
	switch {
	case r.Kind == "pdf" && r.Pages > 1:
		suffix = fmt.Sprintf("-page-%d", r.Page)
	case r.Kind == "xlsx" && r.Pages > 1:
		suffix = fmt.Sprintf("-sheet-%d", r.Page)
	case r.Kind == "grist" && r.Pages > 1:
		suffix = fmt.Sprintf("-table-%d", r.Page)
	}
	return filepath.Join(opts.OutputDir, fmt.Sprintf("%03d-%s%s%s", i+1, base, suffix, ext))
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
