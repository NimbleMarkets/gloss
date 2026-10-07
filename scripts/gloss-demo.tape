# Run from the gloss repository root after `GOWORK=off task build`:
#   env -u NO_COLOR sigmamax scripts/gloss-demo.tape
# Local development build:
#   env -u NO_COLOR ../sigmamax/bin/sigmamax scripts/gloss-demo.tape
# Realtime capture is required for GIF playback and mesh auto-rotation.
# Menlo ships with macOS; on other systems choose an installed monospace font
# with box-drawing glyphs, or use sigmamax's bundled "Go Mono".

Output dist/gloss-demo.gif
Output dist/gloss-demo.txt
Require ./gloss
Set Shell /bin/sh
Set FontFamily "Menlo"
Set FontSize 16
Set Columns 88
Set Rows 30
Set Theme "Catppuccin Mocha"
Set Padding 16
Set Margin 16
Set MarginFill "#201936"
Set WindowBar Colorful
Set BorderRadius 12
Set Framerate 12
Set TypingSpeed 35ms
Set WaitTimeout 60s
Set CursorBlink false

# A brief title card. No build logs or personal shell configuration on screen.
Hide
Type@1ms `unset NO_COLOR; PS1='demo> '; export PS1; printf '\033[2J\033[H\n  \033[1;38;2;167;215;177mgloss\033[0m\n\n  A visual pager for your terminal.\n\n  Images   GIFs   PDFs   3D   Markdown   Tables\n\n  Real Kitty graphics, recorded with sigmamax.\n'`
Enter
Wait+Screen /Real Kitty graphics, recorded with sigmamax\./
Show
Sleep 2s

# One session, eight original fixtures. Software 3D keeps recording portable;
# Kitty is explicit so this showcases pixels rather than half-block fallback.
Hide
Type@1ms `./gloss --render kitty --3d software examples/landscape.png examples/motion.gif examples/shapes.svg examples/field-guide.pdf examples/gloss.stl examples/readme.md examples/sales.xlsx examples/qr-links.csv`
Enter
Wait+Screen /landscape\.png.*png/
Sleep 600ms
Show
Sleep 2s
Screenshot dist/gloss-demo-image.png
Type "+"
Sleep 800ms
Type "f"
Sleep 800ms

# Animation, speed control, and pause: a GIF of gloss playing a GIF.
Hide
Type "]"
Wait+Screen /motion\.gif.*playing/
Sleep 400ms
Show
Sleep 2s
Type ">"
Wait+Screen /2× speed/
Sleep 1500ms
Space
Wait+Screen /paused/
Screenshot dist/gloss-demo-animation.png
Sleep 700ms
Backspace
Space
Sleep 1s

# SVG and PDF remain crisp terminal graphics.
Hide
Type "]"
Wait+Screen /shapes\.svg.*svg/
Sleep 500ms
Show
Sleep 2s
Hide
Type "]"
Wait+Screen /pdf.*page 1\/2/
Sleep 700ms
Show
Sleep 1500ms
Hide
Type "n"
Wait+Screen /pdf.*page 2\/2/
Sleep 500ms
Show
Sleep 2s
Screenshot dist/gloss-demo-pdf.png

# Orbit the original block-letter mesh, then let it rotate itself.
Hide
Type "]"
Wait+Screen /gloss\.stl.*STL/
Sleep 600ms
Show
Sleep 1s
Right@100ms 3
Up
Type "r"
Sleep 3s
Type "r"
Screenshot dist/gloss-demo-mesh.png

# Markdown mixes styled text and local images. Search jumps to the match.
Hide
Type "]"
Wait+Screen /readme\.md.*Markdown/
Sleep 500ms
Show
Sleep 1500ms
Type "/landscapes"
Enter
Wait+Screen /1\/1 matches/
Sleep 2s
Screenshot dist/gloss-demo-search.png

# Tables retain the keyboard-driven pager interaction.
Hide
Type "]"
Wait+Screen /sales\.xlsx.*cell/
Show
Sleep 1s
Down@120ms 3
Right@120ms 2
Sleep 1500ms

# An explicitly selected public documentation URL becomes a scannable QR.
# This is a URL overlay, not a localhost phone-upload handoff.
Hide
Type "]"
Wait+Screen /qr-links\.csv.*cell/
Down
Type "u"
Wait+Screen /Esc\/u close QR/
Sleep 500ms
Show
Sleep 3s
Screenshot dist/gloss-demo-qr.png

Hide
Escape
Type "q"
Wait+Line /demo>\s*$/
Type@1ms `printf '\033[2J\033[H\n  \033[1;38;2;167;215;177mgloss\033[0m\n\n  One pager. Many formats.\n\n  github.com/NimbleMarkets/gloss\n\n  Recorded with sigmamax\n  github.com/NimbleMarkets/sigmamax\n'`
Enter
Wait+Screen /One pager\. Many formats\./
Show
Sleep 3s
Screenshot dist/gloss-demo-poster.png
