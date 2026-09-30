package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
)

// The screens and layers, and the keys that move between them.
func TestScreensAndLayers(t *testing.T) {
	dir := folder(t)
	m := viewing(t, Options{Files: []string{samples + "shapes.svg", samples + "readme.md"}}, "svg")
	send(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	at := func(step string, want screen, layer layer) {
		t.Helper()
		if m.screen != want || m.layer != layer {
			t.Fatalf("%s: screen=%d layer=%d, want %d %d", step, m.screen, m.layer, want, layer)
		}
	}
	at("start", screenDocument, layerNone)
	send(m, press("m"))
	at("m", screenList, layerNone)
	send(m, press("t"))
	at("t", screenGrid, layerNone)
	send(m, press("t"))
	at("t again", screenList, layerNone)
	send(m, escape)
	at("Esc from the list", screenDocument, layerNone)
	send(m, escape)
	at("Esc from a document", screenList, layerNone)
	send(m, press("t"), escape)
	at("Esc from the grid", screenDocument, layerNone)
	// The browser is a screen of its own; its Esc unwinds the finder and
	// the go-to before it closes.
	send(m, press("O"))
	at("O", screenBrowser, layerNone)
	send(m, press("/"), escape)
	at("Esc from the finder", screenBrowser, layerNone)
	send(m, press("G"), escape)
	at("Esc from the go-to", screenBrowser, layerNone)
	send(m, escape)
	at("Esc from the browser", screenDocument, layerNone)
	_ = dir
	// Help lies over any screen and Esc takes it down, leaving the screen.
	send(m, press("m"))
	before := m.screen // The list, in the style last used.
	send(m, press("?"))
	if !m.help {
		t.Fatal("no help")
	}
	send(m, escape)
	at("Esc from help over the list", before, layerNone)
	if m.help {
		t.Fatal("help stayed")
	}
	send(m, escape)
	at("back to the document", screenDocument, layerNone)
	// Layers over the document: one at a time, Esc closes, i replaces.
	send(m, press("i"))
	at("i", screenDocument, layerInfo)
	send(m, press("i"))
	at("i again", screenDocument, layerNone)
	send(m, press("i"), escape)
	at("Esc from the info box", screenDocument, layerNone)
}

func TestOneLayerAtATime(t *testing.T) {
	m, _ := assembled(t)
	send(m, press("c"))
	if m.layer != layerParts || m.partPicker == nil {
		t.Fatalf("c: layer=%d", m.layer)
	}
	send(m, press("i"))
	if m.layer != layerInfo || m.partPicker != nil {
		t.Fatalf("i over the parts: layer=%d picker=%v", m.layer, m.partPicker != nil)
	}
	send(m, press("C"))
	if m.layer != layerColor || m.colorPicker == nil {
		t.Fatalf("C: layer=%d", m.layer)
	}
	send(m, press("c"))
	if m.layer != layerColor {
		t.Fatalf("c while choosing a color went to the parts: layer=%d", m.layer)
	}
	send(m, escape)
	if m.layer != layerNone || m.colorPicker != nil {
		t.Fatalf("Esc: layer=%d", m.layer)
	}
	// Opening the list takes any layer down.
	send(m, press("c"), press("m"))
	if m.screen != screenList || m.layer != layerNone || m.partPicker != nil {
		t.Fatalf("m over the parts: screen=%d layer=%d", m.screen, m.layer)
	}
	// A sheet's column list is a layer too.
	s := tabulated(t, table(3, 3), 1, 1)
	send(s, press("c"))
	if s.layer != layerColumns || s.sheet.picker == nil {
		t.Fatalf("c on a sheet: layer=%d", s.layer)
	}
	send(s, enter)
	if s.layer != layerNone || s.sheet.picker != nil {
		t.Fatalf("Enter: layer=%d", s.layer)
	}
	// A new document takes the layer down with the old one.
	send(s, press("c"))
	s.Update(document.Result{Generation: s.generation + 1, Kind: "csv", Page: 1, Pages: 1, Sheet: table(2, 2)})
	s.generation++
	if s.layer == layerColumns && s.sheet.picker == nil {
		t.Fatal("the layer outlived its picker")
	}
}
