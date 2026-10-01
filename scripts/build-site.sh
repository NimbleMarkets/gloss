#!/bin/sh
set -eu
mkdir -p web/dist
cp web/index.html web/privacy.html web/term.html web/demo.html web/app.html web/style.css web/app.css web/site.js web/runtime.js web/boot.mjs web/library.mjs web/pickers.mjs web/remote.mjs web/download.mjs web/drop.mjs web/save.mjs web/dist/
mkdir -p web/dist/fonts && cp web/fonts/* web/dist/fonts/
cp examples/landscape.png web/dist/social-preview.png
cp THIRD_PARTY_NOTICES.md web/dist/
cd examples/demo
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags='-s -w' -o ../../web/dist/app.wasm .
# A compressed download hides its decoded size; the page uses this for a progress bar.
wc -c < ../../web/dist/app.wasm | tr -d ' ' > ../../web/dist/app.wasm.size
go tool booba-assets ../../web/dist
go tool booba-shim-assets ../../web/dist --shim=pdfium
# PDF pages are drawn by PDFium, from the @embedpdf/pdfium npm package, which
# the generated shim loads from a CDN: third-party code, run with the page's
# own access, from a URL whose output is not fixed. Serve the package from the
# site instead, at the pinned version and checked against its published hash,
# so the pages contact no one else. Bump both lines together:
#   npm view @embedpdf/pdfium@VERSION dist.integrity   (base64; the hash below is hex)
PDFIUM_VERSION=2.14.2
PDFIUM_SHA512=040c03ac0a8a51343f858182ed1aaee54e94664c96b45d31e4c69bd21093595c6b6cd415d674a65f98d05d807d7177950e756fc2cc854615a081acc1211bdba8
pdfium=../../web/dist/vendor/embedpdf-pdfium
rm -rf "$pdfium" && mkdir -p "$pdfium"
curl -fsSL -o "$pdfium/package.tgz" "https://registry.npmjs.org/@embedpdf/pdfium/-/pdfium-$PDFIUM_VERSION.tgz"
if command -v sha512sum >/dev/null 2>&1; then got=$(sha512sum "$pdfium/package.tgz" | cut -d' ' -f1); else got=$(shasum -a 512 "$pdfium/package.tgz" | cut -d' ' -f1); fi
if [ "$got" != "$PDFIUM_SHA512" ]; then echo "build-site: @embedpdf/pdfium $PDFIUM_VERSION does not match its pinned hash ($got)" >&2; exit 1; fi
tar -xzf "$pdfium/package.tgz" -C "$pdfium" --strip-components=1 package/dist/index.browser.js package/dist/pdfium.wasm package/LICENSE package/LICENSE.pdfium
rm "$pdfium/package.tgz"
shim=../../web/dist/booba-shim/pdfium/pdfium-shim.js
sed -e "s|https://cdn.jsdelivr.net/npm/@embedpdf/pdfium@$PDFIUM_VERSION/+esm|../../vendor/embedpdf-pdfium/dist/index.browser.js|" \
    -e "s|https://cdn.jsdelivr.net/npm/@embedpdf/pdfium@$PDFIUM_VERSION/dist/pdfium.wasm|vendor/embedpdf-pdfium/dist/pdfium.wasm|" "$shim" > "$shim.tmp" && mv "$shim.tmp" "$shim"
# A shim for another PDFium version would escape the patch above: fail, not ship it.
if grep -rIl 'cdn.jsdelivr.net' ../../web/dist --exclude-dir=vendor | grep -q .; then
  echo "build-site: a page still loads from cdn.jsdelivr.net: update the PDFium pin in scripts/build-site.sh" >&2
  exit 1
fi
# Use the runtime from the exact Go toolchain that compiled the application.
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ../../web/dist/wasm_exec.js
touch ../../web/dist/.nojekyll
