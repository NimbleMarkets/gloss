// Package browsetest is a harness for testing file-chooser components: an
// in-memory filesystem, a driver that sends keys and settles the commands a
// component answers with, and a script runner whose golden screens are
// readable by a person.
//
// It knows nothing of any one component: it needs only Init, Update, and
// View, so the same scripts can characterize one chooser and then hold
// another to it.
package browsetest

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Epoch is the modification time of an entry given none: files carry fixed
// times so that screens do not change from one day to the next.
var Epoch = time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)

// FS is an in-memory filesystem for choosers, rooted at "/" as os.DirFS("/")
// is: names are slash-separated with no leading slash, and "." is the root.
type FS struct {
	mu      sync.Mutex
	nodes   map[string]*node
	latency time.Duration
	fails   map[string]error
	reads   []string
}

type node struct {
	name string
	dir  bool
	data []byte
	size int64
	mod  time.Time
	mode fs.FileMode
}

// NewFS builds a filesystem from lines, each a path with optional fields:
//
//	home/evan/notes.md              a file
//	home/evan/big.pdf 2048          a file of that size in bytes
//	home/evan/old.txt @2025-12-25   a file with that modification date
//	home/evan/docs/                 a folder, however empty
//	home/evan/hello.txt = hi there  a file with that text
//	"home/evan/two words.txt" 12    a name with spaces is Go-quoted
//
// Folders along a path are made as needed. A leading slash is ignored.
func NewFS(lines ...string) *FS {
	f := &FS{nodes: map[string]*node{".": {name: ".", dir: true, mod: Epoch, mode: fs.ModeDir | 0o755}}, fails: map[string]error{}}
	for _, line := range lines {
		f.Add(line)
	}
	return f
}

// Add adds one line, in the form NewFS takes.
func (f *FS) Add(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	// The name comes first: a Go-quoted string if it has spaces in it.
	var name, rest string
	if strings.HasPrefix(line, `"`) {
		q, err := strconv.QuotedPrefix(line)
		if err != nil {
			panic(fmt.Sprintf("browsetest: bad quoted name in %q: %v", line, err))
		}
		name, _ = strconv.Unquote(q)
		rest = line[len(q):]
	} else {
		name, rest = line, ""
		if i := strings.IndexByte(line, ' '); i >= 0 {
			name, rest = line[:i], line[i:] // The space stays, for " = ".
		}
	}
	var text *string
	if i := strings.Index(rest, " = "); i >= 0 {
		t := rest[i+3:]
		rest, text = rest[:i], &t
	}
	size, mod := int64(-1), Epoch
	for _, extra := range strings.Fields(rest) {
		if date, ok := strings.CutPrefix(extra, "@"); ok {
			t, err := time.Parse("2006-01-02", date)
			if err != nil {
				panic(fmt.Sprintf("browsetest: bad date in %q: %v", line, err))
			}
			mod = t
		} else if n, err := strconv.ParseInt(extra, 10, 64); err == nil {
			size = n
		} else {
			panic(fmt.Sprintf("browsetest: bad field %q in %q", extra, line))
		}
	}
	dir := strings.HasSuffix(name, "/")
	name = strings.Trim(path.Clean("/"+name), "/")
	if name == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for p := path.Dir(name); p != "."; p = path.Dir(p) {
		if _, ok := f.nodes[p]; !ok {
			f.nodes[p] = &node{name: path.Base(p), dir: true, mod: Epoch, mode: fs.ModeDir | 0o755}
		}
	}
	n := &node{name: path.Base(name), dir: dir, mod: mod, mode: 0o644}
	if dir {
		n.mode = fs.ModeDir | 0o755
	} else if text != nil {
		n.data = []byte(*text)
		n.size = int64(len(n.data))
	} else if size >= 0 {
		n.size = size
	} else {
		n.size = 100
	}
	f.nodes[name] = n
}

// Latency makes every folder read take that long, to show a component not
// waiting on its filesystem. Keep it well under the driver's patience.
func (f *FS) Latency(d time.Duration) *FS {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.latency = d
	return f
}

// Fail makes reading the folder fail with err, as when permission is denied.
func (f *FS) Fail(dir string, err error) *FS {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fails[strings.Trim(path.Clean("/"+dir), "/")] = err
	return f
}

// Reads lists the folders read so far, in order.
func (f *FS) Reads() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reads)
}

// ReadCount says how many times the folder has been read.
func (f *FS) ReadCount(dir string) int {
	dir = strings.Trim(path.Clean("/"+dir), "/")
	if dir == "" {
		dir = "."
	}
	n := 0
	for _, r := range f.Reads() {
		if r == dir {
			n++
		}
	}
	return n
}

// ResetReads forgets the reads counted so far.
func (f *FS) ResetReads() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = nil
}

func (f *FS) lookup(name string) (*node, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return n, nil
}

// Open opens a file or a folder.
func (f *FS) Open(name string) (fs.File, error) {
	n, err := f.lookup(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if n.dir {
		return &dirFile{fs: f, name: name, node: n}, nil
	}
	return &openFile{node: n, r: bytes.NewReader(n.data)}, nil
}

// ReadDir lists a folder, in name order as the os does.
func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	f.mu.Lock()
	f.reads = append(f.reads, name)
	latency, fail := f.latency, f.fails[name]
	f.mu.Unlock()
	if latency > 0 {
		time.Sleep(latency)
	}
	if fail != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fail}
	}
	n, err := f.lookup(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if !n.dir {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var entries []fs.DirEntry
	for p, c := range f.nodes {
		if p != "." && path.Dir(p) == name {
			entries = append(entries, fs.FileInfoToDirEntry(info{c}))
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

// Stat describes a file or a folder.
func (f *FS) Stat(name string) (fs.FileInfo, error) {
	n, err := f.lookup(name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	return info{n}, nil
}

type info struct{ n *node }

func (i info) Name() string       { return i.n.name }
func (i info) Size() int64        { return i.n.size }
func (i info) Mode() fs.FileMode  { return i.n.mode }
func (i info) ModTime() time.Time { return i.n.mod }
func (i info) IsDir() bool        { return i.n.dir }
func (i info) Sys() any           { return nil }

type openFile struct {
	node *node
	r    *bytes.Reader
}

func (o *openFile) Stat() (fs.FileInfo, error) { return info{o.node}, nil }
func (o *openFile) Read(p []byte) (int, error) { return o.r.Read(p) }
func (o *openFile) Close() error               { return nil }

type dirFile struct {
	fs   *FS
	name string
	node *node
}

func (d *dirFile) Stat() (fs.FileInfo, error) { return info{d.node}, nil }
func (d *dirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: io.EOF}
}
func (d *dirFile) Close() error { return nil }
func (d *dirFile) ReadDir(int) ([]fs.DirEntry, error) {
	return d.fs.ReadDir(d.name)
}
