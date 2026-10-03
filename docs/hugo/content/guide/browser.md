---
title: "The file browser"
weight: 25
bookToC: false
---

# The file browser

The file browser is how you find a file to open without leaving gloss. It is a
fast, keyboard-first chooser with a mouse as well: it filters as you type,
completes paths with `Tab`, lays folders out in columns like a file manager, and
keeps a sidebar of places. Everything it offers has a key, and none of it changes
a file: it only looks, and hands back what you choose.

**On this page:** [opening it](#opening-it) · [what you see](#what-you-see) ·
[moving around](#moving-around) · [layouts](#layouts) · [breadcrumbs](#breadcrumbs) ·
[the filter](#the-filter) · [Tab completion](#tab-completion) ·
[going to a folder](#going-to-a-folder-g) · [finding files below](#finding-files-below-)
· [file types](#file-types) · [what is shown](#what-is-shown-and-in-what-order) ·
[choosing](#choosing) · [keys at a glance](#keys-at-a-glance) ·
[terminals](#notes-on-terminals)

## Opening it

- Press **`o`** (or `O`) in the viewer. It starts in the folder of the file you
  are viewing, or the current folder if there is none.
- Name a **folder** on the command line, `gloss ~/Pictures`, or drop a folder on
  the terminal: gloss starts in the browser there.
- A file you choose joins the list of files, like one you had dropped, and opens.
  `Esc` closes the browser and leaves the viewer as it was.

The embedded browser demo has no folders, so it does not offer the browser.

## What you see

```text
 ~
  Filter: type to filter, or a path: / ~ ../
                     ..
>         2026-01-02 projects/
     300B 2026-01-02 notes.md
     52kB 2026-01-02 photo.png
    1.2kB 2026-01-02 readme.md

 2/5 · list
```
The top row is the **breadcrumbs**: where you are. Under it is the **filter**,
where you type. Then come the **rows** of the folder, each with its kind's mark
(the screens here are drawn without the marks, which gloss shows as pictures:
📁 📷 📕), its size, and when there is room its date, and the cursor `>` on one.
At the bottom, the **footer** says which row of how many the cursor is on, which
layout is in use, and which kinds of file are chosen (see
[File types](#file-types)). Below the browser, as everywhere in gloss, are the
status bar and the keys that work.

Folders are listed first, then files; in the list the folder above is the first
row, `..`. Files gloss cannot show are greyed and cannot be chosen. A folder you
cannot read says why in the list (`permission denied`, `no such folder`) and
`←` still takes you back.

## Moving around

| Key | Does |
| --- | --- |
| `↑` `↓`, `Ctrl-P` `Ctrl-N` | Move the cursor |
| `PgUp` `PgDn` | Move a screenful |
| `Home` `End` | First and last row, while the filter is empty |
| `Enter` | Open a folder, or choose a file |
| `→` | Into the folder under the cursor |
| `←`, `Backspace` on an empty filter, `Alt-↑` | Up a folder, the cursor on the one you left |
| `Alt-←` `Alt-→`, `Ctrl-O` | Back and forward through the folders you visited |
| `Esc` | Close what floats over the folder, then leave the browser |

Going up lands the cursor on the folder you came from, and gloss remembers where
the cursor was in each folder you have left, so `←` then `→` is a round trip.

**The mouse** works wherever it makes sense: click a crumb to go to that folder,
click a row to put the cursor on it and click it again to open it, click a row in
another column to go there, click a place in the sidebar, click the footer's
`types ▾` to open the menu, and use the wheel to scroll. While the browser has
the mouse, use your terminal's usual override to select text (usually `Shift`
with the drag).

## Layouts

`Ctrl-L` cycles through three layouts of the same folder. Keys, filter, and
completion are the same in all; only the drawing differs. The choice is kept
until you quit gloss.

### List

One column: a mark, the size, the date when the terminal is 60 columns or wider,
and the name. It is the default, and the best for a long folder you will filter.

### Columns

```text
 ~ › projects › gloss
  Filter: type to filter, or a path: / ~ ../
▸ gloss/                      │> internal/                  │  app/
  tiny/                       │  README.md                  │
                              │  main.go                    │
                              │                             │
                              │                             │
                              │                             │
                              │                             │
                              │                             │
                              │                             │
                              │                             │
                              │                             │
 1/3 · columns
```
Miller columns, as in a file manager. The folders above this one are on the left,
with `▸` marking the way you came; this one has the cursor `>`; and on the right
is **what is under the cursor**: a folder's contents, or for a file its size,
date, and folder. As you move down the middle column the right column follows.
Press `→` to step into it and the columns slide across. As many columns are drawn
as the terminal's width allows, at least 22 cells each, up to four.

### Places

```text
 ~ › Documents
  Filter: type to filter, or a path: / ~ ../
 Places             │▸ Documents/              │> taxes/                  │  2025.pdf
  Home              │  Downloads/              │  report.pdf              │
▸ Documents         │  Pictures/               │                          │
  Downloads         │  projects/               │                          │
  Pictures          │                          │                          │
  Root              │                          │                          │
 Recent             │                          │                          │
  ~/projects/gloss  │                          │                          │
                    │                          │                          │
                    │                          │                          │
                    │                          │                          │
 1/2 · places
```
The columns, with a sidebar. **Places** are your home folder, the common folders
in it that exist (Desktop, Documents, Downloads, Pictures, Movies or Videos, Music), and
the root. **Recent** are the other folders you have visited in this visit, latest
first. `▸` marks the place you are in.

`Ctrl-G` moves the cursor into the sidebar (and opens this layout if you are in
another): `↑` `↓` pick, `Enter` goes there, and `Esc`, `Tab`, or just starting to
type takes you back to the folder. A click on a place goes there too. The sidebar
needs a terminal 60 columns wide; narrower, this layout is the plain columns.

## Breadcrumbs

```text
 … › app › browser › deep
  Filter: type to filter, or…
          ..
>    100B notes.md

 2/2 · list
```
The path across the top is made of folders you can click. Your home folder is
`~`, and a path too long for the width is cut from the left, the `…` standing for
the folders left out: clicking it goes to the nearest of them. The last name is
the folder you are in, and always keeps what room there is.

## The filter

Whatever you type narrows the folder at once. It is a fuzzy match: the letters
you type must appear in order in a name, and the **best fit comes first**, not
the first in the alphabet.

```text
 ~
  Filter: re
>   1.2kB 2026-01-02 readme.md
      10B 2026-01-02 README.txt
     99kB 2026-01-02 report.pdf
          2026-01-02 projects/

 1/4 matches · 8 in folder · list
```
Names that *begin* with what you typed rank highest, then those where the letters
start words (`gl` finds `gloss` before `flag-list`), then runs of letters, then
scattered ones. The letters that matched are underlined. It is not case-sensitive
until you type a capital, which makes the match exact (`RE` finds `README.txt` and
not `readme.md`).

- **Globs:** a `*`, `?`, or `[` makes the filter a glob: `*.png`, `img_??.jpg`.
- **Hidden files:** dot-files are not listed, but a filter that starts with a dot
  reaches them: `.git`, `.pro`.
- **Paths:** type a path, `~/Pictures/`, `../`, `/etc/`, and the browser shows
  that folder without leaving this one; the breadcrumbs follow. With a folder
  typed in full, `Enter` goes there; with the cursor moved to a row, `Enter` does
  what it would on that row.
- **Editing:** `←` `→`, `Ctrl-A` `Ctrl-E`, `Ctrl-W` and `Ctrl-U` edit what you
  typed, as in a shell. `←` and `Backspace` go up a folder only when the filter is
  empty.

## Tab completion

`Tab` completes the end of what you typed, a little at a time:

1. **One candidate** is completed in full, a folder gaining its slash so you can
   carry on typing inside it.
2. **Several candidates** are completed to the start they share.
3. **No more to share:** further presses step through the candidates, `Shift-Tab`
   backward. The list stays as it was, with the cursor following, so you see what
   you are choosing from. Typing ends the stepping.

```text
 ~
  Filter: readme.md
>    100B 2026-01-02 readme.md
     100B 2026-01-02 readings.txt

 1/2 matches · 5 in folder · list
```
If nothing begins with what you typed, `Tab` offers the best fuzzy match, so
`rdm` becomes `readme.md`. What `Tab` would add is shown dim after the cursor, and
`→` takes it:

```text
 ~
  Filter: projects/
>         2026-01-02 projects/

 1/1 matches · 4 in folder · list
```
Paths complete segment by segment, and folders you have only begun are completed
too, as in zsh: `~/pi/tr` and `Tab` become `~/Pictures/trips/`, as long as each
begins just one folder. If one begins several, the path is left as typed for you
to say which.

## Going to a folder: `G`

With nothing typed in the filter, **`G`** asks for a folder's path, starting from
the one shown. It is the way to jump far: `~/`, `../..`, or a path pasted from
elsewhere. `Tab` completes **folders only**: the files in the folder are still
listed, so you can see what is there, but they are not offered, and a name only a
file begins is not completed. `Enter` goes there; `Esc` comes back to the listing.

## Finding files below: `/`

With nothing typed, **`/`** searches the folder shown and those under it, by
glob (`*.png`, `report*`, `deep/*.pdf`), extension (`png`), or kind (`images`,
`svg`, `pdf`, `docs`, `word`, `meshes`, `tables`, `excel`, `grist`, `csv`,
`markdown`, `html`, `text`, `json`, `notebooks`), several at once, without regard
to case. `Enter` searches, then opens the file under the cursor; `Ctrl-A` adds
every match to the list; `Esc` returns to the listing. A search is kept safe by
its limits: hidden folders and links are not entered, folders deeper than 6 are
not searched, at most 10,000 entries are looked at and 500 matches shown, and it
stops after 3 seconds, saying which limit it met. `--glob` does the same from the
command line (see [Controls](../controls/)).

## File types

**`Ctrl-F`**, or a click on `types ▾` at the right of the footer, opens a list of
the kinds of file gloss shows: pictures, drawings (SVG), PDF, meshes, Markdown
and text, JSON, notebooks, Word, and tables.

```text
 ~
  Filter: type to filter, or a path: / ~ ../
                     ..                ╭─ File types ───────────────╮
>         2026-01-02 projects/         │  [ ] All types             │
     52kB 2026-01-02 photo.png         │> [x] ▫ Pictures            │
     40kB 2026-01-02 trip.jpg          │  [ ] ▪ Documents           │
                                       │  [ ] ▤ Data                │
                                       ╰ space toggles · esc closes ╯

 2/4 · list                                         types: Pictures ▾
```
- `↑` `↓` move, `Space` or `Enter` checks a kind, and a click does the same. The
  list stays open as you choose, and the folder changes under it.
- With kinds chosen, only files of those kinds are shown, **beside every folder**,
  which is how you get to the files. With none chosen, everything is. The first
  row, *All types*, clears the choice.
- The footer names the kinds chosen. The choice applies to every folder you walk
  into, to what you type in the filter, and to `Tab` completion, and is kept
  until you quit gloss.
- `Esc` or `Ctrl-F` closes the list; any other key closes it and then does what
  it would have done, so typing is never lost. A click elsewhere closes it too.

## What is shown, and in what order

- **Marks.** Each file is marked by kind: 📁 folders, 📷 pictures, 🎨 SVG, 📕 PDF,
  🧊 meshes, 📝 Markdown, HTML, and text, 📃 text by content, 🧾 JSON, 📓
  notebooks, 📄 Word, 📊 tables. Nothing means gloss cannot show it.
- **Greyed files** cannot be chosen. Known kinds are judged by extension; any
  other file is looked into as the folder is listed, and offered when it reads as
  text, so only binary files are greyed. `Ctrl-T` hides the greyed files, and
  again shows them. With `--type`, nothing is greyed.
- **Text files.** `Ctrl-X` sets text files aside, for folders where they are
  clutter, and offers them again.
- **Order.** `Ctrl-S` cycles by name, by date (newest first), and by kind;
  folders always come first.
- **Links** to folders are followed like folders.

Layout, order, the two `Ctrl` toggles, and the kinds chosen are remembered for
the next time you open the browser in this session.

## Choosing

`Enter` on a file (or a click on the row the cursor is on) chooses it: gloss
checks that it can open it, and if so adds it to the list and shows it. If it
cannot, the browser stays open and says why, so choosing something else is one
key away. You can also **drop** a file, or paste a path, on the terminal while
the browser is open; it is added the same way.

## Keys at a glance

| Key | Does |
| --- | --- |
| *type* | Filter the folder, or type a path |
| `↑` `↓` `PgUp` `PgDn` `Home` `End` | Move |
| `Enter` | Open a folder, choose a file |
| `→` / `←` | Into the folder / up a folder |
| `Alt-↑`, `Alt-←` `Alt-→`, `Ctrl-O` | Up; back and forward in the history |
| `Tab` / `Shift-Tab` | Complete; step through candidates |
| `G` | Go to a typed folder (folders complete) |
| `/` | Search below this folder |
| `Ctrl-F` | Choose kinds of file to show |
| `Ctrl-L` | Change layout: list, columns, places |
| `Ctrl-G` | Move to the sidebar of places |
| `Ctrl-S` | Sort: name, date, kind |
| `Ctrl-T` | Hide or show what gloss cannot show |
| `Ctrl-X` | Set text files aside, or offer them again |
| `Esc` | Close the menu or sidebar, go-to, or search; then leave |
| `Ctrl-C` | Quit gloss |

## Notes on terminals

- **Alt keys.** `Alt-↑`, `Alt-←`, and `Alt-→` need your terminal to send Alt as
  Escape (in macOS Terminal and iTerm, set Option to act as Meta). `←`,
  `Backspace`, and `Ctrl-O` do the same without it.
- **The mouse.** The browser asks the terminal for clicks and the wheel. If your
  terminal does not pass them, everything is on the keyboard.
- **Marks.** The marks are emoji; a terminal that draws them one cell wide will
  look slightly off but works the same.

## What it does not do

The browser looks; it does not rename, move, delete, or preview contents (that is
what opening a file is for). It lists what the filesystem says, so a folder that
changes while you look needs you to leave and return to see it.
