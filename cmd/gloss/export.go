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
	return document.Request{Path: path, Type: opts.Type, Page: opts.Page, DPI: opts.DPI, MaxEdge: opts.MaxEdge, Generation: uint64(generation), Parts: opts.Parts, Color: opts.Color, Stdin: opts.IsStdin(path)}
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
	Error         string `json:"error,omitempty"`
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
	return failed
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
			switch {
			case err != nil:
			case opts.JSON:
				entry.Text = string(text)
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
	return failed
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
	target := opts.Output
	if opts.OutputDir != "" {
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
		target = filepath.Join(opts.OutputDir, fmt.Sprintf("%03d-%s%s%s", i+1, base, suffix, ext))
	}
	if target == "-" {
		_, err := stdout.Write(data)
		return "", err
	}
	return document.WriteNew(target, data)
}
