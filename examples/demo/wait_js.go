//go:build js && wasm

package main

import "syscall/js"

func holdRuntime() {
	noop := js.FuncOf(func(js.Value, []js.Value) any { return "" })
	for _, name := range []string{"bubbletea_read", "bubbletea_write", "bubbletea_resize"} {
		js.Global().Set(name, noop)
	}
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("gloss-exit"))
	select {}
}
