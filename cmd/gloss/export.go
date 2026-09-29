package main

import (
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
	return document.Request{Path: path, Type: opts.Type, Page: opts.Page, DPI: opts.DPI, MaxEdge: opts.MaxEdge, Generation: uint64(generation)}
}

// onCPU reports whether meshes are to be drawn without the GPU: --3d names
// a renderer, and any but auto is one of the CPU's.
func onCPU(opts app.Options) bool { return opts.Render3D != "auto" && opts.Render3D != "" }

func exportFiles(opts app.Options, stdout, stderr io.Writer) error {
	loader := &document.Loader{}
	defer loader.Close()
	if opts.OutputDir != "" {
		if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
			return err
		}
	}
	for i, path := range opts.Files {
		r := loader.Load(exportRequest(opts, path, i+1))
		r.CPU = onCPU(opts)
		img, err := document.ExportForVision(r, opts.MaxEdge, opts.VisionProfile)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		target := opts.Output
		if opts.OutputDir != "" {
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if strings.HasPrefix(base, "gloss-stdin-") {
				base = "stdin"
			}
			suffix := ""
			if r.Kind == "pdf" {
				suffix = fmt.Sprintf("-page-%d", r.Page)
			}
			target = filepath.Join(opts.OutputDir, fmt.Sprintf("%03d-%s%s.png", i+1, base, suffix))
		}
		if target == "-" {
			if err := png.Encode(stdout, img); err != nil {
				return err
			}
		} else {
			// Never overwrite an input or an earlier export accidentally.
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				return err
			}
			encodeErr := png.Encode(f, img)
			closeErr := f.Close()
			if encodeErr != nil || closeErr != nil {
				_ = os.Remove(target)
				if encodeErr != nil {
					return encodeErr
				}
				return closeErr
			}
		}
		fmt.Fprintf(stderr, "%s: %d×%d PNG\n", svg.SanitizeForTerminal(target), img.Bounds().Dx(), img.Bounds().Dy())
	}
	return nil
}
