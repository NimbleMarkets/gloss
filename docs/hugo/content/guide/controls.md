---
title: "Controls"
weight: 20
bookToC: false
---

# Controls

| Key | Action |
| --- | --- |
| `q`, `Ctrl-C` | Quit |
| `q`, `Q` | Quit / quit leaving the last view in the scrollback, whatever `-X` said |
| `?`, `Esc` | Show help / dismiss help |
| `Esc` | Back to the file list, with previews, from a document |
| `t` | In the file list, show the files as a grid of thumbnails, and back |
| `m` | Open the file-selection menu |
| `o`, `O` | Browse folders for a file to open |
| `]`, `Tab` / `[`, `Shift-Tab` | Next / previous file |
| `n`, `Space`, `PageDown` / `p`, `b`, `PageUp` | Next / previous PDF page, sheet, or Grist table; next / previous file for other formats |
| `Home` / `End`, `G` | First / last PDF page |
| Arrows, `h j k l` | Move about a spreadsheet by row and column |
| `x`, `X` | Hide the column under the cursor / show every column |
| `c` | List the columns, to show and hide them: `Space` toggles, `a` all, `n` none |
| `Enter` | With `--fetch`, open the web address under the cursor |
| `Esc` | Close a fetched file and return to its cell |
| `+`, `-` | Zoom in / out |
| Arrows, `h j k l` | Pan zoomed images; orbit meshes |
| `f`, `0` | Fit image / reset camera |
| `g` | Toggle Kitty / glyph output when Kitty is supported |
| `R` | Reload file from disk |
| `e` | Export the current page as a PNG in the working directory |
| `i` | Show or hide a box of details about the current file and the terminal |
| `r` | Toggle mesh auto-rotation; reload other formats |
| `5` | Toggle mesh orthographic / perspective projection |
| `c` | List the parts of a 3MF: `Space` shows or hides one, `Enter` focuses on it, `a` all, `n` only it |
| `X` | Show every part of a 3MF again |
| `C` | Pick a color for a mesh: `Tab` between palette, sliders, and hex; `Space` applies the swatch, `Enter` chooses the swatch you have moved to and closes, `r` (the Reset button) restores the file's colors, `s` paints all faces or only plain ones, `Esc` undoes |
| `B` | Pick the background color behind a mesh, with the same picker as `C`: `r` restores the default, `Enter` chooses the swatch you have moved to and closes, `Esc` undoes |
| Click, in the `C` / `B` picker | Click a tab, swatch, or slider bar; a swatch applies at once and a second click on it chooses it and closes; drag along a slider |
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
`--preview` starts there with previews enabled. `t` in the list shows the
files as a grid of thumbnails instead: one picture, made from each file's own
as they are drawn in turn, with the file's kind standing in where it has
none. Arrows move about it, `Enter` opens, a click selects and a second click
opens, and `t` returns to the list. `Esc` from a document comes back to
whichever style was last used.

Drag files from a file manager onto the terminal to add them to the list.
Terminals deliver a drop as a bracketed paste of paths or `file://` URIs; text
that does not read as paths is ignored. Dropped files follow the same format and
size rules as arguments, and any that are skipped are reported on exit. A
folder dropped on its own opens the file browser there, as a folder named on
the command line does; one dropped beside files is skipped.
`gloss` alone opens the viewer with no file, as a drop target. In the
browser demo, dropped files stay in the tab's memory until it reloads.

Press `o` to browse for a file instead, starting in the folder of the one you
are viewing. Type to filter the list, or type a path such as `~/Pictures/` to
go straight there; `Tab` completes, `Enter` opens a file or enters a folder,
and `Esc` cancels. The chosen file joins the list like a dropped one.
`gloss folder` starts in the browser at that folder. Beside files, folders are
skipped, so globs stay safe.

Files gloss cannot show are greyed and cannot be chosen; `Ctrl-T` hides them,
and again shows them. Known kinds are judged by extension; any other file is
looked into as the folder is listed, and offered when it reads as text, so
only binary files are greyed. `Ctrl-X` sets text files aside, for folders
where they are clutter, and offers them again. With `--type`, nothing is
greyed. Each file is marked by kind: 📁 folders, 📷 pictures, 🎨 SVG, 📕 PDF,
🧊 meshes, 📝 Markdown, HTML, and text, 📃 text by content, 🧾 JSON, 📓 notebooks, 📄 Word, 📊 tables.
`Ctrl-S` changes the order: by name, by date with the newest first, or by
kind; folders always come first, and the order is kept for the next browse.
`G`, with nothing typed in the filter, asks for a folder's path, starting from
the one shown: `Tab` completes it, `~/` starts from home, `Enter` goes there,
and `Esc` comes back to the listing.

`/`, likewise, searches the folder shown and those under it. Ask for globs
(`*.png`, `report*`, `deep/*.pdf`), extensions (`png`), or kinds (`images`,
`svg`, `pdf`, `docs`, `word`, `meshes`, `tables`, `excel`, `grist`, `csv`,
`markdown`, `html`, `text`, `json`, `notebooks`), several at once; names are matched
without regard to case. `Enter` searches, then opens the file under the
cursor; `Ctrl-A` adds every match; `Esc` returns to the listing. A search is
kept safe by its limits: hidden folders and links are not entered, folders
deeper than 6 are not searched, at most 10,000 entries are looked at and 500
matches shown, and it stops after 3 seconds, saying which limit it met.
`--glob pattern` does the same from the command line for the folders named,
or the current one: `gloss --glob images --glob '*.stl' ~/models` opens every
match, and with `-o` or `--info` exports or describes them all.
The browser is [picky](https://github.com/pgavlin/picky), carried in
`internal/picky` with those two additions; the embedded demo, which has no
folders, does not offer it.

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
| 3MF | Title, designer, description, application, dates, license, object and triangle counts, the parts by name, extent and surface area in the file's unit, thumbnail size |
| Markdown | Title, lines, words, headings, links, images |
| Word | Title, author, dates, application, pages and words as Word counts them, headings, tables, images, links |
| Excel | Sheets with their size, title, author, dates, application |
| Grist | Tables with their size, time zone, schema version, what is not shown |
| CSV | Separator, rows, columns |
| JSON | Whether it is one value or lines of records, how many, the top-level kind and first keys |
| Notebook | Format version, language, kernel, cells by kind, outputs, pictures |
| HTML | Title, and the lines, words, headings, links, and images of the text |
| Text | Lines, words, characters |

Only fields present in the file are listed. The box sits in the top-right
corner over the document, which stays in use beneath it: the details follow as
you turn pages or change files. `i` or `Esc` closes it.

The box ends with a `Terminal` section: the screen size in cells and, when the
terminal has said, in pixels; whether pictures are drawn as glyphs or Kitty
graphics and whether Kitty graphics were found to be supported; how meshes are
rendered; the terminal program and multiplexer; and which screen is in use.
With no file open, `i` shows that section alone, so `gloss` then `i` says what
the terminal can do.
