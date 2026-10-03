# browsetest

A test harness for [Bubble Tea](https://github.com/charmbracelet/bubbletea) (v2)
components, written for gloss's file chooser (`internal/browse`) and kept free of
it: it needs only a component with `Init`, `Update`, and `View`. It drives the
component the way a person would, with keys, clicks, and resizes, and records what
the screen looked like as plain text you can read in a diff.

> **Status.** This lives inside gloss for now and is named for the chooser it
> tests. It is meant to be taken out as a general tool; see
> [Taking it out](#taking-it-out) for what is generic, what is not, and what
> would change.

```
 a script (text)  ──►  Driver  ──►  your component (Init / Update / View)
  fs + commands          │                    │
                         │             reads folders from
                         ▼                    ▼
                  Screen (the view)       FS (in memory: latency,
                   │  checked, snapshotted   errors, links, counts)
                   ▼
              .golden files        also: a script ──► a VHS tape (GIF)
                                         a person  ──► play / record / replay
```

## A first test

A component by value, as bubbles' are:

```go
type Component[M any] interface {
	Init() tea.Cmd
	Update(tea.Msg) (M, tea.Cmd)
	View() string
}
```

Tell the harness how to build yours and what state a script may check, then point
it at a folder of scripts:

```go
func TestScripts(t *testing.T) {
	browsetest.RunDir(t, browsetest.Config[chooser.Model]{
		Strict: true, // check every screen: see "Soundness checks"
		New: func(fsys *browsetest.FS, start string, options map[string]string) chooser.Model {
			return chooser.New(fsys, start /* , options by name... */)
		},
		Probe: func(m chooser.Model) map[string]string {
			return map[string]string{"dir": m.Dir(), "filter": m.FilterValue()}
		},
	}, "testdata/scripts")
}
```

and a script, `testdata/scripts/basic.txt`:

```
-- fs --
home/evan/readme.md 1200
home/evan/notes.md 300
home/evan/projects/gloss/main.go
-- script --
start /home/evan
size 60 7
type re
snapshot filtered
state dir /home/evan
state filter re
press down enter
```

`go test -update-screens` writes `basic.golden` beside it, which you review like
any diff and commit:

```
=== filtered (60x7)
 ~
  Filter: readme.md
>   1.2kB 2026-01-02 readme.md
          2026-01-02 projects/


 1/2 matches · 3 in folder · list
```

(`readme.md` after `re` is the chooser's completion ghost: the plain text of the
screen cannot show that the rest is dim.) From then on the screens are compared; a script's `state`, `expect`, `reject`,
and the other checks fail the test on the line that says them, with the keys
pressed so far and the screen at that moment.

## The pieces

### `FS`: an in-memory filesystem

`NewFS(lines...)` makes an `fs.ReadDirFS` rooted like `os.DirFS("/")`: names are
slash-separated with no leading slash, and `.` is the root. A line is a path and
optional fields; folders along the way are made as needed:

```
home/evan/notes.md              a file (100 bytes if no size is given)
home/evan/big.pdf 2048          a file of that size
home/evan/old.txt @2025-12-25   a file with that modification date
home/evan/docs/                 a folder, however empty
home/evan/hi.txt = hello        a file with that text
"home/evan/two words.txt" 12    a name with spaces or odd bytes is Go-quoted
home/evan/here -> /home/evan/docs   a symbolic link, to a folder or a file
```

It can make every folder read slow (`Latency`), make one fail (`Fail`, as when
permission is denied), and it counts what is done to it: `Reads`, `ReadCount`,
`Opens`, `Stats`. A link is followed by `Stat` and `Open` and listed as a link,
and a link to nothing, or to itself, fails. `FSFromDir(dir, mount, depth, limit)`
describes a real folder (names, sizes, dates; not contents) as such lines, and
`DemoLines()` is a tree to try things on.

### `Driver`: sending input, settling the result

```go
d := browsetest.New(t, model, 80, 24) // sends the size, runs Init, settles
d.Press("down", "ctrl+n", "alt+up", "enter")
d.Type("hello")
d.Click(12, 3)
d.Wheel(5, 5, false) // down
d.Resize(60, 10)
d.Run(cmd)           // a command you got by calling the component yourself
scr := d.Screen()
```

After each message the driver runs the commands the component answers with,
batches and sequences included, and feeds the results back in, as the runtime
would. A command that has not answered within `Patience` (250 ms) is taken for a
timer, such as a cursor's blink, and let go, so tests neither sleep nor wait on
blinking, and an asynchronous read has settled by the time the next line runs.

Keys are named as `ParseKey` reads them: `enter` (`return`), `esc`, `tab`,
`backspace`, `delete`, `up`, `down`, `left`, `right`, `home`, `end`, `pgup`,
`pgdown`, `space`, `f1` to `f12`, any single character, and `ctrl+`, `alt+`,
`shift+` in front: `shift+tab`, `ctrl+l`, `alt+left`.

### `Screen`: the view as text

`Lines()` and `String()` are the view with styling removed and trailing spaces
trimmed; `Styled()` and `Raw` keep the escape sequences. `Contains`, `Find` (the
row and the **cell** column, which a wide character makes later than characters
would say), and `Row` look at it.

### Scripts

A script is a `.txt` file of two parts, `-- fs --` and `-- script --`, with `#`
for comments. `RunDir` runs each as a subtest and compares its snapshots with the
`.golden` file beside it. Commands, one to a line:

| Command | Does |
| --- | --- |
| `size W H` | The screen size (before anything is sent; default 80 24) |
| `start PATH` | The folder to start in (default `/`) |
| `option KEY VALUE` | An option for `Config.New` |
| `latency DURATION` | Make each folder read take that long |
| `fail DIR MESSAGE` | Make reading a folder fail |
| `add LINE` | Add to the filesystem, in the form of the `fs` part, any time |
| `press KEY...` | Keys by name |
| `type TEXT` | Text, or a quoted string to keep spaces |
| `click X Y`, `wheel X Y up\|down`, `resize W H` | Mouse and size |
| `snapshot [LABEL]` | Record the screen in the golden file |
| `note TEXT` | Put a remark in the golden file |
| `expect TEXT`, `reject TEXT` | Fail unless / if the screen has the text |
| `reject-raw TEXT` | Fail if the text is drawn, escape sequences included |
| `col TEXT N` | Fail unless the text first appears at cell column N |
| `state KEY VALUE` | Fail unless `Probe` says so (`""` for empty) |
| `reads DIR N`, `opens N`, `stats N` | Fail unless the filesystem was used so |

`size`, `start`, `option`, and `latency` come before the first thing that acts on
the component (which is built at that point); the rest may come at any time.

Where VHS has a word for it, that word does too: `Type "re"`, `Down 2`, `Ctrl+L`,
`Screenshot`, and so on, case-insensitively; `Sleep`, `Set`, `Output`, `Hide` and
other words about recording are ignored. What VHS has no word for (`fs`, `state`,
`expect`, `reads`, `click`) is ours.

### Golden files

`go test -update-screens` rewrites them (the flag is registered by this package).
They are screens, not byte streams: ANSI is stripped, so a change to colors alone
does not show; assert those with `Screen.Styled()` in a Go test.

## Soundness checks

With `Config.Strict` (or `Driver.StrictFit`), every settled screen, after every
action, is checked by `Screen.Problems` for the faults that mess up a terminal:

- **More rows than there are.**
- **A row wider than the terminal, by either of two width tables.** Grapheme
  clusters and wcwidth disagree about some emoji (a family of people joined with
  zero-width joiners is two cells to one and six to the other; a heart with an
  emoji selector, two or one), so a row that fits by one and overflows by the
  other wraps on some terminals, and counts.
- **A control character in the text** (a tab, a carriage return, a bell), as a
  file name can carry.

Escape sequences that style the text are not counted. A test that sweeps your
component over many sizes and awkward names with this on, finding what a single
hand-picked case would not, is the best use of it; `internal/browse/sweep_test.go`
is an example.

## Tapes: scripts as VHS recordings

```sh
go run ./internal/browse/browsetest/tape -cmd "gloss examples" -o demo.gif \
    -keys alt+up=Ctrl+Up,alt+left=Ctrl+O -pause 500ms script.txt > demo.tape
vhs demo.tape     # needs vhs, ttyd, and ffmpeg
```

`Tape(file, TapeOptions)` writes a script as a [VHS](https://github.com/charmbracelet/vhs)
tape, so what is tested can also be recorded as a GIF, by the same steps. It types
the command you give, presses the keys, and leaves out what a recording cannot do:
the `fs` part (the recording runs against a real folder), checks, clicks, and
resizes. VHS has no `Home` or `End` and takes `Alt` only with a character, so a
key it cannot send is left as a comment, or stood in for with `-keys`. With `vhs`
installed, a test (see `tape_test.go`) checks that tapes made from a folder of
scripts are ones it accepts.

## Playing with it: play, record, replay

The harness with a person at the keys, on the same in-memory trees:

```go
browsetest.Play(cfg, browsetest.PlayOptions{
	Script: "testdata/scripts/places.txt", // its tree and options; or FS: lines, Start: "/"
	Record: "new.txt",                     // write what you do as a script
	Replay: false,                         // or step through Script instead
	Width: 90, Height: 20,                 // fix the room, to keep goldens alike
	AssertKeys: []string{"dir", "filter"}, // what F2 records as checks
})
```

- **Play** opens the component full screen over the tree, mouse on.
- **Record** keeps typing, keys, clicks, wheel, paste, and resizes as the script
  words for them, merged into runs, with the tree it used in the `fs` part. `F1`
  takes a snapshot, `F2` records `state` checks of the component, `F3` leaves a
  note to edit, `Ctrl-C` ends and writes the script. It reads back through the
  harness, so a bug found by hand becomes a test.
- **Replay** steps a script visibly: `Space` does a step and then the checks that
  follow it, `p` plays by itself (`+`/`-` change the pace), `r` restarts, `q`
  leaves. Each `state`, `expect`, `reject`, and `reads` check is shown passing or
  failing as it is met, and `Play` returns an error if any failed.

A command that wraps this for your component is a few lines of `main` (see
`internal/browse/cmd/play`). Because the playground takes `F1` to `F3`, a
component that uses them is not a fit as it is.

## Wiring a component

What the harness assumes of one:

- It is a value: `Update` returns the new model, as bubbles' do.
- `View()` returns a `string` of `\n`-separated rows (not a `tea.View`).
- It takes a `tea.WindowSizeMsg` for its size, and `tea.KeyPressMsg`,
  `tea.MouseClickMsg`, and `tea.MouseWheelMsg` for input.
- It reads its data from an `fs.ReadDirFS` it is given, in commands it returns
  from `Update` (as a read of a folder is), so that the harness can settle them.
- Anything a script should check, `Probe` returns by name; anything a script
  should set up, `Config.New` reads from its `options` map.

## Limits

- It is the component only: not the program loop, `tea.Quit`, the alt screen, or
  how the renderer diffs. For those, run the real program under
  [teatest](https://github.com/charmbracelet/x/tree/main/exp/teatest) or a pty.
- Goldens are text: colors and styles are not in them.
- Timers are dropped, so behavior that depends on one (blinking, debouncing) is
  not seen.
- It is not a terminal emulator: widths are measured by two tables, not by
  how any one terminal draws.
- A replay waits about 150 ms for a step to settle, so a slow filesystem can show
  a failed check that the headless harness, which settles exactly, would not.

## Taking it out

It is meant to become its own module. What that would mean:

- **Generic now.** `Component`, `Driver`, `Screen`, `ParseKey`, the script runner
  and golden files, the VHS words and `Tape`, and `Play`/record/replay. They
  import only Bubble Tea v2, Lip Gloss v2, and `charmbracelet/x/ansi`.
- **Filesystem-flavored.** `FS`, the `fs`, `latency`, `fail`, `add`, `reads`,
  `opens`, and `stats` script commands, `FSFromDir`, and `DemoLines` assume a
  component that browses a filesystem. A general tool would make the world a
  component runs in (a filesystem, a clock, a server) something you plug in, with
  `FS` as one such. The `Config.New` signature, which takes an `*FS`, is the main
  thing that would change.
- **Chooser-flavored names.** The package name, `DemoLines`, and the README's
  examples. `play`'s `F1`-`F3` hotkeys are a choice to make configurable.
- **What lives outside this package, with the chooser.** `../browsecfg` (how a
  script's `option` lines build the chooser, and what `state` can read),
  `../cmd/play` (the playground's `main`), `../testdata/scripts` (the chooser's
  scripts and goldens), and `../sweep_test.go`. A general tool would document this
  split: the harness here, the wiring beside the component.
- **To add when it goes.** A stable public API (the unexported `step` and the
  script parser are used by `Tape` and `Play`; a script could be a public type),
  a `Config` for the fake world, versioning, a license, and an example module.
- **Open questions.** Whether the assumptions in "Wiring a component" are the
  right ones, and whether two width tables are the right two, are untested beyond
  the one component this was written for.

## Layout of this package

| File | Is |
| --- | --- |
| `driver.go` | `Component`, `Driver`, `Screen`, `Problems`, `ParseKey` |
| `memfs.go` | `FS` |
| `script.go` | `Config`, `RunDir`, `RunScript`, the commands, golden files |
| `vhs.go` | The VHS words a script understands |
| `tape.go`, `tape/` | Scripts as VHS tapes, and the command that makes them |
| `play.go`, `record.go` | `Play`: the player, the recorder, replay |
| `fixtures.go` | `FSFromDir`, `DemoLines` |
| `*_test.go` | The harness tested with small components of its own |
