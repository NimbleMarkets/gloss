---
title: "For agents"
weight: 50
---

# For agents

[`skills/gloss/SKILL.md`](../for-llms/) teaches an agent gloss's
headless surface, in the order an agent needs it: `--text`, a sized PNG export,
`--info --json`, then `--pick` or `--serve`. The
binary carries it, so it always matches the flags it was built with:

```sh
gloss skill install                              # install it for the agents found on this machine
gloss skill install ~/.claude/skills             # or into the skills folder named
gloss skill                                      # print it, to place it by hand
```

Or, with Node, the [skills CLI](https://github.com/vercel-labs/skills) installs
it from the repository for every agent it finds:

```sh
npx skills add NimbleMarkets/gloss
```

## Images for vision models
Export PNGs without opening a terminal UI. The default maximum edge is **1536
pixels**; choose the size appropriate to your model and the detail you need.
This is a configurable size budget, not a promise of identical model token costs.

```sh
gloss --output page.png --max-edge 1536 --page 3 report.pdf
gloss --output diagram.png --max-edge 1024 drawing.svg
gloss --output mesh.png --max-edge 1536 model.stl
gloss --output page.png --vision-profile claude-standard report.pdf   # an alias, see below
gloss --output-dir model-inputs --max-edge 768 photo.png drawing.svg report.pdf
gloss --output-dir pages --page all report.pdf         # every page, or 2-5, or 1,3
cat drawing.svg | gloss --output - --max-edge 1024 > diagram.png
```

The size is a pixel budget, and `--max-edge` is the stable flag for it: set it
from what your model accepts. `--vision-profile` (`openai-high`,
`claude-standard`, `claude-high`) is only a convenience alias that resolves to a
`--max-edge` (1600, 1092, and 1932: the edge at which a square picture fits the
provider's patch budget, checked 2026-09-29). Providers change their budgets, so
the aliases **will go stale**, and no names will be added; harnesses should pass
`--max-edge`. An explicit `--max-edge` wins over a profile.

Standard output carries the answer, in the form asked for: the PNG bytes with
`--output -`, else the paths written, one to a line, as `--pick` prints them.
Nothing is ever overwritten: a taken name gains a number before its extension
(`page.png`, then `page-2.png`), and `--output-dir` names each file after its
input with an index (`001-shapes.png`, `001-report-page-2.png`). `--json`
prints a manifest instead, one object per file and page: `path`, `kind`,
`page`, `pages`, `output`, `width`, `height`, `max_edge` (the edge it was sized
to), and `error` where one failed; when a profile was named, also
`vision_profile` and a short `vision_reason`. A failure is reported on stderr
and the rest go on; the exit status is 1.

### Text for language models

`--text` takes the text out, for a reader that wants words rather than a
picture: the Markdown gloss makes of a Word document, an HTML page, or a
notebook; Markdown as it is; text and JSON as they are, unfenced; and a sheet
as CSV. A PDF gives the text layer of the page asked for. Pictures, SVG, and meshes
have no text, and say so, pointing at `--output`.

```sh
gloss --text report.docx                          # Markdown on stdout
gloss --text --page all --output-dir sheets sales.xlsx   # one CSV per sheet
gloss --text --page 3 report.pdf                  # the text layer of one page
gloss --text --page all --output-dir text report.pdf   # one .txt per page
gloss --text --json notes.txt sales.csv           # [{path, kind, text}, …]
gloss --glob docs --text --output-dir text ~/Documents
```

One text goes to stdout; several (inputs, sheets, or pages) go to files with
`--output-dir`, whose paths are then the answer, or into a `--json` manifest
with a `text` field each. A PDF page with no text layer, as a scan or a figure
has none, is an `error` on that page and never a blank success; the other pages
are still made and the exit status is 1. Fall back to `--output` for such a
page. A PDF's pages are bounded as for export (10,000), and a page's text to
16 MiB.

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
gloss --parts head --view iso -o head.png assembly.3mf   # one part of a project
gloss --color orange --view iso -o part.png part.stl     # painted
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
