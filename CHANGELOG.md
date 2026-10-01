# `gloss` CHANGELOG

## Unreleased

  * Agent protocol
    * `--text` takes the text layer of a PDF: one page with `--page`, a range,
      or `all` into `--output-dir` or `--json`. A page with no text layer is an
      `error` entry, not a blank success. Pictures, SVG, and meshes still point
      at `--output`
    * `--vision-profile` is now only a documented alias for `--max-edge`
      (1600, 1092, 1932 px: the square-safe edge for each budget, wide pictures
      are no longer sized up to the patch budget); `--max-edge` given wins. The
      export manifest gains `max_edge`, and `vision_profile` and
      `vision_reason` when a profile was named
    * With no terminal (stdin not a TTY), `--pick` and `--serve` no longer
      block: they print one JSON object (`url`, `dir`, `timeout_seconds`,
      `resume_token`, `resume`) and exit 0, with the server detached and no
      browser opened; `--timeout` defaults to 10 minutes. The new
      `gloss --resume TOKEN` prints the paths and exits 0, 2, or 124

  * `gloss --skill` prints the skill that teaches agents to use gloss, so an
    installed binary can write out `SKILL.md` for the version it is;
    `gloss --skill --install` puts it where the agents on the machine look for
    skills, and `npx skills add NimbleMarkets/gloss` does the same with Node

## `v0.1.0` (2026-10-01)

The first release, to be tagged `v0.1.0`.  Happy Hacktober!

`gloss` is a visual pager for the terminal: like `less`, for images, SVGs, PDFs,
meshes, Markdown, and documents of many kinds.

  * **Formats**
    * Images: PNG, JPEG, GIF, WebP, BMP, TIFF, and HEIC (pure Go, no CGO)
    * SVG, rasterized by NTCharts; PDF, rendered by PDFium over WebAssembly
    * STL and 3MF meshes, up to 932,067 faces: flat-shaded, drawn on the GPU
      where there is one and in software where not; 3MF colors, parts, and
      slicer filament colors; `c` lists the parts, `C` picks a color
    * Markdown, with local pictures inline; HTML and Word, converted to Markdown
    * JSON, JSONL, and NDJSON, pretty-printed and highlighted; Jupyter notebooks,
      with their picture outputs
    * Plain text, and any file that is not binary, as `less` would show it;
      source is highlighted
    * Excel, CSV, TSV, and Grist documents, as scrollable sheets (`n`/`p` turn
      sheets or tables); columns can be hidden (`x`, `X`, `c`, `--cols`, `--coln`)
    * Every loader is bounded: 128 MiB of input, and limits on pixels, rows,
      and faces
  * **The viewer**
    * Kitty graphics where the terminal has them, colored half-block glyphs
      elsewhere; alternate screen by default, `-X` to stay on the main screen,
      `Q` to quit leaving the view in the scrollback
    * Zoom and pan, mesh orbit, and an ortho/perspective toggle
    * `i` describes the file and the terminal; `e` exports the current view as PNG
    * A file menu, a preview pane, thumbnails, and a file browser (`o`) that
      marks kinds, sorts, searches by glob, extension, or kind, and goes to a
      typed folder (`G`)
    * Drag and drop, and bracketed paste, of files and folders
    * `--fetch` lets `Enter` on a table cell holding an http(s) address download
      and open it; gloss never fetches on its own
    * Table cells with a web address are marked 🔗 and are links to the terminal
  * **For scripts and agents**
    * `--output`, `--output-dir`, and `--output -` export PNGs with no terminal;
      `--max-edge` and `--vision-profile` size them for vision models
    * `--view`, `--camera`, `--projection`, `--parts`, and `--color` pose meshes,
      including multi-view contact sheets
    * `--info [--json]` prints what the info box shows; `--text` and `--page`
      take the text out of documents and sheets, with page ranges
    * `--serve [--pick] [--prompt]` shows the viewer, or asks a human for a file,
      on a temporary localhost page
    * `--type` forces a format; `-` reads stdin
    * A skill for agents in `skills/gloss/SKILL.md`
  * **On the web**
    * A live demo, with a sample of every format, running the real pager as
      WebAssembly at <https://nimblemarkets.github.io/gloss/>
    * An app page for your own files, kept in the browser's storage and never
      uploaded; `?src=` opens a document from an address that allows it
  * **Install**
    * `brew install --cask nimblemarkets/tap/gloss`
    * `.deb` packages for amd64 and arm64
    * Archives for macOS, Linux, and Windows
    * `go install github.com/NimbleMarkets/gloss/cmd/gloss@latest`
  * MIT licensed; the licenses of linked modules are in `THIRD_PARTY_NOTICES.md`
