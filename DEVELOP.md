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
- `web`: the demo site, the page `--serve` shows, and the experimental plain-JS
  `--pick-web` request page (`pick.html`, `pick.mjs`, `pick-api.mjs`).
- `internal/app`: terminal pager, selection menu, and Markdown layout.
- `internal/browse`: the file chooser behind the `o` browser, a Bubble Tea
  component of its own (see below); `internal/browse/browsetest` is its test
  harness.
- `internal/document`: bounded loaders, renderers, and vision image sizing.
- `internal/app/qr.go`: table URL overlay using the external ntcharts-qrcode component.
- `examples`: small runnable fixtures.
- `scripts`: site building and fixture generation.
- `docs/hugo`: the documentation site, published at `/docs/` beside the demo.
- `skills/gloss`: the agent skill the binary carries (`gloss skill`).

## Animated GIFs

`internal/document/gif.go` scans GIF blocks before `image/gif.DecodeAll` to
bound the logical canvas (32 MP), frame count (1,000), and aggregate paletted
frame pixels (64 Mi). The standard library decodes LZW and palettes. Static
loads decode only the first frame and composite it onto the logical canvas;
previews, inline images, and headless exports do not enable animation.

`GIFPlayer` holds immutable decoded frames and a playback position. `Next`
returns a fresh composed canvas, honoring transparency and the
[GIF89a disposal rules](https://www.w3.org/Graphics/GIF/spec-gif89a.txt).
It uses [Go's loop-count semantics](https://pkg.go.dev/image/gif#GIF): -1 plays
once, 0 repeats forever, and positive values count additional repetitions.
A disposal-previous snapshot is kept only while needed. Render and PNG export
commands can retain previous images without concurrent mutation. Sub-20-ms
delays use 100 ms to avoid busy playback; other frame delays are preserved.
The viewer applies its 0.25×–4× speed multiplier after that normalization,
keeping the minimum playback delay at 5 ms. A speed change cancels the current
timer/composition epoch and starts a full new delay for the current frame;
pause, finished playback, and Kitty transmission backpressure still apply.
Speed belongs to the loaded animation: resize/zoom and Space restart retain it;
reload or loading another file starts at 1×. Headless output is unaffected.

`internal/app/animation.go` owns cancellable timers and off-loop composition.
Messages carry a playback owner and epoch, so pause, reload, navigation, and
quit invalidate late work. Hidden documents cancel timers. Kitty waits for
transmission before starting the next delay; it never builds a frame backlog.
The last transmitted Kitty picture stays visible while its replacement encodes.
Only after transmission does the view swap grids and delete the old image, so
picture's transitional glyph fallback never flashes between animation frames.
Window and font-size changes cancel pending composition and render into a fresh
placement at the new geometry. Until it is ready, the old placeholder grid is
clipped to the viewport so it cannot wrap or displace the status bar. Obsolete
resize completions cannot present over the latest layout; pause is preserved.
At most a visible front picture and a pending replacement are retained; leaving
or explicitly choosing glyphs cleans up both. Keep-screen quit retains the
visible frame, and PNG export snapshots that frame even during encoding.
Fresh image IDs avoid ghostty-web's texture cache, consuming the slots inside
blocks already reserved by `nextKittyID`. Retired placements receive a second
ID-specific cleanup if an accepted transmission completes late. Glyphs use
the same picture pipeline as static images. `e` captures the immutable current
frame before its asynchronous export; it never reloads the first frame.

Tests cover composition, offsets, background/previous disposal, delays, finite
and infinite loops, immutable snapshots, pre-decode bounds, cancellation,
stale work, export, and Kitty pacing/cleanup. `examples/motion.gif` is an
original generated fixture; `scripts/gen-assets/gif.go` regenerates it.

## QR component

The reusable encoder and terminal component live in
[ntcharts-qrcode](https://github.com/NimbleMarkets/ntcharts-qrcode), imported as
`github.com/NimbleMarkets/ntcharts-qrcode/qrcode`. Gloss pins a repository
revision in `go.mod`; there is no local replacement or copied implementation.
The library owns module/image generation, quiet zones, Kitty and half-block
rendering, bounds, fit errors, and image cleanup. Its decoder and lifecycle
unit tests live with the component. See its README and DEVELOP for the API,
encoder assessment, and NTCharts exact-size rendering contract.

The pinned revision (`v0.0.0-20261005213812-74d99c6f0981`) uses
`skip2/go-qrcode`. Gloss uses its fixed Medium error-correction default
(approximately 15% recovery). It selects a symbol that fits the encoded
content; it does not automatically raise the correction level. The API accepts
valid UTF-8, but this revision does not add explicit UTF-8 ECI segments.
Do not assume a local workspace's encoder or segment behavior ships in a release.

`internal/app/qr.go` owns the overlay, selected table URL, and export through
gloss's existing non-overwriting save hook. The app supplies `nextKittyID`,
its detected graphics mode and cell geometry, and space inside the overlay.
It forwards event-loop messages and executes commands from every setter,
`Update`, and `Close`. `internal/app/qr_test.go` retains the host integration
checks for keys, layout, removal/replacement cleanup, and PNG export decoding.

Try `./gloss examples/qr-links.csv`, select a URL (Down), and press `u`.
`e` exports `qr.png`. The compact overlay shows a single URL footer, ellipsized
when needed; closing it returns to the original table cell. The complete code
and four-module quiet zone are preserved. No URL is fetched by displaying it,
and localhost URLs do not become reachable from another device.

For joint local development with sibling checkouts, run:

```sh
go work init . ../ntcharts-qrcode
(cd examples/demo && go work init . ../.. ../../../ntcharts-qrcode)
```

If a workspace already exists, use `go work use` to add the same paths.
Both `go.work` files and their sums are ignored. The separate demo workspace
keeps its Bubble Tea WASM replacement out of the native build. Ordinary
`task build` and `task demo-check` then use the local library.
With an active workspace, `task build` always invokes Go so edits in sibling
modules cannot leave a stale binary. Go's own incremental cache still applies.
`task ci`, `task notices`, `task notices-check`, and `task release` always set
`GOWORK=off`: release checks and notices must describe the committed module
pins, not sibling checkouts. CI also forces a build so an earlier workspace
binary cannot be reused. GoReleaser disables workspaces as well.
Run `GOWORK=off task ci` and `GOWORK=off task demo-check` before tagging.
When updating the library, update both module pins (prefer a published tag),
regenerate notices, and validate with workspaces disabled. Do not commit a
filesystem `replace` into either module.

The standalone library's `examples/qrcode` demonstrates two components and
has native and WASM builds. Real terminal/font/tmux and phone-camera checks
remain manual acceptance checks; automated decoders do not replace them.

## The file browser

`internal/browse` is a component of its own: it knows nothing of gloss, and
takes what gloss adds (marks, sort, which files may be chosen) as options, over
any `fs.ReadDirFS`. `internal/app/opener.go` is the adapter. Folders are read
by commands and cached; a *layout* only draws that state (`list`, `columns`,
`places`), so keys, filter, and completion are the same in each. Paths are
slash-separated from the filesystem's root.

Its tests drive it through `internal/browse/browsetest` (see its README), which works for any
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

## Plain web picker

`gloss --pick-web --prompt "Choose a receipt" --timeout 10m` serves the
experimental HTML/JS upload page, without starting Booba or a terminal model.
Off a terminal it detaches as usual; open the returned `url`, then use
`gloss --resume TOKEN` to retrieve the confirmed paths. Localhost is the default;
only this mode supports opt-in network binding.

`cmd/gloss/pick_network.go` separates `--listen IP:port` (default
`127.0.0.1:0`) from `--advertise-host IP-or-DNS-name`. Validation happens before
detaching and again at server creation. Listeners use explicit `tcp4`/`tcp6`
families, so wildcard behavior is consistent across operating systems. A
wildcard requires a non-loopback advertised host; a specific IP supplies its
own default.
DNS names are advertised aliases, never resolved for binding or authorization.
Interface names, scoped/link-local addresses, and reverse proxies are deferred.

After the socket opens, an immutable host policy captures the actual port,
advertised host, specific bind IP if any, and `localhost` for loopback binds.
Wildcard listeners accept only the advertised authority, not arbitrary local
IPs or DNS names. IP literals, DNS case, and HTTP's default port are normalized;
the API requires Origin to match the requesting authority when present.
Forwarded headers cannot override either check. Token checking precedes all
page/API access, and the full viewer retains its localhost policy. Advertising
a Tailscale name is not an interface or client access restriction. HTTPS proxy
support needs a separate explicit origin/trust design.

The same URL is announced on stderr and persisted for detached startup JSON.
No protocol fields or exit codes change. Tests cover wildcard and IPv6 socket
binding, advertised DNS without external resolution, denied hosts/origins,
port failures, and default/wildcard detached upload-and-message round trips.
They cannot prove another device can route to the address: check real LAN and
Tailscale access manually, including client isolation and firewall policies.

Foreground network picks show a QR on terminal stderr automatically.
`cmd/gloss/pick_qr.go` selects this only with terminal stdin/stderr, no detached
token, and a non-loopback listener/advertised host. It runs the server wait
alongside `internal/app/handoff.go`, a small Bubble Tea screen using the same
`terminalPicture` setup and update helpers as the pager. The existing QR
component receives capability/geometry updates and uses the shared image-ID
allocator. There are no synchronous terminal probes or separate input readers.
The server owns settlement; its completion closes the screen through Tea so
ID-specific graphics cleanup runs before quitting. Terminal cancellation
cancels the server context. OS signals remain owned by `served`, not a second
Bubble Tea signal handler. Stdout never carries the screen. Tests decode its
half-block output independently and exercise resizing, graphics toggling, and
settlement cleanup. The detached startup protocol stays unchanged.

`cmd/gloss/pick_web.go` owns the request state. Under the token URL, `GET files`
lists completed uploads as `{state, files: [{id, name, size}]}`; `POST files`
accepts multipart uploads and returns the new file entries; `DELETE files/ID`
removes one; `POST confirm` takes `{ids: [...], message?: "..."}`;
`POST decline` declines. Message text is bounded to 2,000 Unicode code points,
with a 32 KiB confirmation-body cap allowing JSON escapes and 200 upload IDs.
Whitespace-only text becomes absent; other whitespace and Unicode are preserved.
An accepted confirmation can only be retried with the same paths and message.
Only IDs name uploads across this boundary. The page cannot read host paths or
download files. `pick-api.mjs` is the frontend boundary a hosted prototype could
replace; no hosted service or deployment is included here.

API operations serialize upload, removal, and confirmation. The existing
bounded receiver stores files; settlement drains the HTTP response before
shutdown and removes unconfirmed uploads. Refresh recovers completed uploads;
the final result remains in detached status/resume state, not at the page URL.
The page keeps the unsent message in session storage scoped to the token URL,
clearing it on Send/Cancel. Storage failures do not prevent sending. Accepted
messages are persisted with the detached answer and exposed only in JSON;
ordinary stdout stays paths-only. The protocol remains version 1: the optional
`message` field is an additive change. The reflected schema documents it.
There is no durable browser receipt or interrupted-upload resumption in this
picker. Request tests cover the detached round trip, token/origin checks,
confirmation IDs, CSP and other security headers, 200/201-file boundaries,
255-byte filename limits, concurrent uploads and removal/confirmation races,
and QR wait exit codes. `task ci` exercises these under the race detector;
`task web-check` tests the JS adapter.

Uploads have a fixed two-minute total deadline in both `http.Server.ReadTimeout`
and `web/pick-api.mjs`, not an idle timeout. A 128 MiB file needs about 9 Mbps
of uplink before overhead; slower links must send smaller files. `--timeout`
controls the session lifetime and does not extend this per-upload deadline.

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
app with `go:embed` and read through the same document loaders. User files are never uploaded to the public site. URL drops and app `?src=`
links fetch directly in the browser with CORS, credentials omitted, no referrer,
a one-minute deadline, and a streamed byte limit. `--accept` (or `?accept=`
in the public app) validates content using the Go document code before adding it.
There is no public fetch proxy. Browser PDF rendering uses the NTCharts PDFium bridge,
which loads PDFium from the `@embedpdf/pdfium` npm package. The generated shim
points at a CDN, so `scripts/build-site.sh` instead downloads that exact version
from the npm registry, checks it against a pinned SHA-512, serves it from
`vendor/embedpdf-pdfium` on the site, and rewrites the shim to match (and fails
if a CDN address is left). To move to another version, change the version and
hash together in that script. Every page then runs only code from its own
origin, which its Content-Security-Policy (a `<meta>` tag in each page) enforces;
requests to other hosts are limited to document addresses a visitor asks to open. Other
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
