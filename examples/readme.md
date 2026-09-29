# A visual pager

Gloss renders **Markdown with Glamour**, including local raster images and SVGs.
Use `j` / `k` to scroll and `s` to toggle source. Press `m` for the file menu.

## An embedded SVG

![Original shape fixture](shapes.svg)

## Raster images

The same original landscape in PNG and HEIC, decoded locally:

![Layered hills at dusk](landscape.png)

![Layered hills in HEIC](landscape.heic)

## A small table

| Input | Rendering |
| --- | --- |
| Markdown | Glamour |
| SVG | NTCharts SVG |
| STL | NTCharts3d |

```sh
gloss --preview examples/readme.md examples/shapes.svg examples/gloss.stl
```

Remote images are shown as placeholders; local paths resolve beside this file.

## A word in three dimensions

Open `gloss.stl` to orbit GLOSS built from raised blocks. Arrow keys rotate,
`r` toggles auto-rotation, and `0` resets the camera.
