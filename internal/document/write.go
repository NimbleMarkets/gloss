package document

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// WriteNew writes data to path, or, where a file is already there, to the
// nearest free name with a number before the extension: page.png, then
// page-2.png. Nothing is ever overwritten. It gives the path written.
func WriteNew(path string, data []byte) (string, error) {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for n := 1; n < 1000; n++ {
		name := path
		if n > 1 {
			name = fmt.Sprintf("%s-%d%s", stem, n, ext)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, writeErr := f.Write(data)
		if err := errors.Join(writeErr, f.Close()); err != nil {
			_ = os.Remove(name)
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("%s: too many files with this name", path)
}
