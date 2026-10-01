---
name: gloss
description: View and inspect files from the shell with the gloss pager. Export images, PDF pages, SVG, and STL/3MF meshes as PNGs sized for vision models, read document metadata as JSON, and ask the user to pick or view files in a terminal or browser. Use when you need to see a file's visual content, get document metadata, or have the user hand you files.
---

# gloss: seeing files, and being shown them

gloss is a visual pager (`less`, with pictures). For an agent it has four
distinct uses, in the order you will need them:

1. **Read a file's text** — `--text`: Word, HTML, notebooks, and sheets as
   Markdown or CSV, no rendering.
2. **See a file yourself** — headless PNG export, sized for your vision budget.
3. **Learn about a file** — metadata as JSON, no rendering.
4. **Involve the human** — they pick files for you, or view what you show them.

**stdout carries the answer, in the form you asked for.** Bytes with
`--output -`; the text with `--text`; otherwise the *paths written*, one to a
line, so `paths=$(gloss …)` is the whole protocol. `--json` gives a manifest
instead. Diagnostics go to stderr. Exit status 1 means at least one input
failed; the rest were still done.

`gloss --skill` prints this file, so the installed binary can always say what
it itself does; `gloss --skill --install` writes it where the agents on the
machine look for skills.

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
| image, SVG, PDF, mesh | *no text*: an error saying to use `--output` |

One text goes to stdout; for several inputs use `--output-dir` (paths are
printed) or `--json`. `--text` needs no terminal.

## 2. See a file: PNG export

Exports need no terminal and never open a UI. With `--output -` the PNG bytes
are the only thing on stdout; with a file or folder, the paths written are.

```sh
gloss --output page.png --page 3 report.pdf      # one page of a PDF
gloss --output - --max-edge 1024 drawing.svg > diagram.png
gloss --output-dir out --max-edge 1536 photo.png drawing.svg report.pdf
cat drawing.svg | gloss --output - --max-edge 1024 > diagram.png
```

- Default maximum edge is 1536 px; `--max-edge` accepts 1–4096. Aspect ratio is
  preserved, transparency flattened to white, small sources not enlarged.
- Prefer `--vision-profile` over guessing a size:

  | Profile | Use when you are |
  | --- | --- |
  | `openai-high` | an OpenAI model with high-detail vision |
  | `claude-standard` | a Claude model (default tier) |
  | `claude-high` | a Claude model with the high-res tier |

- **Existing files are never overwritten**: a taken name gains a suffix
  (`page.png` → `page-2.png`). stderr reports each file written as
  `path: W×H PNG`, so read the path from there, or use `--output -`.
- `--output` takes one input; `--output-dir` takes many and prefixes each
  name with its index: `001-shapes.png`, `002-landscape.png`; a page or sheet
  adds `-page-2` or `-sheet-2`.
- `--page 3`, `--page 2-5`, `--page 1,3`, or `--page all` with `--output-dir`
  exports several pages of a PDF in one call. `--dpi 36..600` raises raster
  detail for dense pages.
- `--json` with an export prints a manifest to stdout: one object per file and
  page with `path`, `kind`, `page`, `pages`, `output`, `width`, `height`, and
  `error` where one failed. Prefer it over parsing stderr.
- SVG and PDF are rasterized at the requested size.
- `--type image|svg|pdf|stl|3mf|docx|xlsx|grist|csv|json|ipynb|html|text|markdown`
  forces the format for extensionless files or stdin.
- Markdown, plain text, and tables (Excel, Grist, CSV) cannot be exported as PNG:
  read their text directly, or use `--info`.

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

Blocks until the user hands you files, then prints their full paths to stdout.
Exit status: 0 = paths printed, 1 = error, 2 = nothing chosen, 124 = timeout.

```sh
gloss --pick --prompt "Drop the March invoice here"      # in a terminal
gloss --serve --pick --prompt "The invoice, please"      # in the user's browser
paths=$(gloss --pick --prompt "Which export?") || echo "declined or error $?"
```

- Always give `--prompt`: it says what you want and why, on screen throughout.
- `--serve` is for when you have no terminal: it opens a localhost page in the
  user's browser. Files they drop on the page are written to a private temp
  folder; the ones they pick are kept there for you (delete them when done),
  and the rest are removed when gloss exits. `--timeout 10m` bounds the wait.
- Exit 2 means the user declined. Do not silently retry — ask in chat whether
  they want to continue.
- stdout carries only the paths, so it pipes cleanly.

### Show the human a file

```sh
gloss report.pdf                  # terminal pager, if the user has a terminal
gloss --serve report.pdf          # the same viewer, on a localhost web page
```

For a document reachable at a CORS-allowing http(s) address, you can instead
hand the user a browser link — no local gloss needed:

```
https://nimblemarkets.github.io/gloss/app.html?src=<url-encoded-address>
```

The page fetches the document in the user's browser; nothing is uploaded
anywhere. Say that when you send the link.

## Guardrails

- **Never run interactive gloss** (no `--text`, `--output`, `--info`, or
  `--pick`) without a user's terminal: it will fail or hang. The four headless
  modes above are your whole interface.
- Input files and stdin are limited to 128 MiB; images to 32 MP; PDFs to
  10,000 pages. `--info` reports these failures cleanly.
- gloss never fetches the network on its own. `--fetch` is an interactive
  option (Enter on a cell's address); the headless modes never need it.
- Don't parse or convert files yourself when gloss can show them: an exported
  PNG plus `--info --json` is usually cheaper and more accurate than a
  hand-rolled extractor.
- Markdown, plain text, and tables (Excel/Grist/CSV) are for `--text`, not for a
  picture; gloss refuses to export them as PNG by design.
