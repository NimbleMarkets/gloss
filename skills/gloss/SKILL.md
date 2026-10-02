---
name: gloss
description: "Inspect local files with gloss: extract document text and tables, export images, PDF pages, SVG, and meshes as PNGs for vision, read metadata, or ask a human to choose or view files. Use for file inspection and local file handoffs, not document creation or editing."
---

# gloss: seeing files, and being shown them

gloss is a visual pager (`less`, with pictures). For an agent it has four
distinct uses; choose the one that answers the request:

1. **Read a file's text** — `--text`: Word, HTML, notebooks, PDF text layers,
   and sheets as Markdown, text, or CSV, no rendering.
2. **See a file yourself** — headless PNG export, sized for your vision budget.
3. **Learn about a file** — metadata as JSON, no rendering.
4. **Involve the human** — they pick files for you, or view what you show them.

Prefer `--text` for content and `--output` for appearance, scans, figures, or
layout. Use `--info --json` when the kind, page count, or part names are unknown.
After exporting, open the returned PNG with your image-viewing tool; creating
it alone does not inspect its contents.

**stdout carries the payload; diagnostics go to stderr.**

| Mode | stdout |
| --- | --- |
| `--text` | Extracted text; with `--output-dir`, written paths |
| `--output file.png` / `--output-dir` | Actual written paths, one per line |
| `--output -` | PNG bytes only |
| `--info` | Human-readable metadata |
| `--json` with text, export, or info | JSON array of results, including errors |
| `--pick` with terminal stdin | Chosen full paths after confirmation |
| `--pick` / `--serve` without terminal stdin | One JSON object describing the detached session; exit 0 means started, not answered |
| `--resume TOKEN` | Chosen paths; with `--json`, an object with `status`, `paths`, and `error` |
| `--status TOKEN` | One JSON object, at once, saying how the detached session stands: `state`, `settled`, and `paths`, `error`, or `seconds_left` as they apply |

Exit status 1 means an error; in a batch, inspect and use the successful results.
Pick/resume also use 2 for decline and 124 for timeout, as described below.
`--status` exits 0 whenever it reports a state (the state is in the JSON, not
the exit code) and 1 when there is no such session.

`gloss skill` prints this file, so the installed binary can always say what
it itself does; `gloss skill install` writes it where the agents on the
machine look for skills. A file named `skill`, `view` or `help` is opened as
`gloss view skill`.

It handles PNG, JPEG, GIF, WebP, BMP, TIFF, HEIC, SVG, PDF, STL, 3MF,
Markdown, HTML, plain text, JSON/JSONL, Jupyter notebooks, Word, Excel, Grist
documents, and CSV. One binary, no external converters, no CGO. Inputs are limited to 128 MiB.

## 1. Read a file: `--text`

The cheapest way to know what a document says. gloss converts what it can
and hands the text over unchanged otherwise.

```sh
gloss --text report.docx                     # Markdown of the document, on stdout
gloss --text page.html                       # Markdown of the page, scripts dropped
gloss --text notebook.ipynb                  # cells and outputs as Markdown
gloss --text --page 2 sales.xlsx             # one sheet as CSV
gloss --text --page 3 report.pdf             # the text layer of one PDF page
gloss --text --page all --output-dir text report.pdf   # a .txt per page, paths printed
gloss --text --page all --output-dir sheets sales.xlsx   # every sheet, one CSV each
gloss --text --json notes.txt batch.jsonl    # [{"path","kind","text"}, …]
```

| Kind | What comes out |
| --- | --- |
| Word, HTML, notebook | Markdown, as gloss shows it |
| Markdown | as it is |
| plain text, source | as it is, unfenced |
| JSON, JSONL | pretty-printed, records one after another |
| Excel, Grist, CSV | CSV of the sheet or table (`--page` picks which) |
| PDF | the page's text layer (`--page 3`, `2-5`, or `all` with `--output-dir`/`--json`); page 1 by default |
| image, SVG, mesh | *no text*: an error saying to use `--output` |

A PDF page with **no text layer** (a scan, a figure) is an error on that page,
in the `error` field with `--json`, never a blank success: fall back to
`--output` for that page. Other pages are still done and the exit status is 1.

One text goes to stdout; for several inputs or pages use `--output-dir` (paths
are printed) or `--json`. `--text` needs no terminal.

## 2. See a file: PNG export

Exports need no terminal and never open a UI. With `--output -` the PNG bytes
are the only thing on stdout; with a file or folder, the paths written are.

```sh
gloss --output page.png --page 3 report.pdf      # one page of a PDF
gloss --output - --max-edge 1024 drawing.svg > diagram.png
gloss --output-dir out --max-edge 1536 photo.png drawing.svg report.pdf
cat drawing.svg | gloss --output - --max-edge 1024 > diagram.png
```

