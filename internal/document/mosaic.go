package document

import (
	"image"
	"image/color"
	"image/draw"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Tile is one file in a mosaic: its thumbnail when there is one, else a
// stand-in that says what it is, or that it is still to come, or failed.
type Tile struct {
	Name    string
	Image   image.Image
	Kind    string // Shown in place of a picture, for a file that has none.
	Loading bool
	Failed  bool
}

// MosaicBackground is the color between and behind the tiles.
var MosaicBackground = color.RGBA{R: 24, G: 26, B: 30, A: 255}

var (
	mosaicFrame = color.RGBA{R: 250, G: 200, B: 80, A: 255}
	mosaicInk   = color.RGBA{R: 200, G: 204, B: 210, A: 255}
	mosaicDim   = color.RGBA{R: 110, G: 116, B: 124, A: 255}
	mosaicPane  = color.RGBA{R: 38, G: 41, B: 47, A: 255}
)

// Mosaic lays the tiles out in rows of cols, each tileW by tileH pixels,
// with a label row at the foot of each, and a frame around the one
// selected. It is one picture, so that a terminal shows it as one.
func Mosaic(tiles []Tile, cols, tileW, tileH, selected int) *image.RGBA {
	cols = max(1, cols)
	rows := max(1, (len(tiles)+cols-1)/cols)
	out := image.NewRGBA(image.Rect(0, 0, cols*tileW, rows*tileH))
	draw.Draw(out, out.Bounds(), image.NewUniform(MosaicBackground), image.Point{}, draw.Src)
	face := basicfont.Face7x13
	labelH := min(tileH/4, face.Height+6)
	for i, tile := range tiles {
		if tile == (Tile{}) {
			continue // A place kept empty, to fill a row.
		}
		at := image.Rect(0, 0, tileW, tileH).Add(image.Pt(i%cols*tileW, i/cols*tileH))
		pane := image.Rect(at.Min.X+3, at.Min.Y+3, at.Max.X-3, at.Max.Y-labelH)
		switch {
		case tile.Image != nil && !tile.Image.Bounds().Empty():
			fit(out, pane, tile.Image)
		default:
			draw.Draw(out, pane, image.NewUniform(mosaicPane), image.Point{}, draw.Src)
			word := strings.ToUpper(tile.Kind)
			ink := mosaicInk
			switch {
			// The font has the ASCII letters only.
			case tile.Loading:
				word, ink = "...", mosaicDim
			case tile.Failed:
				word, ink = "x", mosaicDim
			case word == "":
				word = "?"
			}
			center(out, pane, word, ink, max(1, min(pane.Dx()/(len(word)*face.Advance+1), pane.Dy()/face.Height/2)))
		}
		name := tile.Name
		if limit := (tileW - 6) / face.Advance; len(name) > limit && limit > 1 {
			name = name[:limit-1] + "…"
		}
		text := image.Rect(at.Min.X+3, at.Max.Y-labelH+(labelH-face.Height)/2, at.Max.X-3, at.Max.Y)
		center(out, text, name, mosaicInk, 1)
		if i == selected {
			frame(out, at, 2, mosaicFrame)
		}
	}
	return out
}

// fit draws img into r, letterboxed, as large as it goes.
func fit(dst *image.RGBA, r image.Rectangle, img image.Image) {
	b := img.Bounds()
	if b.Empty() || r.Empty() {
		return
	}
	scale := min(float64(r.Dx())/float64(b.Dx()), float64(r.Dy())/float64(b.Dy()))
	w, h := max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))
	at := image.Rect(0, 0, w, h).Add(r.Min).Add(image.Pt((r.Dx()-w)/2, (r.Dy()-h)/2))
	xdraw.ApproxBiLinear.Scale(dst, at, img, b, draw.Over, nil)
}

// center writes text in the middle of r, its letters scaled up by scale.
func center(dst *image.RGBA, r image.Rectangle, text string, ink color.RGBA, scale int) {
	face := basicfont.Face7x13
	src := image.NewRGBA(image.Rect(0, 0, len(text)*face.Advance, face.Height))
	(&font.Drawer{Dst: src, Src: image.NewUniform(ink), Face: face, Dot: fixed.P(0, face.Ascent)}).DrawString(text)
	w, h := src.Bounds().Dx()*scale, src.Bounds().Dy()*scale
	at := image.Rect(0, 0, w, h).Add(r.Min).Add(image.Pt((r.Dx()-w)/2, (r.Dy()-h)/2)).Intersect(r)
	xdraw.NearestNeighbor.Scale(dst, at, src, src.Bounds(), draw.Over, nil)
}

// frame draws a border of the given thickness just inside r.
func frame(dst *image.RGBA, r image.Rectangle, thick int, c color.RGBA) {
	paint := image.NewUniform(c)
	draw.Draw(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+thick), paint, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(r.Min.X, r.Max.Y-thick, r.Max.X, r.Max.Y), paint, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+thick, r.Max.Y), paint, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(r.Max.X-thick, r.Min.Y, r.Max.X, r.Max.Y), paint, image.Point{}, draw.Src)
}
