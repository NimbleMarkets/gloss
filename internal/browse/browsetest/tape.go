package browsetest

import (
	"fmt"
	"strconv"
	"strings"
)

// TapeOptions says how a script becomes a VHS tape.
type TapeOptions struct {
	// Command starts the program in the recording, as typed at a shell:
	// "gloss examples". The script's own start folder and fs part are for the
	// harness; the recording runs against a real folder.
	Command string
	// Output is the file the tape records, such as "browser.gif".
	Output string
	// FontSize is the font size of the recording; the window is sized to the
	// script's size in cells, roughly.
	FontSize int
	// Pause is how long the tape waits after each step, for a viewer to see it.
	Pause string
	// Keys stands in a key VHS can send for one it cannot, by the name the
	// script uses: {"alt+up": "Ctrl+Up"}. VHS has no Home or End, and takes
	// Alt only with a character; a key with no stand-in is left as a comment.
	Keys map[string]string
}

// Tape writes a script as a tape for VHS (https://github.com/charmbracelet/vhs),
// so the behavior that is tested can also be recorded: a GIF for the README
// or the docs, made by the same steps the tests take.
//
// Keys and typed text become VHS's. What a tape cannot say is left out: the
// fs part, checks (expect, reject, state, reads), mouse clicks, resizes, and
// the options of the harness (except the layout, which Ctrl+L changes to
// while the recording is hidden, for a program whose layouts that key cycles); each click, resize, and snapshot is noted as a
// comment or a pause so the tape can be read against the script.
func Tape(file string, o TapeOptions) (string, error) {
	_, steps, err := readScript(file)
	if err != nil {
		return "", err
	}
	if o.FontSize == 0 {
		o.FontSize = 16
	}
	if o.Pause == "" {
		o.Pause = "500ms"
	}
	cols, rows, layoutKeys := 80, 24, 0
	for _, s := range steps {
		switch s.cmd {
		case "size":
			if f := strings.Fields(s.arg); len(f) == 2 {
				cols, _ = strconv.Atoi(f[0])
				rows, _ = strconv.Atoi(f[1])
			}
		case "option":
			// The harness starts in a layout; the program is changed to
			// it by the key that cycles them (list, columns, places).
			if k, v, _ := strings.Cut(s.arg, " "); k == "layout" {
				layoutKeys = map[string]int{"columns": 1, "places": 2}[strings.TrimSpace(v)]
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Made from %s by browsetest.Tape; change the script, not this.\n", baseName(file))
	b.WriteString("# The recording runs against a real folder: the script's fs part is not used.\n")
	if o.Output != "" {
		fmt.Fprintf(&b, "Output %s\n", strconv.Quote(o.Output)) // Quoted: VHS reads a bare absolute path as commands.
	}
	fmt.Fprintf(&b, "Set FontSize %d\n", o.FontSize)
	// A cell is about 0.6 of the font size wide and 1.25 high; padding is
	// VHS's default 60 around.
	fmt.Fprintf(&b, "Set Width %d\n", int(float64(cols*o.FontSize)*0.6)+120)
	fmt.Fprintf(&b, "Set Height %d\n", int(float64(rows*o.FontSize)*1.25)+120)
	if o.Command != "" {
		fmt.Fprintf(&b, "Hide\nType %s\nEnter\nSleep 1s\n", strconv.Quote(o.Command))
		for range layoutKeys {
			b.WriteString("Ctrl+L\nSleep 300ms\n")
		}
		b.WriteString("Show\n")
	}
	pause := func() { fmt.Fprintf(&b, "Sleep %s\n", o.Pause) }
	for _, s := range steps {
		switch s.cmd {
		case "note":
			fmt.Fprintf(&b, "\n# %s\n", s.arg)
		case "press":
			for _, k := range strings.Fields(s.arg) {
				if sub, ok := o.Keys[k]; ok {
					b.WriteString(sub + "\n")
				} else if cmd, ok := tapeKey(k); ok {
					b.WriteString(cmd + "\n")
				} else {
					fmt.Fprintf(&b, "# (%s: VHS has no such key)\n", k)
					continue
				}
				pause()
			}
		case "type":
			fmt.Fprintf(&b, "Type %s\n", strconv.Quote(unquote(s.arg)))
			pause()
		case "snapshot":
			b.WriteString("Sleep 1500ms\n")
		case "click", "wheel", "resize":
			fmt.Fprintf(&b, "# (%s %s: VHS cannot do this)\n", s.cmd, s.arg)
		}
	}
	b.WriteString("Sleep 1s\n")
	return b.String(), nil
}

func baseName(file string) string {
	if i := strings.LastIndexAny(file, `/\`); i >= 0 {
		return file[i+1:]
	}
	return file
}

// tapeKey is a key name of ours as a VHS command, and whether VHS has it.
func tapeKey(name string) (string, bool) {
	names := map[string]string{}
	for vhs, ours := range vhsKeys {
		names[ours] = vhs
	}
	if v, ok := names[name]; ok {
		if v == "home" || v == "end" {
			return "", false
		}
		return titled(v), true
	}
	parts := strings.Split(name, "+")
	if len(parts) == 1 || name == "+" {
		return "Type " + strconv.Quote(name), true // A character.
	}
	base := parts[len(parts)-1]
	named := names[base] != "" && base != "home" && base != "end"
	for _, mod := range parts[:len(parts)-1] {
		switch mod {
		case "alt":
			// VHS takes Alt with a character, or Enter.
			if named && base != "enter" {
				return "", false
			}
		case "shift":
			if base != "tab" && len([]rune(base)) != 1 {
				return "", false
			}
		case "ctrl":
		default:
			return "", false
		}
	}
	if !named && len([]rune(base)) != 1 {
		return "", false
	}
	for i, p := range parts {
		if v := names[p]; v != "" {
			p = v
		}
		parts[i] = titled(p)
	}
	return strings.Join(parts, "+"), true
}

// titled capitalizes, as VHS writes its keys: Enter, Ctrl+L, PageDown.
func titled(s string) string {
	switch s {
	case "pageup":
		return "PageUp"
	case "pagedown":
		return "PageDown"
	}
	if len(s) == 1 {
		return strings.ToUpper(s)
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
