// The demo runs the real gloss pager against an embedded, read-only filesystem.
package main

import (
	"fmt"
	"os"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/examples"
	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
	booba "github.com/NimbleMarkets/go-booba"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/spf13/pflag"
)

func main() {
	sample := pflag.String("sample", "", "initial embedded sample filename")
	pflag.Parse()
	files := append([]string(nil), examples.Names...)
	if *sample != "" {
		found := false
		for i, name := range files {
			if name == *sample {
				files = append(files[i:], files[:i]...)
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintln(os.Stderr, "unknown sample:", *sample)
			return
		}
	}
	camera := charts.DefaultCamera()
	camera.Distance = 1.6 // Frame the wide block-letter sculpture more closely.
	dropped := &document.Overlay{Base: examples.Files}
	m := app.New(app.Options{Files: files, FilesFS: dropped, STLCamera: &camera, Render: "auto", Render3D: "software", Page: 1, DPI: 96, Menu: *sample == "", Preview: *sample == ""})
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
