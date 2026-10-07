package document

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"time"
)

// Bound DecodeAll before it allocates: compressed GIFs can describe thousands
// of large frames with very little input. Frames stay paletted; only the
// current composition and a disposal-previous snapshot are RGBA canvases.
const MaxGIFFrames = 1000
const MaxGIFPixels = 64 << 20 // Sum of frame rectangles, one byte per pixel.

// GIFPlayer is an immutable playback position. Next composes a fresh canvas;
// render/export commands may retain any earlier Image without a data race.
// The decoded palettes and frame rectangles are shared between positions.
type GIFPlayer struct {
	data           *gif.GIF
	canvas         *image.RGBA
	restore        *image.RGBA
	frame, repeats int
}

func (p *GIFPlayer) Image() image.Image { return p.canvas }
func (p *GIFPlayer) Frame() int         { return p.frame + 1 }
func (p *GIFPlayer) Frames() int        { return len(p.data.Image) }
func (p *GIFPlayer) Delay() time.Duration {
	delay := p.data.Delay[p.frame]
	// Zero/one-centisecond delays commonly mean an unspecified delay. Avoid
	// spinning or saturating the terminal; all other delays are preserved.
	if delay < 2 {
		return 100 * time.Millisecond
	}
	return time.Duration(delay) * 10 * time.Millisecond
}

func (p *GIFPlayer) Restart() *GIFPlayer { return newGIFPlayer(p.data) }

// Next honors Go's GIF loop semantics: -1 plays once, 0 repeats forever,
// and a positive count repeats that many times after the first pass.
func (p *GIFPlayer) Next() (*GIFPlayer, bool) {
	n := *p
	n.frame++
	if n.frame == len(p.data.Image) {
		if p.data.LoopCount < 0 || (p.data.LoopCount > 0 && p.repeats >= p.data.LoopCount) {
			return p, false
		}
		n = *newGIFPlayer(p.data)
		n.repeats = p.repeats + 1
		return &n, true
	}
	n.canvas = image.NewRGBA(p.canvas.Bounds())
	draw.Draw(n.canvas, n.canvas.Bounds(), p.canvas, image.Point{}, draw.Src)
	switch p.data.Disposal[p.frame] {
	case gif.DisposalBackground:
		draw.Draw(n.canvas, p.data.Image[p.frame].Bounds(), image.NewUniform(gifBackground(p.data)), image.Point{}, draw.Src)
	case gif.DisposalPrevious:
		if p.restore != nil {
			draw.Draw(n.canvas, n.canvas.Bounds(), p.restore, image.Point{}, draw.Src)
		}
	}
	n.paint()
	return &n, true
}

func newGIFPlayer(data *gif.GIF) *GIFPlayer {
	p := &GIFPlayer{data: data, canvas: image.NewRGBA(image.Rect(0, 0, data.Config.Width, data.Config.Height))}
	draw.Draw(p.canvas, p.canvas.Bounds(), image.NewUniform(gifBackground(data)), image.Point{}, draw.Src)
	p.paint()
	return p
}

