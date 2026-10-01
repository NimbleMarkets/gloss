// Package examples contains original fixtures shared by the native and browser demos.
package examples

import "embed"

//go:embed landscape.png landscape.heic shapes.svg field-guide.pdf gloss.stl lantern.3mf readme.md field-notes.html notes.txt batch.jsonl analysis.ipynb field-notes.docx sales.xlsx notes.grist sales.csv
var Files embed.FS

// Names lists the samples in the order the gallery shows them: one of
// each kind gloss opens.
var Names = []string{
	"landscape.png", "landscape.heic", "shapes.svg", "field-guide.pdf",
	"gloss.stl", "lantern.3mf",
	"readme.md", "field-notes.html", "notes.txt", "batch.jsonl", "analysis.ipynb",
	"field-notes.docx", "sales.xlsx", "notes.grist", "sales.csv",
}
