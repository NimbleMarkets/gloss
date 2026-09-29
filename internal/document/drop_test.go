package document

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParseDropPaths(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	for _, name := range []string{"a.png", "my file.png", "it's.svg", "100%.pdf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	abs := func(name string) string { return filepath.Join(dir, name) }
	uri := func(host, name string) string { return "file://" + host + filepath.ToSlash(dir) + "/" + name }
	for _, tt := range []struct {
		name, paste string
		want        []string
	}{
		{"absolute", abs("a.png"), []string{abs("a.png")}},
		{"lines and CRLF", abs("a.png") + "\r\n\r\n  " + abs("100%.pdf") + "  \n", []string{abs("a.png"), abs("100%.pdf")}},
		{"carriage returns between lines", "not a file\r" + abs("a.png") + "\r" + abs("100%.pdf") + "\r", []string{abs("a.png"), abs("100%.pdf")}},
		{"missing absolute path is still a candidate", "/no/such/file.png", []string{"/no/such/file.png"}},
		{"relative prefixes", "./a.png\n../x.png", []string{"./a.png", "../x.png"}},
		{"home", "~/a.png", []string{abs("a.png")}},
		{"bare existing file", "a.png", []string{"a.png"}},
		{"raw spaces", abs("my file.png"), []string{abs("my file.png")}},
		{"backslash escapes on one line", strings.ReplaceAll(abs("my file.png"), " ", `\ `) + " " + abs("a.png"), []string{abs("my file.png"), abs("a.png")}},
		{"single quotes", "'" + abs("my file.png") + "' '" + abs("a.png") + "'", []string{abs("my file.png"), abs("a.png")}},
		{"escaped quote inside single quotes", "'" + dir + `/it'\''s.svg'`, []string{abs("it's.svg")}},
		{"double quotes", `"` + abs("my file.png") + `"`, []string{abs("my file.png")}},
		{"file URI", uri("", "my%20file.png"), []string{abs("my file.png")}},
		{"file URI percent", uri("", "100%25.pdf"), []string{abs("100%.pdf")}},
		{"file URI localhost", uri("localhost", "a.png"), []string{abs("a.png")}},
		{"file URI remote host", uri("example.com", "a.png"), nil},
		{"web URL", "https://example.com/a.png", nil},
		{"prose", "please open the picture for me", nil},
		{"prose with a slash", "/shrug and/or whatever", nil},
		{"typed token", "hello", nil},
		{"unterminated quote", "'" + abs("a.png"), nil},
		{"control characters", abs("a.png") + "\x1b[31m", nil},
		{"junk line beside a path", "not a file\n" + abs("a.png"), []string{abs("a.png")}},
		{"empty", "  \n\n", nil},
		{"huge dump", strings.Repeat(abs("a.png")+"\n", 64<<10/len(abs("a.png"))), nil},
		{"too many entries", strings.Repeat("/a\n", 257), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseDrop(tt.paste); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseDrop(%q)\n got %q\nwant %q", tt.paste, got, tt.want)
			}
		})
	}
}

func TestOverlayKeepsBaseAndSessionFiles(t *testing.T) {
	o := &Overlay{Base: fstest.MapFS{"shapes.svg": {Data: []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")}}}
	name, err := o.Add("../evil/photo.png", []byte("\x89PNG\r\n\x1a\n"))
	if err != nil || name != "dropped/photo.png" {
		t.Fatalf("name=%q err=%v", name, err)
	}
	if got, err := fs.ReadFile(o, name); err != nil || string(got) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("dropped: %q %v", got, err)
	}
	if info, err := fs.Stat(o, name); err != nil || !info.Mode().IsRegular() || info.Size() != 8 {
		t.Fatalf("stat: %v %v", info, err)
	}
	if _, err := fs.ReadFile(o, "shapes.svg"); err != nil {
		t.Fatalf("base: %v", err)
	}
	// A sample's name is reused without shadowing the embedded file.
	if name, err := o.Add("shapes.svg", []byte("mine")); err != nil || name != "dropped/shapes.svg" {
		t.Fatalf("name=%q err=%v", name, err)
	}
	if got, _ := fs.ReadFile(o, "shapes.svg"); string(got) == "mine" {
		t.Fatal("dropped file shadowed an embedded sample")
	}
	if _, err := o.Add("x.png", nil); err == nil {
		t.Fatal("empty file accepted")
	}
	if _, err := o.Add("..", []byte("x")); err == nil {
		t.Fatal("invalid name accepted")
	}
	if _, err := o.Add("big.png", make([]byte, MaxFileBytes+1)); err == nil || !strings.Contains(err.Error(), "128 MiB") {
		t.Fatalf("oversize: %v", err)
	}
	if _, err := o.Open("dropped/missing.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
}

func TestProbeFS(t *testing.T) {
	o := &Overlay{Base: fstest.MapFS{"shapes.svg": {Data: []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")}, "dir": {Mode: fs.ModeDir}}}
	junk, _ := o.Add("notes.bin", []byte("\x00\x01\x02"))
	if kind, err := ProbeFS(o, "shapes.svg", ""); err != nil || kind != "svg" {
		t.Fatalf("kind=%q err=%v", kind, err)
	}
	if _, err := ProbeFS(o, junk, ""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("junk: %v", err)
	}
	if _, err := ProbeFS(o, "dir", ""); err == nil {
		t.Fatal("directory accepted")
	}
	// Without a filesystem, the host rules apply.
	if _, err := ProbeFS(nil, t.TempDir(), ""); !errors.Is(err, ErrDirectory) {
		t.Fatalf("host directory: %v", err)
	}
}

func TestSkippedWording(t *testing.T) {
	got := Skipped("a\x1b]2;t\a.dmg", ErrUnsupported)
	if !strings.HasPrefix(got, "gloss: a") || !strings.HasSuffix(got, ".dmg: unsupported format (skipped)") || strings.Contains(got, "\x1b") {
		t.Fatalf("%q", got)
	}
	if got := Skipped("dir", ErrDirectory); got != "gloss: dir: is a directory (skipped)" {
		t.Fatalf("%q", got)
	}
	_, err := Probe(filepath.Join(t.TempDir(), "gone.png"), "")
	if got := Skipped("gone.png", err); got != "gloss: gone.png: no such file or directory (skipped)" {
		t.Fatalf("%q", got)
	}
}

func TestExtensionsAreTheOnesDetected(t *testing.T) {
	if len(Extensions) == 0 {
		t.Fatal("no extensions")
	}
	for _, ext := range Extensions {
		if ext != strings.ToLower(ext) || !strings.HasPrefix(ext, ".") {
			t.Errorf("%q must be a lower-case extension", ext)
		}
		if _, err := Detect("file"+ext, []byte("no telling"), ""); err != nil {
			t.Errorf("%s: %v", ext, err)
		}
	}
	for _, name := range []string{"file.dmg", "file.txt", "file", "file.go"} {
		if _, err := Detect(name, []byte("no telling"), ""); !errors.Is(err, ErrUnsupported) || slices.Contains(Extensions, filepath.Ext(name)) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
