# `gloss` CHANGELOG

## Unreleased

  * **The file browser**
    * A rebuilt chooser (`o`) with three layouts: list, file-manager columns,
      and columns with a sidebar of common and recently visited folders.
      `Ctrl-L` switches layouts; `Ctrl-G` moves to the sidebar
    * Ranked fuzzy filtering with highlighted matches, smart case, globs,
      and access to hidden files by typing a leading dot
    * `Tab` completes a common prefix, then cycles candidates; `Shift-Tab`
      cycles backward and `→` accepts the dimmed suggestion. `Tab` also expands
      abbreviated folder paths; `G` completes folders only
    * Clickable breadcrumbs, back and forward history, and mouse navigation
      through rows, columns, places, and the file-type menu (`Ctrl-F`)
    * Cursor positions survive folder reads and navigation. Completion works
      after accented and other multibyte characters. Typing and search commands
      close popups; hidden popups no longer capture keys, and the wheel
      scrolls the menu or sidebar under it
    * File detection is deferred until rows are drawn or files are chosen,
      avoiding reads of every unknown file just to list a folder. Pipes,
      devices, and links to them are never opened for detection
    * A new guide explains the layouts, filtering, completion, and keys

  * **URL drops and required formats**
    * `--fetch` also enables dropping or pasting one http(s) URL in the
      terminal, and dropping one on a `--serve` page
    * `--accept 'image/*'` requires images, including SVG; comma-separated
      formats such as `--accept 'image/*,pdf'` allow alternatives. The filter
      applies to initial files, choices, drops, and downloads independently
      of `--type`. Mismatches report the received and required formats;
      rejected downloads are removed
    * The landing-page demo and browser app accept URL drops. Downloads go
      directly from the source to the browser, subject to CORS, without a
      site proxy, credentials, or referrer. Browser downloads have a one-minute
      deadline and enforce the 128 MiB limit while streaming
    * `?accept=image%2F*` restricts a browser app session to images. Documents
      opened from URLs are not saved in the app's persistent file library

  * **The agent contract**
    * `gloss --cancel TOKEN` ends a detached session: it stops the server,
      deletes the session folder with every dropped file, and makes later
      `--status` and `--resume` exit 1
    * `--resume` no longer removes a settled session: `--status` still reports
      it, paths and all, and `--resume` answers again, until `--cancel` or a
      day after the session's timeout
    * The startup, `--status`, and `--resume --json` objects carry
      `"protocol": 1`
    * `--text --json` on a PDF page adds `chars` and `images`, and
      `"sparse": true` for a page with images and under 100 characters of
      text, which is better exported as a picture
    * `--grep PATTERN` searches the text layer of a PDF and prints only the
      matching pages, with an excerpt; `--json` gives one object per match.
      Matches stop at 200

  * **Commands**
    * `gloss skill` prints the agent skill and `gloss skill install [FOLDER]`
      installs it; they replace `--skill` and `--install`, which still work
      but are no longer listed
    * `gloss view` is the default command, spelled out: `gloss view FILE` is
      `gloss FILE`, and is how to open a file named `skill`, `view` or `help`
    * `gloss help [command]` shows the usage of gloss or of one command; the
      shell completions offer the commands

  * **Fixes**
    * Exports and `--text` refuse a page, sheet, table, or 3MF part the file
      does not have, naming what it has, instead of giving the last one; a
      range past the end makes the pages there are and reports the rest once.
      Each failure is reported once on stderr, not twice
    * A 3MF too large to draw, or to unpack within 128 MiB, exports its
      embedded thumbnail with a `note` in the manifest and on stderr saying so;
      `--info` describes a package too large to unpack from its root model
    * Mesh exports of named views outline edges, so holes and openings no
      longer vanish in flat-shaded front, back, and top views
    * `--text` into a file or folder writes a notebook's or Word document's
      pictures beside the text and links them; on stdout a `note` says they
      were left out. The browser app no longer shows dropped files under
      `dropped/`
    * Grist summary tables show what they group by as Grist does (the site's
      name, not `Sites[1]`) and are titled `Table [by Column]`
    * `e` in a `--serve` page downloads the PNG in the browser instead of
      writing it into the directory gloss was started in
    * The browser app no longer keeps showing the previous picture after
      moving to another file of the same size (a texture cache in ghostty-web
      is worked around with a new Kitty image id per picture)

  * **Development and packaging**
    * The file chooser is a separate Bubble Tea component. Its test harness
      provides an in-memory filesystem, scripted golden screens, layout and
      terminal-safety checks, and session recording and replay; `task browse:*`
      exposes the tests, playground, benchmarks, and VHS recordings
    * `task build` skips rebuilding when its sources, embedded assets,
      dependencies, and requested version have not changed. A test checks
      that the build's source list covers everything the binary uses
    * License notice generation finds Go's license in Homebrew installations
      and reports an error if it cannot read it, instead of silently omitting
      the runtime license

## `v0.2.0` (2026-10-02)

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
    * `gloss --status TOKEN` reports a detached session's state immediately
      as JSON, without changing the session, state file, or dropped files.
      It exits 0 when a state is reported and 1 when no such pick exists
    * In the browser, `--prompt` is a persistent heading with a native
      **Choose files** button. Choosing or dropping files opens them for
      review; `Enter` in the viewer confirms the answer

  * `gloss --skill` prints the skill that teaches agents to use gloss, so an
    installed binary can write out `SKILL.md` for the version it is;
    `gloss --skill --install` puts it where the agents on the machine look for
    skills, and `npx skills add NimbleMarkets/gloss` does the same with Node

  * **The viewer**
    * `B` opens a background color picker for meshes; the choice survives
      reloads and appears in previews. PNG exports still flatten onto white
    * The color picker supports mouse clicks and slider dragging. `Enter`
      chooses the swatch you moved to, and `Esc` undoes changes
    * `C` can recolor a whole model, including a 3MF with its own colors;
      `r` restores the file's colors, and `s` limits painting to plain faces
    * `r` starts a mesh turning immediately

  * **Shells and documentation**
    * Bash, zsh, and fish completions ship in the release archives, Debian
      packages, and Homebrew cask, alongside the generated man page
    * A documentation site provides a user guide and generated command
      reference; `--help` links to it. Developer instructions live in
      DEVELOP.md, and the README is a short introduction

  * **Bounds and reliability**
    * Tighter allocation and expansion limits for JSON, CSV, Excel, Word,
      HTML, Markdown, PDF, SVG, 3MF, and ZIP inputs; parsing and rendering
      deadlines keep malformed files from blocking the caller indefinitely
    * Page ranges are capped at 10,000 entries, part and column index lists
      at 65,536 entries, and GIF frames with no area are refused
    * Browser uploads have a session budget; Windows device and stream names
      are refused, and detached servers that fail to start are cleaned up
    * Opt-in fetching refuses redirects from HTTPS to HTTP
    * The browser site serves pinned, hash-checked PDFium assets itself,
      applies a Content-Security-Policy, and does not save documents opened
      through `?src=` in the file library
    * More loader fuzz tests, weekly fuzzing, and Windows compilation checks

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
