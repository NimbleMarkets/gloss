package document

import (
	"image"
	"image/color"
	"testing"
)

func TestMosaicLaysTilesOutWithLabelsAndAFrame(t *testing.T) {
	red := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			red.Set(x, y, color.RGBA{R: 200, A: 255})
		}
	}
	tiles := []Tile{
		{Name: "one.png", Image: red},
		{Name: "two.csv", Kind: "csv"},
		{Name: "three.pdf", Loading: true},
		{Name: "four.bin", Failed: true},
		{Name: "five.png", Image: red},
	}
	m := Mosaic(tiles, 2, 100, 60, 4)
	if m.Bounds().Dx() != 200 || m.Bounds().Dy() != 180 {
		t.Fatalf("size %v", m.Bounds())
	}
	// The picture fills the tile's width, letterboxed, above the label row.
	if c := m.RGBAAt(50, 20); c.R != 200 || c.G != 0 {
		t.Errorf("no picture in the first tile: %v", c)
	}
	// The frame around the selected tile is not the background.
	if c := m.RGBAAt(1, 121); c == MosaicBackground {
		t.Errorf("the selected tile has no frame: %v", c)
	}
	if c := m.RGBAAt(101, 1); c != MosaicBackground {
		t.Errorf("an unselected tile is framed: %v", c)
	}
	// Labels and stand-ins put ink where there is no picture.
	inked := func(r image.Rectangle) bool {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if m.RGBAAt(x, y) != MosaicBackground {
					return true
				}
			}
		}
		return false
	}
	if !inked(image.Rect(100, 10, 200, 50)) || !inked(image.Rect(0, 70, 100, 110)) || !inked(image.Rect(100, 70, 200, 110)) {
		t.Error("a stand-in tile is blank")
	}
	if !inked(image.Rect(0, 48, 100, 60)) {
		t.Error("the label row is blank")
	}
	if got := Mosaic(nil, 2, 100, 60, 0); got.Bounds().Dx() != 200 || got.Bounds().Dy() != 60 {
		t.Errorf("empty mosaic %v", got.Bounds())
	}
}
