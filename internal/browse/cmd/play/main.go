// Command play is the file chooser in your terminal over an in-memory
// filesystem, the way the browsetest harness sees it: to try it, to record
// what you do as a script, and to replay a script step by step.
//
//	go run ./internal/browse/cmd/play                       # the demo tree
//	go run ./internal/browse/cmd/play -script FILE          # that script's tree and options
//	go run ./internal/browse/cmd/play -from ~/Downloads     # a copy of a real folder
//	go run ./internal/browse/cmd/play -record NEW.txt       # and write what you do as a script
//	go run ./internal/browse/cmd/play -replay -script FILE  # watch a script, its checks shown
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NimbleMarkets/gloss/internal/browse/browsecfg"
	"github.com/NimbleMarkets/gloss/internal/browse/browsetest"
)

func main() {
	var (
		script = flag.String("script", "", "a script whose filesystem, start, and options are used (and replayed with -replay)")
		from   = flag.String("from", "", "a real folder, copied (names, sizes, dates) into the filesystem under /fixture")
		depth  = flag.Int("depth", 4, "how deep -from looks")
		limit  = flag.Int("limit", 3000, "how many entries -from takes")
		record = flag.String("record", "", "write what you do to this file, as a script (Ctrl-C ends)")
		replay = flag.Bool("replay", false, "step through -script instead of taking keys")
		size   = flag.String("size", "", "the room given to the chooser, WxH (recordings: default 90x20)")
		delay  = flag.Duration("delay", 700*time.Millisecond, "the pace of a replay playing by itself")
		start  = flag.String("start", "", "the folder to start in")
	)
	var options optionFlags
	flag.Var(&options, "option", "an option for the chooser, NAME=VALUE ("+browsecfg.Options+"); repeatable")
	flag.Parse()

	o := browsetest.PlayOptions{
		Script: *script, Record: *record, Replay: *replay, Delay: *delay, Start: *start,
		Options: options.m, AssertKeys: browsecfg.StateKeys,
	}
	switch {
	case *from != "":
		lines, err := browsetest.FSFromDir(*from, "/fixture", *depth, *limit)
		if err != nil {
			fail(err)
		}
		o.FS = lines
		if o.Start == "" {
			o.Start = "/fixture"
		}
		fmt.Fprintf(os.Stderr, "copied %d entries of %s to /fixture\n", len(lines), *from)
	case *script == "":
		o.FS = browsetest.DemoLines()
		if o.Start == "" {
			o.Start = "/home/evan"
		}
	}
	if *size != "" {
		w, h, ok := strings.Cut(*size, "x")
		var err1, err2 error
		o.Width, err1 = strconv.Atoi(w)
		o.Height, err2 = strconv.Atoi(h)
		if !ok || err1 != nil || err2 != nil {
			fail(fmt.Errorf("-size wants WxH, such as 90x20"))
		}
	} else if *record != "" {
		o.Width, o.Height = 90, 20
	}
	if err := browsetest.Play(browsecfg.Config(), o); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "play:", err)
	os.Exit(1)
}

type optionFlags struct{ m map[string]string }

func (o *optionFlags) String() string { return "" }
func (o *optionFlags) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok {
		return fmt.Errorf("want NAME=VALUE, not %q", s)
	}
	if o.m == nil {
		o.m = map[string]string{}
	}
	o.m[k] = v
	return nil
}
