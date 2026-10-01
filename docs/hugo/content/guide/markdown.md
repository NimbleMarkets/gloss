---
title: "Markdown"
weight: 40
bookToC: false
---

# Markdown

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
