//go:build js && wasm

package main

import (
	"fmt"
	"os"
	"syscall/js"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
)

// acceptDrops registers window.gloss_drop(names, contents) beside Booba's
// bubbletea_resize. The browser has no disk, so the page hands over the bytes
// and the pager is told where they were kept.
func acceptDrops(send func(tea.Msg), files *document.Overlay) {
	js.Global().Set("gloss_drop", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 {
			return 0
		}
		var paths []string
		for i := 0; i < args[0].Length() && i < args[1].Length(); i++ {
			name, content := args[0].Index(i).String(), args[1].Index(i)
			// Check before allocating: the page's own limit is advisory.
			if content.Length() > document.MaxFileBytes {
				fmt.Fprintln(os.Stderr, document.Skipped(name, document.ErrTooLarge))
				continue
			}
			data := make([]byte, content.Length())
			js.CopyBytesToGo(data, content)
			path, err := files.Add(name, data)
			if err != nil {
				fmt.Fprintln(os.Stderr, document.Skipped(name, err))
				continue
			}
			paths = append(paths, path)
		}
		if len(paths) > 0 {
			// Send blocks until the update loop is ready; do not stall the page.
			go send(app.DropMsg{Paths: paths})
		}
		return len(paths)
	}))
}

// saveExport hands an export to the page, which offers it as a download.
func saveExport(name string, png []byte) (string, error) {
	save := js.Global().Get("gloss_save")
	if save.Type() != js.TypeFunction {
		return "", fmt.Errorf("this page cannot save files")
	}
	data := js.Global().Get("Uint8Array").New(len(png))
	js.CopyBytesToJS(data, png)
	save.Invoke(name, data)
	return name, nil
}
