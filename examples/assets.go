// Package examples contains original fixtures shared by the native and browser demos.
package examples

import "embed"

//go:embed landscape.png landscape.heic shapes.svg field-guide.pdf gloss.stl readme.md
var Files embed.FS

var Names = []string{"landscape.png", "landscape.heic", "shapes.svg", "field-guide.pdf", "gloss.stl", "readme.md"}