func (p *GIFPlayer) paint() {
	p.restore = nil
	if p.data.Disposal[p.frame] == gif.DisposalPrevious {
		p.restore = image.NewRGBA(p.canvas.Bounds())
		draw.Draw(p.restore, p.restore.Bounds(), p.canvas, image.Point{}, draw.Src)
	}
	frame := p.data.Image[p.frame]
	draw.Draw(p.canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
}

// Transparent animations use a transparent canvas, as image viewers do.
// Opaque GIFs use the logical-screen background color when one is defined.
func gifBackground(g *gif.GIF) color.Color {
	for _, c := range g.Image[0].Palette {
		if _, _, _, a := c.RGBA(); a == 0 {
			return color.Transparent
		}
	}
	if palette, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(palette) {
		return palette[g.BackgroundIndex]
	}
	return color.Transparent
}

func decodeGIF(data []byte, animate bool) (image.Image, *GIFPlayer, error) {
	if len(data) > MaxFileBytes {
		return nil, nil, ErrTooLarge
	}
	cfg, err := gif.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxPixels/cfg.Height {
		return nil, nil, fmt.Errorf("GIF canvas exceeds %d pixels", MaxPixels)
	}
	var g *gif.GIF
	if animate {
		if err := checkGIF(data); err != nil {
			return nil, nil, err
		}
		g, err = gif.DecodeAll(bytes.NewReader(data))
	} else {
		var first image.Image
		first, err = gif.Decode(bytes.NewReader(data))
		if err == nil {
			g = &gif.GIF{Image: []*image.Paletted{first.(*image.Paletted)}, Delay: []int{0}, Disposal: []byte{0}, Config: cfg, LoopCount: -1, BackgroundIndex: data[11]}
		}
	}
	if err != nil {
		return nil, nil, err
	}
	for _, frame := range g.Image {
		if frame.Bounds().Empty() || !frame.Bounds().In(image.Rect(0, 0, cfg.Width, cfg.Height)) {
			return nil, nil, fmt.Errorf("GIF has an empty or out-of-canvas frame")
		}
	}
	p := newGIFPlayer(g)
	if len(g.Image) == 1 {
		return p.Image(), nil, nil
	}
	return p.Image(), p, nil
}

// checkGIF walks block lengths and descriptors without decompressing pixels.
// The standard decoder remains responsible for LZW, palettes and extensions.
func checkGIF(data []byte) error {
	if len(data) < 13 || (string(data[:6]) != "GIF87a" && string(data[:6]) != "GIF89a") {
		return fmt.Errorf("invalid GIF header")
	}
	width, height := int(binary.LittleEndian.Uint16(data[6:8])), int(binary.LittleEndian.Uint16(data[8:10]))
	if width <= 0 || height <= 0 || width > MaxPixels/height {
		return fmt.Errorf("GIF canvas exceeds %d pixels", MaxPixels)
	}
	pos := 13
	take := func(n int) ([]byte, error) {
		if n > len(data)-pos {
			return nil, io.ErrUnexpectedEOF
		}
		part := data[pos : pos+n]
		pos += n
		return part, nil
	}
	table := func(packed byte) error {
		if packed&0x80 != 0 {
			_, err := take(3 << ((packed & 7) + 1))
			return err
		}
		return nil
	}
	blocks := func() error {
		for {
			size, err := take(1)
			if err != nil {
				return err
			}
			if size[0] == 0 {
				return nil
			}
			if _, err = take(int(size[0])); err != nil {
				return err
			}
		}
	}
	if err := table(data[10]); err != nil {
		return err
	}
	frames, pixels := 0, 0
	for {
		block, err := take(1)
		if err != nil {
			return err
		}
		switch block[0] {
		case 0x3b:
			if frames == 0 {
				return fmt.Errorf("GIF has no frames")
			}
			return nil
		case 0x21:
			if _, err := take(1); err != nil {
				return err
			}
			if err := blocks(); err != nil {
				return err
			}
		case 0x2c:
			descriptor, err := take(9)
			if err != nil {
				return err
			}
			x, y := int(binary.LittleEndian.Uint16(descriptor[:2])), int(binary.LittleEndian.Uint16(descriptor[2:4]))
			w, h := int(binary.LittleEndian.Uint16(descriptor[4:6])), int(binary.LittleEndian.Uint16(descriptor[6:8]))
			if w == 0 || h == 0 || x+w > width || y+h > height {
				return fmt.Errorf("GIF has an empty or out-of-canvas frame")
			}
			frames++
			if frames > MaxGIFFrames {
				return fmt.Errorf("GIF exceeds %d frames", MaxGIFFrames)
			}
			if w > (MaxGIFPixels-pixels)/h {
				return fmt.Errorf("GIF frames exceed %d decoded pixels", MaxGIFPixels)
			}
			pixels += w * h
			if err := table(descriptor[8]); err != nil {
				return err
			}
			if _, err := take(1); err != nil {
				return err
			} // LZW minimum code size.
			if err := blocks(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("invalid GIF block 0x%02x", block[0])
		}
	}
}
