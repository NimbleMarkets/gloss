package browse

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// What a file is called is whatever its owner typed: it may hold bytes that
// are not text, which a terminal would act on (a bell, a carriage return, an
// escape sequence that sets the title), and sequences of characters whose
// width terminals disagree on (a family of people joined with zero-width
// joiners is two cells to one table and six to another; a heart with an emoji
// selector, two or one). A name is drawn as a name, never as what it says.

// cleanRunes is a name made safe to draw, and where each of its original
// letters went: at[i] is the index in the result of the i-th original rune, or
// -1 if it was dropped. Control characters are shown as the symbols for them
// (␇ for a bell, ␛ for escape), and a cluster of characters that the two width
// tables count differently is drawn as its first character alone.
func cleanRunes(s string) (out []rune, at []int) {
	at = make([]int, 0, len(s))
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		runes := g.Runes()
		switch {
		case len(runes) == 1:
			at = append(at, len(out))
			out = append(out, visible(runes[0]))
		case hasControl(runes): // A carriage return and a line feed make one cluster.
			for _, r := range runes {
				at = append(at, len(out))
				out = append(out, visible(r))
			}
		case ansi.StringWidth(g.Str()) == ansi.StringWidthWc(g.Str()): // The tables agree: as it is.
			for _, r := range runes {
				at = append(at, len(out))
				out = append(out, r)
			}
		default: // They do not: the first character alone, the joiners, selectors, and modifiers left off.
			at = append(at, len(out))
			out = append(out, runes[0])
			for range runes[1:] {
				at = append(at, -1)
			}
		}
	}
	return out, at
}

// cleanName is cleanRunes as a string.
func cleanName(s string) string {
	if plain(s) {
		return s
	}
	r, _ := cleanRunes(s)
	return string(r)
}

// plain says a string needs no cleaning: printable ASCII.
func plain(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] >= 0x7f {
			return false
		}
	}
	return true
}

func hasControl(runes []rune) bool {
	for _, r := range runes {
		if r < 0x20 || r >= 0x7f && r < 0xa0 {
			return true
		}
	}
	return false
}

// visible is a character as drawn: a control character as its symbol.
func visible(r rune) rune {
	switch {
	case r < 0x20:
		return 0x2400 + r // ␀ to ␟
	case r == 0x7f:
		return 0x2421 // ␡
	case r >= 0x80 && r < 0xa0:
		return '�'
	}
	return r
}
