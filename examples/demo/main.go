// The demo runs the real gloss pager against an embedded, read-only filesystem.
package main

import (
	"fmt"
	"os"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/examples"
	"github.com/NimbleMarkets/gloss/internal/app"
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
	m := app.New(app.Options{Files: files, FilesFS: examples.Files, STLCamera: &camera, Render: "auto", Render3D: "software", Page: 1, DPI: 96, Menu: *sample == "", Preview: *sample == ""})
	var opts []tea.ProgramOption
	if runtime.GOOS == "js" {
		opts = append(opts, tea.WithoutSignalHandler())
	}
	err := booba.Run(m, opts...)
	_ = m.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	holdRuntime()
}
