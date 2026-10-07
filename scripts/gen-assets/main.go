// Regenerate the original raster and PDF fixtures: go run ./scripts/gen-assets.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"

	"github.com/gen2brain/h265/heic"
)

func main() {
	img := image.NewNRGBA(image.Rect(0, 0, 640, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 640; x++ {
			t := float64(y) / 400
			c := color.NRGBA{uint8(25 + 70*t), uint8(34 + 80*t), uint8(80 + 90*t), 255}
			if math.Hypot(float64(x-470), float64(y-110)) < 48 {
				c = color.NRGBA{255, 194, 110, 255}
			}
			for k := 0; k < 3; k++ {
				ridge := 220 + float64(k*45) + 45*math.Sin(float64(x)/90+float64(k)*2) + 25*math.Cos(float64(x)/43+float64(k))
				if float64(y) > ridge {
					c = []color.NRGBA{{78, 85, 131, 255}, {41, 64, 101, 255}, {17, 39, 63, 255}}[k]
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	write("examples/motion.gif", func(b *bytes.Buffer) error { return motionGIF(b) })
	write("examples/landscape.png", func(b *bytes.Buffer) error { return png.Encode(b, img) })
	write("examples/landscape.heic", func(b *bytes.Buffer) error { return heic.Encode(b, desert(), heic.EncodeOptions{Quality: 85}) })
	if err := os.WriteFile("examples/field-guide.pdf", pdf(), 0644); err != nil {
		panic(err)
	}
	if err := os.WriteFile("examples/gloss.stl", blockWord(), 0644); err != nil {
		panic(err)
	}
}

// A warm desert scene contrasts with the PNG's blue hills at dusk.
func desert() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 640, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 640; x++ {
			c := color.NRGBA{35, 155, 163, 255}
			if math.Hypot(float64(x-155), float64(y-115)) < 65 {
				c = color.NRGBA{255, 226, 158, 255}
			}
			for k, sand := range []color.NRGBA{{239, 176, 107, 255}, {215, 115, 69, 255}, {156, 66, 49, 255}} {
				ridge := 245 + float64(k*55) + 35*math.Sin(float64(x)/130+float64(k)*2)
				if float64(y) > ridge {
					c = sand
				}
			}
			// Saguaro silhouette with two raised arms.
			if (x >= 458 && x < 482 && y >= 155 && y < 337) ||
				(x >= 420 && x < 443 && y >= 188 && y < 252) ||
				(x >= 420 && x < 470 && y >= 232 && y < 252) ||
				(x >= 500 && x < 522 && y >= 172 && y < 224) ||
				(x >= 470 && x < 522 && y >= 204 && y < 224) {
				c = color.NRGBA{23, 70, 64, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// Each pixel in an original 5x7 alphabet becomes a closed raised block.
// Separate blocks leave narrow seams that make the sculpture's depth visible.
func blockWord() []byte {
	letters := []string{
		"01110/10001/10000/10111/10001/10001/01110",
		"10000/10000/10000/10000/10000/10000/11111",
		"01110/10001/10001/10001/10001/10001/01110",
		"01111/10000/10000/01110/00001/00001/11110",
		"01111/10000/10000/01110/00001/00001/11110",
	}
	var b bytes.Buffer
	b.WriteString("solid gloss_blocks\n")
	faces := [][4]int{{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {3, 7, 6, 2}, {0, 4, 7, 3}, {1, 2, 6, 5}}
	for letter, glyph := range letters {
		for row, line := range strings.Split(glyph, "/") {
			for col, bit := range line {
				if bit != '1' {
					continue
				}
				x, y := float64(letter*6+col)-14.5, float64(6-row)-3.5
				// A little variation in relief catches the directional light.
				z := 1.3 + .15*float64((row+col)%3)
				v := [][3]float64{{x + .06, y + .06, 0}, {x + .94, y + .06, 0}, {x + .94, y + .94, 0}, {x + .06, y + .94, 0}, {x + .06, y + .06, z}, {x + .94, y + .06, z}, {x + .94, y + .94, z}, {x + .06, y + .94, z}}
				for _, face := range faces {
					for _, tri := range [][3]int{{face[0], face[1], face[2]}, {face[0], face[2], face[3]}} {
						b.WriteString("facet normal 0 0 0\nouter loop\n")
						for _, i := range tri {
							fmt.Fprintf(&b, "vertex %.3f %.3f %.3f\n", -math.Sin(math.Pi/180*25)*v[i][0]+math.Cos(math.Pi/180*25)*v[i][2], math.Cos(math.Pi/180*25)*v[i][0]+math.Sin(math.Pi/180*25)*v[i][2], v[i][1])
						}
						b.WriteString("endloop\nendfacet\n")
					}
				}
			}
		}
	}
	b.WriteString("endsolid gloss_blocks\n")
	return b.Bytes()
}

func write(path string, encode func(*bytes.Buffer) error) {
	var b bytes.Buffer
	if err := encode(&b); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0644); err != nil {
		panic(err)
	}
}

func pdf() []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 640 400] /Resources << /Font << /F1 5 0 R >> >> /Contents 6 0 R >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 640 400] /Resources << /Font << /F1 5 0 R >> >> /Contents 7 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	for i, title := range []string{"A FIELD GUIDE TO GLOSS", "SMALL FILES. MANY VIEWS."} {
		stream := fmt.Sprintf("0.07 0.10 0.16 rg 0 0 640 400 re f\n0.4 0.85 0.7 rg 40 330 70 5 re f\n1 1 1 rg BT /F1 28 Tf 40 280 Td (%s) Tj ET\n0.7 0.75 0.85 rg BT /F1 16 Tf 40 245 Td (Images, vectors, documents and meshes.) Tj ET\n0.4 0.85 0.7 rg 40 100 140 90 re f\n0.45 0.55 0.95 rg 205 100 160 90 re f\n1 0.65 0.4 rg 390 100 210 90 re f\n1 1 1 rg BT /F1 12 Tf 40 40 Td (PAGE %d / 2  -  Press n and p to turn pages) Tj ET\n", title, i+1)
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream))
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return b.Bytes()
}
