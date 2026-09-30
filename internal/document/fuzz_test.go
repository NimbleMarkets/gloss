package document

import (
	"testing"
)

// The parsers take files from anywhere. Each must refuse or read what it
// is given without panicking, however the bytes are arranged; the loader's
// recover is a net, not a plan. These run as tests on their seeds, and as
// fuzzers with go test -fuzz=Fuzz -run=^$ ./internal/document/.

func FuzzParseSTL(f *testing.F) {
	f.Add([]byte(facetSTL))
	f.Add(make([]byte, 84))
	f.Add([]byte("solid\nfacet normal 0 0 0\nouter loop\nvertex 0 0 0\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if mesh, err := ParseSTL(data); err == nil && mesh == nil {
			t.Fatal("no mesh and no error")
		}
	})
}

func FuzzParse3MF(f *testing.F) {
	f.Add(simple3MF(f))
	f.Add(assembly3MF(f))
	f.Add([]byte("PK\x03\x04 not really a package"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if model, err := Parse3MF(data); err == nil && (model == nil || model.Mesh == nil && model.Thumbnail == nil) {
			t.Fatal("no model and no error")
		}
	})
}

func FuzzReadCSV(f *testing.F) {
	f.Add("a,b,c\n1,2,3\n")
	f.Add("a\tb\n\"quoted, with\ncomma\"\t2\n")
	f.Add("\xff\xfe\x00")
	f.Fuzz(func(t *testing.T, data string) {
		ReadCSV("f.csv", []byte(data))
	})
}

func FuzzReadJSON(f *testing.F) {
	f.Add(`{"a": [1, 2, {"b": null}]}`)
	f.Add("{\"a\":1}\n{\"b\":2}\n")
	f.Add("{not json")
	f.Fuzz(func(t *testing.T, data string) {
		ReadJSON("f.json", []byte(data))
	})
}

func FuzzReadText(f *testing.F) {
	f.Add("plain\ttext\r\n```\n")
	f.Add("\xff")
	f.Fuzz(func(t *testing.T, data string) {
		ReadText("f.txt", []byte(data))
		IsText([]byte(data))
	})
}

func FuzzReadHTML(f *testing.F) {
	f.Add("<html><body><h1>x</h1><table><tr><td>1</td></tr></table></body></html>")
	f.Add("<<<>>> &amp; <a href=x")
	f.Fuzz(func(t *testing.T, data string) {
		ReadHTML([]byte(data))
	})
}

func FuzzReadNotebook(f *testing.F) {
	f.Add(`{"cells":[{"cell_type":"code","source":"x","outputs":[{"output_type":"stream","text":["y"]}]}],"nbformat":4}`)
	f.Add(`{"cells": "no"}`)
	f.Fuzz(func(t *testing.T, data string) {
		ReadNotebook([]byte(data))
	})
}

func FuzzOpenWorkbook(f *testing.F) {
	f.Add(workbook(f, map[string]string{"Sheet1": `<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1"><v>2.5</v></c></row>`}, map[string]string{"xl/sharedStrings.xml": `<sst><si><t>x</t></si></sst>`}))
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if book, err := OpenWorkbook(data); err == nil {
			for i := range book.Names() {
				book.Sheet(i)
			}
		}
	})
}

func FuzzOpenWord(f *testing.F) {
	f.Add(wordDocument(f, `<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`, nil))
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, data []byte) {
		OpenWord(data)
	})
}

func FuzzSharedStrings(f *testing.F) {
	f.Add(`<sst><si><t>a</t></si><si><r><t>b</t></r><rPh><t>c</t></rPh></si></sst>`)
	f.Fuzz(func(t *testing.T, data string) {
		sharedStrings([]byte(data))
	})
}