- Size is a pixel budget: **pass `--max-edge`** (1–4096, default 1536) set from
  what your model accepts. Aspect ratio is preserved, transparency flattened
  to white, small sources not enlarged.
- `--vision-profile` is only a convenience alias for a `--max-edge`, and it
  **will go stale** as models change; no names are added. An explicit
  `--max-edge` wins over it.

  | Profile | Resolves to |
  | --- | --- |
  | `openai-high` | `--max-edge 1600` |
  | `claude-standard` | `--max-edge 1092` |
  | `claude-high` | `--max-edge 1932` |

- **Existing files are never overwritten**: a taken name gains a suffix
  (`page.png` → `page-2.png`). Read the actual path from stdout, or the
  `output` field with `--json`; stderr's `path: W×H PNG` is diagnostic only.
- `--output` takes one input; `--output-dir` takes many and prefixes each
  name with its index: `001-shapes.png`, `002-landscape.png`; a page or sheet
  adds `-page-2` or `-sheet-2`.
- `--page 3`, `--page 2-5`, `--page 1,3`, or `--page all` with `--output-dir`
  exports several pages of a PDF in one call. `--dpi 36..600` raises raster
  detail for dense pages.
- `--json` with an export prints a manifest to stdout: one object per file and
  page with `path`, `kind`, `page`, `pages`, `output`, `width`, `height`,
  `max_edge` (the edge it was sized to), and `error` where one failed. When a
  profile was named, `vision_profile` and a short `vision_reason` say what it
  resolved to. Prefer it over parsing stderr.
- SVG and PDF are rasterized at the requested size.
- `--type image|svg|pdf|stl|3mf|docx|xlsx|grist|csv|json|ipynb|html|text|markdown`
  forces the format for extensionless files or stdin.
- Markdown, plain text, and tables (Excel, Grist, CSV) cannot be exported as PNG:
  read their text directly, or use `--info`. For a PDF try `--text` first; the
  PNG is for scans, figures, and layout.

### Many files at once: `--glob`

`--glob` searches the folders named (or the current one) for files by glob,
extension, or kind, and puts the matches in the folders' place. It is the safe
way to find files: hidden folders and links are never entered, depth, entry
count, matches, and time are capped, and stderr says when a cap was hit.

```sh
gloss --glob images --info --json ~/photos           # describe every picture
gloss --glob '*.stl' --glob 3mf --output-dir out models/   # export every mesh
gloss --glob 'report*' --glob pdf --info ~/Documents
```

Kinds: `images`, `svg`, `pdf`, `docs`, `word`, `meshes`, `tables`, `excel`,
`grist`, `csv`, `markdown`, `html`, `text`, `json`, `notebooks`.

### Meshes (STL, 3MF): several views beat one big view

A single view hides surfaces. Export a contact sheet instead:

```sh
gloss --output sheet.png --view all model.stl          # six named views, one sheet
gloss --output sheet.png --view iso,front,top model.3mf
gloss --output head.png --parts head --view iso assembly.3mf
gloss --output posed.png --camera 20,-120 --projection perspective model.stl
```

- `--view` accepts `front,back,left,right,top,bottom,iso`, or `all`.
- `--parts name,name` / `--partn 2,4-6` isolate parts of a 3MF assembly —
  also the way to see a model too large to render whole.
- `--color orange` or `#rrggbb` paints plain meshes before export.

## 3. Learn about a file: `--info`

Cheap, no rendering, no UI. Run this **before** exporting when you don't know
the file: it tells you the kind, PDF page count, sheet names, mesh part names,
triangle counts, image dimensions, EXIF, and more.

```sh
gloss --info model.3mf report.pdf
gloss --info --json -p 3 report.pdf
```

`--json` prints an array, one object per file: `path`, `kind`, `page`/`pages`
for PDFs, and `details` by section and label (e.g. `details.Model.Triangles`).
A file that fails gets an `error` field; the others are still described and
the exit status is 1 — treat partial results as usable:

```sh
gloss --info --json a.pdf b.bin | jq '.[] | select(.error == null) | .kind'
```

## 4. Involve the human

### Ask for files: `--pick`

In a terminal, `--pick` blocks until the user hands you files, then prints
their full paths to stdout. Exit status: 0 = paths printed, 1 = error,
2 = the user declined, 124 = timeout.

```sh
gloss --pick --prompt "Choose the March invoice so I can check its totals" --prompt-loc top
paths=$(gloss --pick --prompt "Which export?") || echo "declined or error $?"
```

**With no terminal** (stdin is not one — the usual case for an agent), `--pick`
and `--serve` do **not block**. They start a localhost server apart from your
process and print one JSON object on stdout, exit 0:

