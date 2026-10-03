// Command tape writes a browsetest script as a VHS tape.
//
//	go run ./internal/browse/browsetest/tape -cmd "gloss examples" -o browser.gif \
//	    internal/browse/testdata/scripts/columns.txt > browser.tape
//	vhs browser.tape
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

func main() {
	var o browsetest.TapeOptions
	flag.StringVar(&o.Command, "cmd", "", "the command that starts the program, typed at a shell")
	flag.StringVar(&o.Output, "o", "", "the file the tape records, such as browser.gif")
	flag.IntVar(&o.FontSize, "font", 0, "the font size (default 16)")
	flag.StringVar(&o.Pause, "pause", "", "how long to wait after each step (default 500ms)")
	keys := flag.String("keys", "", "stand-ins for keys VHS lacks, as ours=VHS pairs: alt+up=Ctrl+Up,alt+left=Ctrl+O")
	flag.Parse()
	if *keys != "" {
		o.Keys = map[string]string{}
		for _, pair := range strings.Split(*keys, ",") {
			from, to, ok := strings.Cut(pair, "=")
			if !ok {
				fmt.Fprintf(os.Stderr, "tape: -keys wants ours=VHS pairs, not %q\n", pair)
				os.Exit(2)
			}
			o.Keys[from] = to
		}
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: tape [-cmd COMMAND] [-o OUTPUT] SCRIPT")
		os.Exit(2)
	}
	tape, err := browsetest.Tape(flag.Arg(0), o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tape:", err)
		os.Exit(1)
	}
	fmt.Print(tape)
}
