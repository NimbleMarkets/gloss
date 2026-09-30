package document

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// Colors a mesh can be given by name, for the command line.
var namedColors = map[string]color.RGBA{
	"white": {R: 240, G: 240, B: 240, A: 255}, "grey": {R: 128, G: 128, B: 128, A: 255}, "gray": {R: 128, G: 128, B: 128, A: 255},
	"black": {R: 40, G: 40, B: 40, A: 255}, "red": {R: 220, G: 50, B: 47, A: 255}, "orange": {R: 255, G: 140, A: 255},
	"yellow": {R: 240, G: 200, B: 40, A: 255}, "green": {R: 60, G: 170, B: 80, A: 255}, "cyan": {R: 42, G: 161, B: 152, A: 255},
	"blue": {R: 38, G: 139, B: 210, A: 255}, "purple": {R: 108, G: 113, B: 196, A: 255}, "magenta": {R: 211, G: 54, B: 130, A: 255},
	"pink": {R: 255, G: 150, B: 190, A: 255}, "brown": {R: 140, G: 90, B: 50, A: 255},
}

// ParseColor reads a color as #rgb, #rrggbb, with or without the #, or one
// of a few names.
func ParseColor(s string) (color.RGBA, error) {
	s = strings.TrimSpace(s)
	if c, ok := namedColors[strings.ToLower(s)]; ok {
		return c, nil
	}
	hex := strings.TrimPrefix(s, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 6 {
		return color.RGBA{}, fmt.Errorf("%q is not a color: use #rrggbb or a name such as orange", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
}

// DefaultMeshColor is the color faces get when their file names none.
func DefaultMeshColor() color.RGBA { return meshColor }

// HasColors says whether the file gave any face a color of its own.
func (m *Mesh) HasColors() bool {
	for _, plain := range m.plain {
		if !plain {
			return true
		}
	}
	return false
}

// Recolor paints the faces the file left plain, or all of them, in place.
// The file's own colors are kept aside for RestoreColors.
func (m *Mesh) Recolor(shade color.RGBA, all bool) {
	vertices := m.geometry.Vertices
	if all && m.saved == nil {
		m.saved = make([]color.RGBA, len(vertices))
		for i := range vertices {
			m.saved[i] = vertices[i].Color
		}
	}
	for i := range vertices {
		if all || m.plain[i] {
			vertices[i].Color = shade
		}
	}
}

// RestoreColors puts back the colors the file gave, and the default on the
// faces it left plain.
func (m *Mesh) RestoreColors() {
	for i := range m.geometry.Vertices {
		switch {
		case m.plain[i]:
			m.geometry.Vertices[i].Color = meshColor
		case m.saved != nil:
			m.geometry.Vertices[i].Color = m.saved[i]
		}
	}
	m.saved = nil
}
