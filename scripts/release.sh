#!/bin/sh
set -eu
version=${1:?usage: release.sh vX.Y.Z}
case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo 'VERSION must be a version tag such as v0.1.0' >&2; exit 1 ;;
esac
case "$version" in
  *[!a-zA-Z0-9.+-]*) echo 'Invalid version characters' >&2; exit 1 ;;
esac
mkdir -p dist
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT HUP INT TERM
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
  os=${target%/*}
  arch=${target#*/}
  name="gloss_${version}_${os}_${arch}"
  mkdir -p "$stage/$name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -mod=readonly -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$stage/$name/gloss" ./cmd/gloss
  cp README.md THIRD_PARTY_NOTICES.md "$stage/$name/"
  tar -czf "dist/$name.tar.gz" -C "$stage" "$name"
done
cd dist
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "gloss_${version}_"*.tar.gz > "gloss_${version}_checksums.txt"
else
  shasum -a 256 "gloss_${version}_"*.tar.gz > "gloss_${version}_checksums.txt"
fi
