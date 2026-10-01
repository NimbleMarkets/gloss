package document

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

const maxPackageEntries = 4096

// opc is an Office Open XML package: a ZIP archive of XML parts, as 3MF,
// Word, and Excel files are. Parts are read as they are asked for, within a
// budget: an archive of a few kilobytes can unpack to gigabytes.
type opc struct {
	archive *zip.Reader // An fs.FS of the package, for images referred to by name.
	files   map[string]*zip.File
	budget  int64
}

func openOPC(data []byte) (*opc, error) {
	archive, err := openZip(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(archive.File) > maxPackageEntries {
		return nil, fmt.Errorf("has more than %d entries", maxPackageEntries)
	}
	p := &opc{archive: archive, files: map[string]*zip.File{}, budget: MaxFileBytes}
	for _, f := range archive.File {
		p.files[strings.ToLower(f.Name)] = f
	}
	return p, nil
}

func (p *opc) has(name string) bool {
	return p.files[strings.ToLower(strings.TrimPrefix(name, "/"))] != nil
}

func (p *opc) read(name string) ([]byte, error) {
	f := p.files[strings.ToLower(strings.TrimPrefix(name, "/"))]
	if f == nil {
		return nil, fmt.Errorf("lacks %s", name)
	}
	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, p.budget+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if p.budget -= int64(len(data)); p.budget < 0 {
		return nil, fmt.Errorf("unpacks to more than 128 MiB")
	}
	return data, nil
}

// relationships maps the ids in a part's .rels file to the parts they name,
// as paths in the package.
func (p *opc) relationships(part string) map[string]string {
	dir, base := path.Split(part)
	data, err := p.read(dir + "_rels/" + base + ".rels")
	if err != nil {
		return nil
	}
	var rels struct {
		Relationship []struct {
			ID         string `xml:"Id,attr"`
			Target     string `xml:"Target,attr"`
			TargetMode string `xml:"TargetMode,attr"`
		}
	}
	if xml.Unmarshal(data, &rels) != nil {
		return nil
	}
	out := map[string]string{}
	for _, r := range rels.Relationship {
		if r.TargetMode == "External" {
			out[r.ID] = r.Target
			continue
		}
		target := r.Target
		if !strings.HasPrefix(target, "/") {
			target = path.Join(dir, target)
		}
		out[r.ID] = strings.TrimPrefix(path.Clean(target), "/")
	}
	return out
}

// opcKind tells the packages gloss reads apart by the parts they hold.
func opcKind(archive *zip.Reader) string {
	for _, f := range archive.File {
		switch name := strings.ToLower(f.Name); {
		case strings.HasPrefix(name, "3d/") && strings.HasSuffix(name, ".model"):
			return "3mf"
		case name == "word/document.xml":
			return "docx"
		case name == "xl/workbook.xml":
			return "xlsx"
		}
	}
	return ""
}

// officeFields describes a document by what the package says of it: who
// made it and when, and with what. The dates are as W3C writes them.
func (p *opc) officeFields() []Field {
	var core struct {
		Title          string `xml:"title"`
		Subject        string `xml:"subject"`
		Creator        string `xml:"creator"`
		Description    string `xml:"description"`
		LastModifiedBy string `xml:"lastModifiedBy"`
		Created        string `xml:"created"`
		Modified       string `xml:"modified"`
	}
	if data, err := p.read("docProps/core.xml"); err == nil {
		_ = xml.Unmarshal(data, &core)
	}
	var app struct {
		Application string `xml:"Application"`
		Company     string `xml:"Company"`
		Pages       string `xml:"Pages"`
		Words       string `xml:"Words"`
	}
	if data, err := p.read("docProps/app.xml"); err == nil {
		_ = xml.Unmarshal(data, &app)
	}
	when := func(s string) string {
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
				if layout == "2006-01-02" {
					return t.Format("2006-01-02")
				}
				return t.Format("2006-01-02 15:04")
			}
		}
		return strings.TrimSpace(s)
	}
	return []Field{{"Title", core.Title}, {"Subject", core.Subject}, {"Description", core.Description}, {"Author", core.Creator}, {"Changed by", core.LastModifiedBy},
		{"Created", when(core.Created)}, {"Changed", when(core.Modified)}, {"Application", app.Application}, {"Company", app.Company}, {"Pages", app.Pages}, {"Words", app.Words}}
}
