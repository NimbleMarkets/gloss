# gloss

A visual pager for the terminal: like `less`, for images, SVGs, PDFs, STL and 3MF meshes, Markdown, and Word and
Excel files.
Built in Go on [NTCharts](https://github.com/NimbleMarkets/ntcharts),
[NTCharts SVG](https://github.com/NimbleMarkets/ntcharts-svg),
[NTCharts PDF](https://github.com/NimbleMarkets/ntcharts-pdf), and
[NTCharts3d](https://github.com/NimbleMarkets/ntcharts3d).

## Live demo

[**Try gloss in your browser →**](https://nimblemarkets.github.io/gloss/)

[![Embedded landscape sample — open the live gloss demo](examples/landscape.png)](https://nimblemarkets.github.io/gloss/)

The live terminal runs the actual Go pager using WebAssembly and Booba, with
embedded PNG, HEIC, SVG, two-page PDF, block-built GLOSS sculpture (STL), and Markdown samples. Choose a format
or use the menu and preview pane. Click the terminal to focus its keyboard.
GitHub READMEs cannot run interactive iframes; the image above opens the Pages demo.

## Build and run

Requires Go **1.26.8+** and [Task](https://taskfile.dev/) for the development commands.
Without Task, build with `go build -o bin/gloss ./cmd/gloss`. Go's automatic toolchain selection can download that
version. Dependencies are pinned in `go.mod`; sibling checkouts aren't needed.

```sh
task build
./bin/gloss photo.png drawing.svg report.pdf model.stl
./bin/gloss examples/shapes.svg examples/tetrahedron.stl
./bin/gloss --menu photo.png report.pdf model.stl
./bin/gloss --preview photo.png report.pdf model.stl
task install                     # installs gloss into your Go bin directory
```

Kitty graphics are selected automatically on supporting terminals, with colored
half-block glyphs as a fallback. The program uses the alternate screen and
restores the terminal on exit. With `-X` (`--no-alt-screen`, as in `less`) it
draws on the main screen instead: the scrollback is left alone, and the last
view stays where it was drawn when you quit. Direct PNG transport works over SSH; no shared
filesystem or external converter is needed. In tmux, enable passthrough with
`set -g allow-passthrough on`.

```sh
gloss --page 12 report.pdf
gloss --dpi 300 report.pdf        # higher PDF raster resolution
gloss --render glyph photo.png   # universal terminal rendering
gloss --render kitty photo.png   # override capability detection
gloss --3d software model.stl    # bypass GPU initialization
gloss --3d wireframe model.stl
cat drawing.svg | gloss
cat model.stl | gloss --type stl -
gloss -- -filename.png
gloss -X photo.png               # keep the scrollback; the picture stays after q
gloss --serve report.pdf         # show the viewer on a web page instead
gloss --serve --pick             # ask the user for a file; print its path
gloss                            # no file yet; drop files or press o to browse
gloss ~/Pictures                 # browse a folder for a file to open
```

Options use `pflag` GNU syntax and may appear before or after filenames. Both
`--page=3` and `-p3` work; boolean short flags can be grouped (`-mP`). Use `--` to
end option parsing. `--type image|svg|pdf|stl|3mf|docx|xlsx|markdown` overrides detection for all
inputs. Content detection supports extensionless files. A `-` reads stdin once
into a temporary file, removed on exit; keyboard input comes from the controlling
terminal. Interactive output must be a terminal; image export works in scripts.
`gloss --help` lists flags.

Unsupported files, directories, and other non-regular entries are reported on
stderr and skipped, so globs can include unrelated entries. Directories are not
traversed. If none remain, gloss exits with an error, unless a folder was
named: then it opens the file browser there. Supported files
still enforce size limits; `--type` explicitly forces an input format.

Common short options: `-h` help, `-V` version, `-m` menu, `-P` preview,
`-X` main screen,
`-p` page, `-d` DPI, `-r` render mode, `-t` type, `-o` output PNG,
`-O` output directory, and `-s` maximum image edge.

```sh
gloss report.pdf -p3 -o page.png --vision-profile openai-high
gloss photo.png drawing.svg model.stl -mP
```

## Controls

| Key | Action |
| --- | --- |
| `q`, `Ctrl-C` | Quit |
| `?`, `Esc` | Show help / dismiss help |
| `m` | Open the file-selection menu |
| `o`, `O` | Browse folders for a file to open |
| `]`, `Tab` / `[`, `Shift-Tab` | Next / previous file |
| `n`, `Space`, `PageDown` / `p`, `b`, `PageUp` | Next / previous PDF page or sheet; next / previous file for other formats |
| `Home` / `End`, `G` | First / last PDF page |
| Arrows, `h j k l` | Scroll a spreadsheet by row and column |
| `+`, `-` | Zoom in / out |
| Arrows, `h j k l` | Pan zoomed images; orbit meshes |
| `f`, `0` | Fit image / reset camera |
| `g` | Toggle Kitty / glyph output when Kitty is supported |
| `R` | Reload file from disk |
| `e` | Export the current page as a PNG in the working directory |
| `i` | Show or hide a box of details about the current file |
| `r` | Toggle mesh auto-rotation; reload other formats |
| `5` | Toggle mesh orthographic / perspective projection |
| Drag / Shift-drag / wheel | Mesh (STL, 3MF) orbit / pan / zoom |
| Drop files, or paste their paths | Add files to the list; one opens at once, several open the menu |

At a PDF boundary, page navigation stays on that page. Use `[` and `]` to change
files. Loading and rendering run asynchronously; errors appear in the viewer
with retry and next-file controls.

The file menu preserves argument order and marks the active file with `*`.
Use arrows or `j`/`k` to select, `Enter` to open, and `Esc` to cancel without
changing the current page, zoom, or camera. `PageUp`/`PageDown` scroll through
long lists; `Home`/`End` jump to the ends. Full paths distinguish duplicate names.
Press `v` to toggle an independent preview pane. PDF previews show page 1 at
reduced DPI; mesh previews are drawn as the viewer draws them, and 3MF previews show the
picture the file carries, when it has one: a slicer's rendering of the plate
if there is one, otherwise the declared thumbnail. The pane appears in terminals
at least 64 columns wide and 9 rows high. `--menu` starts in the selector;
`--preview` starts there with previews enabled.

Drag files from a file manager onto the terminal to add them to the list.
Terminals deliver a drop as a bracketed paste of paths or `file://` URIs; text
that does not read as paths is ignored. Dropped files follow the same format and
size rules as arguments, and any that are skipped are reported on exit.
`gloss` alone opens the viewer with no file, as a drop target. In the
browser demo, dropped files stay in the tab's memory until it reloads.

Press `o` to browse for a file instead, starting in the folder of the one you
are viewing. Type to filter the list, or type a path such as `~/Pictures/` to
go straight there; `Tab` completes, `Enter` opens a file or enters a folder,
and `Esc` cancels. The chosen file joins the list like a dropped one.
`gloss folder` starts in the browser at that folder. Beside files, folders are
skipped, so globs stay safe.

Files gloss cannot show are greyed and cannot be chosen; `Ctrl-T` hides them,
and again shows them. They are judged by extension, so a file with none stays
available: gloss may still recognize its content. With `--type`, nothing is
greyed. The browser is [picky](https://github.com/pgavlin/picky); the embedded
demo, which has no folders, does not offer it.

Press `e` to export what you are viewing: the current image, SVG, or PDF page,
or a mesh from the camera's position. The PNG is rendered as `--output` would
render it, honors `--max-edge`, and is named after the file
(`report-page-3.png`). An existing file is never replaced; the name gains a
number instead. The browser demo offers the PNG as a download.

Press `i` for what a file says about itself. Every format shows its path, size,
and modification time, followed by:

| Format | Details |
| --- | --- |
| Images | Format, pixel dimensions, color model; from Exif in JPEG, TIFF, and HEIC: camera, lens, date taken, exposure, orientation, location, software, artist, copyright |
| SVG | Declared size, view box, title, description, element count |
| PDF | Version, page count, size of the current page, title, author, subject, keywords, creator, producer, dates |
| STL | Encoding, name, triangle count, extent, surface area (STL records no unit) |
| 3MF | Title, designer, description, application, dates, license, object and triangle counts, extent and surface area in the file's unit, thumbnail size |
| Markdown | Title, lines, words, headings, links, images |
| Word | Title, author, dates, application, pages and words as Word counts them, headings, tables, images, links |
| Excel | Sheets with their size, title, author, dates, application |

Only fields present in the file are listed. The box sits in the top-right
corner over the document, which stays in use beneath it: the details follow as
you turn pages or change files. `i` or `Esc` closes it.

## Asking for a file

A program that cannot show a terminal, such as an agent working for you, can
still ask you for a file, or show you one:

```sh
gloss --serve --pick             # prints the paths you send, one to a line
gloss --serve report.pdf         # shows you the file; prints nothing
gloss --pick                     # the same question, asked in the terminal
```

`--serve` starts a temporary server on this machine, opens the viewer on a page
in your browser, and ends when you quit the viewer or close the tab. The page is
the native viewer, not the demo: `o` browses your own folders, and a pasted path
is read from your disk. Files dropped on the page are handed to gloss.

`--pick` waits for you to hand files over by dropping them, pasting their
paths, or choosing them with `o`. The viewer shows what you gave and says what
`Enter` will send; `Enter` sends it and quits, and `q` sends nothing. With files
named on the command line and none handed over, `Enter` sends the one on screen.
Standard output carries only the answer, as full paths. In a terminal the viewer
draws on the terminal itself, so the answer can be piped.

| Exit status | Meaning |
| --- | --- |
| 0 | Paths were printed |
| 1 | An error |
| 2 | Nothing was chosen |
| 124 | `--timeout` ran out |

Files dropped on a page are written to a folder of their own under the system's
temporary directory, readable by you alone. If they are the answer to a pick
they are left there for the program that asked, which should delete them when
it is done. Otherwise they are removed when gloss exits.

The server listens on 127.0.0.1 only, on a port chosen at random. The page's
address carries a token, without which nothing is served, so other programs and
other pages cannot reach the viewer or drop files on it. `--no-open` prints the
address without opening a browser; `--timeout 10m` gives up after that long.
With `--serve` or `--pick`, standard input is read only when `-` is named.

## Markdown

Open `.md`, `.markdown`, or `.mdown` files, or pipe text with `--type markdown`.
Glamour renders headings, lists, tables, and syntax-highlighted code. Local
Markdown image references (including reference-style links) use the existing
raster and SVG renderers, shown as block figures following their text line.
Paths resolve relative to the Markdown file, or the working directory for stdin.
Missing and remote images show placeholders; HTML image tags are not rendered.

```sh
gloss examples/readme.md
gloss --preview examples/readme.md examples/shapes.svg
cat README.md | gloss --type markdown -
```

Use `j`/`k`, arrows, or the mouse wheel to scroll; `Space`/`b` page down/up;
`Ctrl-D`/`Ctrl-U` move half a page; `Home`/`End` jump to the ends. Press `s` to
switch between rendered Markdown and source. File navigation remains `[`/`]`.
Documents are limited to 2 MiB of UTF-8 and 32 image references, with a combined
16-megapixel decoded image budget after resizing. Embedded images fit within
1600 pixels. Markdown PNG export is not supported: send Markdown text directly
to a model and export individual images when needed.

## Images for vision models

Export PNGs without opening a terminal UI. The default maximum edge is **1536
pixels**; choose the size appropriate to your model and the detail you need.
This is a configurable size budget, not a promise of identical model token costs.

```sh
gloss --output page.png --max-edge 1536 --page 3 report.pdf
gloss --output diagram.png --max-edge 1024 drawing.svg
gloss --output mesh.png --max-edge 1536 model.stl
gloss --output page.png --vision-profile openai-high report.pdf
gloss --output diagram.png --vision-profile claude-standard drawing.svg
gloss --output-dir model-inputs --max-edge 768 photo.png drawing.svg report.pdf
cat drawing.svg | gloss --output - --max-edge 1024 > diagram.png
```

Exports preserve aspect ratio, fit within the requested edge (1–4096), flatten
transparency onto white, and contain no terminal chrome. Smaller raster sources
are not enlarged. SVG and PDF are rasterized for the requested size (PDF remains
subject to the 600-DPI and pixel-budget caps). Mesh exports are square and use
the default NTCharts3d camera, or the viewer's when saved with `e`. They are
drawn on the GPU, and in software where there is none or where `--3d` names
another renderer. Software draws every face, without the viewer's triangle
sampling, and the same picture. A single view does not reveal hidden surfaces.

### Views of a mesh

```sh
gloss --output front.png --view front model.stl
gloss --output sheet.png --view all --vision-profile claude-high model.3mf
gloss --output sheet.png --view iso,front,top,right model.stl
gloss --output posed.png --camera 20,-120 --projection perspective model.stl
gloss --view top model.stl       # the viewer starts there; f returns to it
```

`--view` names where a mesh is seen from: `front`, `back`, `left`, `right`,
`top`, `bottom`, or `iso`, which is from the front, the right, and above. X runs
to the right, Y away from the viewer at the front, and Z up. `--camera` places
the camera by its elevation and azimuth in degrees, the azimuth counted from the
X axis, and optionally its distance. `--projection` is `ortho` or `perspective`.

Several views, as `front,top` or `all` for the six sides, are exported as one
sheet of square tiles, each named in its corner. The sheet as a whole keeps to
`--max-edge` and the vision profile. On the GPU the mesh is uploaded once for
all of them.

A view fits the mesh to nine tenths of the picture, unless `--camera` gives a
distance. A mesh that is long toward the camera, a plank seen from its end, is
drawn smaller: in NTCharts3d's orthographic projection distance is also scale,
and the camera must stand clear of the mesh. Views asked for are lit from over
the viewer's shoulder, so that the back and the underside show as much as the
front. With no view asked for, the camera and the light are the viewer's.

### What a file says about itself

```sh
gloss --info model.3mf report.pdf
gloss --info --json -p 3 report.pdf
```

`--info` prints what the viewer shows with `i`, and does not open the viewer.
`--json` prints an array with an object for each file: its `path` and `kind`,
`page` and `pages` for a PDF, and `details` by section and label, as
`details.Model.Triangles`. Values are as the viewer words them. A file that
cannot be described has an `error` in place of what is missing, the others are
described all the same, and the exit status is 1.

Model profiles also fit the rounded patch budget, which a maximum edge alone
cannot enforce. These profiles implement sizing envelopes, not a measured
accuracy optimum or exact billing calculation. Verified against official
[OpenAI](https://developers.openai.com/api/docs/guides/images-vision) and
[Claude](https://platform.claude.com/docs/en/build-with-claude/vision) documentation
on 2026-09-29:

| Profile | Maximum edge | Patch size / budget | Largest square |
| --- | --- | --- | --- |
| `openai-high` | 2048 | 32×32 / 2500 | 1600×1600 |
| `claude-standard` | 1568 | 28×28 / 1568 | 1092×1092 |
| `claude-high` | 2576 | 28×28 / 4784 | 1932×1932 |

The OpenAI profile is a conservative common envelope for GPT-6 Astra and GPT-5.6
with API `detail: high`; it does not set that API parameter. Claude high applies
to models supporting the high-resolution tier (currently 4.7 and later).
An explicit `--max-edge` can further reduce a profile's output. Smaller inputs
remain smaller. Model/API rules can change; inspect the dimensions printed on
stderr and consult the target model's documentation.

PNG is useful for text, diagrams, and thin lines because it is lossless. For
dense documents, retain an overview and supply detail crops where needed. For
3D interpretation, several views reveal more than one larger image: see
[views of a mesh](#views-of-a-mesh). Automatic detail crops and JPEG output are
not implemented yet.

`--output` accepts one input; `--output-dir` exports all supplied inputs in order,
with numbered filenames. Each PDF exports the selected `--page` (page 1 by
default, clamped to the document's page range). Existing output files are never
overwritten. `--output -` sends PNG bytes to redirected stdout, with diagnostics
on stderr. Export works without a TTY and cannot be combined with menu flags.

## Formats and current limits

- PNG, JPEG, GIF, WebP, BMP, TIFF: first frame/page, up to 32 megapixels. Animated
  playback and AVIF aren't supported.
- HEIC/HEIF: primary still image, up to 32 megapixels, with container rotation
  and mirroring. Uses the [pure-Go h265 decoder](https://github.com/gen2brain/h265),
  without CGO or external converters. Unsupported HEVC features report decoder
  errors. HEIC works in Markdown, previews, PNG exports, and the browser demo.
- SVG: NTCharts' pure-Go SVG renderer, rasterized to a 2400-pixel maximum edge.
  SVG support follows the underlying oksvg renderer, not a full browser engine.
- PDF: PDFium via embedded WebAssembly; no Poppler, MuPDF, CGO, or external
  runtime installation. Pages render at 150 DPI by default (`--dpi 36..600`),
  with a 32-megapixel raster budget and a 10,000-page limit. Password-protected
  PDFs aren't supported. This version provides visual paging, without text search.
- STL: ASCII and binary, flat-shaded triangles, up to 932,067 faces: as many as
  NTCharts3d draws, which is as many as any GPU is sure to hold. Normals are recomputed from vertex winding. GPU
  rendering falls back to software and then wireframe. Software draws at full
  size for Kitty output, smaller only while its frames are slow, and samples
  meshes above 20,000 triangles; wireframe samples above 2,000 triangles, so
  large models can lose detail in fallback modes.
- 3MF: the core specification's meshes, components, and build transforms;
  colors from base materials and color groups; objects kept in separate parts
  of the package, as slicers write them. Projects from Bambu Studio and its
  relatives are drawn in their filament colors, which those programs keep in
  settings of their own; colors painted onto faces are not read. Textures, beam lattices, slices, and
  encrypted content are not read. The same 932,067-face limit applies: a larger
  model is shown by its embedded thumbnail, and described by `i`. A package may
  hold 4,096 entries and unpack to 128 MiB.
- Word (`.docx`, `.docm`): turned into Markdown and shown as such, so `s` shows
  the Markdown. Headings, lists, tables, links, pictures, and bold, italic, and
  struck text are kept; page layout, headers and footers, footnotes, comments,
  and text boxes are not. A document longer than 2 MiB of Markdown is cut.
- Excel (`.xlsx`, `.xlsm`): each sheet is a grid that scrolls by row and column,
  with `n` and `p` turning between sheets. Cells show their values, formulas
  by their last result, and dates and times where a cell's style says so. Up to
  100,000 rows and 1,024 columns of a sheet are read; the rest are counted.
  Formatting, merged cells, charts, and pictures are not shown.
- Input files and stdin are limited to 128 MiB. One active document and, when
  enabled, one independent preview are kept open.
  Images, SVGs, and PDF pages zoom by cropping the existing raster, up to 64×;
  use a higher PDF DPI for more detail. There is no URL fetching or file watching.

## Development

```sh
task test
task ci                         # formatting, modules, race tests, vet, build
go build -ldflags '-X main.version=0.1.0' -o bin/gloss ./cmd/gloss
```

Tests cover CLI validation, malformed files, STL geometry, PDF rendering and
navigation, SVG rasterization, viewport cropping, terminal-safe labels, and
stale asynchronous results. The example SVG and STL are small original fixtures.

The layout follows NTCharts' conventions, with one module for the CLI and a separate browser-demo module:

- `cmd/gloss`: CLI flags, stdin handling, export orchestration, and the
  temporary server behind `--serve`.
- `web`: the demo site, and the page `--serve` shows.
- `internal/app`: terminal pager, selection menu, and Markdown layout.
- `internal/document`: bounded loaders, renderers, and vision image sizing.
- `examples`: small runnable fixtures.
- `scripts`: release packaging.

`task --list` lists development commands. GitHub Actions runs `task ci` on Linux
and macOS for pushes and pull requests. Pushing a `v*` tag runs checks, packages
macOS/Linux amd64 and arm64 binaries, and publishes archives and SHA-256 checksums
to a GitHub Release. Locally, run `task release VERSION=v0.1.0` to produce the
same archives in `dist/` without publishing. Release binaries use software STL
rendering when native GPU support is unavailable.

### Building and embedding the demo

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
which downloads pinned PDFium 2.14.2 assets from jsDelivr; other runtime assets
are served alongside the site. Meshes are drawn with WebGPU where the browser
has it, and by the software renderer where it does not.

Embed the standalone terminal on another site:

```html
<iframe src="https://nimblemarkets.github.io/gloss/demo.html?sample=field-guide.pdf"
        title="gloss live terminal" width="100%" height="560"
        style="border:0" loading="lazy"></iframe>
```

Omit `sample` to start in the file menu; accepted filenames are listed in
`examples/assets.go`. The native keys work in the demo; quitting offers a
restart button. Choosing another format restarts the embedded terminal at that sample; clicking
the active format preserves the session. Restart explicitly reloads it. The
loading screen reports received bytes and compilation/startup stages.

The Pages workflow builds for pull requests and deploys pushes to `main`.
Set repository **Settings → Pages → Source → GitHub Actions** to enable hosting.
The fixtures are original; `task gen-assets` regenerates PNG, HEIC, PDF, and the block-letter STL.
