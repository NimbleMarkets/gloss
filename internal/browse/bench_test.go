package browse_test

import (
	"fmt"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/browse"
	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

// huge is a folder of n files beside one with a few, as a photo dump is.
func huge(n int) *browsetest.FS {
	f := browsetest.NewFS("data/small/a.txt")
	for i := 0; i < n; i++ {
		f.Add(fmt.Sprintf("data/big/IMG_%06d.jpg 1000", i))
	}
	return f
}

func benchmark(b *testing.B, layout browse.Layout, keys string) {
	f := huge(100000)
	d := browsetest.New(b, browse.New(f, "/data/big", browse.WithLayout(layout)), 120, 40)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Type(keys)
		_ = d.Screen()
		d.Press("ctrl+u")
	}
}

func BenchmarkTypeInHugeFolderList(b *testing.B)    { benchmark(b, browse.LayoutList, "img_00123") }
func BenchmarkTypeInHugeFolderColumns(b *testing.B) { benchmark(b, browse.LayoutColumns, "img_00123") }

func BenchmarkViewHugeFolderColumns(b *testing.B) {
	f := huge(100000)
	d := browsetest.New(b, browse.New(f, "/data", browse.WithLayout(browse.LayoutColumns)), 120, 40)
	// The cursor starts on big/, whose listing is the last column.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.M.View()
	}
}
