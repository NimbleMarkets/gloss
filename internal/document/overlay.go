package document

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"
)

// Overlay adds session-only files to a read-only filesystem, for hosts such
// as the browser that have no disk. Added files live under "dropped/" so they
// cannot shadow the base.
type Overlay struct {
	Base  fs.FS
	mu    sync.RWMutex
	files map[string][]byte
}

// Add stores data under the final element of name and returns its path.
func (o *Overlay) Add(name string, data []byte) (string, error) {
	name = "dropped/" + path.Base(strings.ReplaceAll(name, `\`, "/"))
	if !fs.ValidPath(name) || path.Dir(name) != "dropped" {
		return "", fmt.Errorf("invalid file name")
	}
	if len(data) == 0 {
		return "", ErrEmpty
	}
	if len(data) > MaxFileBytes {
		return "", ErrTooLarge
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.files == nil {
		o.files = map[string][]byte{}
	}
	o.files[name] = data
	return name, nil
}

func (o *Overlay) Open(name string) (fs.File, error) {
	o.mu.RLock()
	data, ok := o.files[name]
	o.mu.RUnlock()
	if ok {
		return &overlayFile{Reader: bytes.NewReader(data), name: path.Base(name)}, nil
	}
	if o.Base == nil || strings.HasPrefix(name, "dropped/") {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return o.Base.Open(name)
}

type overlayFile struct {
	*bytes.Reader
	name string
}

func (f *overlayFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *overlayFile) Close() error               { return nil }
func (f *overlayFile) Name() string               { return f.name }
func (f *overlayFile) Mode() fs.FileMode          { return 0444 }
func (f *overlayFile) ModTime() time.Time         { return time.Time{} }
func (f *overlayFile) IsDir() bool                { return false }
func (f *overlayFile) Sys() any                   { return nil }
