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
	// Meshes are drawn with WebGPU where the browser has it; NTCharts3d
	// falls back to software, and then wireframe, where it does not. Each
	// starts fitted to its frame.
	return app.Options{Save: saveExport, Files: files, Render: "auto", Render3D: "auto", Page: 1, DPI: 96, Menu: sample == "", Preview: sample == ""}, true
}

// appOptions opens the app: nothing embedded, a drop target for the
// visitor's own files, which the page hands over.
func appOptions() (app.Options, bool) {
	opts, ok := options("")
	opts.Files, opts.Menu, opts.Preview = nil, false, false
	return opts, ok
}

func main() {
	sample := pflag.String("sample", "", "initial embedded sample filename")
	appMode := pflag.Bool("app", false, "start empty, for the visitor's own files, with no samples")
	accept := pflag.String("accept", "", "required content formats")
	pflag.Parse()
	settings, ok := options(*sample)
	if *appMode {
		settings, ok = appOptions()
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "unknown sample:", *sample)
		return
	}
	filter, err := document.ParseAccept(*accept)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	settings.Accept = filter
	dropped := &document.Overlay{Base: examples.Files}
	settings.FilesFS = dropped
	m := app.New(settings)
	var opts []tea.ProgramOption
	if runtime.GOOS == "js" {
		opts = append(opts, tea.WithoutSignalHandler())
	}
	p := booba.NewProgram(m, opts...)
	acceptDrops(p.Send, dropped, filter)
	_, err = p.Run()
	_ = m.Close()
	for _, line := range m.Skipped() {
		fmt.Fprintln(os.Stderr, line)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	holdRuntime()
}
