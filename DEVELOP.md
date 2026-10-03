# Developing gloss

gloss is written in Go on [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [NTCharts](https://github.com/NimbleMarkets/ntcharts), with
[NTCharts SVG](https://github.com/NimbleMarkets/ntcharts-svg),
[NTCharts PDF](https://github.com/NimbleMarkets/ntcharts-pdf), and
[NTCharts3d](https://github.com/NimbleMarkets/ntcharts3d). The user
documentation is on the [documentation site](https://nimblemarkets.github.io/gloss/docs/);
this file is for working on gloss itself. [AGENTS.md](AGENTS.md) is the same
ground for coding agents.

## Build and run

Requires Go **1.26.8+** and [Task](https://taskfile.dev/) for the development
commands. Without Task, build with `go build -o gloss ./cmd/gloss`. Go's
automatic toolchain selection can download that version. Dependencies are
pinned in `go.mod`; sibling checkouts aren't needed. Clone with
`--recurse-submodules` (or run `git submodule update --init`) to get the
documentation theme.

```sh
task build                       # ./gloss
task run -- photo.png            # go run, with arguments
task install                     # installs gloss into your Go bin directory
./gloss examples/shapes.svg examples/tetrahedron.stl
go build -ldflags '-X main.version=0.1.0' -o gloss ./cmd/gloss
```

`task --list` lists the development commands.

## Test

```sh
task test
task fuzz                       # fuzz each loader, 60s apiece (FUZZTIME=5m, FUZZ=FuzzParseSTL)
task ci                         # formatting, modules, race tests, vet (also for Windows), build, notices, docs
```

Tests cover CLI validation, malformed files, STL geometry, PDF rendering and
navigation, SVG rasterization, viewport cropping, terminal-safe labels, and
stale asynchronous results. The example SVG and STL are small original
fixtures; `task gen-assets` regenerates the PNG, HEIC, PDF, the block-letter
STL, and, with `python3`, the Grist document, which is written by hand to
Grist's layout rather than saved from Grist.

GitHub Actions runs `task ci` on Linux and macOS for pushes and pull requests. Windows is only
cross-compiled and vetted (`task cross-windows`), not tested.

## Layout

The layout follows NTCharts' conventions, with one module for the CLI and a
separate browser-demo module:

- `cmd/gloss`: CLI flags, stdin handling, export orchestration, and the
  temporary server behind `--serve`.
- `web`: the demo site, and the page `--serve` shows.
- `internal/app`: terminal pager, selection menu, and Markdown layout.
- `internal/browse`: the file chooser behind the `o` browser, a Bubble Tea
  component of its own (see below); `internal/browse/browsetest` is its test
  harness.
- `internal/document`: bounded loaders, renderers, and vision image sizing.
- `examples`: small runnable fixtures.
- `scripts`: site building and fixture generation.
- `docs/hugo`: the documentation site, published at `/docs/` beside the demo.
- `skills/gloss`: the agent skill the binary carries (`gloss skill`).

## The file browser

`internal/browse` is a component of its own: it knows nothing of gloss, and
takes what gloss adds (marks, sort, which files may be chosen) as options, over
any `fs.ReadDirFS`. `internal/app/opener.go` is the adapter. Folders are read
by commands and cached; a *layout* only draws that state (`list`, `columns`,
`places`), so keys, filter, and completion are the same in each. Paths are
slash-separated from the filesystem's root.

Its tests drive it through `internal/browse/browsetest`, which works for any
component with `Init`, `Update`, and `View`: an in-memory filesystem (with
latency, injected read errors, and read counts), a driver that sends keys and
clicks and settles the commands that follow, and scripts. A script is a file in
`internal/browse/testdata/scripts`: an `fs` part listing files and a `script`
part of commands (`press`, `type`, `click`, `snapshot`, `state`, ...; see
`browsetest.RunDir`). Snapshots are compared with the `.golden` file beside the
script, which is a readable screen:

```sh
task browse:test                                 # run them
task browse:screens                              # write the golden screens
```

Scripts understand the words of [VHS](https://github.com/charmbracelet/vhs)
tapes where the two overlap (`Type "re"`, `Down 2`, `Ctrl+L`, `Alt+Up`,
`Screenshot`; recording commands such as `Sleep` and `Set` are ignored), so a
tape reads as one. What VHS has no word for is ours: `fs`, `state`, `expect`,
`reads`, `click`. A script can also become a tape, to record a GIF of the
behavior it tests:

```sh
task browse:tape SCRIPT=columns     # dist/browse/columns.tape
task browse:gif SCRIPT=columns      # records dist/browse/columns.gif (needs vhs, ttyd, ffmpeg)
task browse:gifs                    # the showcase scripts
```

The tasks are the usual ones: `task browse:test`, `task browse:screens` (rewrite
the golden screens), and `task browse:bench`. `CMD`, `KEYS`, and `PAUSE` change
what the tape runs, the keys it substitutes, and its pace; the tool behind
them is `go run ./internal/browse/browsetest/tape` (see its `-h`).

The tape types the command, presses the keys, and leaves out what a recording
cannot do (checks, clicks, resizes, the `fs` part: it runs in a real folder).
VHS has no Home or End, and takes Alt only with a character, so a script's
`alt+up` is left as a comment unless `-keys alt+up=Ctrl+Up,alt+left=Ctrl+O`
stands in keys the browser also answers to. With `vhs` installed, a test
checks that every script makes a tape VHS accepts.

### Playing with it

The harness can also be driven by a person, on the same in-memory trees the
scripts use:

```sh
task browse:play                              # the chooser over a demo tree
task browse:play -- -from ~/Downloads         # over a copy of a real folder (names, sizes, dates)
task browse:play -- -script internal/browse/testdata/scripts/places.txt   # a script's tree and options
task browse:record NAME=thing                 # play, and keep what you do as a script
task browse:replay SCRIPT=places              # watch a script step by step, its checks shown
```

While recording, `F1` takes a snapshot, `F2` records checks of the state
(`state dir …`, `state current …`), `F3` leaves a note to edit; `Ctrl-C` ends and
writes the script, and `browse:record` then writes its golden screens. The
recording keeps the typing, keys, clicks, and wheel as the script words for them,
at a fixed size (90x20) so goldens stay small and alike, and puts the tree it
used in the script's `fs` part (look before sharing a `-from` copy: names and
sizes are real). In a replay `Space` does a step and then its checks, `p` plays
by itself (`+`/`-` change the pace), `r` starts again, `q` leaves; the status line
shows each check as it passes or fails, and the command exits non-zero if any did.

### Soundness checks

Every settled screen of a script is checked (`Screen.Problems`) for the faults
that mess up a terminal: more rows than there are; a row wider than the terminal
by **either** of two width tables, grapheme clusters and wcwidth, which disagree
about some emoji (a family of people joined with zero-width joiners is two cells
to one and six to the other), so a row that fits by one and overflows by the
other wraps on some terminals; and a control character in the text, as a file
name can carry. `col TEXT N` asserts the cell column something is drawn at, which
a wide character shifts from where letters would say, and `reject-raw TEXT`
asserts that nothing a name says reaches the terminal as an escape sequence.
`TestNoSizeOrNameMakesAnUnsoundScreen` sweeps every layout over 26 widths and 9
heights with awkward names (CJK, emoji, joined and selected sequences, combining
marks, very long names, bells, escapes) and fails on any unsound screen. Names
are drawn as names (`names.go`): control bytes as their symbols (`␇`, `␛`), and a
cluster the two tables count differently as its first character.

To try an idea, add a script, run it with `-update-screens`, and read the
golden file; then keep it. After changing how anything is drawn, the diff of
the golden files is the review. `go test -bench . ./internal/browse` times
typing and drawing in a folder of 100,000 files.

## Documentation

The documentation is written from the code where it can be, so it cannot say
what gloss does not do. gloss has one default command, `view` (what `gloss FILE`
runs), and two small ones, `skill` and `help`; `view`'s options are grouped into
*domains* (opening and viewing, documents, meshes, exporting, text and details,
handing files over, agents), and `cmd/gloss/domains.go` is where an option is
given its domain. A test fails for an option that has none.

```sh
task docs                       # the gloss(1) man page, shell completions, the command reference, two guide pages
task docs:hugo:serve            # the site, while you edit it (needs hugo, extended)
task docs:hugo:build            # the site, in docs/hugo/public
```

- The **command reference**, the **man page**, and the **shell completions** are
  generated by gloss itself (`--docs-markdown`, `--docs-man`, `--docs-completions`,
  hidden from `--help`) from its options. What an option's value may be (`--type`,
  `--view`, and so on) is listed in `cmd/gloss/completions.go`; a test holds each
  list to the option's usage text.
- The **guide** (`docs/hugo/content/guide`) is written by hand, except two pages
  that `internal/tools/docsite` makes: *For LLMs*, from `skills/gloss/SKILL.md`,
  and *Development*, from this file. Edit those sources, not the generated pages.
- The theme, [hugo-book](https://github.com/alex-shpak/hugo-book), is a Git
  submodule. The site wears the Nimble brand from `docs/hugo/assets/_custom.scss`
  (the palette in both color modes, and the same Open Sans fonts as the demo,
  mounted from `web/fonts`), with a wordmark partial in `docs/hugo/layouts`.
- The man page and the completions ship in the release archives, the Debian
  package, and the Homebrew cask (`manpages` and `completions` in
  `.goreleaser.yaml`).

## Releases

Pushing a `v*` tag runs checks, and [GoReleaser](https://goreleaser.com)
packages macOS, Linux, and Windows amd64 and arm64 binaries: archives, `.deb`
packages, and SHA-256 checksums go to a GitHub Release, and a cask to the
[Homebrew tap](https://github.com/NimbleMarkets/homebrew-tap). Locally,
`task release` produces the same in `dist/` as a snapshot, without publishing.
Release binaries use software STL rendering when native GPU support is
unavailable.

## The browser demo

```sh
task demo                         # embedded gallery in your terminal
task demo -- --sample landscape.heic
task serve-wasm-site               # http://localhost:8000
task build-wasm-site               # static site in web/dist
task web-check                     # browser helper tests; Node 18+
```

`examples/demo` pins the same Bubble Tea WASM fork used by the NTCharts demos;
this replacement does not affect the native CLI. Samples are compiled into the
app with `go:embed` and read through the same document loaders. No user files
are fetched or uploaded. Browser PDF rendering uses the NTCharts PDFium bridge,
which loads PDFium from the `@embedpdf/pdfium` npm package. The generated shim
points at a CDN, so `scripts/build-site.sh` instead downloads that exact version
from the npm registry, checks it against a pinned SHA-512, serves it from
`vendor/embedpdf-pdfium` on the site, and rewrites the shim to match (and fails
if a CDN address is left). To move to another version, change the version and
hash together in that script. Every page then runs only code from its own
origin, which its Content-Security-Policy (a `<meta>` tag in each page) enforces;
the only request to another host is a `?src=` address a visitor asks for. Other
runtime assets are served alongside the site. Meshes are drawn with WebGPU where the browser
has it, and by the software renderer where it does not.

Embed the standalone terminal on another site:

```html
<iframe src="https://nimblemarkets.github.io/gloss/demo.html?sample=field-guide.pdf"
        title="gloss live terminal" width="100%" height="560"
        style="border:0" loading="lazy"></iframe>
```

Omit `sample` to start in the file menu; accepted filenames are listed in
`examples/assets.go`. The native keys work in the demo; quitting offers a
restart button. Choosing another format restarts the embedded terminal at that
sample; clicking the active format preserves the session. Restart explicitly
reloads it. The loading screen reports received bytes and compilation and
startup stages.

The Pages workflow builds for pull requests and deploys pushes to `main`. Set
repository **Settings → Pages → Source → GitHub Actions** to enable hosting.

## License

MIT; see [LICENSE](LICENSE). The licenses of the modules gloss links
are in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
