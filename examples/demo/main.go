// The demo runs the real gloss pager against an embedded, read-only filesystem.
package main

import (
	"fmt"
	"os"
	"runtime"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/examples"
	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
	booba "github.com/NimbleMarkets/go-booba"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/spf13/pflag"
)

// options opens the gallery on its menu, or on one sample with the others
// after it. It reports false for a sample the gallery does not hold.
func options(sample string) (app.Options, bool) {
	files := append([]string(nil), examples.Names...)
	if sample != "" {
		i := slices.Index(files, sample)
		if i < 0 {
			return app.Options{}, false
		}
		files = append(files[i:], files[:i]...)
	}
	camera := charts.DefaultCamera()
	camera.Distance = 1.6 // Frame the wide block-letter sculpture more closely.
	// Meshes are drawn with WebGPU where the browser has it; NTCharts3d
	// falls back to software, and then wireframe, where it does not.
	return app.Options{Save: saveExport, Files: files, STLCamera: &camera, Render: "auto", Render3D: "auto", Page: 1, DPI: 96, Menu: sample == "", Preview: sample == ""}, true
}

func main() {
	sample := pflag.String("sample", "", "initial embedded sample filename")
	pflag.Parse()
	settings, ok := options(*sample)
	if !ok {
		fmt.Fprintln(os.Stderr, "unknown sample:", *sample)
		return
	}
	dropped := &document.Overlay{Base: examples.Files}
	settings.FilesFS = dropped
	m := app.New(settings)
	var opts []tea.ProgramOption
	if runtime.GOOS == "js" {
		opts = append(opts, tea.WithoutSignalHandler())
	}
	p := booba.NewProgram(m, opts...)
	acceptDrops(p.Send, dropped)
	_, err := p.Run()
	_ = m.Close()
	for _, line := range m.Skipped() {
		fmt.Fprintln(os.Stderr, line)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	holdRuntime()
}
