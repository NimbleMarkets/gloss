package document

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"
)

const wordNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`

// wordDocument packs a body, with the parts beside it.
func wordDocument(t *testing.T, body string, extra map[string]string) []byte {
	t.Helper()
	parts := map[string]string{
		"word/document.xml": `<?xml version="1.0"?><w:document ` + wordNS + `><w:body>` + body + `<w:sectPr/></w:body></w:document>`,
		"_rels/.rels":       `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
	}
	for name, content := range extra {
		parts[name] = content
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, content := range parts {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(f, content)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func p(style, text string) string {
	pPr := ""
	if style != "" {
		pPr = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
	}
	return `<w:p>` + pPr + `<w:r><w:t xml:space="preserve">` + text + `</w:t></w:r></w:p>`
}

func TestWordToMarkdown(t *testing.T) {
	styles := `<?xml version="1.0"?><w:styles ` + wordNS + `><w:style w:type="paragraph" w:styleId="Titre1"><w:name w:val="heading 1"/></w:style><w:style w:type="paragraph" w:styleId="Fancy"><w:name w:val="Fancy"/><w:pPr><w:outlineLvl w:val="2"/></w:pPr></w:style></w:styles>`
	body := p("Title", "Trip Report") + p("Subtitle", "Two days out") +
		p("Heading1", "Day one") + p("Titre1", "Jour un") + p("Heading2", "Morning") + p("Fancy", "Styled third") +
		`<w:p><w:r><w:t xml:space="preserve">Plain, </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>bold </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>still bold</w:t></w:r><w:r><w:t>, </w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>italic</w:t></w:r><w:r><w:t xml:space="preserve"> and </w:t></w:r><w:r><w:rPr><w:strike/></w:rPr><w:t>struck</w:t></w:r><w:r><w:t>.</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Line one</w:t><w:br/><w:t>line two</w:t><w:tab/><w:t>after a tab</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>See </w:t></w:r><w:hyperlink r:id="rId5"><w:r><w:t>the site</w:t></w:r></w:hyperlink><w:r><w:t> and </w:t></w:r><w:hyperlink w:anchor="top"><w:r><w:t>the top</w:t></w:r></w:hyperlink><w:r><w:t>.</w:t></w:r></w:p>` +
		p("", "Stars * and _underscores_ and #hash and [brackets] stay literal") +
		`<w:p/><w:p><w:pPr><w:pStyle w:val="Quote"/></w:pPr><w:r><w:t>A quoted line.</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>first bullet</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>nested bullet</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr></w:pPr><w:r><w:t>first step</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr></w:pPr><w:r><w:t>second step</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc>` + p("", "Item") + `</w:tc><w:tc>` + p("", "Count") + `</w:tc></w:tr><w:tr><w:tc>` + p("", "apples") + p("", "and pears") + `</w:tc><w:tc>` + p("", "3 | 4") + `</w:tc></w:tr></w:tbl>` +
		`<w:p><w:r><w:drawing><wp:inline><wp:docPr id="1" name="Picture 1" descr="A red square"/><a:graphic><a:graphicData><pic:pic><pic:blipFill><a:blip r:embed="rId7"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>` +
		p("", "The end.")
	rels := `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/trip" TargetMode="External"/>` +
		`<Relationship Id="rId7" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/></Relationships>`
	numbering := `<?xml version="1.0"?><w:numbering ` + wordNS + `><w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl><w:lvl w:ilvl="1"><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum>` +
		`<w:abstractNum w:abstractNumId="1"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num><w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num></w:numbering>`
	var picture bytes.Buffer
	red := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range red.Pix {
		red.Pix[i] = 255
	}
	png.Encode(&picture, red)
	data := wordDocument(t, body, map[string]string{"word/_rels/document.xml.rels": rels, "word/numbering.xml": numbering, "word/styles.xml": styles, "word/media/image1.png": picture.String()})
	doc, err := OpenWord(data)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(`
# Trip Report

## Two days out

# Day one

# Jour un

## Morning

### Styled third

Plain, **bold still bold**, *italic* and ~~struck~~.

Line one  
line two	after a tab

See [the site](https://example.com/trip) and the top.

Stars \* and \_underscores\_ and \#hash and \[brackets\] stay literal

> A quoted line.

- first bullet
  - nested bullet

1. first step
2. second step

| Item | Count |
| --- | --- |
| apples and pears | 3 \| 4 |

![A red square](media/image1.png)

The end.
`) + "\n"
	if got := string(doc.Markdown); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if doc.Headings != 6 || doc.Tables != 1 || doc.Images != 1 || doc.Links != 1 {
		t.Fatalf("%+v", doc)
	}
	l := &Loader{}
	defer l.Close()
	r := l.Load(Request{Path: write(t, "trip.docx", data), Page: 1, DPI: 72, Generation: 1})
	if r.Err != nil || r.Kind != "docx" || r.Markdown == nil {
		t.Fatalf("err=%v kind=%q", r.Err, r.Kind)
	}
	if len(r.Markdown.Images) != 1 || r.Markdown.Images[0].Err != nil || r.Markdown.Images[0].Image == nil {
		t.Fatalf("the picture in the package was not found: %+v", r.Markdown.Images)
	}
	expect(t, r.Info, map[string]string{"Format": "Word document", "Headings": "6", "Tables": "1", "Images": "1", "Links": "1"})
}

func TestWordInfoAndDetection(t *testing.T) {
	core := `<?xml version="1.0"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/"><dc:title>Memo</dc:title><dc:creator>A. Writer</dc:creator><dcterms:modified>2026-05-06T07:08:09Z</dcterms:modified></cp:coreProperties>`
	app := `<?xml version="1.0"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Microsoft Office Word</Application><Pages>3</Pages><Words>1234</Words></Properties>`
	data := wordDocument(t, p("", "Hello"), map[string]string{"docProps/core.xml": core, "docProps/app.xml": app})
	l := &Loader{}
	defer l.Close()
	r := l.Load(Request{Path: write(t, "memo.docm", data), Page: 1, DPI: 72, Generation: 1})
	if r.Err != nil || r.Kind != "docx" {
		t.Fatalf("err=%v kind=%q", r.Err, r.Kind)
	}
	expect(t, r.Info, map[string]string{"Title": "Memo", "Author": "A. Writer", "Changed": "2026-05-06 07:08", "Application": "Microsoft Office Word", "Pages": "3", "Words": "1,234"})
	if kind, err := Detect("letter", data, ""); err != nil || kind != "docx" {
		t.Fatalf("Detect = %q, %v", kind, err)
	}
	for name, data := range map[string][]byte{
		"no document": archive3MF(t, map[string]string{"word/styles.xml": "<w:styles/>"}),
		"malformed":   wordDocument(t, `<w:p><w:r><w:t>unclosed`, nil),
	} {
		if _, err := OpenWord(data); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A document longer than Markdown may be is cut, and says so.
	long := strings.Repeat(p("", strings.Repeat("word ", 200)), MaxMarkdownBytes/1000)
	doc, err := OpenWord(wordDocument(t, long, nil))
	if err != nil || len(doc.Markdown) > MaxMarkdownBytes || !strings.HasSuffix(strings.TrimSpace(string(doc.Markdown)), "cut here.") {
		t.Fatalf("err=%v len=%d tail=%q", err, len(doc.Markdown), string(doc.Markdown[max(0, len(doc.Markdown)-80):]))
	}
}
