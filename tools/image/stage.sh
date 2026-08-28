#!/usr/bin/env bash
# Stages the container image's build context: the two Linux binaries with
# the console inside them, the licence, and the Catalogue baseline
# (ADR-0068 §2).
#
# The image is assembled rather than compiled, so this is where the compiling
# happens. It runs on a workstation and in the release job unchanged, which
# is what makes the image a consumer of the same binaries the release
# attaches rather than a second build of one commit.
#
# Nothing here reaches a network except `npm ci` and the Go module cache,
# both of which are the ordinary build's dependencies rather than the
# image's: once dist/image is staged, `docker build` needs no network beyond
# its base layers (REQ-006).
#
# Environment:
#   VERSION     the version the console names itself with, and the label the
#               image carries (default: `git describe`, else `development`)
#   CATALOGUE   the Catalogue artefact to carry (default: the one this
#               repository imports, at the collector version devenv/.env pins)
#   SKIP_CONSOLE=1  leave internal/consoleassets/dist as it stands, for a
#               rebuild that has not touched the console
#
# Usage: tools/image/stage.sh

set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"

out=dist/image
version=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo development)}

# The Catalogue this repository imports is the image's baseline. One pin
# serves both: devenv/.env carries the collector version, and moving it
# means re-importing the artefact beside it.
catalogue=${CATALOGUE:-}
if [ -z "$catalogue" ]; then
  # shellcheck disable=SC2012
  catalogue=$(ls -1 devenv/estate/catalogues/catalogue-*.json 2>/dev/null | sort | tail -1)
fi
if [ ! -f "$catalogue" ]; then
  echo "stage: no Catalogue artefact to carry; import one or name it in CATALOGUE" >&2
  exit 1
fi

rm -rf "$out"
mkdir -p "$out/catalogues"

if [ "${SKIP_CONSOLE:-0}" != "1" ]; then
  echo "stage: building the console at $version"
  [ -d console/node_modules ] || (cd console && npm ci)
  (cd console && TELECRAFT_VERSION="$version" npm run build)
  # The air-gap rule over the bytes about to travel inside a binary: a
  # bundler can introduce a request no import statement shows (ADR-0045 §5).
  (cd console && npm run check:zero-cdn)
  (cd console && npm run bundle)
fi

for arch in amd64 arm64; do
  echo "stage: building linux/$arch"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -trimpath -o "$out/telecraft-linux-$arch" ./cmd/telecraft
done

cp LICENSE "$out/LICENSE"
cp "$catalogue" "$out/catalogues/"
printf '%s\n' "$version" > "$out/VERSION"

echo "stage: staged $out at $version"
find "$out" -type f | sort | while read -r f; do
  printf '  %s  %s\n' "$(sha256sum "$f" | cut -d' ' -f1)" "$f"
done
