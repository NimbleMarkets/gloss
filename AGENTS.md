# AGENTS.md

## What gloss is

A visual pager for the terminal: like `less`, for images, SVGs, PDFs, STL and 3MF
meshes, Markdown, HTML, JSON, Jupyter notebooks, Word, Excel, Grist, and CSV
files.
Written in Go (module `github.com/NimbleMarkets/gloss`, Go 1.26.8+) on Bubble
Tea, NTCharts, NTCharts SVG, NTCharts PDF, and NTCharts3d. The README is the
user-facing reference; keep it and this file consistent with the code.

## Repository layout

- `cmd/gloss`: CLI flags (pflag GNU syntax), stdin handling, PNG export
  orchestration, and the temporary server behind `--serve`.
- `internal/app`: terminal pager, file-selection menu, preview pane, file
  browser, and Markdown layout.
- `internal/document`: bounded loaders, renderers, and vision-image sizing.
- `web`: the browser demo site and the page `--serve` shows; has its own
  Node-tested helpers (`node --test web/*.test.mjs`).
- `examples`: small original runnable fixtures; `examples/demo` is a separate
  Go module pinning the Bubble Tea WASM fork used by the NTCharts demos.
- `scripts`: site building (`build-site.sh`) and fixture
  generation (`gen-assets`, `gen-grist.py`).

## Capabilities

Supported inputs (with per-format limits detailed in the README "Formats and
current limits" section):

- **Images**: PNG, JPEG, GIF, WebP, BMP, TIFF (first frame, up to 32 MP) and
  HEIC/HEIF via a pure-Go HEVC decoder — no CGO or external converters.
- **SVG**: rasterized by NTCharts' pure-Go renderer (oksvg feature set), up to
  a 2400-pixel edge.
- **PDF**: PDFium over embedded WebAssembly; pages at 150 DPI default
  (`--dpi 36..600`), 32 MP budget, 10,000-page limit, no password-protected
  files, no text search.
- **STL**: ASCII and binary, up to 932,067 faces, flat-shaded; GPU rendering
  falls back to software and then wireframe (`--3d software|wireframe`).
- **3MF**: core-spec meshes, components, build transforms, base-material and
  color-group colors, Bambu-Studio-style filament colors from slicer settings;
  part listing/selection (`c` key, `--parts`, `--partn`); too-large projects
  fall back to the embedded thumbnail.
- **Markdown**: Glamour-rendered with local raster/SVG images inline; `s`
  toggles source.
- **JSON / JSONL / NDJSON**: pretty-printed and highlighted, numbered records
  for line-delimited files, syntax errors reported with line number.
- **Jupyter notebooks** (nbformat 4): Markdown and code cells with outputs,
  including PNG/JPEG/GIF/SVG picture outputs.
- **HTML**: converted to Markdown via html-to-markdown; nothing is fetched.
- **Word** (.docx/.docm): converted to Markdown.
- **Excel** (.xlsx/.xlsm): sheets as scrollable grids (`n`/`p` switch sheets);
  values, formula results, styled dates.
- **Grist** (.grist): a SQLite database read in pure Go (no CGO, and in the
  browser demo) through `github.com/neomantra/sqlittle`; user tables as
  sheets (`n`/`p` switch tables) with labels as headers, in Grist's row and
  column order. Stored values only: no formula evaluation, access rules, or
  writing. Other SQLite databases are refused.
- **CSV/TSV**: separator auto-detected; shown like a sheet. Table columns can
  be hidden interactively or via `--cols`/`--coln`.
- **Fetching** (opt-in `--fetch`): `Enter` on a table cell holding an http(s)
  address downloads and opens it; gloss never fetches on its own. Such cells
  are marked 🔗 and drawn as terminal hyperlinks (OSC 8), so the terminal,
  not gloss, opens them in a browser when clicked.

Key viewer features: Kitty graphics with colored half-block glyph fallback;
alternate screen by default, `-X` for main screen; file menu (`--menu`),
preview pane (`--preview` or `v`), file browser (`o`), drag-and-drop and
bracketed-paste path input, zoom/pan, per-format info box (`i`), PNG export of
the current view (`e`), mesh orbit/pan/zoom, ortho/perspective toggle, mesh
color picker (`C`, `--color`), reload (`R`/`r`).

Agent-facing / scriptable surface:

- `--output file.png` / `--output-dir` / `--output -`: headless PNG export for
  vision models, no TTY needed; `--max-edge 1..4096`, aspect preserved, white
  flattening, never overwrites.
- `--vision-profile openai-high|claude-standard|claude-high`: sizing envelopes
  matching model patch budgets (see README table).
- `--view front|back|left|right|top|bottom|iso[,...]|all`, `--camera`,
  `--projection`, `--parts`, `--partn`, `--color`: posed mesh exports,
  including multi-view contact sheets.
- `--info [--json]`: print per-file metadata (what the `i` box shows) without
  opening the viewer; JSON mode emits an array of objects with `error` entries
  for files that fail.
- `--serve [--pick] [--prompt] [--timeout] [--no-open]`: temporary
  localhost-only token-guarded web viewer for environments without a terminal;
  `--pick` prints chosen paths on stdout (exit 0 paths, 1 error, 2 nothing
  chosen, 124 timeout). This is how a non-terminal agent asks a human for a
  file or shows one.
- `--type` overrides content detection; `-` reads stdin once into a temp file.

## Build, test, verify

Uses [Task](https://taskfile.dev/); without it, `go build -o gloss ./cmd/gloss`.

- `task build` / `task run -- <args>` / `task install`
- `task test` — `go test ./...`; `task test-race`, `task vet`
- `task ci` — the full gate: `fmt-check`, `go-tidy-check`, `go-verify`,
  `test-race`, `vet`, `build`. Run this before considering work done.
- `task demo` / `task demo-check` — embedded gallery and its separate module.
- `task build-wasm-site` / `task serve-wasm-site` / `task web-check` — browser
  demo site (Node 18+ for `web-check`).
- `task release` — GoReleaser snapshot into `dist/`, publishing nothing:
  macOS/Linux/Windows amd64/arm64 archives, `.deb` packages, and the Homebrew
  cask (`.goreleaser.yaml`; `task release-check` validates it). Pushing a
  `v*` tag publishes them via GitHub Actions, the cask to
  `NimbleMarkets/homebrew-tap`.
- `task gen-assets` — regenerates the original PNG/HEIC/PDF/STL fixtures, and
  the Grist ones (needs `python3`).

Version is stamped via `-ldflags "-X main.version=$VERSION"`. Dependencies are
pinned in `go.mod`; builds use `-mod=readonly`.

## Conventions

- Go, formatted with `gofmt` (enforced by `task ci`); follow NTCharts' layout
  conventions and the existing code style.
- All loaders must stay bounded: respect the 128 MiB input cap, per-format
  pixel/row/face limits, and never fetch the network unless `--fetch` is given.
- Never overwrite an existing output file; number the name instead.
- Interactive output requires a terminal; export (`--output`) and `--info`
  paths must keep working without a TTY and write only the payload to stdout.
- Fixtures in `examples/` are original; do not add third-party sample files
  without checking licensing (see `THIRD_PARTY_NOTICES.md`).
- `skills/gloss/SKILL.md` teaches external agents gloss's headless surface
  (export, `--info`, `--pick`, `--serve`, the app link). If you change
  agent-facing flags, limits, or exit codes, update it in the same change.
