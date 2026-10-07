# A visual pager

Gloss renders **Markdown with Glamour**, including local raster images and SVGs.
Use `j` / `k` to scroll and `s` to toggle source. Press `m` for the file menu.

## An embedded SVG

![Original shape fixture](shapes.svg)

## Raster images

Open `motion.gif` from the gallery for an animated orbit; Space pauses or resumes it.

Two original landscapes, decoded locally: blue hills at dusk in PNG and a sunny desert in HEIC.

![Layered hills at dusk](landscape.png)

![Sunlit dunes and a cactus in HEIC](landscape.heic)

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
