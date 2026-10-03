package document

import (
	"fmt"
	"io/fs"
	"strings"
)

// AcceptFilter restricts a session to the named content kinds. An empty filter
// accepts every supported kind. It never trusts --type or a binary extension.
type AcceptFilter string

func ParseAccept(value string) (AcceptFilter, error) {
	if value == "" {
		return "", nil
	}
	for _, item := range strings.Split(value, ",") {
		switch strings.TrimSpace(item) {
		case "image", "images", "image/*", "svg", "pdf", "stl", "3mf", "docx", "xlsx", "grist", "csv", "json", "ipynb", "html", "text", "markdown", "md":
		default:
			return "", fmt.Errorf("--accept: unknown format %q; use gloss format names, or image/*", item)
		}
	}
	return AcceptFilter(value), nil
}

// Check identifies the bytes without a forced type or misleading extension.
// Text formats without a distinctive signature retain their filename hint.
// This checks format, not complete document validity; loaders still validate.
func (a AcceptFilter) Check(name string, data []byte) error {
	if a == "" {
		return nil
	}
	kind, err := Detect("", data, "")
	if err != nil {
		return fmt.Errorf("content does not match --accept %s: %w", a, err)
	}
	if kind == "text" {
		if hinted, e := Detect(name, data, ""); e == nil {
			switch hinted {
			case "text", "markdown", "csv", "json", "ipynb", "html":
				kind = hinted
			}
		}
	}
	for _, item := range strings.Split(string(a), ",") {
		item = strings.TrimSpace(item)
		switch item {
		case "image", "images", "image/*":
			switch kind {
			case "png", "jpeg", "gif", "webp", "bmp", "tiff", "heic", "heif", "svg":
				return nil
			}
		case "md":
			item = "markdown"
		}
		if item == kind {
			return nil
		}
	}
	return fmt.Errorf("received %s; required --accept %s", kind, a)
}

func (a AcceptFilter) CheckFile(files fs.FS, name string) error {
	if a == "" {
		return nil
	}
	data, err := readFileFrom(files, name)
	if err != nil {
		return err
	}
	return a.Check(name, data)
}