```sh
gloss --pick --prompt "The invoice, please" --timeout 10m < /dev/null
# {"status":"waiting","url":"http://127.0.0.1:41233/<token>/","dir":"/tmp/gloss-pick-…",
#  "timeout_seconds":600,"resume_token":"<token>","resume":"gloss --resume <token>","pick":true}
```

1. Give the `url` to the human as a clickable link, explain the request, and
   say how to finish: choose or drop files, review them, then press `Enter`
   in the viewer to send; `q` declines. In the browser, **Choose files** opens
   the native file picker. Pasting local paths or `o` also works in the viewer.
   All files handed over during the session are sent together. If only files
   named on the command line are present, `Enter` sends the one on screen.
2. Retrieve the answer with `gloss --resume <token>`. It
   waits for the answer and then behaves like a terminal pick: paths on stdout
   and exit 0, 2 (declined), or 124 (the timeout ran out). It needs no
   long-lived process of yours: the server is detached and keeps its answer.
   `--resume <token> --timeout 30s` stops *waiting* after 30 s (exit 124, with
   stderr saying it is still waiting); a settled pick answers at once.
   To look without waiting, between other work, `gloss --status <token>` prints
   one JSON object at once and exits 0:
   `{"state":"waiting","settled":false,"seconds_left":412}`. `state` is
   `waiting`, `picked` (with `paths`), `declined`, `timeout`, `closed` (a page
   that only showed something was closed), or `failed` (with `error`, as when
   the server died). It changes nothing: it does not take the answer, end the
   session, or touch the files, so ask as often as you like and then `--resume`
   to collect the answer. Exit 1 means there is no such session, because the
   token is wrong or an answer was already collected and its state removed.
3. Inspect the returned paths with `--text`, `--info --json`, or `--output`
   as appropriate, then continue the user's task. Do not treat startup JSON
   as a successful file selection.
4. **Clean up the session directory when its files are no longer needed.**
   Use the exact `dir` returned at startup; preserve artifacts the user wants
   to keep. Dropped files are private copies there. Never delete original
   paths returned from browsing or pasting. gloss removes its directory on
   timeout or decline, but keeps selected drops after an answer.

- Give `--prompt` a concise request and purpose, such as "Choose the March
  invoice so I can check its totals against your spreadsheet." The browser
  shows the full request as a persistent heading above the viewer. In a
  terminal, use `--prompt-loc top` for prominence; the box defaults to the
  bottom, is capped at four lines, and is hidden if the screen is too small.
- Off a terminal `--timeout` defaults to 10 minutes; the server never outlives it.
- On a terminal `--serve --pick` still blocks and opens the browser; `--no-open`
  prints the address (on stderr) instead. Off a terminal that is the default.
- The server is on 127.0.0.1 only, and nothing is served without the token in
  the `url`; treat the `url` and the resume token as secrets.
- The human's browser must reach the machine where gloss runs. A localhost
  link from a remote host or container does not automatically reach that
  session from the human's computer. Use the environment's supported local
  forwarding or file-handoff mechanism when needed; do not expose the server
  publicly. Files browsed or pasted in the viewer are paths on the gloss host;
  browser-chosen or dropped files come from the human's computer.
- Exit 2 means the user declined. Do not silently retry — ask in chat whether
  they want to continue.
- stdout carries only the answer (paths, or the one JSON object), so it pipes.

### Show the human a file

```sh
gloss report.pdf                  # terminal pager, if the user has a terminal
gloss --serve --prompt "Review page 3 and check the chart labels" --page 3 report.pdf
                                  # (no terminal: prints the JSON object above; show the url)
```

For a document reachable at a CORS-allowing http(s) address, you can instead
hand the user a browser link — no local gloss needed:

```
https://nimblemarkets.github.io/gloss/app.html?src=<url-encoded-address>
```

The page fetches the document in the user's browser; nothing is uploaded
anywhere. Say that when you send the link.

## Guardrails

- **Never run the interactive pager** (none of `--text`, `--output`, `--info`,
  `--pick`, or `--serve`) without a user's terminal: it will fail or hang. The
  four headless modes above are your whole interface; `--pick` and `--serve`
  are headless when stdin is not a terminal, and return at once.
- Input files and stdin are limited to 128 MiB; images to 32 MP; PDFs to
  10,000 pages. `--info` reports these failures cleanly.
- gloss never fetches the network on its own. `--fetch` is an interactive
  option (Enter on a cell's address); the headless modes never need it.
- Use gloss's extraction and rendering when they answer the task. Prefer
  `--text` for document content; add PNGs when appearance matters. Specialized
  analysis or editing may still need other tools.
- Markdown, plain text, and tables (Excel/Grist/CSV) are for `--text`, not for a
  picture; gloss refuses to export them as PNG by design.
