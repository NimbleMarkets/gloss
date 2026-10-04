package app

import (
	"image"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
)

// Each picture shown with Kitty graphics goes under an id of its own: the
// browser app's terminal would otherwise keep showing the last picture of the
// same size. Glyphs have no ids to change.
func TestEachKittyPictureHasItsOwnID(t *testing.T) {
	t.Cleanup(func() { picture.ForceKittyCapability(picture.KittyCapabilityUnknown) })
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	glyph := New(Options{Files: []string{"a.png"}, Render: "glyph", Page: 1, DPI: 72})
	defer glyph.Close()
	first := glyph.picID
	if glyph.showPicture(img); glyph.picID != first {
		t.Fatal("a glyph picture changed its id")
	}
	m := New(Options{Files: []string{"a.png"}, Render: "kitty", Page: 1, DPI: 72})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	first = m.picID
	m.showPicture(img)
	second := m.picID
	m.source, m.zoom = img, 1
	m.refreshImage()
	if second == first || m.picID == second || m.pic.Mode() != picture.PictureKitty {
		t.Fatalf("ids %d, %d, %d; mode %v", first, second, m.picID, m.pic.Mode())
	}
	if w, h := m.pic.CellPixelSize(); w <= 0 || h <= 0 {
		t.Fatalf("cell size lost: %d×%d", w, h)
	}
}
