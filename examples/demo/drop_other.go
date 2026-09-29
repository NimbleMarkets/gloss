//go:build !js || !wasm

package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
)

func acceptDrops(func(tea.Msg), *document.Overlay) {}
