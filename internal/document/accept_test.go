package document

import (
	"testing"
)

func TestAcceptContentNotFilename(t *testing.T) {
	images, err := ParseAccept("image/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"photo.png", "photo", "lie.html"} {
		if err := images.Check(name, pngBytes(t)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"login.png", "login.md", "login"} {
		if err := images.Check(name, []byte("<!doctype html><html>Sign in</html>")); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	if err := images.Check("x.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAccept("pdf,nonsense"); err == nil {
		t.Fatal("accepted unknown filter")
	}
	combined, _ := ParseAccept("image/*,pdf")
	if err := combined.Check("x", []byte("%PDF-1.7\n")); err != nil {
		t.Fatal(err)
	}
}

func TestDropURL(t *testing.T) {
	for _, text := range []string{"https://example.test/a.png", " https://example.test/a?x=1&y=2\n"} {
		if DropURL(text) == "" {
			t.Fatalf("ignored %q", text)
		}
	}
	for _, text := range []string{"https://a.test\nhttps://b.test", "file:///etc/passwd", "https://user:secret@a.test", "look https://a.test", "https://"} {
		if DropURL(text) != "" {
			t.Fatalf("accepted %q", text)
		}
	}
}
