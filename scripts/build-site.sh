#!/bin/sh
set -eu
mkdir -p web/dist
cp web/index.html web/privacy.html web/term.html web/demo.html web/app.html web/style.css web/app.css web/site.js web/runtime.js web/boot.mjs web/library.mjs web/pickers.mjs web/remote.mjs web/download.mjs web/drop.mjs web/save.mjs web/dist/
mkdir -p web/dist/fonts && cp web/fonts/* web/dist/fonts/
cp examples/landscape.png web/dist/social-preview.png
cp THIRD_PARTY_NOTICES.md web/dist/
cd examples/demo
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags='-s -w' -o ../../web/dist/app.wasm .
go tool booba-assets ../../web/dist
go tool booba-shim-assets ../../web/dist --shim=pdfium
# Use the runtime from the exact Go toolchain that compiled the application.
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ../../web/dist/wasm_exec.js
touch ../../web/dist/.nojekyll
