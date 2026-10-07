package main

import (
	"image"
	"image/color"
	"image/gif"
	"io"
	"math"
)

// motionGIF is an original looping orbit, drawn directly into indexed frames.
func motionGIF(w io.Writer) error {
	const width, height, frames = 192, 112, 32
	palette := color.Palette{
		color.RGBA{18, 27, 46, 255}, color.RGBA{48, 65, 91, 255},
		color.RGBA{91, 178, 193, 255}, color.RGBA{255, 187, 96, 255},
	}
	g := &gif.GIF{LoopCount: 0, Config: image.Config{ColorModel: palette, Width: width, Height: height}}
	for i := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		angle := 2 * math.Pi * float64(i) / frames
		cx, cy := 96+62*math.Cos(angle), 56+32*math.Sin(angle)
		for y := range height {
			for x := range width {
				dx, dy := float64(x)-96, float64(y)-56
				if math.Abs(dx*dx/(62*62)+dy*dy/(32*32)-1) < .065 {
					frame.SetColorIndex(x, y, 1)
				}
				if dx*dx+dy*dy < 9*9 {
					frame.SetColorIndex(x, y, 2)
				}
				if math.Hypot(float64(x)-cx, float64(y)-cy) < 8 {
					frame.SetColorIndex(x, y, 3)
				}
			}
		}
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 8)
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}
	return gif.EncodeAll(w, g)
}
