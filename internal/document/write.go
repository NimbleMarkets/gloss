package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// WriteNew writes data to path, or, where a file is already there, to the
// nearest free name with a number before the extension: page.png, then
// page-2.png. Nothing is ever overwritten. It gives the path written.
func WriteNew(path string, data []byte) (string, error) {
	written, _, err := WriteNewFrom(path, bytes.NewReader(data), 0o644)
	return written, err
}

// WriteNewFrom is WriteNew for a stream: it copies r to path, or to the nearest
// free name, with the permissions perm, and gives the path and the bytes
// written. The name is claimed with O_EXCL before the first byte is read, so
// nothing needs to be held while r, possibly slow, is read, and nothing that was
// there is ever overwritten. A copy that fails leaves no file behind. A reader
// that should be bounded is bounded by the caller, as with io.LimitReader.
func WriteNewFrom(path string, r io.Reader, perm os.FileMode) (string, int64, error) {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for n := 1; n < 1000; n++ {
		name := path
		if n > 1 {
			name = fmt.Sprintf("%s-%d%s", stem, n, ext)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", 0, err
		}
		size, copyErr := io.Copy(f, r)
		if err := errors.Join(copyErr, f.Close()); err != nil {
			_ = os.Remove(name)
			return "", 0, err
		}
		return name, size, nil
	}
	return "", 0, fmt.Errorf("%s: too many files with this name", path)
}

// MkdirNew makes the folder path, or, where something is already there, the
// nearest free name with a number after it: pictures, then pictures-2. It
// gives the folder made.
func MkdirNew(path string) (string, error) {
	for n := 1; n < 1000; n++ {
		name := path
		if n > 1 {
			name = fmt.Sprintf("%s-%d", path, n)
		}
		err := os.Mkdir(name, 0o755)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("%s: too many folders with this name", path)
}
