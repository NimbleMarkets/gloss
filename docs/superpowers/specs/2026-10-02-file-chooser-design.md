# File chooser component

Written autonomously on 2026-10-02 while the owner was away; every decision
below is mine, flagged so it can be overruled.

## Goal

Finding files is a core part of gloss. The browser (`o`) should be the most
useful file chooser a terminal pager can offer, with more than one *topology*
for navigating (as Finder has list, columns, and a sidebar), and with the
pieces that make a chooser fast: breadcrumbs, tab completion, and filtering.

## What exists

- `internal/picky`: a vendored fork of `pgavlin/picky`. A single-column list,
  a filter that doubles as a path input, tab completion to a common prefix.
  Swallows read errors; no horizontal navigation; one layout.
- `internal/app/opener.go`: wraps picky and holds browsing logic that is not
  gloss's alone: go-to, the read log, sorting, hiding. Not reusable or testable
  without the whole pager.

## Decisions

1. **New package `internal/browse`**, a Bubble Tea component with no
   dependency on `internal/app` or `internal/document`. gloss-specific
   behavior (marks, sort, which files are selectable) is injected by options.
   It subsumes picky; picky is deleted once the opener moves over (its MIT
   notice stays with any code carried across).
2. **Harness first**: `internal/browse/browsetest`, generic over any
   component with `Init/Update/View`, so it can also characterize picky:
   an in-memory filesystem (latency, injected errors, read counting), a key
   driver that settles async commands, a screen type, and a script runner
   (`testdata/*.txt`, golden screens in `*.golden`, `-update` to rewrite).
   New ideas are added as scripts; the goldens are readable screens.
3. **Core state is layout-independent**: a committed folder, a cache of
   listings, a remembered cursor per folder, and a filter/path input. A
   *layout* only draws that state, so topologies share behavior.
4. **Layouts**: `list` (as today) and `columns` (Miller columns: ancestors,
   the folder, and a peek at what is under the cursor: a folder's listing or
   a file's details). More can follow (places sidebar, tree).
5. **Navigation**: `←`/`backspace` on an empty filter goes up, `→` goes into a
   folder; `Enter` opens a folder or chooses a file; `alt+↑` always goes up.
6. **Breadcrumbs**: a header of segments, home as `~`, elided from the left,
   clickable with the mouse.
7. **Tab completion**: path aware (`~`, `..`, segments), extends to the
   longest common prefix, then cycles candidates on repeated Tab (Shift-Tab
   back), shows the pending completion as ghost text.
8. **Filter**: ranked (prefix, word boundary, consecutive, subsequence),
   smart case, matched letters highlighted, globs (`*.png`), an extension
   shortcut (`.png`), and `.`-prefixed queries reach hidden files.
9. **Errors are shown**, not swallowed ("permission denied" in the list).
10. **App integration last**, keeping the existing app tests green: the
    opener becomes a thin adapter; `Ctrl-L` switches layouts. The default
    stays the list until the owner chooses otherwise.

## Out of scope

Multi-select, file operations (rename, delete), preview rendering beyond
file details (gloss's own preview pane covers that), network filesystems.

## Status (end of the first session)

Built: harness, `internal/browse` (list, columns, places layouts; ranked
filter; tab completion with ghost text, cycling, and partial paths;
breadcrumbs; history; mouse), the app adapter, picky removed, guide page.
Not built: a visual file preview in the last column (needs the app's picture
widget placed inside the layout), a tree layout, multi-select.
