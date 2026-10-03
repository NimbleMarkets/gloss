---
title: "Formats and limits"
weight: 60
bookToC: false
---

# Formats and limits

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
- Mesh color: an STL, and the faces of a 3MF its file leaves plain, are drawn
  in one default blue. `C` opens a color picker over the mesh: a palette of
  swatches, one slider per channel, or a hex number typed in, each change
  painted as it is made; `Enter` chooses the swatch you have moved to (or keeps
  the color set) and closes, changing nothing if you have not moved; `Esc`
  undoes it. A 3MF with colors of its own is recolored whole, and the picker's
  Reset button (or `r`) gives its colors back; `s` limits the paint to the faces
  its file left plain. `--color #rrggbb`, or a name such as `orange`,
  paints the viewer and exports alike.
- 3MF parts: what the build places is listed by name, from the model or the
  slicer's settings, and `c` opens that list over the mesh: `Space` shows or
  hides a part, `Enter` shows it alone with the camera fitted to it, `n` keeps
  only it, `a` and `X` bring all back. `--parts name,name` and `--partn 2,4-6`
  choose parts for the viewer, an export, or `--info`, so an agent can pose
  one part of an assembly. The triangle limit applies to the parts shown, so a
  project too large to draw whole can be seen part by part.
- 3MF: the core specification's meshes, components, and build transforms;
  colors from base materials and color groups; objects kept in separate parts
  of the package, as slicers write them. Projects from Bambu Studio and its
  relatives are drawn in their filament colors, which those programs keep in
  settings of their own; colors painted onto faces are not read. Textures, beam lattices, slices, and
  encrypted content are not read. The same 932,067-face limit applies: a larger
  model is shown by its embedded thumbnail, and described by `i`. A package may
  hold 4,096 entries and unpack to 128 MiB.
- Plain text (`.txt`, `.text`, `.log`, and any file that is not binary):
  shown as it is, in the document view, so that no line of it is read as a
  heading, a list, or emphasis. Returns and a byte order mark are dropped,
  bytes that are not UTF-8 stand as �, and a file longer than 2 MiB is cut.
  A file of no known kind is read as text when its first 8 KiB hold no NUL
  byte, are UTF-8, and are mostly printable, as `less` would show it; source
  code and configuration open that way, highlighted by
  [chroma](https://github.com/alecthomas/chroma) in the language its name or
  first line says. Only binary files are unsupported.
- JSON (`.json`, `.jsonl`, `.ndjson`): pretty-printed and highlighted as a
  fenced block; `s` shows it as it is in the file. A file with one value to a
  line, such as a training set, a batch request, or a log export, is shown as
  numbered records, whatever its name. Malformed JSON is refused with the line
  it fails on. Extensionless JSON and stdin are recognised by parsing. The
  view is cut at 2 MiB of pretty-printed text; `i` says how many records
  there are.
- Jupyter notebooks (`.ipynb`, nbformat 4): Markdown cells as they are, code
  cells fenced in the kernel's language after their `In [n]` count, and each
  output after its cell: text as it was printed, errors without their colour
  codes, and PNG, JPEG, GIF, and SVG outputs as pictures. Widgets, HTML, and
  LaTeX outputs fall back to their text.
- HTML (`.html`, `.htm`): converted to Markdown with
  [html-to-markdown](https://github.com/JohannesKaufmann/html-to-markdown), so
  saved pages and wiki exports read as documents: headings, lists, tables,
  links, code, and pictures beside the file are kept; scripts, styles, and
  layout are not. Nothing is fetched. A page may be 8 MiB.
- Word (`.docx`, `.docm`): turned into Markdown and shown as such, so `s` shows
  the Markdown. Headings, lists, tables, links, pictures, and bold, italic, and
  struck text are kept; page layout, headers and footers, footnotes, comments,
  and text boxes are not. A document longer than 2 MiB of Markdown is cut.
- Excel (`.xlsx`, `.xlsm`): each sheet is a grid that scrolls by row and column,
  with `n` and `p` turning between sheets. Cells show their values, formulas
  by their last result, and dates and times where a cell's style says so. Up to
  100,000 rows and 1,024 columns of a sheet are read; the rest are counted.
  Formatting, merged cells, charts, and pictures are not shown.
- Grist (`.grist`): a Grist document is a SQLite database, and each of its
  tables is shown as a sheet is, with `n` and `p` turning between tables. The
  first row holds the columns' labels, and rows and columns stand in the order
  Grist keeps them in; summary tables come after the others. Cells show what
  is stored: formulas by their last result, as Grist saved it, and an error
  by its name (`#TypeError`). Dates and times are written out, a reference
  shows what Grist shows for it, or `Table[row]` where it shows the row,
  lists are joined with commas, and attachments are named, not opened. A
  hyperlink cell shows its words and leads to its address, as an Excel link
  does. Row
  ids, the positions rows are sorted by, Grist's helper columns, and a
  summary's list of the rows behind each line are left out. Nothing is worked out anew: no formulas, access rules, widgets, or
  number formats, and nothing is written. Up to 100,000 rows and 1,024
  columns of a table are read. A document in SQLite's WAL mode is not read,
  nor is any other SQLite database: gloss is not a database browser.
- CSV (`.csv`, `.tsv`): shown as a sheet is. The separator is read from the
  file: a comma, tab, semicolon, or pipe, whichever the first lines agree on;
  a `.tsv` is read as tabs. Quoted values may hold the separator and line
  breaks. Only the file's name says it is a table: text is not sniffed.
- Columns of a table: `x` hides the one under the cursor, `X` brings them all
  back, and `c` lists them to tick and untick. Hidden columns keep their
  letters, and one column always stays. `--cols Name,City` shows only the
  columns with those headers (or letters), and `--coln 2,4-6` those numbered
  so, in every table opened; a table with none of them is shown whole.
- Web addresses in tables: a cell that holds an address or, in Excel and
  Grist, links to one is marked 🔗, and is a link to the terminal as well:
  click it as your terminal has links clicked (often with Cmd, Ctrl, or
  Shift held, since gloss takes plain clicks) and the terminal opens it in
  your browser. gloss itself starts no browser. A sheet has a cursor, and the
  status bar shows the address under it. With `--fetch`, `Enter` on such
  a cell downloads what it names and opens it like a dropped file; a picture
  is shown as one, and `Esc` closes it and returns to the cell. Only http and
  https are fetched, of no more than 128 MiB, and only on `Enter` or a URL drop/paste: gloss never
  fetches on its own. Fetched files are removed when closed or when gloss
  exits, unless they were picked.
- Input files and stdin are limited to 128 MiB. One active document and, when
  enabled, one independent preview are kept open.
  Images, SVGs, and PDF pages zoom by cropping the existing raster, up to 64×;
  use a higher PDF DPI for more detail. There is no file watching, and no
  fetching unless `--fetch` is given; even then only `Enter` on a cell or an explicit URL drop/paste fetches.
