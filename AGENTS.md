# AGENTS.md

## What gloss is

A visual pager for the terminal: like `less`, for images, SVGs, PDFs, STL and 3MF
meshes, Markdown, HTML, JSON, Jupyter notebooks, Word, Excel, Grist, and CSV
files.
Written in Go (module `github.com/NimbleMarkets/gloss`, Go 1.26.8+) on Bubble
Tea, NTCharts, NTCharts SVG, NTCharts PDF, and NTCharts3d. The README is the
short introduction; the user reference is the guide on the documentation site
(`docs/hugo/content/guide`); DEVELOP.md is for working on gloss. Keep them and
this file consistent with the code.

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
- `docs/hugo`: the documentation site (Hugo, hugo-book theme as a submodule),
  published at `/docs/` beside the demo. The guide pages (`content/guide`) are
  written by hand, except *For LLMs* and *Development*, which are generated
  from the skill and DEVELOP.md; `content/command` is generated from the
  options. Generated pages are ignored.
- `scripts`: site building (`build-site.sh`) and fixture
  generation (`gen-assets`, `gen-grist.py`).

## Capabilities

Supported inputs (with per-format limits detailed in the guide's "Formats and
limits" page, `docs/hugo/content/guide/formats.md`):

- **Images**: PNG, JPEG, GIF, WebP, BMP, TIFF (first frame, up to 32 MP) and
  HEIC/HEIF via a pure-Go HEVC decoder — no CGO or external converters.
- **SVG**: rasterized by NTCharts' pure-Go renderer (oksvg feature set), up to
  a 2400-pixel edge.
- **PDF**: PDFium over embedded WebAssembly; pages at 150 DPI default
  (`--dpi 36..600`), 32 MP budget, 10,000-page limit, no password-protected
  files, no text search in the viewer. `--text` extracts a page's text layer
  without rasterizing (16 MiB per page); a page with none is an error.
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
- `--vision-profile openai-high|claude-standard|claude-high`: only a
  convenience alias for a `--max-edge` (it goes stale; harnesses pass
  `--max-edge`, and no vendor names are added). The manifest carries
  `max_edge`, plus `vision_profile` and `vision_reason` when one was named.
- `--view front|back|left|right|top|bottom|iso[,...]|all`, `--camera`,
  `--projection`, `--parts`, `--partn`, `--color`: posed mesh exports,
  including multi-view contact sheets.
- `--info [--json]`: print per-file metadata (what the `i` box shows) without
  opening the viewer; JSON mode emits an array of objects with `error` entries
  for files that fail.
- `--serve [--pick] [--prompt] [--timeout] [--no-open]`: temporary
  localhost-only token-guarded web viewer; `--pick` prints chosen paths on
  stdout after `Enter` confirms them. The browser shows `--prompt` as a
  persistent accessible heading with a native **Choose files** button;
  `--prompt-loc` positions the prompt box in the terminal only. Pick returns
  exit 0 paths, 1 error, 2 nothing chosen, 124 timeout and blocks, in
  a terminal. With stdin not a terminal, both `--pick` and `--serve` instead
  detach the server (`cmd/gloss/detach.go`), print one JSON object (`url`,
  `dir`, `timeout_seconds`, `resume_token`, `resume`), exit 0, and open no
  browser; `--timeout` defaults to 10 minutes. `--resume TOKEN` reads the
  state file and answers as a pick does (0, 2, 124). `--status TOKEN` only
  looks: it prints one JSON object (`state` waiting, picked, declined, timeout,
  closed, or failed; `settled`; `paths`; `error`; `seconds_left`) and exits 0
  whenever it reported, 1 for no such pick. It must change nothing, not the
  state file nor the folder: `observe` in `detach.go` works out an overdue or
  dead waiting state for both, and only `--resume` writes the result. Dropped
  files stay in the
  private `dir` for the caller to delete, and are removed on timeout/decline.
  This is how a non-terminal agent asks a human for a file or shows one.
- `--type` overrides content detection; `-` reads stdin once into a temp file.

## Build, test, verify

Uses [Task](https://taskfile.dev/); without it, `go build -o gloss ./cmd/gloss`.

- `task build` / `task run -- <args>` / `task install`
- `task test` — `go test ./...`; `task test-race`, `task vet`
- `task ci` — the full gate: `fmt-check`, `go-tidy-check`, `go-verify`,
  `test-race`, `vet`, `staticcheck`, `vulncheck`, `build`. Run this before
  considering work done.
- `task docs` — the `gloss(1)` man page (`docs/man`), the command reference
  (`gloss --docs-man`/`--docs-markdown --docs-hugo`, hidden options) and the
  two guide pages (`internal/tools/docsite`, from DEVELOP.md and
  skills/gloss/SKILL.md). `docs:hugo:serve` / `docs:hugo:build` need `hugo`
  (extended); `docs-check` runs in `task ci` and needs no Hugo.
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
- gloss has one default command, `view` (`gloss FILE` runs it), and a few small
  ones (`skill`, `help`) in `cmd/gloss/verbs.go`. A command is recognized only as
  the first argument, by its exact name; anything else is an implicit `view`, so
  a file named like a command is opened as `gloss view skill`. Do not add a
  command for a file's job: those are options of `view`. Every visible option
  belongs to exactly one *domain* in `cmd/gloss/domains.go` (the clusters the
  man page and reference are grouped by; a command's domain has none); a new
  option without one fails the tests. Each domain's examples may only use real
  options. The old `--skill` and `--install` still work, hidden, for scripts.
- Keep the README short and direct; anything for developers goes in DEVELOP.md.
  User-facing detail (keys, formats and limits, export, handing files over)
  belongs in the guide, `docs/hugo/content/guide`: a new page needs a title and
  weight and a link from the guide's `_index.md` (a test checks). Do not
  hand-edit the generated pages: `content/command`, `guide/development.md`,
  `guide/for-llms.md`.
- Fixtures in `examples/` are original; do not add third-party sample files
  without checking licensing (see `THIRD_PARTY_NOTICES.md`).
- `skills/gloss/SKILL.md` teaches external agents gloss's headless surface
  (export, `--info`, `--pick`, `--serve`, the app link). If you change
  agent-facing flags, limits, or exit codes, update it in the same change.
