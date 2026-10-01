---
title: "gloss"
type: docs
bookToC: false
---

# gloss

A visual pager for the terminal: like `less`, for images, SVGs, PDFs, STL and 3MF meshes, Markdown, HTML, plain text, JSON, Jupyter notebooks, Word, Excel, Grist, and CSV files.

[**Try it in your browser →**](https://nimblemarkets.github.io/gloss/) The live terminal runs the actual Go pager as WebAssembly, with one sample of every kind.

## Quick start

```sh
brew install --cask nimblemarkets/tap/gloss

gloss photo.png report.pdf model.stl      # page through them
gloss --menu *.png                        # a menu to choose among files
gloss --page 3 --output page3.png report.pdf   # a PNG, with no terminal
gloss --info --json model.3mf             # what a file says about itself
```

## Where to look

- **[Guide](guide/)**: installing, the keys, asking the user for a file, the formats gloss opens and their limits, and how to use it from an agent.
- **[Command reference](command/)**: every option, grouped by what it is for. gloss has no subcommands; it is one command with clusters of options.
- **[For LLMs](guide/for-llms/)**: the skill gloss carries to teach an agent its headless surface (`gloss --skill`).

The man page, `gloss(1)`, ships in the release archives and the Debian package.

## Source

gloss is MIT-licensed and built in Go on [NTCharts](https://github.com/NimbleMarkets/ntcharts). The source, issues, and releases are on [GitHub](https://github.com/NimbleMarkets/gloss).
