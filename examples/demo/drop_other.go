//go:build !js || !wasm

package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
)

func acceptDrops(func(tea.Msg), *document.Overlay) {}

// Nil keeps the pager's default: exports are written to the working directory.
var saveExport func(name string, png []byte) (string, error)
