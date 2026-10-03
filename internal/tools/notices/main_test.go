package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoLicense(t *testing.T) {
	for _, tc := range []struct {
		name, root, local, parent, want string
	}{
		{"standard", "go", "Go license", "", "Go license"},
		{"homebrew", "libexec", "", "Go license", "Go license"},
		{"prefer GOROOT", "libexec", "Go license", "other license", "Go license"},
		{"missing standard", "go", "", "", ""},
		{"missing homebrew", "libexec", "", "", ""},
		{"unrelated parent", "go", "", "other license", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, tc.root)
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, text := range map[string]string{
				filepath.Join(root, "LICENSE"): tc.local,
				filepath.Join(dir, "LICENSE"):  tc.parent,
			} {
				if text != "" {
					if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			got, err := goLicense(root)
			if tc.want == "" {
				if err == nil || !strings.Contains(err.Error(), filepath.Join(root, "LICENSE")) {
					t.Fatalf("missing license: got %q, error %v", got, err)
				}
				if tc.root == "libexec" && !strings.Contains(err.Error(), filepath.Join(dir, "LICENSE")) {
					t.Errorf("error omits the fallback path: %v", err)
				}
				return
			}
			if err != nil || string(got) != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
